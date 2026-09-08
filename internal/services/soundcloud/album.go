package soundcloud

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"Cryon2/internal/domain"
	"Cryon2/internal/logging"
	"Cryon2/internal/trackmeta"
)

var _ domain.AlbumResolver = (*Service)(nil)

// scFullTrack — полная карточка трека SoundCloud (из /search/tracks,
// /tracks?ids=… и массива tracks внутри альбома). Общая для поиска и альбомов,
// чтобы маппинг в domain.Track жил в одном месте.
type scFullTrack struct {
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	DurationMs int    `json:"duration"` // api-v2 отдаёт длительность в мс
	ArtworkURL string `json:"artwork_url"`
	Permalink  string `json:"permalink_url"`
	User       struct {
		Username string `json:"username"`
		// AvatarURL — обложка аккаунта. У большой части треков artwork_url пустой
		// (SoundCloud сам показывает в таком случае аватар автора), и без этого
		// запаса карточки в выдаче оставались без обложки.
		AvatarURL string `json:"avatar_url"`
	} `json:"user"`
}

// toDomain переводит трек SoundCloud в доменную модель. album может быть пустым
// (для одиночного трека из поиска) — тогда поле Album не заполняется.
//
// Исполнителя НЕЛЬЗЯ брать из имени аккаунта: по запросу артиста выдача почти
// целиком состоит из перезаливов, где автором загрузки числится случайный
// аккаунт («Mirarion», «N1P3NDO»), а настоящий исполнитель стоит в заголовке
// «Баста - 8800». Поэтому сначала пробуем разобрать заголовок (общая логика с
// YouTube — internal/trackmeta) и лишь потом падаем на имя аккаунта: у
// официальных профилей заголовок как раз без разделителя.
func (t scFullTrack) toDomain(album, artworkHost string) domain.Track {
	artist, title := trackmeta.SplitArtistTitle(t.Title)
	if artist == "" {
		artist = strings.TrimSpace(t.User.Username)
	}
	title = trackmeta.CleanTitle(title)
	if title == "" {
		title = strings.TrimSpace(t.Title)
	}

	artists := []string{}
	if artist != "" {
		artists = []string{artist}
	}
	// artwork_url часто пустой — тогда берём аватар автора, как делает сам
	// SoundCloud, иначе в сетке остаётся серая заглушка.
	artwork := t.ArtworkURL
	if strings.TrimSpace(artwork) == "" {
		artwork = t.User.AvatarURL
	}
	return domain.Track{
		ID:           strconv.FormatInt(t.ID, 10),
		Service:      domain.ServiceSoundCloud,
		Title:        title,
		Artists:      artists,
		Album:        album,
		DurationMs:   t.DurationMs,
		ArtworkURL:   normalizeArtwork(artwork, artworkHost),
		ExternalURL:  t.Permalink,
		PlayableKind: domain.PlayableStream,
	}
}

// scPlaylist — альбом/сет SoundCloud из выдачи /search/albums. Массив tracks
// приходит частично «заглушками» (только id) — их дотягиваем через /tracks.
type scPlaylist struct {
	ID         int64         `json:"id"`
	Title      string        `json:"title"`
	Permalink  string        `json:"permalink_url"`
	ArtworkURL string        `json:"artwork_url"`
	TrackCount int           `json:"track_count"`
	Tracks     []scFullTrack `json:"tracks"`
}

// ResolveAlbum ищет альбом/сет на SoundCloud по «исполнитель + название» и
// возвращает его полный трек-лист. SoundCloud отдаёт нецензурные версии, поэтому
// агрегатор (app.go) спрашивает его ПЕРВЫМ. Реализует domain.AlbumResolver.
// «Не нашли» — пустой срез без ошибки: агрегатор попробует другой источник.
func (s *Service) ResolveAlbum(ctx context.Context, artist, title string) ([]domain.Track, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return []domain.Track{}, nil
	}
	clientID, err := s.ensureClientID(ctx)
	if err != nil {
		// Без client_id альбомный каталог недоступен — это не ошибка резолва.
		return []domain.Track{}, nil
	}

	query := title
	if a := strings.TrimSpace(artist); a != "" {
		query = a + " " + title
	}

	playlists, err := s.searchAlbums(ctx, query, clientID)
	if err != nil {
		// client_id мог устареть — переоткрываем свежий и повторяем один раз.
		if fresh, ferr := s.refreshClientID(ctx); ferr == nil && fresh != "" && fresh != clientID {
			clientID = fresh
			playlists, err = s.searchAlbums(ctx, query, clientID)
		}
	}
	if err != nil {
		logging.L().Debug("soundcloud: поиск альбома не удался", "err", err)
		return []domain.Track{}, nil
	}
	if len(playlists) == 0 {
		return []domain.Track{}, nil
	}

	// Берём альбом с наиболее близким названием, иначе — самый релевантный (первый).
	want := strings.ToLower(title)
	chosen := &playlists[0]
	for i := range playlists {
		t := strings.ToLower(playlists[i].Title)
		if t != "" && (strings.Contains(t, want) || strings.Contains(want, t)) {
			chosen = &playlists[i]
			break
		}
	}

	return s.expandPlaylistTracks(ctx, chosen, clientID), nil
}

// searchAlbums запрашивает альбомы/сеты SoundCloud по строке запроса.
func (s *Service) searchAlbums(ctx context.Context, query, clientID string) ([]scPlaylist, error) {
	u, _ := url.Parse("https://api-v2.soundcloud.com/search/albums")
	q := u.Query()
	q.Set("q", query)
	q.Set("client_id", clientID)
	q.Set("limit", "10")
	u.RawQuery = q.Encode()

	body, err := s.fetchText(ctx, u.String())
	if err != nil {
		return nil, err
	}
	var sr struct {
		Collection []scPlaylist `json:"collection"`
	}
	if err := json.Unmarshal([]byte(body), &sr); err != nil {
		return nil, err
	}
	return sr.Collection, nil
}

// expandPlaylistTracks разворачивает трек-лист альбома, сохраняя порядок.
// Полные треки берём из выдачи; «заглушки» (только id) дотягиваем пачками
// через /tracks?ids=….
func (s *Service) expandPlaylistTracks(ctx context.Context, pl *scPlaylist, clientID string) []domain.Track {
	order := make([]int64, 0, len(pl.Tracks))
	full := make(map[int64]scFullTrack, len(pl.Tracks))
	missing := make([]int64, 0)
	for _, t := range pl.Tracks {
		if t.ID == 0 {
			continue
		}
		order = append(order, t.ID)
		if strings.TrimSpace(t.Title) != "" {
			full[t.ID] = t
		} else {
			missing = append(missing, t.ID)
		}
	}

	// /tracks?ids=… принимает до 50 id за раз и может вернуть их в другом
	// порядке — раскладываем по id и восстанавливаем исходный порядок альбома.
	const batch = 50
	for i := 0; i < len(missing); i += batch {
		end := i + batch
		if end > len(missing) {
			end = len(missing)
		}
		for _, ft := range s.fetchTracksByIDs(ctx, missing[i:end], clientID) {
			full[ft.ID] = ft
		}
	}

	out := make([]domain.Track, 0, len(order))
	artworkHost := s.ensureArtworkHost(ctx)
	for _, id := range order {
		if ft, ok := full[id]; ok && strings.TrimSpace(ft.Title) != "" {
			out = append(out, ft.toDomain(pl.Title, artworkHost))
		}
	}
	return out
}

// fetchTracksByIDs дотягивает полные карточки треков по их числовым id.
func (s *Service) fetchTracksByIDs(ctx context.Context, ids []int64, clientID string) []scFullTrack {
	if len(ids) == 0 {
		return nil
	}
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatInt(id, 10)
	}
	u, _ := url.Parse("https://api-v2.soundcloud.com/tracks")
	q := u.Query()
	q.Set("ids", strings.Join(parts, ","))
	q.Set("client_id", clientID)
	u.RawQuery = q.Encode()

	body, err := s.fetchText(ctx, u.String())
	if err != nil {
		return nil
	}
	var arr []scFullTrack
	if err := json.Unmarshal([]byte(body), &arr); err != nil {
		return nil
	}
	return arr
}
