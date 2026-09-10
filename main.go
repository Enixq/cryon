package main

import (
	"embed"

	"Cryon2/internal/core"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

// App — тонкая десктопная обёртка над core.App. Нужна, чтобы Wails-биндинги
// остались в прежнем пространстве имён: генератор берёт пакет и имя структуры
// с типа, переданного в Bind, и фронтенд по-прежнему вызывает
// window.go.main.App.*. Методы *core.App промотируются в набор методов
// обёртки автоматически.
type App struct{ *core.App }

func main() {
	app := &App{core.NewApp()}
	// Десктоп: события/браузер/диалоги идут через Wails-рантайм. Хост читает
	// ctx лениво, поэтому его можно внедрить до Startup (где ctx появится).
	app.UseWailsRuntime()

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
			// Отдаём локальные аудиофайлы по /local/<id> и стрим-прокси
			// /stream/... для HTML5-фолбэка, когда mpv недоступен
			// (WebView2 не играет file://).
			Handler: app.AssetHandler(),
		},
		BackgroundColour: &options.RGBA{R: 8, G: 10, B: 25, A: 1},
		OnStartup:        app.Startup,
		OnShutdown:       app.Shutdown,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Ошибка запуска Cryon2:", err.Error())
	}
}
