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
	// ServiceVK is retained for metadata in previously saved library items.
	ServiceVK    ServiceID = "vk"
	ServiceLocal ServiceID = "local"
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
	DJSessionID          string       `json:"djSessionId,omitempty"`
	DJPlanID             string       `json:"djPlanId,omitempty"`
	ForcedSource         bool         `json:"forcedSource,omitempty"`
	RecommendationReason string       `json:"recommendationReason,omitempty"`
	FromCache            bool         `json:"fromCache,omitempty"`
	ID                   string       `json:"id"`
	Service              ServiceID    `json:"service"`
	Title                string       `json:"title"`
	Artists              []string     `json:"artists"`
	Album                string       `json:"album,omitempty"`
	ReleaseDate          string       `json:"releaseDate,omitempty"`
	Genre                string       `json:"genre,omitempty"`
	DurationMs           int          `json:"durationMs,omitempty"`
	PlayCount            int          `json:"playCount,omitempty"`
	Quality              string       `json:"quality,omitempty"`
	ArtworkURL           string       `json:"artworkUrl,omitempty"`
	ExternalURL          string       `json:"externalUrl,omitempty"`
	PlayableKind         PlayableKind `json:"playableKind"`
}

// MusicService — единый контракт адаптера источника.
// Любая новая интеграция подключается через этот интерфейс.
type MusicService interface {
	ID() ServiceID
	Search(ctx context.Context, query string) ([]Track, error)
}

// PlaylistBrowser exposes source playlists without requiring a user account.
// Sources that only support local/user playlists simply do not implement it.
type PlaylistBrowser interface {
	SearchPlaylists(ctx context.Context, query string, limit int) ([]Playlist, error)
	PlaylistTracks(ctx context.Context, playlistID string) ([]Track, error)
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

// CanonicalTrack is a source-independent musical work used by library,
// recommendation and playlist layers. Source-specific audio stays in
// PlaybackCandidate records.
type CanonicalTrack struct {
	ID                 string    `json:"id"`
	Title              string    `json:"title"`
	NormalizedTitle    string    `json:"normalizedTitle"`
	Artist             string    `json:"artist"`
	NormalizedArtist   string    `json:"normalizedArtist"`
	Album              string    `json:"album,omitempty"`
	DurationMs         int       `json:"durationMs,omitempty"`
	ArtworkURL         string    `json:"artworkUrl,omitempty"`
	Explicit           bool      `json:"explicit"`
	IdentityConfidence float64   `json:"identityConfidence"`
	MetadataSource     ServiceID `json:"metadataSource"`
}

// VersionKind describes a recording variant so the resolver can avoid choosing
// a live, cover or clean version when an original is available.
type VersionKind string

const (
	VersionUnknown  VersionKind = "unknown"
	VersionOriginal VersionKind = "original"
	VersionLive     VersionKind = "live"
	VersionCover    VersionKind = "cover"
	VersionRemaster VersionKind = "remaster"
	VersionClean    VersionKind = "clean"
	VersionRemix    VersionKind = "remix"
	VersionAcoustic VersionKind = "acoustic"
	VersionExplicit VersionKind = "explicit"
)

// PlaybackCandidate binds one playable source item to a CanonicalTrack.
type PlaybackCandidate struct {
	CheckedAt          string      `json:"checkedAt,omitempty"`
	CheckElapsedMS     int64       `json:"checkElapsedMs,omitempty"`
	AvailabilityState  string      `json:"availabilityState,omitempty"`
	RejectionReason    string      `json:"rejectionReason,omitempty"`
	ID                 string      `json:"id"`
	CanonicalTrackID   string      `json:"canonicalTrackId"`
	Track              Track       `json:"track"`
	IdentityConfidence float64     `json:"identityConfidence"`
	Version            VersionKind `json:"version"`
	Explicit           bool        `json:"explicit"`
	Official           bool        `json:"official"`
	Availability       bool        `json:"availability"`
	SourcePriority     int         `json:"sourcePriority"`
	SelectionReason    string      `json:"selectionReason,omitempty"`
}
