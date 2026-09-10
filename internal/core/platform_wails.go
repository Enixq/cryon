//go:build !android

package core

import "github.com/wailsapp/wails/v2/pkg/runtime"

// wailsHost — реализация platformHost поверх Wails-рантайма (десктоп).
// Держит указатель на App и читает a.ctx ЛЕНИВО в момент вызова: контекст Wails
// появляется только в Startup, а хост внедряется раньше (UseWailsRuntime из
// main). Тег `!android` исключает файл (и импорт Wails) из gomobile-сборки.
type wailsHost struct{ app *App }

func (h *wailsHost) Emit(event string, data ...interface{}) {
	runtime.EventsEmit(h.app.ctx, event, data...)
}

func (h *wailsHost) OpenURL(url string) {
	runtime.BrowserOpenURL(h.app.ctx, url)
}

func (h *wailsHost) PickFile(title, filterName, filterPattern string) (string, error) {
	opts := runtime.OpenDialogOptions{Title: title}
	if filterPattern != "" {
		opts.Filters = []runtime.FileFilter{{DisplayName: filterName, Pattern: filterPattern}}
	}
	return runtime.OpenFileDialog(h.app.ctx, opts)
}

func (h *wailsHost) PickDirectory(title string) (string, error) {
	return runtime.OpenDirectoryDialog(h.app.ctx, runtime.OpenDialogOptions{Title: title})
}

// UseWailsRuntime переводит App на десктопный Wails-рантайм: события, браузер
// и системные диалоги начинают доставляться через Wails. Вызывается обёрткой в
// package main до wails.Run (хост читает a.ctx лениво — контекст появится в
// Startup). На Android-сборке файл (и этот метод) исключён тегом `!android`:
// мобильный код использует StartMobileServer, который сам ставит sseHost.
func (a *App) UseWailsRuntime() {
	a.platform = &wailsHost{app: a}
}
