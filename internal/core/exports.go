package core

import (
	"context"
	"io/fs"
	"net/http"
)

// Экспортируемая поверхность core: тонкий слой, через который десктопная
// обёртка (package main) и мобильный пакет (gomobile) управляют App, не
// касаясь приватных полей. Бизнес-методы остаются на *App и промотиваются
// в Wails-биндинги через встраивание (см. main.go).

// Startup — точка входа Wails OnStartup: сохраняет контекст рантайма,
// инициализирует хранилище, восстанавливает состояние, запускает health-check
// и (в сборке с тегом cryonlan) LAN-сервер.
func (a *App) Startup(ctx context.Context) { a.startup(ctx) }

// Shutdown — точка входа Wails OnShutdown: останавливает плеер и закрывает
// хранилище.
func (a *App) Shutdown(ctx context.Context) { a.shutdown(ctx) }

// AssetHandler возвращает HTTP-обработчик локальных аудиофайлов и
// стрим-прокси (/local/<id>, /stream/...). На десктопе он подключается к
// ассет-серверу Wails, на мобайле — в mux встроенного сервера.
func (a *App) AssetHandler() http.Handler {
	return &localAssetHandler{app: a}
}

// StartMobileServer поднимает встроенный HTTP-сервер на 127.0.0.1:<случайный
// порт> (только сборка с тегом cryonmobile): отдаёт SPA из spa, RPC /api/call,
// SSE /api/events и AssetHandler. Возвращает базовый URL для WebView и функцию
// остановки. Побочно назначает a.platform = sseHost, поэтому события backend
// начинают уходить во фронтенд через SSE.
// На сборках без тега cryonmobile возвращает ошибку (см. exports_mobile_stub.go).
func (a *App) StartMobileServer(spa fs.FS) (string, func(), error) {
	return a.startMobileServer(spa)
}
