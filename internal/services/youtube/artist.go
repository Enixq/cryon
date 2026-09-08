package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"Cryon2/internal/domain"
	"Cryon2/internal/logging"
)

// Каталог исполнителя на YouTube Music реализуем через yt-dlp: он умеет читать
// внутренний API YouTube Music без OAuth. Поиск по music.youtube.com отдаёт
// browse-id альбомов (MPREb_…), а раскрытие такого id даёт полный трек-лист
// релиза с обложками — то, чего не даёт обычный видео-поиск. Так «зайти в
// альбом → увидеть все треки» работает как в Spotify, даже без токенов.

const (
	maxArtistAlbums       = 10 // сколько релизов тянем на страницу исполнителя
	albumFetchConcurrency = 4  // параллельные вызовы yt-dlp (не душим CPU/сеть)
	albumCacheTTL         = 30 * time.Minute
)

var _ domain.ArtistBrowser = (*Service)(nil)
var _ domain.AlbumResolver = (*Service)(nil)

// cachedAlbum — трек-лист альбома, добытый при построении страницы артиста,
// чтобы повторное открытие того же альбома (GetAlbumTracks) было мгновенным.
type cachedAlbum struct {
	tracks []domain.Track
	at     time.Time
}

func (s *Service) cacheAlbum(id string, tracks []domain.Track) {
	if id == "" || len(tracks) == 0 {
		return
	}
	s.albumMu.Lock()
	if s.albumCache == nil {
		s.albumCache = make(map[string]cachedAlbum)
	}
	s.albumCache[id] = cachedAlbum{tracks: tracks, at: time.Now()}
	s.albumMu.Unlock()
}

func (s *Service) cachedAlbumTracks(id string) ([]domain.Track, bool) {
	s.albumMu.RLock()
	defer s.albumMu.RUnlock()
	c, ok := s.albumCache[id]
	if !ok || time.Since(c.at) > albumCacheTTL {
		return nil, false
	}
	return c.tracks, true
}

// GetArtist собирает релизы исполнителя: ищет его на YouTube Music, берёт
// browse-id альбомов и раскрывает их (параллельно) в полноценные альбомы с
// обложками и трек-листами. Топ-треки намеренно не заполняем — их доклеивает
// агрегатор (app.go) из мультипоиска с приоритетом нецензурных версий.
func (s *Service) GetArtist(ctx context.Context, name string) (domain.ArtistInfo, error) {
	info := domain.ArtistInfo{Name: name, Service: domain.ServiceYouTube}
	name = strings.TrimSpace(name)
	if name == "" {
		return info, nil
	}

	searchURL := "https://music.youtube.com/search?q=" + url.QueryEscape(name)
	pl, err := s.runYtDlpJSON(ctx, searchURL)
	if err != nil {
		return info, err
	}

	albumIDs := make([]string, 0, maxArtistAlbums)
	seen := make(map[string]bool)
	for _, e := range pl.Entries {
		if e.IEKey != "YoutubeTab" || !strings.HasPrefix(e.ID, "MPREb_") || seen[e.ID] {
			continue
		}
		seen[e.ID] = true
		albumIDs = append(albumIDs, e.ID)
		if len(albumIDs) >= maxArtistAlbums {
			break
		}
	}

	albums := s.fetchAlbums(ctx, albumIDs, name)
	for _, al := range albums {
		if al.Kind == "single" || al.Kind == "ep" {
			info.Singles = append(info.Singles, al)
		} else {
			info.Albums = append(info.Albums, al)
		}
	}
	if len(albums) > 0 {
		info.ArtworkURL = albums[0].ArtworkURL
	}
	return info, nil
}

// AlbumTracks возвращает полный трек-лист альбома по его browse-id
// (MPREb_…). Сначала пробуем кэш, набитый в GetArtist, иначе тянем yt-dlp.
func (s *Service) AlbumTracks(ctx context.Context, albumID string) ([]domain.Track, error) {
	albumID = strings.TrimSpace(albumID)
	if albumID == "" {
		return []domain.Track{}, nil
	}
	if tracks, ok := s.cachedAlbumTracks(albumID); ok {
		return tracks, nil
	}
	_, tracks, ok := s.fetchAlbum(ctx, albumID, "")
	if !ok {
		return []domain.Track{}, nil
	}
	s.cacheAlbum(albumID, tracks)
	return tracks, nil
}

// ResolveAlbum ищет релиз по «исполнитель + название» на YouTube Music и
// возвращает его полный трек-лист. Так альбом, открытый со страницы поиска (без
// ID каталога), дополняется до настоящего релиза. Реализует
// domain.AlbumResolver.
func (s *Service) ResolveAlbum(ctx context.Context, artist, title string) ([]domain.Track, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return []domain.Track{}, nil
	}
	query := title
	if a := strings.TrimSpace(artist); a != "" {
		query = a + " " + title
	}
	searchURL := "https://music.youtube.com/search?q=" + url.QueryEscape(query)
	pl, err := s.runYtDlpJSON(ctx, searchURL)
	if err != nil {
		return nil, err
	}

	// Собираем album browse-id из выдачи (не больше горстки — раскрытие каждого
	// это отдельный запрос в сеть).
	const maxCandidates = 5
	ids := make([]string, 0, maxCandidates)
	seen := make(map[string]bool)
	for _, e := range pl.Entries {
		if e.IEKey != "YoutubeTab" || !strings.HasPrefix(e.ID, "MPREb_") || seen[e.ID] {
			continue
		}
		seen[e.ID] = true
		ids = append(ids, e.ID)
		if len(ids) >= maxCandidates {
			break
		}
	}

	want := strings.ToLower(title)
	var firstTracks []domain.Track
	for _, id := range ids {
		al, tracks, ok := s.fetchAlbum(ctx, id, strings.TrimSpace(artist))
		if !ok {
			continue
		}
		s.cacheAlbum(id, tracks)
		if firstTracks == nil {
			firstTracks = tracks
		}
		// Точное (нестрогое) совпадение названия — берём сразу.
		if strings.Contains(strings.ToLower(al.Title), want) || strings.Contains(want, strings.ToLower(al.Title)) {
			return tracks, nil
		}
	}
	// Совпадения по названию не нашлось — возвращаем первый валидный релиз как
	// наиболее релевантный выдаче (лучше полный список, чем один трек из поиска).
	if firstTracks != nil {
		return firstTracks, nil
	}
	return []domain.Track{}, nil
}

// fetchAlbums раскрывает альбомы параллельно, сохраняя исходный порядок выдачи.
func (s *Service) fetchAlbums(ctx context.Context, ids []string, artist string) []domain.Album {
	if len(ids) == 0 {
		return nil
	}
	out := make([]domain.Album, len(ids))
	ok := make([]bool, len(ids))
	sem := make(chan struct{}, albumFetchConcurrency)
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			al, tracks, good := s.fetchAlbum(ctx, id, artist)
			if good {
				out[i] = al
				ok[i] = true
				s.cacheAlbum(id, tracks)
			}
		}(i, id)
	}
	wg.Wait()

	albums := make([]domain.Album, 0, len(ids))
	for i := range out {
		if ok[i] {
			albums = append(albums, out[i])
		}
	}
	return albums
}

// fetchAlbum раскрывает один browse-id в альбом + его трек-лист.
func (s *Service) fetchAlbum(ctx context.Context, id, artist string) (domain.Album, []domain.Track, bool) {
	pl, err := s.runYtDlpJSON(ctx, "https://music.youtube.com/browse/"+id)
	if err != nil {
		logging.L().Debug("youtube: album fetch failed", "id", id, "err", err)
		return domain.Album{}, nil, false
	}
	if len(pl.Entries) == 0 {
		return domain.Album{}, nil, false
	}
	kind, title := parseAlbumTitle(pl.Title)
	cover := bestThumb(pl.Thumbnails)
	var artists []string
	if artist != "" {
		artists = []string{artist}
	}
	tracks := make([]domain.Track, 0, len(pl.Entries))
	for _, e := range pl.Entries {
		if e.ID == "" || e.Title == "" {
			continue
		}
		artwork := firstNonEmpty(bestThumb(e.Thumbnails), cover, thumbnailURL(e.ID))
		tracks = append(tracks, domain.Track{
			ID:           e.ID,
			Service:      domain.ServiceYouTube,
			Title:        e.Title,
			Artists:      artists,
			Album:        title,
			DurationMs:   int(e.Duration * 1000),
			ArtworkURL:   artwork,
			ExternalURL:  "https://music.youtube.com/watch?v=" + e.ID,
			PlayableKind: domain.PlayableStream,
		})
	}
	if len(tracks) == 0 {
		return domain.Album{}, nil, false
	}
	al := domain.Album{
		ID:          id,
		Service:     domain.ServiceYouTube,
		Title:       title,
		Artist:      artist,
		ArtworkURL:  cover,
		Kind:        kind,
		TrackCount:  len(tracks),
		ExternalURL: "https://music.youtube.com/browse/" + id,
	}
	return al, tracks, true
}

// ytPlaylist / ytEntry / ytThumb — то, что нам нужно из JSON yt-dlp (-J).
type ytPlaylist struct {
	Title      string    `json:"title"`
	Thumbnails []ytThumb `json:"thumbnails"`
	Entries    []ytEntry `json:"entries"`
}

type ytEntry struct {
	IEKey      string    `json:"ie_key"`
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	URL        string    `json:"url"`
	Duration   float64   `json:"duration"`
	Thumbnails []ytThumb `json:"thumbnails"`
}

type ytThumb struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// runYtDlpJSON запускает yt-dlp в режиме единого JSON без скачивания.
func (s *Service) runYtDlpJSON(ctx context.Context, target string) (*ytPlaylist, error) {
	// Ограничиваем время: yt-dlp ходит в сеть и без дедлайна может подвесить
	// построение страницы исполнителя.
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, "yt-dlp", "--no-warnings", "--skip-download", "--flat-playlist", "--encoding", "utf-8", "-J", target)
	// Форсируем UTF-8, иначе на Windows кириллица приходит «ромбиками».
	cmd.Env = append(os.Environ(), "PYTHONIOENCODING=utf-8", "PYTHONUTF8=1")
	hideConsole(cmd)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("yt-dlp: %w", err)
	}
	var pl ytPlaylist
	if err := json.Unmarshal(out, &pl); err != nil {
		return nil, fmt.Errorf("yt-dlp: разбор JSON: %w", err)
	}
	return &pl, nil
}

// parseAlbumTitle разбирает заголовок YouTube Music вида «Album - Название»,
// «Single - …», «EP - …» на вид релиза и чистое название.
func parseAlbumTitle(raw string) (kind, title string) {
	title = strings.TrimSpace(raw)
	kind = "album"
	for prefix, k := range map[string]string{
		"Album - ":    "album",
		"Single - ":   "single",
		"EP - ":       "ep",
		"Playlist - ": "album",
	} {
		if strings.HasPrefix(title, prefix) {
			return k, strings.TrimSpace(strings.TrimPrefix(title, prefix))
		}
	}
	return kind, title
}

// bestThumb выбирает самую крупную квадратную обложку из списка.
func bestThumb(thumbs []ytThumb) string {
	if len(thumbs) == 0 {
		return ""
	}
	sorted := make([]ytThumb, len(thumbs))
	copy(sorted, thumbs)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Width*sorted[i].Height > sorted[j].Width*sorted[j].Height
	})
	for _, t := range sorted {
		if strings.TrimSpace(t.URL) != "" {
			return t.URL
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
