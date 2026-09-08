package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:         "Cryon2",
		Width:         1600,
		Height:        980,
		MinWidth:      1280,
		MinHeight:     800,
		DisableResize: false,
		Frameless:     false,
		AssetServer: &assetserver.Options{
			Assets: assets,
			// Отдаём локальные аудиофайлы по /local/<id> для HTML5-фолбэка,
			// когда mpv недоступен (WebView2 не играет file://).
			Handler: &localAssetHandler{app: app},
		},
		BackgroundColour: &options.RGBA{R: 8, G: 10, B: 25, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Ошибка запуска Cryon2:", err.Error())
	}
}
