//go:build tray && bindings
// +build tray,bindings

package main

import (
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
)

// Stubs pour la génération des bindings (app/tray_wails référencent ces variables)
var (
	configReloadFromGUI chan struct{}
	embeddedTrayIcon    []byte
	GlobalApp          *App
)

// main pour la génération des bindings uniquement — pas d'uinput, pas de device, pas de tray.
// Évite le blocage de wails dev sur uinput.CreateKeyboard.
func main() {
	app := NewApp()
	GlobalApp = app
	err := wails.Run(&options.App{
		Bind: []interface{}{app},
	})
	if err != nil {
		panic(err)
	}
}
