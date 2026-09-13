//go:build android

// Package mobile — gomobile-поверхность Cryon для Android. Единственная
// задача пакета: принять от Kotlin-оболочки каталог данных приложения,
// поднять весь бэкенд (core.App) со встроенным HTTP-сервером на 127.0.0.1 и
// вернуть базовый URL для WebView. Наружу торчат только Start/Stop —
// всё остальное (RPC, SSE, стримы) идёт по HTTP внутри устройства.
//
// Сборка: gomobile bind -target=android -androidapi 24 -o cryonmobile.aar ./mobile
// (см. scripts/build-android.ps1).
package mobile

import (
	"Cryon2/internal/core"
	"Cryon2/internal/logging"
	"context"
	"embed"
	"errors"
	"io/fs"
	"strings"
	"sync"
)

//go:embed all:dist
var assets embed.FS

var (
	mu      sync.Mutex
	app     *core.App
	stopSrv func()
	cancel  context.CancelFunc
)

// Start инициализирует бэкенд и поднимает встроенный сервер на петле.
// dataDir — каталог данных Android (context.getFilesDir()); там SQLite
// хранит библиотеку, историю и настройки. Возвращает базовый URL вида
// "http://127.0.0.1:<порт>", который WebView грузит как главную страницу.
// Повторный вызов без Stop — ошибка (Kotlin-оболочка зовёт Start один раз
// на процесс; Stop — в onDestroy).
func Start(dataDir string) (string, error) {
	mu.Lock()
	defer mu.Unlock()
	if app != nil {
		return "", errAlreadyStarted
	}

	// КЛЮЧЕВОЕ для Android: перенаправляем каталог данных в filesDir приложения
	// ДО NewApp. Иначе store.New через os.UserConfigDir() целится в недоступный
	// «$HOME/.config», SQLite не открывается, и весь бэкенд остаётся без БД.
	if strings.TrimSpace(dataDir) != "" {
		logging.SetDataDirOverride(dataDir)
	}

	a := core.NewApp()
	spa, err := fs.Sub(assets, "dist")
	if err != nil {
		return "", err
	}

	// Сначала поднимаем сервер — он назначает a.platform = sseHost (события
	// backend начинают уходить в WebView). Только потом Startup стартует
	// health-check, чтобы его горутина читала уже установленный platform.
	baseURL, stop, err := a.StartMobileServer(spa)
	if err != nil {
		return "", err
	}

	// РАВНОЗНАЧНО Wails OnStartup на десктопе. Без этого a.ctx == nil, и
	// context.WithTimeout(a.ctx, …) в горутинах поиска/подбора потока паникует
	// — а паника в отдельной горутине net/http не перехватывается и роняет весь
	// процесс (это и был «краш на вкладке поиск»). Здесь же создаётся схема
	// SQLite и восстанавливаются токены/настройки.
	ctx, cancelFn := context.WithCancel(context.Background())
	a.Startup(ctx)

	app = a
	stopSrv = stop
	cancel = cancelFn
	return baseURL, nil
}

// Stop останавливает встроенный сервер и закрывает бэкенд. Безопасно вызывать
// повторно и до Start — no-op.
func Stop() {
	mu.Lock()
	defer mu.Unlock()
	if stopSrv != nil {
		stopSrv()
		stopSrv = nil
	}
	if cancel != nil {
		cancel()
		cancel = nil
	}
	if app != nil {
		app.Shutdown(nil)
		app = nil
	}
}

var errAlreadyStarted = errors.New("mobile: сервер уже запущен")
