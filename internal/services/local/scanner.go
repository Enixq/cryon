// Package local реализует источник музыки из локальных аудиофайлов:
// сканирование папок, чтение метаданных (теги, обложки) и поиск.
package local

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dhowden/tag"

	"Cryon2/internal/domain"
	"Cryon2/internal/logging"
)

// supportedExts — расширения аудиофайлов, которые умеем читать.
var supportedExts = map[string]bool{
	".mp3":  true,
	".flac": true,
	".m4a":  true,
	".aac":  true,
	".ogg":  true,
	".wav":  true,
	".opus": true,
}

// LocalTrack — трек локальной библиотеки: доменный трек плюс путь к файлу.
type LocalTrack struct {
	domain.Track
	FilePath string `json:"filePath"`
}

// IsSupportedAudio сообщает, поддерживается ли файл по его расширению.
func IsSupportedAudio(path string) bool {
	return supportedExts[strings.ToLower(filepath.Ext(path))]
}

// ScanFolder рекурсивно обходит папку и возвращает найденные аудиотреки
// с прочитанными метаданными. Ошибки чтения отдельных файлов не прерывают
// обход — такие файлы пропускаются с записью в лог.
func ScanFolder(ctx context.Context, root string) ([]LocalTrack, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, &os.PathError{Op: "scan", Path: root, Err: os.ErrInvalid}
	}

	var out []LocalTrack
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Пропускаем недоступные каталоги/файлы, но продолжаем обход.
			logging.L().Warn("сканирование: пропуск пути", "path", path, "err", err)
			return nil
		}
		// Даём возможность отменить длительный обход.
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if d.IsDir() || !IsSupportedAudio(path) {
			return nil
		}
		track, perr := readTrack(path)
		if perr != nil {
			logging.L().Warn("сканирование: не удалось прочитать метаданные", "path", path, "err", perr)
			return nil
		}
		out = append(out, track)
		return nil
	})
	if walkErr != nil {
		return out, walkErr
	}
	return out, nil
}

// readTrack читает метаданные одного аудиофайла в LocalTrack.
func readTrack(path string) (LocalTrack, error) {
	f, err := os.Open(path)
	if err != nil {
		return LocalTrack{}, err
	}
	defer f.Close()

	id := trackID(path)
	title := localFilenameTitle(path)
	durationMs := mobileFilenameDuration(path)
	var artists []string
	var album string

	// Метаданные читаем best-effort: если тегов нет, используем имя файла.
	if meta, merr := tag.ReadFrom(f); merr == nil {
		if t := strings.TrimSpace(meta.Title()); t != "" {
			title = t
		}
		if a := strings.TrimSpace(meta.Artist()); a != "" {
			artists = []string{a}
		}
		album = strings.TrimSpace(meta.Album())
	}

	return LocalTrack{
		Track: domain.Track{
			ID:           id,
			Service:      domain.ServiceLocal,
			Title:        title,
			Artists:      artists,
			Album:        album,
			DurationMs:   durationMs,
			ArtworkURL:   "", // обложка отдаётся отдельным методом по требованию
			PlayableKind: domain.PlayableStream,
		},
		FilePath: path,
	}, nil
}

// trackID формирует стабильный идентификатор трека из абсолютного пути.
// Одинаковый файл всегда получает один и тот же id между сканированиями.
func trackID(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	sum := sha1.Sum([]byte(strings.ToLower(abs)))
	return hex.EncodeToString(sum[:])
}

// FileModTime возвращает время изменения файла (для инкрементального скана).
func FileModTime(path string) (time.Time, error) {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime(), nil
}

var mobileFilenamePattern = regexp.MustCompile(`^\d+--(\d+)--(.+)$`)

func localFilenameTitle(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if match := mobileFilenamePattern.FindStringSubmatch(base); len(match) == 3 {
		return match[2]
	}
	return base
}

func mobileFilenameDuration(path string) int {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	match := mobileFilenamePattern.FindStringSubmatch(base)
	if len(match) != 3 {
		return 0
	}
	value, err := strconv.Atoi(match[1])
	if err != nil || value < 0 {
		return 0
	}
	return value
}
