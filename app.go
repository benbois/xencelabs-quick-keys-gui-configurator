//go:build tray
// +build tray

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"gopkg.in/yaml.v3"
)

// ConfigDTO is the JSON-friendly config for Wails bindings.
// Uses map[string]ButtonCfg because JSON object keys are strings.
type ConfigDTO struct {
	Device DeviceSettingsDTO `json:"device"`
	Layers []LayerDTO        `json:"layers"`
}

type DeviceSettingsDTO struct {
	Brightness              string   `json:"brightness"`
	Orientation            int      `json:"orientation"`
	WheelSpeed             string   `json:"wheel_speed"`
	OverlayDuration        uint8    `json:"overlay_duration"`
	KeyboardLayout         string   `json:"keyboard_layout"`
	SleepTimeout           uint8    `json:"sleep_timeout"`
	InitialLayer           string   `json:"initial_layer"`
	DoubleClickMs          int      `json:"double_click_ms"`
	Button8Double          []string `json:"button_8_double"`
	ShowBatteryInLayerName bool     `json:"show_battery_in_layer_name"`
}

type LayerDTO struct {
	Name       string                 `json:"name"`
	Color      RGB                    `json:"color"`
	WheelSpeed string                 `json:"wheel_speed"`
	Buttons    map[string]ButtonCfg   `json:"buttons"`
	Wheel      struct {
		Left  []string `json:"left"`
		Right []string `json:"right"`
	} `json:"wheel"`
}

// guiWindowState stores window dimensions for persistence.
type guiWindowState struct {
	WidthDp  float32 `json:"width_dp"`
	HeightDp float32 `json:"height_dp"`
}

// App holds the Wails application context and bindings.
type App struct {
	ctx context.Context
	mu  sync.Mutex
}

// NewApp creates a new App instance.
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. Store the context and start the tray (after Wails/GTK is ready).
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	onDeviceStateChangeToGUI = func(connected bool) {
		runtime.EventsEmit(a.ctx, "deviceStateChanged", connected)
	}
	if trayAvailable() {
		startTray(getTrayIconPath())
	}
}

// GetConfigPath returns the config file path.
func (a *App) GetConfigPath() (string, error) {
	return getConfigPath()
}

// BatteryStatus holds battery info for the GUI.
type BatteryStatus struct {
	Percent  int  `json:"percent"`  // 0-100, -1 if unknown
	Charging bool `json:"charging"` // true when charging
}

// GetBattery returns battery percent (0-100, -1 if unknown) and charging status.
func (a *App) GetBattery() BatteryStatus {
	pct, charging := trayGetBatteryStatus()
	return BatteryStatus{Percent: pct, Charging: charging}
}

// GetDeviceState returns "Plugged" when the device is connected AND recognized (real communication),
// "Unplugged" otherwise. Avoids showing "Plugged" for devices that don't respond.
func (a *App) GetDeviceState() string {
	if trayGetRecognized() {
		return "Plugged"
	}
	return "Unplugged"
}

// GetAvailableKeys returns the list of key names usable in the config.
func (a *App) GetAvailableKeys() []string {
	return AvailableKeys()
}

// LoadConfig loads the config and returns it as JSON-friendly DTO.
func (a *App) LoadConfig() (*ConfigDTO, error) {
	configPath, err := getConfigPath()
	if err != nil {
		return nil, err
	}
	cfg, err := loadConfig(configPath)
	if err != nil {
		return nil, err
	}
	return configToDTO(cfg), nil
}

// SaveConfig saves the config from DTO.
func (a *App) SaveConfig(dto *ConfigDTO) error {
	configPath, err := getConfigPath()
	if err != nil {
		return err
	}
	cfg := dtoToConfig(dto)
	if err := saveConfig(configPath, cfg); err != nil {
		return err
	}
	// Signal the device daemon to reload config immediately
	if configReloadFromGUI != nil {
		select {
		case configReloadFromGUI <- struct{}{}:
		default:
		}
	}
	return nil
}

// BackupConfig creates a backup of the config file.
func (a *App) BackupConfig() error {
	configPath, err := getConfigPath()
	if err != nil {
		return err
	}
	return backupConfig(configPath)
}

// ValidateYAMLResult holds the result of YAML validation.
type ValidateYAMLResult struct {
	Error string `json:"error"`
	Line  int    `json:"line"`
}

// ValidateConfigYAML validates YAML content. If valid, Error is empty and Line is 0.
func (a *App) ValidateConfigYAML(content string) ValidateYAMLResult {
	var cfg Config
	if err := yaml.Unmarshal([]byte(content), &cfg); err != nil {
		msg := err.Error()
		line := 0
		if re := regexp.MustCompile(`yaml: line (\d+):`); re.MatchString(msg) {
			if m := re.FindStringSubmatch(msg); len(m) >= 2 {
				if n, e := strconv.Atoi(m[1]); e == nil {
					line = n
				}
			}
		}
		return ValidateYAMLResult{Error: msg, Line: line}
	}
	if len(cfg.Layers) == 0 {
		return ValidateYAMLResult{Error: "no layers defined in config", Line: 0}
	}
	return ValidateYAMLResult{}
}

// ReadConfigFile returns the raw config file content.
func (a *App) ReadConfigFile() (string, error) {
	configPath, err := getConfigPath()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// WriteConfigFile writes raw content to the config file and signals the daemon to reload.
func (a *App) WriteConfigFile(content string) error {
	configPath, err := getConfigPath()
	if err != nil {
		return err
	}
	configPath, err = filepath.Abs(configPath)
	if err != nil {
		return err
	}
	tmpPath := configPath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(content), 0644); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, configPath); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if configReloadFromGUI != nil {
		select {
		case configReloadFromGUI <- struct{}{}:
		default:
		}
	}
	return nil
}

// GetGUIState returns the saved window size.
func (a *App) GetGUIState() (width, height float64, err error) {
	configPath, err := getConfigPath()
	if err != nil {
		return 0, 0, err
	}
	ws := loadGUIState(configPath)
	return float64(ws.WidthDp), float64(ws.HeightDp), nil
}

// SaveGUIState saves the window size.
func (a *App) SaveGUIState(width, height float64) error {
	configPath, err := getConfigPath()
	if err != nil {
		return err
	}
	saveGUIState(configPath, guiWindowState{
		WidthDp:  float32(width),
		HeightDp: float32(height),
	})
	return nil
}

// Quit terminates the application.
func (a *App) Quit() {
	runtime.Quit(a.ctx)
}

// ShowWindow shows the config window (called from tray).
func (a *App) ShowWindow() {
	if a.ctx != nil {
		runtime.WindowShow(a.ctx)
		runtime.WindowCenter(a.ctx) // recentre si la fenêtre était hors écran au démarrage
	}
}

// configToDTO converts Config to ConfigDTO.
func configToDTO(cfg *Config) *ConfigDTO {
	if cfg == nil {
		return nil
	}
	dto := &ConfigDTO{
		Device: DeviceSettingsDTO{
			Brightness:              cfg.Device.Brightness,
			Orientation:             cfg.Device.Orientation,
			WheelSpeed:              cfg.Device.WheelSpeed,
			OverlayDuration:         cfg.Device.OverlayDuration,
			KeyboardLayout:          cfg.Device.KeyboardLayout,
			SleepTimeout:            cfg.Device.SleepTimeout,
			InitialLayer:            cfg.Device.InitialLayer,
			DoubleClickMs:           cfg.Device.DoubleClickMs,
			Button8Double:           cfg.Device.Button8Double,
			ShowBatteryInLayerName:  cfg.Device.ShowBatteryInLayerName,
		},
		Layers: make([]LayerDTO, len(cfg.Layers)),
	}
	for i, l := range cfg.Layers {
		buttons := make(map[string]ButtonCfg)
		for k, v := range l.Buttons {
			buttons[strconv.Itoa(k)] = v
		}
		dto.Layers[i] = LayerDTO{
			Name:       l.Name,
			Color:      l.Color,
			WheelSpeed: l.WheelSpeed,
			Buttons:    buttons,
			Wheel: struct {
				Left  []string `json:"left"`
				Right []string `json:"right"`
			}{
				Left:  l.Wheel.Left,
				Right: l.Wheel.Right,
			},
		}
	}
	return dto
}

// dtoToConfig converts ConfigDTO to Config.
func dtoToConfig(dto *ConfigDTO) *Config {
	if dto == nil {
		return nil
	}
	cfg := &Config{
		Device: DeviceSettings{
			Brightness:             dto.Device.Brightness,
			Orientation:            dto.Device.Orientation,
			WheelSpeed:            dto.Device.WheelSpeed,
			OverlayDuration:       dto.Device.OverlayDuration,
			KeyboardLayout:       dto.Device.KeyboardLayout,
			SleepTimeout:         dto.Device.SleepTimeout,
			InitialLayer:         dto.Device.InitialLayer,
			DoubleClickMs:        dto.Device.DoubleClickMs,
			Button8Double:        dto.Device.Button8Double,
			ShowBatteryInLayerName: dto.Device.ShowBatteryInLayerName,
		},
		Layers: make([]Layer, len(dto.Layers)),
	}
	for i, l := range dto.Layers {
		buttons := make(map[int]ButtonCfg)
		for k, v := range l.Buttons {
			if idx, err := strconv.Atoi(k); err == nil {
				buttons[idx] = v
			}
		}
		cfg.Layers[i] = Layer{
			Name:       l.Name,
			Color:      l.Color,
			WheelSpeed: l.WheelSpeed,
			Buttons:    buttons,
			Wheel: struct {
				Left  []string `yaml:"left"`
				Right []string `yaml:"right"`
			}{
				Left:  l.Wheel.Left,
				Right: l.Wheel.Right,
			},
		}
	}
	return cfg
}

// loadGUIState and saveGUIState - need to be in main or a shared file.
// They're in gui.go which we're removing. We need to move them here or to a config file.
func loadGUIState(configPath string) guiWindowState {
	path := guiStatePath(configPath)
	data, err := os.ReadFile(path)
	if err != nil {
		return guiWindowState{WidthDp: 1024, HeightDp: 768}
	}
	var ws guiWindowState
	if err := json.Unmarshal(data, &ws); err != nil {
		return guiWindowState{WidthDp: 1024, HeightDp: 768}
	}
	if ws.WidthDp < 1024 {
		ws.WidthDp = 1024
	}
	if ws.HeightDp < 768 {
		ws.HeightDp = 768
	}
	return ws
}

func saveGUIState(configPath string, ws guiWindowState) {
	path := guiStatePath(configPath)
	dir := filepath.Dir(path)
	os.MkdirAll(dir, 0755)
	data, _ := json.MarshalIndent(ws, "", "  ")
	os.WriteFile(path, data, 0644)
}

func guiStatePath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "gui_state.json")
}