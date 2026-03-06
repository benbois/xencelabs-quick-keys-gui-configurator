//go:build tray && !wails
// +build tray,!wails

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/getlantern/systray"
)

var (
	baseIconData []byte // icône de base
	lastIconData []byte // garde les octets vivants pour le callback C (évite GC)
	mBattery     *systray.MenuItem
	mStatus      *systray.MenuItem
	mLayer       *systray.MenuItem
)

func init() {
	onTrayStateChange = trayUpdateTooltip
}

func startTray(iconPath string) {
	debugTray := os.Getenv("XENCELABS_TRAY_DEBUG") == "1"
	if debugTray {
		writeTrayDebug("startTray: DISPLAY=%q WAYLAND=%q DBUS=%q trayAvail=%v",
			os.Getenv("DISPLAY"), os.Getenv("WAYLAND_DISPLAY"),
			os.Getenv("DBUS_SESSION_BUS_ADDRESS"), trayAvailable())
	}
	if !trayAvailable() {
		if debugTray {
			writeTrayDebug("startTray: skip (no display)")
		}
		return
	}
	go func() {
		if debugTray {
			writeTrayDebug("startTray: launching systray goroutine")
		}
		systray.Run(trayOnReady(iconPath), trayOnExit)
		if debugTray {
			writeTrayDebug("startTray: systray exited")
		}
	}()
}

func trayOnReady(iconPath string) func() {
	return func() {
		iconData := loadTrayIcon(iconPath)
		if os.Getenv("XENCELABS_TRAY_DEBUG") == "1" {
			exe, _ := os.Executable()
			writeTrayDebug("trayOnReady: iconPath=%q exeDir=%s iconLoaded=%v",
				iconPath, filepath.Dir(exe), iconData != nil)
		}
		if iconData != nil {
			baseIconData = iconData
			traySetBaseIcon()
		} else {
			systray.SetTitle("Xencelabs Quick Keys")
		}
		// Menu (left click) — 1→Status 2→Layer 3→Battery
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
			systray.Quit()
			os.Exit(0)
		}()
		trayUpdateTooltip()
	}
}

func trayOnExit() {}

func stopTray() {
	// getlantern/systray runs in its own loop; no explicit cleanup needed
}

// traySetBaseIcon affiche l'icône de base (sans modification)
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
		// Try relative to executable
		exe, err := os.Executable()
		if err != nil {
			return nil
		}
		path = filepath.Join(filepath.Dir(exe), "xencelabs-icon.png")
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

	// Sur Linux (appindicator), SetTooltip est un no-op. SetTitle/label est ce qui s'affiche au survol.
	tooltip := fmt.Sprintf("Xencelabs Quick Keys — Battery: %s", battStr)
	systray.SetTitle(tooltip)
	systray.SetTooltip(tooltip) // pour macOS/Windows si supporté
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
