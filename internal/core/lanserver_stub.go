//go:build !cryonlan

package core
// startLANServer — заглушка для обычной десктоп-сборки (без тега cryonlan).
// Настоящий LAN-сервер (lanserver.go) включается сборкой
// `wails build -tags cryonlan`, поэтому «слепой» серверный код не может
// сломать рабочий десктоп-билд.
func (a *App) startLANServer() {}
