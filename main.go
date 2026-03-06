package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf16"

	"github.com/bendahl/uinput"
	"github.com/karalabe/hid"
)

const (
	VendorID       = 0x28BD
	ProdIDWire     = 0x5202
	ProdIDWire2    = 0x5204
	ProdIDWireless = 0x5203
)

// showTrayInfoPopup affiche les infos via le système de notification (notify-send)
func showTrayInfoPopup() {
	text := trayGetStatusText()
	title := "Xencelabs Quick Keys"

	env := os.Environ()
	if os.Getenv("DISPLAY") == "" {
		env = append(env, "DISPLAY=:0")
	}
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
		if r := os.Getenv("XDG_RUNTIME_DIR"); r != "" {
			env = append(env, "DBUS_SESSION_BUS_ADDRESS=unix:path="+r+"/bus")
		}
	}

	if p, err := exec.LookPath("notify-send"); err == nil {
		cmd := exec.Command(p, "-t", "5000", title, text)
		cmd.Env = env
		cmd.Run()
	}
}

// getTrayIconPath returns the path to the tray icon.
func getTrayIconPath() string {
	tryPaths := func(names []string) string {
		// 1. À côté de l'exécutable
		if exe, err := os.Executable(); err == nil {
			dir := filepath.Dir(exe)
			for _, name := range names {
				p := filepath.Join(dir, name)
				if _, err := os.Stat(p); err == nil {
					return p
				}
			}
		}
		// 2. Répertoire partagé (installation système)
		for _, base := range []string{"/usr/local/share/xencelabs-quick-keys", "/usr/share/xencelabs-quick-keys"} {
			for _, name := range names {
				p := filepath.Join(base, name)
				if _, err := os.Stat(p); err == nil {
					return p
				}
			}
		}
		// 3. Répertoire courant (développement)
		for _, name := range names {
			if _, err := os.Stat(name); err == nil {
				return name
			}
		}
		return ""
	}
	return tryPaths([]string{"xencelabs-icon.png", "xencelabs-quick-keys.png", "xencelabs-logo.png"})
}

func padBytes(input []byte) []byte {
	out := make([]byte, 32)
	copy(out, input)
	return out
}

// writeReport sends a 32-byte report to the device. For wireless dongle, deviceId (6 bytes) is inserted at offset 10.
func writeReport(dev *hid.Device, report []byte, deviceId []byte) {
	if len(report) < 32 {
		report = padBytes(report)
	}
	out := make([]byte, 32)
	copy(out, report)
	if len(deviceId) >= 6 {
		copy(out[10:16], deviceId[:6])
	}
	dev.Write(out)
}

func encodeText(text string) []byte {
	utf16Units := utf16.Encode([]rune(text))
	buf := new(bytes.Buffer)
	for _, unit := range utf16Units {
		binary.Write(buf, binary.LittleEndian, unit)
	}
	b := buf.Bytes()
	if len(b) > 16 {
		return b[:16]
	}
	return b
}

func cmdSetKeyText(keyIndex uint8, text string) []byte {
	encoded := encodeText(text)
	lenVal := uint8(len(encoded))
	if lenVal > 16 {
		lenVal = 16
	}

	header := []byte{0x02, 0xb1, 0x00, keyIndex + 1, 0x00, lenVal}
	report := make([]byte, 32)
	copy(report[0:], header)
	copy(report[16:], encoded)
	return report
}

func cmdSetWheelColor(r, g, b uint8) []byte {
	return padBytes([]byte{0x02, 0xb4, 0x01, 0x01, 0x00, 0x00, r, g, b})
}

func cmdSetOrientation(deg int) []byte {
	val := uint8(1)
	switch deg {
	case 90:
		val = 2
	case 180:
		val = 3
	case 270:
		val = 4
	}
	return padBytes([]byte{0x02, 0xb1, val})
}

func cmdSetBrightness(level string) []byte {
	val := uint8(2) // medium
	switch strings.ToLower(level) {
	case "off":
		val = 0
	case "low":
		val = 1
	case "full":
		val = 3
	}
	return padBytes([]byte{0x02, 0xb1, 0x0a, 0x01, val})
}

func cmdSetWheelSpeed(speed string) []byte {
	val := uint8(3) // normal
	switch strings.ToLower(speed) {
	case "slowest":
		val = 5
	case "slower":
		val = 4
	case "normal":
		val = 3
	case "faster":
		val = 2
	case "fastest":
		val = 1
	}
	return padBytes([]byte{0x02, 0xb4, 0x04, 0x01, 0x01, val})
}

func cmdSleepTimeout(mins uint8) []byte {
	return padBytes([]byte{0x02, 0xb4, 0x08, 0x01, mins})
}

// getWirelessDeviceId sends discovery to the dongle and waits for the paired device to report (0xf8).
// Returns deviceId (6 bytes) or nil if not found within timeout.
func getWirelessDeviceId(dev *hid.Device, timeout time.Duration) []byte {
	// Discovery command: ask dongle which devices are connected
	dev.Write(padBytes([]byte{0x02, 0xb8, 0x01}))

	buf := make([]byte, 64)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		n, err := dev.Read(buf)
		if err != nil || n < 16 {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		// Device status: 0xf8 at start (Linux may strip report ID) or at buf[1] (with report ID 0x02)
		offset := -1
		if buf[0] == 0xf8 {
			offset = 0 // report ID stripped
		} else if buf[0] == 0x02 && n > 1 && buf[1] == 0xf8 {
			offset = 1 // report ID present
		}
		if offset >= 0 {
			state := buf[offset+1]
			if state == 2 || state == 3 { // connecting or connected
				deviceId := make([]byte, 6)
				copy(deviceId, buf[offset+9:offset+15])
				return deviceId
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	return nil
}

// encodeTextOverlay encodes text to UTF-16LE for overlay (up to 32 chars = 64 bytes).
func encodeTextOverlay(text string) []byte {
	utf16Units := utf16.Encode([]rune(text))
	buf := new(bytes.Buffer)
	for _, unit := range utf16Units {
		binary.Write(buf, binary.LittleEndian, unit)
	}
	b := buf.Bytes()
	if len(b) > 64 {
		return b[:64]
	}
	return b
}

// msgsShowOverlayText constructs the packets needed to show text on the OLED.
// Protocol: 0x05 (start) + 0x06 (continue) packets, matching node-xencelabs-quick-keys.
func msgsShowOverlayText(duration uint8, text string) [][]byte {
	allBytes := encodeTextOverlay(text)
	var packets [][]byte

	// Match node createOverlayChunk: byte 5 = char_count*2, data at offset 16
	createReport := func(cmd byte, dur byte, data []byte, hasMore bool) []byte {
		report := make([]byte, 32)
		report[0] = 0x02
		report[1] = 0xb1
		report[2] = cmd
		report[3] = dur
		report[4] = 0x00
		report[5] = uint8(len(data))
		if hasMore {
			report[6] = 0x01
		} else {
			report[6] = 0x00
		}
		if len(data) > 0 {
			copy(report[16:], data)
		}
		return report
	}

	// Packet 1: 0x05 Start - first 8 chars (16 bytes)
	chunk1 := allBytes
	if len(chunk1) > 16 {
		chunk1 = chunk1[:16]
	}
	packets = append(packets, createReport(0x05, duration, chunk1, false))

	// Packet 2: 0x06 Continue - next 8 chars, or empty (required for display update)
	var chunk2 []byte
	if len(allBytes) > 16 {
		chunk2 = allBytes[16:]
		if len(chunk2) > 16 {
			chunk2 = chunk2[:16]
		}
	}
	hasMore2 := len(allBytes) > 32
	packets = append(packets, createReport(0x06, duration, chunk2, hasMore2))

	// Remaining packets for text > 16 chars
	for offset := 32; offset < len(allBytes); offset += 16 {
		end := offset + 16
		if end > len(allBytes) {
			end = len(allBytes)
		}
		chunk := allBytes[offset:end]
		hasMore := end < len(allBytes)
		packets = append(packets, createReport(0x06, duration, chunk, hasMore))
	}

	return packets
}

// main for non-tray build is in main_notray.go
// main for tray build (Wails) is in main_wails.go

func mainNotray() {
	configPath, err := getConfigPath()
	if err != nil {
		log.Fatalf("Cannot get config path: %v", err)
	}

	// Setup UInput (once, reused across reconnections)
	keyboard, err := uinput.CreateKeyboard("/dev/uinput", []byte("Xencelabs QuickKeys Virtual"))
	if err != nil {
		log.Fatalf("Failed to create virtual keyboard: %v", err)
	}
	defer keyboard.Close()

	// Setup Signal Handling
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	// SIGUSR1 : afficher les infos (goroutine dédiée, indépendante du select principal)
	sigusr1 := make(chan os.Signal, 1)
	signal.Notify(sigusr1, syscall.SIGUSR1)
	go func() {
		for range sigusr1 {
			showTrayInfoPopup()
		}
	}()

	// Tray icon (si DISPLAY/WAYLAND et build avec -tags tray)
	iconPath := getTrayIconPath()
	if os.Getenv("XENCELABS_TRAY_DEBUG") == "1" {
		trayDebugEnv()
	}
	if trayAvailable() {
		startTray(iconPath)
	}

	// Hotplug loop: reload config and reconnect on each device connection
	for {
		// Wait for device
		devInfo := findDevice()
		if devInfo == nil {
			log.Println("Xencelabs Quick Keys device not found, waiting...")
			time.Sleep(2 * time.Second)
			continue
		}

		// Load config from file on each connection (fresh read)
		loadedConfig, err := loadConfig(configPath)
		if err != nil {
			log.Fatalf("Error loading config: %v", err)
		}
		var config *Config = loadedConfig
		var configMu sync.RWMutex

		dev, err := devInfo.Open()
		if err != nil {
			logOpenFailThrottled(err)
			time.Sleep(2 * time.Second)
			continue
		}
		resetOpenFailLog()

		log.Printf("Connected to %04x:%04x", devInfo.VendorID, devInfo.ProductID)
		if devInfo.ProductID == ProdIDWire || devInfo.ProductID == ProdIDWire2 {
			log.Printf("[battery] Wired device (no battery)")
		}
		traySetConnected(true)

		var deviceId []byte
		if devInfo.ProductID == ProdIDWireless {
			log.Println("Wireless dongle: discovering paired device...")
			deviceId = getWirelessDeviceId(dev, 15*time.Second)
			if deviceId == nil {
				log.Println("No paired device found. Turn on the Quick Keys and wait a few seconds.")
				dev.Close()
				time.Sleep(2 * time.Second)
				continue
			}
			log.Printf("Found paired device: %x", deviceId)
			time.Sleep(100 * time.Millisecond) // Brief pause before init
		}

		// Initialize Global Settings & First Layer
		configMu.RLock()
		cfg := config
		configMu.RUnlock()
		currentLayerIdx := resolveInitialLayerIndex(cfg)
		initGlobalSettings(dev, cfg, deviceId)
		layer := cfg.Layers[currentLayerIdx]
		refreshLayer(dev, layer, cfg.Device.WheelSpeed, cfg.Device.OverlayDuration, true, cfg.Device.ShowBatteryInLayerName, deviceId)
		traySetLayer(layer.Name, layer.Color.R, layer.Color.G, layer.Color.B)

		buf := make([]byte, 32)
		var prevBtn1, prevBtn2 uint8

		// Flush/Read initial state to prevent immediate trigger on startup if a button is held
		// Also process battery report if device sent one in response to initGlobalSettings
		if n, err := dev.Read(buf); err == nil && n >= 10 {
			traySetReceivedData()
			offset := 0
			if buf[0] == 0xf0 {
				offset = -1
			}
			battOff := 0
			if buf[0] == 0xf8 {
				battOff = 1
			}
			if n >= 5+battOff && buf[battOff] == 0x02 && buf[battOff+1] == 0xb4 && buf[battOff+2] == 0x10 {
				pct := int(buf[battOff+4])
				if pct > 100 {
					pct = int(buf[battOff+3])
				}
				if pct <= 100 {
					traySetBattery(pct, false)
				}
			} else if pct, charging := parseBatteryReport(buf, n); pct >= 0 {
				traySetBattery(pct, charging)
			} else if p := parseBattery0xb4(buf, n); p >= 0 {
				traySetBattery(p, false)
			} else if buf[offset+1] == 0xf0 {
				prevBtn1 = buf[offset+2]
				prevBtn2 = buf[offset+3]
			}
		}

		// Input loop (runs until device disconnects or signal)
		deviceDone := make(chan struct{})
		configReload := make(chan struct{}, 1)
		go watchConfigFile(configPath, configReload, deviceDone)
		isWireless := len(deviceId) > 0

		// Requête batterie HID périodique (filaire + sans fil) ; fallback sysfs/upower pour wireless
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			loggedNoBattery := false
			if pct, charging := readBattery(&loggedNoBattery); pct >= 0 {
				traySetBattery(pct, charging)
				log.Printf("[battery] %d%% (sysfs/upower)", pct)
			} else {
				log.Printf("[battery] sysfs/upower: none, waiting for HID reports (first 5 non-key reports will be logged)")
			}
			time.Sleep(2 * time.Second)
			select {
			case <-deviceDone:
				return
			default:
				writeReport(dev, padBytes([]byte{0x02, 0xb4, 0x10}), deviceId)
				time.Sleep(50 * time.Millisecond)
				writeReport(dev, padBytes([]byte{0x02, 0xb4, 0x11}), deviceId)
			}
			for {
				select {
				case <-deviceDone:
					return
				case <-ticker.C:
					writeReport(dev, padBytes([]byte{0x02, 0xb4, 0x10}), deviceId)
					time.Sleep(50 * time.Millisecond)
					writeReport(dev, padBytes([]byte{0x02, 0xb4, 0x11}), deviceId)
					if pct, charging := readBattery(&loggedNoBattery); pct >= 0 {
						traySetBattery(pct, charging)
					}
				}
			}
		}()

		configMu.RLock()
		cfg = config
		configMu.RUnlock()
		dblClick := newDoubleClickState(cfg.Device.DoubleClickMs)
		dblClick.onLayerCycle = func() {
			dblClick.clearState()
			configMu.RLock()
			cfg := config
			configMu.RUnlock()
			currentLayerIdx = (currentLayerIdx + 1) % len(cfg.Layers)
			layer := cfg.Layers[currentLayerIdx]
			log.Printf("Switching to Layer: %s", layer.Name)
			refreshLayer(dev, layer, cfg.Device.WheelSpeed, cfg.Device.OverlayDuration, true, cfg.Device.ShowBatteryInLayerName, deviceId)
			traySetLayer(layer.Name, layer.Color.R, layer.Color.G, layer.Color.B)
		}
		dblClick.onBatteryOverlay = func() {
			configMu.RLock()
			cfg := config
			configMu.RUnlock()
			go showBatteryOverlay(dev, deviceId, cfg.Device.OverlayDuration)
		}
		var nonKeyReportCount int
		go func() {
			for {
				n, err := dev.Read(buf)
				if err != nil {
					log.Printf("Device disconnected: %v", err)
					dev.Close()
					close(deviceDone)
					return
				}
				if n < 10 {
					continue
				}
				traySetReceivedData()

				// Wireless: detect remote disconnect (0xf8 state 4) to trigger reconnection
				if isWireless {
					off := -1
					if buf[0] == 0xf8 {
						off = 0
					} else if buf[0] == 0x02 && n > 2 && buf[1] == 0xf8 {
						off = 1
					}
					if off >= 0 && buf[off+1] == 4 {
						log.Println("Wireless remote disconnected, reconnecting...")
						dev.Close()
						close(deviceDone)
						return
					}
				}

				offset := 0
				if buf[0] == 0xf0 {
					offset = -1
				}

				// Battery report: 0x02 0xb4 0x10 (sous-type 0x10 = batterie)
				// Wireless: préfixe 0xf8, payload à buf[1..] → battOff=1
				// Les 0xb4 0x01/0x04/0x08 sont des ACK de commandes
				debugBatt := os.Getenv("XENCELABS_DEBUG") == "1"
				battOff := 0
				if buf[0] == 0xf8 {
					battOff = 1
				}
				if n >= 5+battOff && buf[battOff] == 0x02 && buf[battOff+1] == 0xb4 && buf[battOff+2] == 0x10 {
					pct := int(buf[battOff+4])
					if pct > 100 {
						pct = int(buf[battOff+3])
					}
					if pct <= 100 {
						if debugBatt {
							log.Printf("battery (0xb4 0x10): %d%%", pct)
						}
						traySetBattery(pct, false)
					}
					continue
				}
				if n >= 4+battOff && buf[battOff] == 0x02 && (buf[battOff+1] == 0xb4 || buf[battOff+1] == 0xb1) {
					continue // ACK de commande (b4, b1), ignorer
				}
				// Format batterie 0xf2 0x01 XX [YY] (YY=charging si présent)
				if pct, charging := parseBatteryReport(buf, n); pct >= 0 {
					if debugBatt {
						hex := ""
						for i := 0; i < n && i < 8; i++ {
							hex += fmt.Sprintf(" %02x", buf[i])
						}
						log.Printf("battery (0xf2 0x01): %d%% charging=%v raw:%s", pct, charging, hex)
					}
					traySetBattery(pct, charging)
					continue
				}
				// Fallback: scan pour 0xb4 0x10 à n'importe quelle position
				if p := parseBattery0xb4(buf, n); p >= 0 {
					if debugBatt {
						hex := ""
						for i := 0; i < n && i < 12; i++ {
							hex += fmt.Sprintf(" %02x", buf[i])
						}
						log.Printf("battery (0xb4 scan): %d%% raw:%s", p, hex)
					}
					traySetBattery(p, false)
					continue
				}
				// Log tout rapport non-clavier — aide au diagnostic si batterie jamais reçue
				if buf[offset+1] != 0xf0 {
					if nonKeyReportCount < 5 {
						nonKeyReportCount++
						hex := ""
						for i := 0; i < n && i < 16; i++ {
							hex += fmt.Sprintf(" %02x", buf[i])
						}
						log.Printf("[battery] HID report #%d (non-key):%s", nonKeyReportCount, hex)
					}
					if debugBatt && nonKeyReportCount >= 5 {
						hex := ""
						for i := 0; i < n && i < 12; i++ {
							hex += fmt.Sprintf(" %02x", buf[i])
						}
						log.Printf("HID report (non-key):%s", hex)
					}
				}

				if buf[offset+1] == 0xf0 {
					keys1 := buf[offset+2]
					keys2 := buf[offset+3]
					wheel := buf[offset+7]

					// Debug: XENCELABS_DEBUG=1 — log uniquement les boutons (pas la molette)
					if os.Getenv("XENCELABS_DEBUG") == "1" && (keys1 != prevBtn1 || keys2 != prevBtn2) {
						log.Printf("raw k1=%02x k2=%02x wheel=%02x", keys1, keys2, wheel)
					}

					configMu.RLock()
					cfg := config
					configMu.RUnlock()
					layerChanged := handleButtons(keyboard, keys1, keys2, prevBtn1, prevBtn2, cfg.Layers[currentLayerIdx], cfg.Device.KeyboardLayout, cfg.Device.Button8Double, dblClick)

				if layerChanged {
					dblClick.clearState()
					configMu.RLock()
					cfg := config
					configMu.RUnlock()
					currentLayerIdx = (currentLayerIdx + 1) % len(cfg.Layers)
					layer := cfg.Layers[currentLayerIdx]
					log.Printf("Switching to Layer: %s", layer.Name)
					refreshLayer(dev, layer, cfg.Device.WheelSpeed, cfg.Device.OverlayDuration, true, cfg.Device.ShowBatteryInLayerName, deviceId)
					traySetLayer(layer.Name, layer.Color.R, layer.Color.G, layer.Color.B)
				}

					prevBtn1 = keys1
					prevBtn2 = keys2

					if wheel > 0 {
						configMu.RLock()
						cfg := config
						configMu.RUnlock()
						handleWheel(keyboard, wheel, cfg.Layers[currentLayerIdx], cfg.Device.KeyboardLayout)
					}
				}
			}
		}()

		// Wait for device disconnect, config reload, or signal
		for {
			select {
			case <-deviceDone:
				traySetConnected(false)
				log.Println("Reconnecting...")
				time.Sleep(1 * time.Second)
				goto nextConnection
			case <-configReload:
				var newConfig *Config
				var err error
				for attempt := 0; attempt < 3; attempt++ {
					newConfig, err = loadConfig(configPath)
					if err == nil {
						break
					}
					log.Printf("Config reload attempt %d failed: %v", attempt+1, err)
					time.Sleep(200 * time.Millisecond)
				}
				if err != nil {
					log.Printf("Config reload failed after retries: %v", err)
				} else {
					configMu.Lock()
					config = newConfig
					configMu.Unlock()
					configMu.RLock()
					cfg := config
					configMu.RUnlock()
					if currentLayerIdx >= len(cfg.Layers) {
						currentLayerIdx = len(cfg.Layers) - 1
						if currentLayerIdx < 0 {
							currentLayerIdx = 0
						}
					}
					initGlobalSettings(dev, cfg, deviceId)
					layer := cfg.Layers[currentLayerIdx]
					refreshLayer(dev, layer, cfg.Device.WheelSpeed, cfg.Device.OverlayDuration, true, cfg.Device.ShowBatteryInLayerName, deviceId)
					traySetLayer(layer.Name, layer.Color.R, layer.Color.G, layer.Color.B)
					log.Println("Config reloaded")
				}
			case sig := <-sigs:
				if sig == syscall.SIGQUIT {
					log.Println("Received SIGQUIT (Ctrl+\\), ignoring because we are likely injecting it.")
					continue
				}
				log.Printf("Received signal: %v, exiting...", sig)
				dev.Close()
				return
			}
		}
	nextConnection:
	}
}

// parseBatteryReport extrait le % batterie et le statut charge des rapports 0xf2 0x01 XX YY.
// Format Quick Keys: XX = pourcentage, YY = 0x01 charging, 0x02 full, 0x00 discharging.
func parseBatteryReport(buf []byte, n int) (pct int, charging bool) {
	pct = -1
	tryParse := func(off int) bool {
		if off+3 > n || buf[off] != 0xf2 || buf[off+1] != 0x01 {
			return false
		}
		p := int(buf[off+2])
		if p <= 100 {
			pct = p
			// YY à off+3 (buf[4] quand off=1)
			if off+4 <= n {
				switch buf[off+3] {
				case 1, 2:
					charging = true
				}
			}
			return true
		}
		return false
	}
	for _, off := range []int{0, 1, 11, 17} {
		if tryParse(off) {
			return
		}
	}
	for i := 0; i+3 <= n; i++ {
		if tryParse(i) {
			return
		}
	}
	return -1, false
}

// parseBattery0xb4 scanne le buffer pour 0xb4 0x10 (ou 0x02 0xb4 0x10) et extrait le %.
func parseBattery0xb4(buf []byte, n int) (pct int) {
	for i := 0; i+4 <= n; i++ {
		var p int
		if buf[i] == 0x02 && i+5 <= n && buf[i+1] == 0xb4 && buf[i+2] == 0x10 {
			p = int(buf[i+4])
			if p > 100 {
				p = int(buf[i+3])
			}
		} else if buf[i] == 0xb4 && i+4 <= n && buf[i+1] == 0x10 {
			p = int(buf[i+3])
			if p > 100 {
				p = int(buf[i+2])
			}
		} else if buf[i] == 0xf8 && i+6 <= n && buf[i+1] == 0x02 && buf[i+2] == 0xb4 && buf[i+3] == 0x10 {
			p = int(buf[i+5])
			if p > 100 {
				p = int(buf[i+4])
			}
		} else {
			continue
		}
		if p >= 0 && p <= 100 {
			return p
		}
	}
	return -1
}

// readBattery essaie sysfs puis upower pour obtenir le niveau batterie et le statut charge.
// loggedNoBattery évite de spammer le log quand sysfs/upower ne trouvent rien (HID fournit la batterie).
func readBattery(loggedNoBattery *bool) (pct int, charging bool) {
	debugBatt := os.Getenv("XENCELABS_DEBUG") == "1"
	if p, c := readBatteryFromSysfs(); p >= 0 {
		if debugBatt {
			log.Printf("battery: %d%% charging=%v (sysfs)", p, c)
		}
		return p, c
	}
	if p, c := readBatteryFromUpower(); p >= 0 {
		if debugBatt {
			log.Printf("battery: %d%% charging=%v (upower)", p, c)
		}
		return p, c
	}
	if debugBatt && loggedNoBattery != nil && !*loggedNoBattery {
		*loggedNoBattery = true
		log.Print("battery: not found (sysfs/upower)")
	}
	return -1, false
}

// readBatteryFromSysfs lit le niveau batterie et le statut charge depuis /sys/class/power_supply.
func readBatteryFromSysfs() (pct int, charging bool) {
	entries, err := os.ReadDir("/sys/class/power_supply")
	if err != nil {
		return -1, false
	}
	vendorMatch := strings.ToLower(fmt.Sprintf("%04x", VendorID))

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		capPath := filepath.Join("/sys/class/power_supply", name, "capacity")
		if _, err := os.Stat(capPath); err != nil {
			continue
		}

		match := false
		nameLower := strings.ToLower(name)
		// HID battery: hid-XXXX-YYYY ou hidrawN
		if strings.Contains(nameLower, "hid") && strings.Contains(nameLower, "battery") &&
			strings.Contains(nameLower, vendorMatch) {
			match = true
		}
		if !match && strings.Contains(nameLower, vendorMatch) {
			match = true
		}
		if !match {
			devicePath, err := filepath.EvalSymlinks(filepath.Join("/sys/class/power_supply", name, "device"))
			if err != nil {
				continue
			}
			pathLower := strings.ToLower(devicePath + "/" + name)
			if strings.Contains(pathLower, vendorMatch) {
				match = true
			}
			if !match && (matchVendorProduct(devicePath, VendorID, ProdIDWireless) ||
				matchVendorProduct(devicePath, VendorID, ProdIDWire) ||
				matchVendorProduct(devicePath, VendorID, ProdIDWire2) ||
				matchVendorProduct(devicePath, VendorID, 0)) {
				match = true
			}
		}
		if match {
			if p := readCapacity(capPath); p >= 0 {
				c := readChargingFromSysfs(filepath.Join("/sys/class/power_supply", name, "status"))
				return p, c
			}
		}
	}
	return -1, false
}

func readChargingFromSysfs(statusPath string) bool {
	data, err := os.ReadFile(statusPath)
	if err != nil {
		return false
	}
	s := strings.TrimSpace(strings.ToLower(string(data)))
	return s == "charging" || s == "full"
}

func readCapacity(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return -1
	}
	var pct int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &pct); err == nil && pct >= 0 && pct <= 100 {
		return pct
	}
	return -1
}

// readBatteryFromUpower utilise upower (D-Bus) comme fallback si sysfs ne fournit pas la batterie.
func readBatteryFromUpower() (pct int, charging bool) {
	path, err := exec.LookPath("upower")
	if err != nil {
		return -1, false
	}
	out, err := exec.Command(path, "-e").Output()
	if err != nil {
		return -1, false
	}
	vendorMatch := strings.ToLower(fmt.Sprintf("%04x", VendorID))
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lineLower := strings.ToLower(line)
		if !strings.Contains(lineLower, "battery") && !strings.Contains(lineLower, "hid_") {
			continue
		}
		info, err := exec.Command(path, "-i", line).Output()
		if err != nil {
			continue
		}
		infoStr := strings.ToLower(string(info))
		if !strings.Contains(infoStr, vendorMatch) {
			continue
		}
		infoS := string(info)
		var p int
		var c bool
		for _, l := range strings.Split(infoS, "\n") {
			l = strings.TrimSpace(l)
			if strings.HasPrefix(l, "percentage:") {
				if _, err := fmt.Sscanf(l, "percentage: %d%%", &p); err == nil && p >= 0 && p <= 100 {
					// continue to parse state
				}
			}
			if strings.HasPrefix(l, "state:") {
				ls := strings.ToLower(l)
				c = strings.Contains(ls, "charging") || strings.Contains(ls, "fully-charged")
			}
		}
		if p >= 0 && p <= 100 {
			return p, c
		}
	}
	return -1, false
}

// matchVendorProduct vérifie si le device path correspond au vendor/product USB.
// Pour USB HID, idVendor/idProduct sont dans le parent du device (interface -> device).
func matchVendorProduct(devicePath string, vendorID, productID uint16) bool {
	// Essayer device/../idVendor (parent USB)
	for _, base := range []string{devicePath, filepath.Dir(devicePath)} {
		v, err := os.ReadFile(filepath.Join(base, "idVendor"))
		if err != nil {
			continue
		}
		p, err := os.ReadFile(filepath.Join(base, "idProduct"))
		if err != nil {
			continue
		}
		var vID, pID uint16
		if _, err := fmt.Sscanf(strings.TrimSpace(string(v)), "%04x", &vID); err != nil {
			continue
		}
		if _, err := fmt.Sscanf(strings.TrimSpace(string(p)), "%04x", &pID); err != nil {
			continue
		}
		if vID == vendorID && (productID == 0 || pID == productID) {
			return true
		}
	}
	return false
}

func findDevice() *hid.DeviceInfo {
	infos := hid.Enumerate(VendorID, 0)
	for _, info := range infos {
		if info.ProductID == ProdIDWire || info.ProductID == ProdIDWireless || info.ProductID == ProdIDWire2 {
			if info.Interface == 2 {
				return &info
			}
		}
	}
	return nil
}

// initGlobalSettings sets things that don't change between layers
func initGlobalSettings(dev *hid.Device, cfg *Config, deviceId []byte) {
	// Subscribe first (required for wireless before other commands)
	writeReport(dev, padBytes([]byte{0x02, 0xb0, 0x04}), deviceId) // Subscribe Key events
	time.Sleep(30 * time.Millisecond)
	writeReport(dev, padBytes([]byte{0x02, 0xb4, 0x10}), deviceId) // Subscribe Battery
	time.Sleep(30 * time.Millisecond)

	writeReport(dev, cmdSetOrientation(cfg.Device.Orientation), deviceId)
	time.Sleep(15 * time.Millisecond)
	writeReport(dev, cmdSetBrightness(cfg.Device.Brightness), deviceId)
	time.Sleep(15 * time.Millisecond)
	writeReport(dev, cmdSetWheelSpeed(cfg.Device.WheelSpeed), deviceId)
	time.Sleep(15 * time.Millisecond)
	writeReport(dev, cmdSleepTimeout(cfg.Device.SleepTimeout), deviceId)
	time.Sleep(30 * time.Millisecond)
}

// refreshLayer updates button labels, ring color, wheel speed and shows overlay
func refreshLayer(dev *hid.Device, layer Layer, globalSpeed string, overlayDuration uint8, showOverlay bool, showBatteryInLayerName bool, deviceId []byte) {
	// Set Color
	writeReport(dev, cmdSetWheelColor(layer.Color.R, layer.Color.G, layer.Color.B), deviceId)

	// Set Wheel Speed (Layer overrides Global)
	targetSpeed := layer.WheelSpeed
	if targetSpeed == "" {
		targetSpeed = globalSpeed
	}
	writeReport(dev, cmdSetWheelSpeed(targetSpeed), deviceId)

	// Show overlay (layer name) first - before labels so user sees layer name immediately
	if showOverlay {
		dur := overlayDuration
		if dur == 0 {
			dur = 2
		}
		time.Sleep(30 * time.Millisecond)
		overlayName := formatLayerNameForDevice(layer.Name, trayGetBattery(), showBatteryInLayerName)
		pkts := msgsShowOverlayText(dur, overlayName)
		for i, p := range pkts {
			writeReport(dev, p, deviceId)
			if i < len(pkts)-1 {
				time.Sleep(40 * time.Millisecond)
			}
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Set Labels (Buttons 0-7)
	for idx := 0; idx <= 7; idx++ {
		if btn, ok := layer.Buttons[idx]; ok {
			writeReport(dev, cmdSetKeyText(uint8(idx), btn.Label), deviceId)
		} else {
			writeReport(dev, cmdSetKeyText(uint8(idx), ""), deviceId)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// showBatteryOverlay affiche "BATTERIE ▍▍▍ NN%" sur l'OLED du device.
func showBatteryOverlay(dev *hid.Device, deviceId []byte, overlayDuration uint8) {
	pct := trayGetBattery()
	text := "BATTERIE —"
	if pct >= 0 {
		text = fmt.Sprintf("BATTERIE ▍▍▍ %d%%", pct)
	}
	dur := overlayDuration
	if dur == 0 {
		dur = 2
	}
	pkts := msgsShowOverlayText(dur, text)
	for i, p := range pkts {
		writeReport(dev, p, deviceId)
		if i < len(pkts)-1 {
			time.Sleep(40 * time.Millisecond)
		}
	}
}

// formatLayerNameForDevice formate le nom du layer pour l'affichage sur le device (overlay OLED).
// Si showBattery=true, concatène " ▍▍▍ NN%" (ou " ▍▍▍ —" si inconnu) après le nom.
func formatLayerNameForDevice(name string, battery int, showBattery bool) string {
	if name == "" {
		return ""
	}
	if !showBattery {
		return name
	}
	if battery >= 0 {
		return name + fmt.Sprintf(" ▍▍▍ %d%%", battery)
	}
	return name + " ▍▍▍ —"
}

// doubleClickCfg holds Keys (single) and KeysDouble for button 8 double-click handling.
type doubleClickCfg struct {
	Keys       []string
	KeysDouble []string
}

// doubleClickState gère la détection du double-click et le report différé du simple click (bouton 8 uniquement).
type doubleClickState struct {
	mu                  sync.Mutex
	timeoutMs           int
	commitDelayMs       int
	graceMs             int
	pending             map[int]*time.Timer
	pendingCommit       map[int]*time.Timer
	pendingCfg          map[int]doubleClickCfg
	doubleClickNext     map[int]bool
	lastSimpleClickTime map[int]time.Time
	onLayerCycle        func()
	onBatteryOverlay    func()
}

func newDoubleClickState(timeoutMs int) *doubleClickState {
	if timeoutMs <= 0 {
		timeoutMs = 500
	}
	return &doubleClickState{
		timeoutMs:           timeoutMs,
		commitDelayMs:       80,
		graceMs:             150,
		pending:             make(map[int]*time.Timer),
		pendingCommit:       make(map[int]*time.Timer),
		pendingCfg:          make(map[int]doubleClickCfg),
		doubleClickNext:     make(map[int]bool),
		lastSimpleClickTime: make(map[int]time.Time),
	}
}

func (d *doubleClickState) clearState() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, t := range d.pending {
		t.Stop()
	}
	for _, t := range d.pendingCommit {
		t.Stop()
	}
	d.pending = make(map[int]*time.Timer)
	d.pendingCommit = make(map[int]*time.Timer)
	d.pendingCfg = make(map[int]doubleClickCfg)
	d.doubleClickNext = make(map[int]bool)
	d.lastSimpleClickTime = make(map[int]time.Time)
}

func (d *doubleClickState) onPress(btn int, cfg doubleClickCfg, kb uinput.Keyboard, layout string) bool {
	if len(cfg.KeysDouble) == 0 {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if t, ok := d.pending[btn]; ok {
		stopped := t.Stop()
		delete(d.pending, btn)
		delete(d.pendingCfg, btn)
		if stopped {
			d.doubleClickNext[btn] = true
			return true
		}
		return false
	}
	if t, ok := d.pendingCommit[btn]; ok {
		t.Stop()
		delete(d.pendingCommit, btn)
		d.doubleClickNext[btn] = true
		return true
	}
	if isInternalBatteryOverlay(cfg.KeysDouble) {
		if t, ok := d.lastSimpleClickTime[btn]; ok && time.Since(t) < time.Duration(d.graceMs)*time.Millisecond {
			delete(d.lastSimpleClickTime, btn)
			d.doubleClickNext[btn] = true
			return true
		}
	}
	return false
}

func (d *doubleClickState) onRelease(btn int, cfg doubleClickCfg, kb uinput.Keyboard, layout string) (handled bool) {
	d.mu.Lock()
	if d.doubleClickNext[btn] {
		delete(d.doubleClickNext, btn)
		d.mu.Unlock()
		if isInternalBatteryOverlay(cfg.KeysDouble) && d.onBatteryOverlay != nil {
			d.onBatteryOverlay()
		} else {
			sendKeysPressRelease(kb, cfg.KeysDouble, layout)
		}
		return true
	}
	if len(cfg.KeysDouble) == 0 {
		d.mu.Unlock()
		return false
	}
	cfgCopy := cfg
	d.mu.Unlock()
	t := time.AfterFunc(time.Duration(d.timeoutMs)*time.Millisecond, func() {
		d.mu.Lock()
		delete(d.pending, btn)
		dc := d.pendingCfg[btn]
		delete(d.pendingCfg, btn)
		d.mu.Unlock()
		if !isInternalCycle(dc.Keys) {
			sendKeysPressRelease(kb, dc.Keys, layout)
			return
		}
		commit := time.AfterFunc(time.Duration(d.commitDelayMs)*time.Millisecond, func() {
			d.mu.Lock()
			delete(d.pendingCommit, btn)
			d.lastSimpleClickTime[btn] = time.Now()
			d.mu.Unlock()
			if d.onLayerCycle != nil {
				d.onLayerCycle()
			}
		})
		d.mu.Lock()
		d.pendingCommit[btn] = commit
		d.mu.Unlock()
	})
	d.mu.Lock()
	if old, ok := d.pending[btn]; ok {
		old.Stop()
	}
	d.pending[btn] = t
	d.pendingCfg[btn] = cfgCopy
	d.mu.Unlock()
	return true
}

// handleButtons returns true if a layer cycle was requested
func handleButtons(kb uinput.Keyboard, k1, k2, pk1, pk2 uint8, layer Layer, layout string, button8Double []string, dblClick *doubleClickState) bool {
	layerCycleRequested := false

	handleBtn := func(btn int, isPressed, wasPressed bool) bool {
		if isPressed == wasPressed {
			return false
		}
		cfg, ok := layer.Buttons[btn]
		if !ok {
			if btn == 8 {
				cfg = ButtonCfg{Label: "Layer", Keys: []string{"INTERNAL_LAYER_CYCLE"}}
			} else {
				return false
			}
		}
		keysDouble := ([]string)(nil)
		if btn == 8 && len(button8Double) > 0 {
			keysDouble = button8Double
		}
		if isInternalCycle(cfg.Keys) && !isInternalBatteryOverlay(keysDouble) {
			if isPressed {
				layerCycleRequested = true
			}
			return true
		}
		hasDouble := len(keysDouble) > 0
		dcCfg := doubleClickCfg{Keys: cfg.Keys, KeysDouble: keysDouble}
		if isPressed {
			if isInternalBatteryOverlay(cfg.Keys) && dblClick != nil && dblClick.onBatteryOverlay != nil {
				dblClick.onBatteryOverlay()
				return true
			}
			if hasDouble && dblClick != nil && dblClick.onPress(btn, dcCfg, kb, layout) {
				return true
			}
			if !hasDouble {
				sendKeys(kb, cfg.Keys, true, layout)
			}
			return true
		}
		if isInternalBatteryOverlay(cfg.Keys) && dblClick != nil && dblClick.onBatteryOverlay != nil {
			return true
		}
		if hasDouble && dblClick != nil && dblClick.onRelease(btn, dcCfg, kb, layout) {
			return true
		}
		if !hasDouble {
			sendKeys(kb, cfg.Keys, false, layout)
		}
		return true
	}

	for i := 0; i < 8; i++ {
		mask := uint8(1 << i)
		handleBtn(i, (k1&mask) > 0, (pk1&mask) > 0)
	}
	handleBtn(8, (k2&1) > 0, (pk2&1) > 0)
	handleBtn(9, (k2&2) > 0, (pk2&2) > 0)

	return layerCycleRequested
}

func handleWheel(kb uinput.Keyboard, val uint8, layer Layer, layout string) {
	if val == 1 { // Right
		sendKeysPressRelease(kb, layer.Wheel.Right, layout)
	} else if val == 2 { // Left
		sendKeysPressRelease(kb, layer.Wheel.Left, layout)
	}
}

func isInternalCycle(keys []string) bool {
	for _, k := range keys {
		if k == "INTERNAL_LAYER_CYCLE" {
			return true
		}
	}
	return false
}

func isInternalBatteryOverlay(keys []string) bool {
	for _, k := range keys {
		if k == "INTERNAL_BATTERY_OVERLAY" {
			return true
		}
	}
	return false
}

// keyMap returns uinput key code. For azerty, maps logical key (KEY_A = "A") to physical key.
func keyMap(name string, layout string) int {
	qwerty := map[string]int{
		"KEY_A": uinput.KeyA, "KEY_B": uinput.KeyB, "KEY_C": uinput.KeyC,
		"KEY_D": uinput.KeyD, "KEY_E": uinput.KeyE, "KEY_F": uinput.KeyF,
		"KEY_G": uinput.KeyG, "KEY_H": uinput.KeyH, "KEY_I": uinput.KeyI,
		"KEY_J": uinput.KeyJ, "KEY_K": uinput.KeyK, "KEY_L": uinput.KeyL,
		"KEY_M": uinput.KeyM, "KEY_N": uinput.KeyN, "KEY_O": uinput.KeyO,
		"KEY_P": uinput.KeyP, "KEY_Q": uinput.KeyQ, "KEY_R": uinput.KeyR,
		"KEY_S": uinput.KeyS, "KEY_T": uinput.KeyT, "KEY_U": uinput.KeyU,
		"KEY_V": uinput.KeyV, "KEY_W": uinput.KeyW, "KEY_X": uinput.KeyX,
		"KEY_Y": uinput.KeyY, "KEY_Z": uinput.KeyZ,
		"KEY_1": uinput.Key1, "KEY_2": uinput.Key2, "KEY_3": uinput.Key3,
		"KEY_4": uinput.Key4, "KEY_5": uinput.Key5, "KEY_6": uinput.Key6,
		"KEY_7": uinput.Key7, "KEY_8": uinput.Key8, "KEY_9": uinput.Key9,
		"KEY_0":        uinput.Key0,
		"KEY_LEFTCTRL": uinput.KeyLeftctrl, "KEY_LEFTSHIFT": uinput.KeyLeftshift,
		"KEY_LEFTALT": uinput.KeyLeftalt, "KEY_TAB": uinput.KeyTab,
		"KEY_ENTER": uinput.KeyEnter, "KEY_ESC": uinput.KeyEsc,
		"KEY_BACKSPACE": uinput.KeyBackspace, "KEY_BACKSLASH": uinput.KeyBackslash,
		"KEY_MINUS": uinput.KeyMinus, "KEY_EQUAL": uinput.KeyEqual,
		"KEY_LEFTBRACE": uinput.KeyLeftbrace, "KEY_RIGHTBRACE": uinput.KeyRightbrace,
		"KEY_PAGEUP": uinput.KeyPageup, "KEY_PAGEDOWN": uinput.KeyPagedown,
		// Arrow Keys
		"KEY_LEFT": uinput.KeyLeft, "KEY_RIGHT": uinput.KeyRight,
		"KEY_UP": uinput.KeyUp, "KEY_DOWN": uinput.KeyDown,

		"KEY_VOLUMEDOWN": uinput.KeyVolumedown, "KEY_VOLUMEUP": uinput.KeyVolumeup,
		"KEY_MUTE": uinput.KeyMute, "KEY_SPACE": uinput.KeySpace,
		"KEY_F1": uinput.KeyF1, "KEY_F2": uinput.KeyF2, "KEY_F3": uinput.KeyF3,
		"KEY_F4": uinput.KeyF4, "KEY_F5": uinput.KeyF5, "KEY_F6": uinput.KeyF6,
		"KEY_F7": uinput.KeyF7, "KEY_F8": uinput.KeyF8, "KEY_F9": uinput.KeyF9,
		"KEY_F10": uinput.KeyF10, "KEY_F11": uinput.KeyF11, "KEY_F12": uinput.KeyF12,
	}
	// AZERTY (fr-FR): KEY_A = physical key for "A" (KeyQ), KEY_Q = KeyA, KEY_Z = KeyW, KEY_W = KeyZ
	azerty := map[string]int{
		"KEY_A": uinput.KeyQ, "KEY_Q": uinput.KeyA, "KEY_Z": uinput.KeyW, "KEY_W": uinput.KeyZ,
	}
	for k, v := range qwerty {
		if _, ok := azerty[k]; !ok {
			azerty[k] = v
		}
	}
	m := qwerty
	if strings.ToLower(layout) == "azerty" {
		m = azerty
	}
	if v, ok := m[name]; ok {
		return v
	}
	log.Printf("Unknown key: %s", name)
	return 0
}

func sendKeys(kb uinput.Keyboard, keys []string, press bool, layout string) {
	for _, kName := range keys {
		code := keyMap(kName, layout)
		if code != 0 {
			if press {
				kb.KeyDown(code)
			} else {
				kb.KeyUp(code)
			}
		}
	}
}

func sendKeysPressRelease(kb uinput.Keyboard, keys []string, layout string) {
	for _, kName := range keys {
		code := keyMap(kName, layout)
		if code != 0 {
			kb.KeyPress(code)
		}
	}
}
