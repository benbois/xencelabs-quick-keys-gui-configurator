//go:build !tray
// +build !tray

package main

func trayAvailable() bool                      { return false }
func startTray(iconPath string)                {}
func trayDebugEnv()                           {}
func trayGetStatusText() string                { return "Tray non disponible" }
func traySetBattery(percent int, charging bool) {}
func trayGetBattery() int                      { return -1 }
func traySetConnected(connected bool)          {}
func traySetReceivedData()                     {}
func trayGetConnected() bool                   { return false }
func trayGetRecognized() bool                  { return false }
func traySetLayer(name string, r, g, b uint8) {}
