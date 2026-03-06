//go:build tray
// +build tray

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	trayState struct {
		sync.Mutex
		battery       int  // 0-100, -1 = unknown
		charging      bool // true = en charge
		connected     bool
		receivedData  bool // true when we've received at least one HID report (proves real communication)
		layerName     string
		layerR        uint8
		layerG        uint8
		layerB        uint8
	}
)

func trayAvailable() bool {
	return os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
}

func trayDebugEnv() {
	writeTrayDebug("main: DISPLAY=%q WAYLAND=%q DBUS=%q HOME=%q trayAvail=%v",
		os.Getenv("DISPLAY"), os.Getenv("WAYLAND_DISPLAY"),
		os.Getenv("DBUS_SESSION_BUS_ADDRESS"), os.Getenv("HOME"),
		trayAvailable())
}

func writeTrayDebug(format string, args ...interface{}) {
	stateHome := os.Getenv("XDG_STATE_HOME")
	home := os.Getenv("HOME")
	var logPath string
	if stateHome != "" {
		logPath = filepath.Join(stateHome, "xencelabs-quick-keys", "tray-debug.log")
	} else if home != "" {
		logPath = filepath.Join(home, ".local", "state", "xencelabs-quick-keys", "tray-debug.log")
	} else {
		logPath = "/tmp/xencelabs-quick-keys-tray-debug.log"
	}
	dir := filepath.Dir(logPath)
	os.MkdirAll(dir, 0755)
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "[%s] ", time.Now().Format("15:04:05.000"))
	fmt.Fprintf(f, format, args...)
	fmt.Fprintln(f)
}

func trayWriteStatusFile(tooltip string) {
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		dir = filepath.Join(os.Getenv("HOME"), ".local", "state")
	}
	dir = filepath.Join(dir, "xencelabs-quick-keys")
	os.MkdirAll(dir, 0755)
	f, err := os.Create(filepath.Join(dir, "status.txt"))
	if err != nil {
		return
	}
	fmt.Fprintf(f, "Xencelabs Quick Keys\n\n%s", trayGetStatusText())
	f.Close()
}

// onTrayStateChange is called after state updates; set by tray_systray or tray_wails
var onTrayStateChange func()

// onDeviceStateChangeToGUI is called when device connects/disconnects; set by App for Wails GUI live update
var onDeviceStateChangeToGUI func(connected bool)

func traySetBattery(percent int, charging bool) {
	trayState.Lock()
	wasRecognized := trayState.connected && (trayState.battery >= 0 || trayState.receivedData)
	trayState.battery = percent
	trayState.charging = charging
	nowRecognized := trayState.connected && (trayState.battery >= 0 || trayState.receivedData)
	trayState.Unlock()
	if onTrayStateChange != nil {
		onTrayStateChange()
	}
	if !wasRecognized && nowRecognized && onDeviceStateChangeToGUI != nil {
		go onDeviceStateChangeToGUI(true)
	}
}

func traySetConnected(connected bool) {
	trayState.Lock()
	trayState.connected = connected
	trayState.receivedData = false
	trayState.battery = -1
	trayState.charging = false
	trayState.Unlock()
	if onTrayStateChange != nil {
		onTrayStateChange()
	}
	if onDeviceStateChangeToGUI != nil {
		go func(c bool) { onDeviceStateChangeToGUI(c) }(connected)
	}
}

func trayGetStatusText() string {
	trayState.Lock()
	batt := trayState.battery
	charging := trayState.charging
	conn := trayState.connected
	layer := trayState.layerName
	trayState.Unlock()

	status := "Disconnected"
	if conn {
		status = "Connected"
	}
	battStr := "—"
	if batt >= 0 {
		battStr = fmt.Sprintf("%d%%", batt)
		if charging {
			battStr += " (charging)"
		} else {
			battStr += " (discharging)"
		}
	}
	layerStr := strings.TrimSpace(strings.ReplaceAll(layer, "%", ""))
	if layerStr == "" {
		layerStr = "—"
	}
	return fmt.Sprintf("Status: %s\nLayer: %s\nBattery: %s", status, layerStr, battStr)
}

func trayGetBattery() int {
	trayState.Lock()
	b := trayState.battery
	trayState.Unlock()
	return b
}

// trayGetBatteryStatus returns (percent 0-100, charging). percent is -1 if unknown.
func trayGetBatteryStatus() (percent int, charging bool) {
	trayState.Lock()
	percent = trayState.battery
	charging = trayState.charging
	trayState.Unlock()
	return percent, charging
}

// traySetReceivedData marks that we've received at least one HID report (proves real communication).
// Emits deviceStateChanged(true) when we transition to "recognized" so the GUI updates promptly.
func traySetReceivedData() {
	trayState.Lock()
	wasRecognized := trayState.connected && (trayState.battery >= 0 || trayState.receivedData)
	trayState.receivedData = true
	nowRecognized := trayState.connected && (trayState.battery >= 0 || trayState.receivedData)
	trayState.Unlock()
	if !wasRecognized && nowRecognized && onDeviceStateChangeToGUI != nil {
		go onDeviceStateChangeToGUI(true)
	}
}

// trayGetConnected returns true when the device is connected and opened.
func trayGetConnected() bool {
	trayState.Lock()
	c := trayState.connected
	trayState.Unlock()
	return c
}

// trayGetRecognized returns true when the device is connected AND we have evidence of real communication
// (battery data or at least one HID report). Used to avoid showing "Plugged" for devices that don't respond.
func trayGetRecognized() bool {
	trayState.Lock()
	conn := trayState.connected
	batt := trayState.battery
	recv := trayState.receivedData
	trayState.Unlock()
	return conn && (batt >= 0 || recv)
}

func traySetLayer(name string, r, g, b uint8) {
	trayState.Lock()
	trayState.layerName = name
	trayState.layerR, trayState.layerG, trayState.layerB = r, g, b
	trayState.Unlock()
	if onTrayStateChange != nil {
		onTrayStateChange()
	}
}
