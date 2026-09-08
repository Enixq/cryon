package store

import (
	"context"
	"time"

	"Cryon2/internal/domain"
)

// Store — интерфейс локального хранилища (SQLite).
type Store interface {
	Init(ctx context.Context) error
	Close() error

	FavoriteAdd(ctx context.Context, t domain.Track) error
	FavoriteRemove(ctx context.Context, service domain.ServiceID, trackID string) error
	FavoriteList(ctx context.Context) ([]domain.Track, error)

	TokenGet(ctx context.Context, service domain.ServiceID) (Token, bool, error)
	TokenUpsert(ctx context.Context, service domain.ServiceID, token Token) error

	SettingGet(ctx context.Context, key string) (string, bool, error)
	SettingSet(ctx context.Context, key string, value string) error

	// Кэш ответов внешних API рекомендаций (Last.fm) с TTL.
	RecoCacheGet(ctx context.Context, key string) (string, bool, error)
	RecoCacheSet(ctx context.Context, key string, value string, ttl time.Duration) error

	// Локальная библиотека.
	LocalFolderAdd(ctx context.Context, path string) error
	LocalFolderRemove(ctx context.Context, path string) error
	LocalFolderList(ctx context.Context) ([]string, error)

	// Кэш просканированных локальных треков (для мгновенной загрузки при старте).
	LocalTrackReplaceAll(ctx context.Context, tracks []LocalTrackRow) error
	LocalTrackList(ctx context.Context) ([]LocalTrackRow, error)

	// История прослушивания.
	HistoryAdd(ctx context.Context, t domain.Track) error
	HistoryList(ctx context.Context, limit int) ([]HistoryRow, error)
	HistoryClear(ctx context.Context) error

	// Оповещения (истёк токен, новые релизы, ошибки воспроизведения,
	// завершение сканирования локальной библиотеки, статус импорта).
	NotificationAdd(ctx context.Context, n NotificationRow) error
	NotificationList(ctx context.Context, limit int) ([]NotificationRow, error)
	NotificationMarkAllRead(ctx context.Context) error
	NotificationRemove(ctx context.Context, id string) error
	NotificationClear(ctx context.Context) error

	// Пользовательские плейлисты.
	PlaylistCreate(ctx context.Context, title, description string) (domain.UserPlaylist, error)
	PlaylistList(ctx context.Context) ([]domain.UserPlaylist, error)
	PlaylistUpdate(ctx context.Context, id, title, description string) error
	PlaylistDelete(ctx context.Context, id string) error
	PlaylistTracks(ctx context.Context, playlistID string) ([]domain.Track, error)
	PlaylistAddTrack(ctx context.Context, playlistID string, t domain.Track) error
	PlaylistRemoveTrack(ctx context.Context, playlistID string, service domain.ServiceID, trackID string) error
	// PlaylistReorder задаёт новый порядок треков плейлиста. keys — треки в
	// желаемом порядке; треки, отсутствующие в keys, остаются в конце.
	PlaylistReorder(ctx context.Context, playlistID string, keys []PlaylistTrackKey) error

	// ResetAllData очищает все пользовательские данные (история, избранное,
	// плейлисты, локальную библиотеку, настройки, токены, кэш рекомендаций).
	// Используется для сброса к «чистому аккаунту» — например, перед передачей
	// приложения другому пользователю.
	ResetAllData(ctx context.Context) error

	// Локальная учётная запись и активная сессия. На первом этапе данные
	// установки общие: профиль нужен как offline-вход в приложение, а не как
	// обещание изоляции между несколькими людьми на одном Windows-профиле.
	AccountRegister(ctx context.Context, login, email, passwordHash string) (Account, error)
	AccountLogin(ctx context.Context, login string) (Account, string, error)
	AccountActivate(ctx context.Context, accountID string) error
	AccountSetGuest(ctx context.Context) (Account, error)
	AccountCurrent(ctx context.Context) (Account, bool, error)
	AccountLogout(ctx context.Context) error
}

// PlaylistTrackKey идентифицирует трек внутри плейлиста (service+trackID).
type PlaylistTrackKey struct {
	Service domain.ServiceID `json:"service"`
	TrackID string           `json:"trackId"`
}

// LocalTrackRow — строка кэша локальной библиотеки в хранилище.
type LocalTrackRow struct {
	ID       string
	Title    string
	Artists  []string
	Album    string
	FilePath string
}

// HistoryRow — запись истории прослушивания: трек и время воспроизведения.
type HistoryRow struct {
	Track      domain.Track
	PlayedAtMs int64
}

// NotificationRow — запись оповещения в хранилище. Kind — один из
// "info"/"success"/"warning"/"error" (совпадает с фронтендом). CreatedAtMs —
// unix-время в миллисекундах.
type NotificationRow struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Title       string `json:"title"`
	Message     string `json:"message"`
	CreatedAtMs int64  `json:"createdAtMs"`
	Read        bool   `json:"read"`
}

// Token — сохранённые учётные данные источника.
type Token struct {
	AccessToken  string
	RefreshToken string
	ExpiresAtMs  int64
	Scope        string
	TokenType    string
}

// Account — безопасное представление локального профиля для UI.
type Account struct {
	ID      string `json:"id"`
	Login   string `json:"login"`
	Email   string `json:"email"`
	IsGuest bool   `json:"isGuest"`
}
