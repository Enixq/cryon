package domain

import "context"

// ServiceID — идентификатор источника музыки. Значения совпадают
// с SourceId на фронтенде (frontend/src/shared/types.ts).
type ServiceID string

const (
	ServiceYouTube    ServiceID = "youtube"
	ServiceSoundCloud ServiceID = "soundcloud"
	ServiceSpotify    ServiceID = "spotify"
	ServiceYandex     ServiceID = "yandex"
	ServiceVK         ServiceID = "vk"
	ServiceLocal      ServiceID = "local"
)

// PlayableKind — способ воспроизведения трека.
type PlayableKind string

const (
	// PlayableStream — есть прямой аудиопоток (YouTube, локальные файлы).
	PlayableStream PlayableKind = "stream"
	// PlayableEmbeddedWeb — воспроизведение через встроенный веб-плеер источника.
	PlayableEmbeddedWeb PlayableKind = "embedded_web"
	// PlayableExternal — можно только открыть во внешнем сервисе.
	PlayableExternal PlayableKind = "external_only"
)

// Track — единый доменный трек. JSON-теги совпадают с фронтендом,
// чтобы Wails-биндинги отдавали данные без дополнительного маппинга.
type Track struct {
	ID           string       `json:"id"`
	Service      ServiceID    `json:"service"`
	Title        string       `json:"title"`
	Artists      []string     `json:"artists"`
	Album        string       `json:"album,omitempty"`
	DurationMs   int          `json:"durationMs,omitempty"`
	ArtworkURL   string       `json:"artworkUrl,omitempty"`
	ExternalURL  string       `json:"externalUrl,omitempty"`
	PlayableKind PlayableKind `json:"playableKind"`
}

// MusicService — единый контракт адаптера источника.
// Любая новая интеграция подключается через этот интерфейс.
type MusicService interface {
	ID() ServiceID
	Search(ctx context.Context, query string) ([]Track, error)
}

// NewReleaser — опциональная возможность адаптера отдавать свежие релизы
// («Радар новинок»). Реализуют её только источники, у которых есть доступ к
// каталогу новинок (Yandex с токеном, Spotify с ключами); остальные адаптеры
// просто не приводятся к этому интерфейсу, и агрегатор их пропускает.
type NewReleaser interface {
	NewReleases(ctx context.Context, limit int) ([]Track, error)
}

// ArtistReleaser — опциональная возможность адаптера отдавать свежие релизы
// КОНКРЕТНОГО исполнителя по имени. Нужна для «засева» Радара новинок по вкусу:
// вместо фильтрации редакционной ленты движок берёт свежие альбомы знакомых и
// похожих артистов напрямую, поэтому радар наполняется именно теми, кого слушает
// пользователь, а не общим мейнстримом. Реализуют источники с доступом к каталогу
// артистов (Yandex с токеном, Spotify с ключами); остальные просто не приводятся
// к этому интерфейсу, и засев их пропускает.
type ArtistReleaser interface {
	ArtistNewReleases(ctx context.Context, artist string, limit int) ([]Track, error)
}
