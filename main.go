package main

import (
	"bytes"
	"encoding/binary"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"

	"github.com/bendahl/uinput"
	"github.com/karalabe/hid"
	"gopkg.in/yaml.v3"
)

const (
	VendorID       = 0x28BD
	ProdIDWire     = 0x5202
	ProdIDWire2    = 0x5204 // Some wired variants
	ProdIDWireless = 0x5203
)

// Configuration Structures
type Config struct {
	Device DeviceSettings `yaml:"device"`
	Layers []Layer        `yaml:"layers"`
}

type DeviceSettings struct {
	Brightness   string `yaml:"brightness"`
	Orientation  int    `yaml:"orientation"`
	WheelSpeed   string `yaml:"wheel_speed"`
	SleepTimeout uint8  `yaml:"sleep_timeout"`
	WheelColor   struct {
		R uint8 `yaml:"r"`
		G uint8 `yaml:"g"`
		B uint8 `yaml:"b"`
	} `yaml:"wheel_color"`
}

type Layer struct {
	Name    string            `yaml:"name"`
	Buttons map[int]ButtonCfg `yaml:"buttons"`
	Wheel   struct {
		Left  []string `yaml:"left"`
		Right []string `yaml:"right"`
	} `yaml:"wheel"`
}

type ButtonCfg struct {
	Label string   `yaml:"label"`
	Keys  []string `yaml:"keys"`
}

func padBytes(input []byte) []byte {
	out := make([]byte, 32)
	copy(out, input)
	return out
}

func encodeText(text string) []byte {
	// Encode to UTF-16LE
	utf16Units := utf16.Encode([]rune(text))
	buf := new(bytes.Buffer)
	for _, unit := range utf16Units {
		binary.Write(buf, binary.LittleEndian, unit)
	}
	b := buf.Bytes()
	// Pad to 16 bytes if necessary for key labels
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
	// Payload starts at index 16 in the 32-byte report
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

func main() {
	// Load Config
	cfgData, err := os.ReadFile("config.yaml")
	if err != nil {
		log.Fatalf("Error reading config: %v", err)
	}
	var config Config
	if err := yaml.Unmarshal(cfgData, &config); err != nil {
		log.Fatalf("Error parsing YAML: %v", err)
	}

	// Setup UInput (Virtual Keyboard)
	// Cast string to []byte for the name argument
	keyboard, err := uinput.CreateKeyboard("/dev/uinput", []byte("Xencelabs QuickKeys Virtual"))
	if err != nil {
		log.Fatalf("Failed to create virtual keyboard: %v (Do you have permissions on /dev/uinput?)", err)
	}
	defer keyboard.Close()

	// 3. Connect to Device
	devInfo := findDevice()
	if devInfo == nil {
		log.Fatal("Xencelabs Quick Keys device not found")
	}

	dev, err := devInfo.Open()
	if err != nil {
		log.Fatalf("Failed to open device: %v", err)
	}
	defer dev.Close()

	log.Printf("Connected to %04x:%04x", devInfo.VendorID, devInfo.ProductID)

	// 4. Initialize Device Settings
	initDevice(dev, &config)

	// 5. Input Loop
	// We need to track state to handle key holds vs presses
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	// Input buffer
	buf := make([]byte, 32)

	// Previous button state bitmap to detect edges
	var prevBtn1, prevBtn2 uint8

	go func() {
		for {
			n, err := dev.Read(buf)
			if err != nil {
				log.Printf("Read error: %v", err)
				break
			}
			if n < 10 {
				continue
			}

			// Adjust offset if report ID is missing or shifted
			// Standard packet: [0x02, 0xf0, keys1, keys2, ... ]
			offset := 0
			if buf[0] == 0xf0 {
				offset = -1 // Data shifted
			}

			if buf[offset+1] == 0xf0 {
				// Input Report
				keys1 := buf[offset+2]
				keys2 := buf[offset+3]
				wheel := buf[offset+7]

				// Handle Buttons
				handleButtons(keyboard, keys1, keys2, prevBtn1, prevBtn2, config.Layers[0])
				prevBtn1 = keys1
				prevBtn2 = keys2

				// Handle Wheel
				if wheel > 0 {
					handleWheel(keyboard, wheel, config.Layers[0])
				}
			} else if buf[offset+1] == 0xf2 {
				// Battery Report
				log.Printf("Battery: %d%%", buf[offset+3])
			}
		}
	}()

	<-sigs
	log.Println("Exiting...")
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

func initDevice(dev *hid.Device, cfg *Config) {
	// Subscribe
	dev.Write(padBytes([]byte{0x02, 0xb0, 0x04})) // Key events
	dev.Write(padBytes([]byte{0x02, 0xb4, 0x10})) // Battery

	// Settings
	dev.Write(cmdSetOrientation(cfg.Device.Orientation))
	dev.Write(cmdSetBrightness(cfg.Device.Brightness))
	dev.Write(cmdSetWheelSpeed(cfg.Device.WheelSpeed))
	dev.Write(cmdSleepTimeout(cfg.Device.SleepTimeout))
	dev.Write(cmdSetWheelColor(cfg.Device.WheelColor.R, cfg.Device.WheelColor.G, cfg.Device.WheelColor.B))

	// Labels
	layer := cfg.Layers[0] // Only handling first layer for MVP
	for idx, btn := range layer.Buttons {
		if idx <= 7 {
			dev.Write(cmdSetKeyText(uint8(idx), btn.Label))
		}
	}

	// Small delay to ensure commands process
	time.Sleep(100 * time.Millisecond)
}

func handleButtons(kb uinput.Keyboard, k1, k2, pk1, pk2 uint8, layer Layer) {
	// Map hardware bit index to config index
	// k1: bits 0-7 -> buttons 0-7
	// k2: bit 0 -> button 8 (wheel button)

	// Check Buttons 0-7
	for i := 0; i < 8; i++ {
		mask := uint8(1 << i)
		isPressed := (k1 & mask) > 0
		wasPressed := (pk1 & mask) > 0

		if isPressed != wasPressed {
			if _, ok := layer.Buttons[i]; ok {
				sendKeys(kb, layer.Buttons[i].Keys, isPressed)
			}
		}
	}

	// Check Button 8 (Wheel center)
	maskExtra := uint8(1)
	isExtra := (k2 & maskExtra) > 0
	wasExtra := (pk2 & maskExtra) > 0
	if isExtra != wasExtra {
		if _, ok := layer.Buttons[8]; ok {
			sendKeys(kb, layer.Buttons[8].Keys, isExtra)
		}
	}
}

func handleWheel(kb uinput.Keyboard, val uint8, layer Layer) {
	// 1 = Right, 2 = Left
	if val == 1 {
		// Right
		sendKeysPressRelease(kb, layer.Wheel.Right)
	} else if val == 2 {
		// Left
		sendKeysPressRelease(kb, layer.Wheel.Left)
	}
}

// Map string names to uinput key codes
func keyMap(name string) int {
	m := map[string]int{
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
		"KEY_MINUS": uinput.KeyMinus, "KEY_EQUAL": uinput.KeyEqual,
		"KEY_LEFTBRACE": uinput.KeyLeftbrace, "KEY_RIGHTBRACE": uinput.KeyRightbrace,
	}
	if v, ok := m[name]; ok {
		return v
	}
	log.Printf("Unknown key: %s", name)
	return 0
}

func sendKeys(kb uinput.Keyboard, keys []string, press bool) {
	for _, kName := range keys {
		code := keyMap(kName)
		if code != 0 {
			if press {
				kb.KeyDown(code)
			} else {
				kb.KeyUp(code)
			}
		}
	}
}

func sendKeysPressRelease(kb uinput.Keyboard, keys []string) {
	for _, kName := range keys {
		code := keyMap(kName)
		if code != 0 {
			kb.KeyPress(code)
		}
	}
}
