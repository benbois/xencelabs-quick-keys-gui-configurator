//go:build tray && !bindings
// +build tray,!bindings

package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/bendahl/uinput"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend
var assets embed.FS

//go:embed xencelabs-icon.png
var iconData []byte

// embeddedTrayIcon is used by the systray when no icon file is found on disk.
var embeddedTrayIcon = iconData

// GlobalApp is set when Wails starts; tray uses it to show the config window.
var GlobalApp *App

// configReloadFromGUI signals the device daemon to reload config when the GUI saves.
var configReloadFromGUI chan struct{}

func main() {
	showGUIOnStart := false
	for _, arg := range os.Args[1:] {
		if arg == "-show-gui" || arg == "--show-gui" {
			showGUIOnStart = true
			break
		}
	}

	configPath, err := getConfigPath()
	if err != nil {
		log.Fatalf("Cannot get config path: %v", err)
	}

	keyboard, err := uinput.CreateKeyboard("/dev/uinput", []byte("Xencelabs QuickKeys Virtual"))
	if err != nil {
		log.Fatalf("Failed to create virtual keyboard: %v", err)
	}
	defer keyboard.Close()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	sigusr1 := make(chan os.Signal, 1)
	signal.Notify(sigusr1, syscall.SIGUSR1)
	go func() {
		for range sigusr1 {
			showTrayInfoPopup()
		}
	}()

	configReloadFromGUI = make(chan struct{}, 1)
	app := NewApp()
	GlobalApp = app

	if os.Getenv("XENCELABS_TRAY_DEBUG") == "1" {
		trayDebugEnv()
	}
	// Tray started in app.startup() after Wails/GTK is ready (avoids GTK conflict)

	go runDeviceDaemon(configPath, keyboard, sigs)

	ws := loadGUIState(configPath)
	if ws.WidthDp < 1024 {
		ws.WidthDp = 1024
	}
	if ws.HeightDp < 768 {
		ws.HeightDp = 768
	}

	err = wails.Run(&options.App{
		Title:             "Xencelabs Quick Keys - Configuration",
		Width:             int(ws.WidthDp),
		Height:            int(ws.HeightDp),
		MinWidth:          1024,
		MinHeight:         768,
		HideWindowOnClose: true, // Fermer la fenêtre la cache, l'app reste en systray
		OnBeforeClose: func(ctx context.Context) bool {
			if w, h := runtime.WindowGetSize(ctx); w > 0 && h > 0 && GlobalApp != nil {
				GlobalApp.SaveGUIState(float64(w), float64(h))
			}
			return false // ne pas empêcher la fermeture
		},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		Linux: &linux.Options{
			Icon:          iconData,
			ApplicationID: "xencelabs-quick-keys",
		},
		OnStartup:        app.startup,
		OnShutdown:       func(_ context.Context) { stopTray() },
		Bind:             []interface{}{app},
		StartHidden:     !showGUIOnStart,
		DisableResize:    false,
		WindowStartState: options.Normal,
	})
	if err != nil {
		log.Fatalf("Wails error: %v", err)
	}
}

func runDeviceDaemon(configPath string, keyboard uinput.Keyboard, sigs chan os.Signal) {
	for {
		devInfo := findDevice()
		if devInfo == nil {
			log.Println("Xencelabs Quick Keys device not found, waiting...")
			time.Sleep(2 * time.Second)
			continue
		}

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
			time.Sleep(100 * time.Millisecond)
		}

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

		// Flush: also process battery report if device sent one in response to initGlobalSettings
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

		deviceDone := make(chan struct{})
		configReload := make(chan struct{}, 1)
		go watchConfigFile(configPath, configReload, deviceDone)
		isWireless := len(deviceId) > 0

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
					continue
				}
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

		doConfigReload := func() {
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
		}

		for {
			select {
			case <-deviceDone:
				traySetConnected(false)
				log.Println("Reconnecting...")
				time.Sleep(1 * time.Second)
				goto nextConnection
			case <-configReload:
				doConfigReload()
			case <-configReloadFromGUI:
				doConfigReload()
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
