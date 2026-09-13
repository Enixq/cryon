package logging

import (
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

var (
	once   sync.Once
	logger *slog.Logger

	// dataDirOverride принудительно задаёт базовый каталог данных. Нужен на
	// Android: там os.UserConfigDir() указывает на «$HOME/.config», которого нет
	// и который недоступен для записи из песочницы приложения, поэтому store.New
	// падал и весь бэкенд оставался без БД (настройки/локалка/история — мертвы).
	// Kotlin-оболочка передаёт context.getFilesDir() в mobile.Start, а тот —
	// сюда, ДО создания App. Пустая строка = поведение по умолчанию (десктоп).
	dataDirOverride   string
	dataDirOverrideMu sync.RWMutex
)

// SetDataDirOverride задаёт базовый каталог данных приложения (см. dataDirOverride).
// Вызывать до первого DataDir/NewApp. Пустая строка сбрасывает override.
func SetDataDirOverride(dir string) {
	dataDirOverrideMu.Lock()
	dataDirOverride = dir
	dataDirOverrideMu.Unlock()
}

// L возвращает общий структурный логгер приложения.
// Пишет в stderr в текстовом виде; на этапе backend это удобно для отладки,
// позже можно переключить на JSON или файл без изменения вызовов.
func L() *slog.Logger {
	once.Do(func() {
		level := slog.LevelInfo
		if os.Getenv("CRYON_DEBUG") != "" {
			level = slog.LevelDebug
		}
		handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})
		logger = slog.New(handler)
	})
	return logger
}

// DataDir возвращает каталог для данных приложения (БД, кэш) и создаёт его.
func DataDir(appName string) (string, error) {
	dataDirOverrideMu.RLock()
	base := dataDirOverride
	dataDirOverrideMu.RUnlock()
	if base == "" {
		var err error
		base, err = os.UserConfigDir()
		if err != nil {
			return "", err
		}
	}
	dir := filepath.Join(base, appName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}
