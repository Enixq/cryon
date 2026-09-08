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
)

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
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, appName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}
