package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

var (
	lastOpenFailLog     time.Time
	openFailLogMu       sync.Mutex
	openFailLoggedOnce  bool
)

// logOpenFailThrottled logs "Failed to open device" only once until we successfully connect.
func logOpenFailThrottled(err error) {
	openFailLogMu.Lock()
	defer openFailLogMu.Unlock()
	if openFailLoggedOnce {
		return
	}
	openFailLoggedOnce = true
	log.Printf("Failed to open device: %v, retrying every 2s...", err)
}

// resetOpenFailLog clears the throttle so we log again after a successful connection.
func resetOpenFailLog() {
	openFailLogMu.Lock()
	openFailLoggedOnce = false
	openFailLogMu.Unlock()
}

// Configuration Structures (shared between tray and non-tray builds)
type Config struct {
	Device DeviceSettings `yaml:"device"`
	Layers []Layer        `yaml:"layers"`
}

type DeviceSettings struct {
	Brightness              string   `yaml:"brightness"`
	Orientation             int      `yaml:"orientation"`
	WheelSpeed              string   `yaml:"wheel_speed"`
	OverlayDuration         uint8    `yaml:"overlay_duration"`
	KeyboardLayout          string   `yaml:"keyboard_layout"`
	SleepTimeout            uint8    `yaml:"sleep_timeout"`
	InitialLayer            string   `yaml:"initial_layer"`
	DoubleClickMs           int      `yaml:"double_click_ms"`
	Button8Double           []string `yaml:"button_8_double"`
	ShowBatteryInLayerName  bool     `yaml:"show_battery_in_layer_name"`
}

type RGB struct {
	R uint8 `yaml:"r"`
	G uint8 `yaml:"g"`
	B uint8 `yaml:"b"`
}

type Layer struct {
	Name       string            `yaml:"name"`
	Color      RGB               `yaml:"color"`
	WheelSpeed string            `yaml:"wheel_speed"`
	Buttons    map[int]ButtonCfg `yaml:"buttons"`
	Wheel      struct {
		Left  []string `yaml:"left"`
		Right []string `yaml:"right"`
	} `yaml:"wheel"`
}

type ButtonCfg struct {
	Label string   `yaml:"label"`
	Keys  []string `yaml:"keys"`
}

func getConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "Xencelab", "config.yaml"), nil
}

func resolveInitialLayerIndex(cfg *Config) int {
	s := strings.TrimSpace(cfg.Device.InitialLayer)
	if s == "" {
		return 0
	}
	var idx int
	if _, err := fmt.Sscanf(s, "%d", &idx); err == nil && idx >= 0 && idx < len(cfg.Layers) {
		return idx
	}
	sLower := strings.ToLower(s)
	for i, l := range cfg.Layers {
		if strings.ToLower(strings.TrimSpace(l.Name)) == sLower {
			return i
		}
	}
	return 0
}

func watchConfigFile(configPath string, reloadCh chan<- struct{}, done <-chan struct{}) {
	var lastMod time.Time
	if info, err := os.Stat(configPath); err == nil {
		lastMod = info.ModTime()
	}
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			info, err := os.Stat(configPath)
			if err != nil {
				continue
			}
			if info.ModTime().After(lastMod) {
				lastMod = info.ModTime()
				time.Sleep(300 * time.Millisecond)
				if info2, err := os.Stat(configPath); err == nil {
					lastMod = info2.ModTime()
					select {
					case reloadCh <- struct{}{}:
					default:
					}
				}
			}
		}
	}
}

const defaultConfigYAML = `device:
  brightness: medium
  orientation: 0
  wheel_speed: normal
  overlay_duration: 2
  keyboard_layout: azerty
  sleep_timeout: 30
  initial_layer: 0
  double_click_ms: 500
  button_8_double: ["INTERNAL_BATTERY_OVERLAY"]
  show_battery_in_layer_name: true

layers:
  - name: "General"
    color: { r: 192, g: 192, b: 192 }
    wheel_speed: fastest
    buttons:
      0:
        label: "Copy"
        keys: ["KEY_LEFTCTRL", "KEY_C"]
      8:
        label: "Layer"
        keys: ["INTERNAL_LAYER_CYCLE"]
      9:
        label: "Enter"
        keys: ["KEY_ENTER"]
    wheel:
      left: ["KEY_VOLUMEDOWN"]
      right: ["KEY_VOLUMEUP"]
`

func ensureConfigExists(configPath string) error {
	_, err := os.Stat(configPath)
	if err == nil {
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(configPath, []byte(defaultConfigYAML), 0644)
}

func loadConfig(configPath string) (*Config, error) {
	if err := ensureConfigExists(configPath); err != nil {
		return nil, err
	}
	cfgData, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	var config Config
	if err := yaml.Unmarshal(cfgData, &config); err != nil {
		return nil, err
	}
	if len(config.Layers) == 0 {
		return nil, fmt.Errorf("no layers defined in config")
	}
	if config.Device.OverlayDuration == 0 {
		config.Device.OverlayDuration = 2
	}
	if config.Device.DoubleClickMs <= 0 {
		config.Device.DoubleClickMs = 500
	}
	if config.Device.Button8Double == nil {
		config.Device.Button8Double = []string{"INTERNAL_BATTERY_OVERLAY"}
	}
	return &config, nil
}

func backupConfig(configPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	dir := filepath.Dir(configPath)
	backupPath := filepath.Join(dir, "config_save.yaml")
	return os.WriteFile(backupPath, data, 0644)
}

func saveConfig(configPath string, cfg *Config) error {
	configPath, err := filepath.Abs(configPath)
	if err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	tmpPath := configPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, configPath); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}
