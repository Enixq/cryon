package domain

import "context"

// Album — релиз исполнителя (альбом, сингл или EP) для страницы артиста.
// Обложка, год и вид (kind) позволяют оформить сетку релизов как в Spotify.
// Треки не входят в карточку намеренно: их подгружают лениво по AlbumID при
// открытии/воспроизведении, чтобы не делать десятки запросов на одну страницу.
type Album struct {
	ID         string    `json:"id"`
	Service    ServiceID `json:"service"`
	Title      string    `json:"title"`
	Artist     string    `json:"artist,omitempty"`
	Year       int       `json:"year,omitempty"`
	ArtworkURL string    `json:"artworkUrl,omitempty"`
	// Kind — "album" | "single" | "ep". Для вкладок «Альбомы» и «Синглы и EP».
	Kind       string `json:"kind,omitempty"`
	TrackCount int    `json:"trackCount,omitempty"`
	// ExternalURL — ссылка на релиз во внешнем сервисе (fallback-открытие).
	ExternalURL string `json:"externalUrl,omitempty"`
}

// ArtistInfo — агрегированная информация об исполнителе для страницы артиста:
// фото, популярные треки и релизы (альбомы/синглы). Собирается либо из каталога
// сервиса-провайдера (ArtistBrowser, например Yandex с токеном), либо, если
// каталог недоступен, эвристически из мультипоиска по всем рабочим источникам.
type ArtistInfo struct {
	Name       string    `json:"name"`
	Service    ServiceID `json:"service,omitempty"`
	ArtworkURL string    `json:"artworkUrl,omitempty"`
	// TopTracks — популярные/наиболее релевантные треки исполнителя.
	TopTracks []Track `json:"topTracks"`
	// Albums — полноформатные альбомы (и EP), самые свежие сверху.
	Albums []Album `json:"albums"`
	// Singles — синглы и EP.
	Singles []Album `json:"singles"`
	// AppearsOn — релизы ДРУГИХ исполнителей, где артист лишь участвует
	// (совместки, сборники). Их нельзя мешать с собственной дискографией, но
	// показать отдельным блоком «Встречается в» полезно — как в Spotify.
	AppearsOn []Album `json:"appearsOn"`
}

// ArtistBrowser — опциональная возможность адаптера отдавать каталог
// исполнителя: его фото, популярные треки и релизы. Реализуют источники со
// структурированным каталогом (Yandex с токеном); остальные адаптеры к этому
// интерфейсу не приводятся, и агрегатор берёт данные из мультипоиска.
type ArtistBrowser interface {
	// GetArtist возвращает каталог исполнителя по имени. Если исполнитель не
	// найден — пустой ArtistInfo без ошибки (агрегатор попробует другой путь).
	GetArtist(ctx context.Context, name string) (ArtistInfo, error)
	// AlbumTracks возвращает полный трек-лист релиза по его ID.
	AlbumTracks(ctx context.Context, albumID string) ([]Track, error)
}

// AlbumResolver — опциональная возможность адаптера найти конкретный альбом по
// паре «исполнитель + название» и вернуть его полный трек-лист. Нужна, чтобы
// альбом, собранный на странице поиска из отдельных треков (без ID каталога),
// при открытии дополнялся до настоящего релиза со всеми треками. Реализуют
// источники со структурированным каталогом (Yandex с токеном, YouTube Music
// через yt-dlp); остальные к интерфейсу не приводятся.
type AlbumResolver interface {
	// ResolveAlbum ищет релиз по исполнителю и названию. Пустой срез без
	// ошибки означает «не нашли» — агрегатор попробует другой источник.
	ResolveAlbum(ctx context.Context, artist, title string) ([]Track, error)
}
