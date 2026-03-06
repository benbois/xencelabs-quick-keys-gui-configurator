//go:build tray && wails
// +build tray,wails

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/slytomcat/systray"
)

var (
	baseIconData []byte
	lastIconData []byte
	mBattery     *systray.MenuItem
	mStatus      *systray.MenuItem
	mLayer       *systray.MenuItem
	trayEndFn    func()
)

func init() {
	onTrayStateChange = trayUpdateTooltip
}

func startTray(iconPath string) {
	if !trayAvailable() {
		trayWriteStatusFile(trayGetStatusText())
		return
	}
	start, end := systray.RunWithExternalLoop(trayOnReady(iconPath), trayOnExit)
	trayEndFn = end
	start()
}

func stopTray() {
	if trayEndFn != nil {
		trayEndFn()
		trayEndFn = nil
	}
}

func trayOnReady(iconPath string) func() {
	return func() {
		iconData := loadTrayIcon(iconPath)
		if iconData == nil && len(embeddedTrayIcon) > 0 {
			iconData = embeddedTrayIcon
		}
		if iconData != nil {
			baseIconData = iconData
			traySetBaseIcon()
		} else {
			systray.SetTitle("Xencelabs Quick Keys")
		}
		mStatus = systray.AddMenuItem("Status: Disconnected", "")
		mStatus.Disable()
		mLayer = systray.AddMenuItem("Layer: —", "")
		mLayer.Disable()
		mBattery = systray.AddMenuItem("Battery: —", "")
		mBattery.Disable()
		systray.AddSeparator()
		mConfig := systray.AddMenuItem("Configure...", "Open configuration window")
		go func() {
			for range mConfig.ClickedCh {
				if GlobalApp != nil {
					GlobalApp.ShowWindow()
				}
			}
		}()
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("Quit", "")
		go func() {
			<-mQuit.ClickedCh
			stopTray()
			os.Exit(0)
		}()
		trayUpdateTooltip()
	}
}

func trayOnExit() {}

func traySetBaseIcon() {
	if len(baseIconData) == 0 {
		return
	}
	lastIconData = make([]byte, len(baseIconData))
	copy(lastIconData, baseIconData)
	systray.SetIcon(lastIconData)
}

func loadTrayIcon(path string) []byte {
	if path == "" {
		exe, err := os.Executable()
		if err != nil {
			return nil
		}
		dir := filepath.Dir(exe)
		for _, name := range []string{"xencelabs-icon.png", "xencelabs-quick-keys.png", "xencelabs-logo.png"} {
			p := filepath.Join(dir, name)
			if data, err := os.ReadFile(p); err == nil {
				return data
			}
		}
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return data
}

func trayUpdateTooltip() {
	trayState.Lock()
	batt := trayState.battery
	charging := trayState.charging
	trayState.Unlock()

	battStr := "—"
	if batt >= 0 {
		battStr = fmt.Sprintf("%d%%", batt)
		if charging {
			battStr += " (charging)"
		} else {
			battStr += " (discharging)"
		}
	}
	tooltip := fmt.Sprintf("Xencelabs Quick Keys — Battery: %s", battStr)
	systray.SetTitle(tooltip)
	systray.SetTooltip(tooltip)
	trayUpdateMenu()
	trayWriteStatusFile(tooltip)
}

func trayUpdateMenu() {
	if mStatus == nil || mLayer == nil || mBattery == nil {
		return
	}
	trayState.Lock()
	batt := trayState.battery
	charging := trayState.charging
	conn := trayState.connected
	layer := trayState.layerName
	trayState.Unlock()

	battStr := "—"
	if batt >= 0 {
		battStr = fmt.Sprintf("%d%%", batt)
		if charging {
			battStr += " (charging)"
		} else {
			battStr += " (discharging)"
		}
	}
	status := "Disconnected"
	if conn {
		status = "Connected"
	}
	layerStr := strings.TrimSpace(strings.ReplaceAll(layer, "%", ""))
	if layerStr == "" {
		layerStr = "—"
	}
	mStatus.SetTitle("Status: " + status)
	mLayer.SetTitle("Layer: " + layerStr)
	mBattery.SetTitle("Battery: " + battStr)
}
