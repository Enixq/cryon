//go:build android

package core

// UseWailsRuntime — заглушка для Android-сборки: Wails-рантайма на устройстве
// нет, платформенный хост внедряет StartMobileServer (sseHost). Существование
// метода нужно, чтобы общий десктопный main.go компилировался под android
// (gomobile требует биндируемый пакет без веток по GOOS на стороне вызова).
func (a *App) UseWailsRuntime() {}
