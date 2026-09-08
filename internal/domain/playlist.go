package domain

// Playlist — плейлист источника.
type Playlist struct {
	ID          string    `json:"id"`
	Service     ServiceID `json:"service"`
	Name        string    `json:"name"`
	TracksTotal int       `json:"tracksTotal"`
	ExternalURL string    `json:"externalUrl,omitempty"`
}

// UserPlaylist — пользовательский плейлист, созданный внутри Cryon2
// и хранящийся локально. JSON-теги согласованы с фронтендом.
type UserPlaylist struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	TrackCount  int    `json:"trackCount"`
	CreatedAtMs int64  `json:"createdAtMs,omitempty"`
	// CoverURLs — обложки первых треков (до 4) для превью-коллажа на карточке.
	// Пусто для локальных треков без внешней обложки.
	CoverURLs []string `json:"coverUrls,omitempty"`
}
