package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"Cryon2/internal/domain"
	"Cryon2/internal/logging"
	"Cryon2/internal/trackmeta"
	"Cryon2/internal/websearch"
)

// Service — адаптер YouTube Music. При заданном ключе сначала использует
// официальный YouTube Data API; без него пробует Bing, а затем yt-dlp.
type Service struct {
	httpClient *http.Client

	// mu защищает apiKey, который может меняться из настроек в рантайме.
	mu     sync.RWMutex
	apiKey string

	// albumMu защищает кэш трек-листов альбомов (см. artist.go).
	albumMu    sync.RWMutex
	albumCache map[string]cachedAlbum
}

var _ domain.MusicService = (*Service)(nil)

func New(apiKey string) *Service {
	return &Service{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func (s *Service) ID() domain.ServiceID {
	return domain.ServiceYouTube
}

// HasAPIKey сообщает, задан ли API-ключ YouTube Data. Ключ необязателен,
// но даёт наиболее надёжный поиск и не требует внешнего yt-dlp.exe.
func (s *Service) HasAPIKey() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.apiKey != ""
}

// ValidateAPIKey проверяет ключ небольшим запросом к официальному API.
func (s *Service) ValidateAPIKey(ctx context.Context) error {
	s.mu.RLock()
	apiKey := s.apiKey
	s.mu.RUnlock()
	if apiKey == "" {
		return fmt.Errorf("youtube: API-ключ не задан")
	}
	u, err := url.Parse("https://www.googleapis.com/youtube/v3/videos")
	if err != nil {
		return err
	}
	q := u.Query()
	q.Set("part", "id")
	q.Set("id", "dQw4w9WgXcQ")
	q.Set("key", apiKey)
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("YouTube Data API вернул HTTP %d", resp.StatusCode)
	}
	return nil
}

// SetAPIKey обновляет ключ YouTube Data в рантайме (после ввода в настройках).
// Пустая строка возвращает адаптер к бесключевому режиму (Bing + парсинг).
func (s *Service) SetAPIKey(apiKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.apiKey = strings.TrimSpace(apiKey)
}

func (s *Service) Search(ctx context.Context, query string) ([]domain.Track, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []domain.Track{}, nil
	}
	log := logging.L()
	if tracks, err := s.searchByAPI(ctx, query); err == nil && len(tracks) > 0 {
		log.Debug("youtube: found via data api", "count", len(tracks))
		return tracks, nil
	} else if err != nil {
		log.Warn("youtube: data api search failed", "err", err)
	}

	results, err := websearch.FetchBingSearch(ctx, "music.youtube.com", query, 20)
	if err == nil && len(results) > 0 {
		tracks := tracksFromBing(results)
		if len(tracks) > 0 {
			log.Debug("youtube: found via bing", "count", len(tracks))
			return tracks, nil
		}
	}

	if err != nil {
		log.Warn("youtube: bing search failed", "err", err)
	}

	log.Debug("youtube: falling back to yt-dlp search")
	return s.searchByYtDlp(ctx, query)
}

// searchByAPI использует YouTube Data API v3. Это предпочтительный путь для
// установленного приложения: запрос выполняется внутри бинарника и не зависит
// от наличия yt-dlp в PATH пользователя.
func (s *Service) searchByAPI(ctx context.Context, query string) ([]domain.Track, error) {
	s.mu.RLock()
	apiKey := s.apiKey
	s.mu.RUnlock()
	if apiKey == "" {
		return nil, nil
	}

	u, err := url.Parse("https://www.googleapis.com/youtube/v3/search")
	if err != nil {
		return nil, err
	}
	params := u.Query()
	params.Set("part", "snippet")
	params.Set("type", "video")
	params.Set("maxResults", "20")
	params.Set("q", query)
	params.Set("key", apiKey)
	u.RawQuery = params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("YouTube Data API вернул HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Items []struct {
			ID struct {
				VideoID string `json:"videoId"`
			} `json:"id"`
			Snippet struct {
				Title        string `json:"title"`
				ChannelTitle string `json:"channelTitle"`
				Thumbnails   struct {
					Medium struct {
						URL string `json:"url"`
					} `json:"medium"`
				} `json:"thumbnails"`
			} `json:"snippet"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	tracks := make([]domain.Track, 0, len(payload.Items))
	for _, item := range payload.Items {
		videoID, rawTitle := strings.TrimSpace(item.ID.VideoID), strings.TrimSpace(item.Snippet.Title)
		if videoID == "" || rawTitle == "" {
			continue
		}
		artist, title := resolveArtistTitle(item.Snippet.ChannelTitle, rawTitle)
		artwork := strings.TrimSpace(item.Snippet.Thumbnails.Medium.URL)
		if artwork == "" {
			artwork = thumbnailURL(videoID)
		}
		var artists []string
		if artist != "" {
			artists = []string{artist}
		}
		tracks = append(tracks, domain.Track{ID: videoID, Service: domain.ServiceYouTube, Title: title, Artists: artists, ArtworkURL: artwork, ExternalURL: "https://music.youtube.com/watch?v=" + videoID, PlayableKind: domain.PlayableStream})
	}

	// search.list не отдаёт длительность — из-за этого в выдаче все тайминги
	// были «0:00». Дотягиваем её одним дешёвым videos.list (contentDetails).
	if ids := make([]string, 0, len(tracks)); len(tracks) > 0 {
		for _, t := range tracks {
			ids = append(ids, t.ID)
		}
		if durations := s.fetchVideoDurations(ctx, apiKey, ids); len(durations) > 0 {
			for i := range tracks {
				if ms, ok := durations[tracks[i].ID]; ok {
					tracks[i].DurationMs = ms
				}
			}
		}
	}
	return tracks, nil
}

// fetchVideoDurations возвращает длительность видео (в мс) по их id через
// videos.list?part=contentDetails. search.list длительность не отдаёт, поэтому
// без этого запроса тайминги в выдаче нулевые. videos.list принимает до 50 id
// за раз и стоит 1 единицу квоты (против 100 за search) — дёшево. Ошибки не
// фатальны: чего не дотянули, останется 0:00.
func (s *Service) fetchVideoDurations(ctx context.Context, apiKey string, ids []string) map[string]int {
	out := make(map[string]int, len(ids))
	const batch = 50
	for i := 0; i < len(ids); i += batch {
		end := i + batch
		if end > len(ids) {
			end = len(ids)
		}
		u, err := url.Parse("https://www.googleapis.com/youtube/v3/videos")
		if err != nil {
			continue
		}
		q := u.Query()
		q.Set("part", "contentDetails")
		q.Set("id", strings.Join(ids[i:end], ","))
		q.Set("key", apiKey)
		u.RawQuery = q.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			continue
		}
		resp, err := s.httpClient.Do(req)
		if err != nil {
			continue
		}
		var payload struct {
			Items []struct {
				ID             string `json:"id"`
				ContentDetails struct {
					Duration string `json:"duration"`
				} `json:"contentDetails"`
			} `json:"items"`
		}
		derr := json.NewDecoder(resp.Body).Decode(&payload)
		resp.Body.Close()
		if derr != nil {
			continue
		}
		for _, it := range payload.Items {
			if ms := parseISO8601Duration(it.ContentDetails.Duration); ms > 0 {
				out[it.ID] = ms
			}
		}
	}
	return out
}

// iso8601DurationRe разбирает длительность YouTube вида «PT3M45S», «PT1H2M3S»,
// «PT45S» (изредка с днями — «P1DT…»).
var iso8601DurationRe = regexp.MustCompile(`^P(?:(\d+)D)?T(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?$`)

// parseISO8601Duration переводит ISO-8601 длительность YouTube в миллисекунды.
// Возвращает 0, если строка пустая или не распознана.
func parseISO8601Duration(s string) int {
	s = strings.TrimSpace(s)
	m := iso8601DurationRe.FindStringSubmatch(s)
	if m == nil {
		return 0
	}
	days, _ := strconv.Atoi(m[1])
	hours, _ := strconv.Atoi(m[2])
	mins, _ := strconv.Atoi(m[3])
	secs, _ := strconv.Atoi(m[4])
	total := ((days*24+hours)*60+mins)*60 + secs
	return total * 1000
}

// normalizeChannelName приводит имя канала к имени исполнителя. YouTube
// автогенерирует для лейблов каналы вида «Исполнитель - Topic», а сам
// «YouTube Music» исполнителем не является — такие значения отбрасываем, чтобы
// они не попадали в историю и не становились затравкой рекомендаций.
func normalizeChannelName(raw string) string {
	name := strings.TrimSpace(raw)
	name = strings.TrimSpace(strings.TrimSuffix(name, " - Topic"))
	name = strings.TrimSpace(strings.TrimSuffix(name, " - Тема"))
	if strings.EqualFold(name, "YouTube Music") || strings.EqualFold(name, "YouTube") {
		return ""
	}
	return name
}

// bingTitleSuffixes — хвосты, которые Bing подмешивает в заголовок страницы.
var bingTitleSuffixes = []string{
	" - YouTube Music",
	" – YouTube Music",
	" | YouTube Music",
	" - YouTube",
	" – YouTube",
}

// artistTitleSeparators — разделители в порядке предпочтения. На YouTube Music
// заголовок почти всегда имеет вид «Исполнитель - Название».
var artistTitleSeparators = trackmeta.Separators

// splitBingTitle разбирает заголовок выдачи на исполнителя и название. Пустой
// artist означает «разобрать не удалось» — тогда исполнителя лучше не выдумывать:
// mapTrack на фронтенде покажет «Неизвестный исполнитель», а профиль вкусов
// (internal/recommendations) не получит фиктивного артиста. Раньше здесь всем
// трекам подставлялось «YouTube Music», и этот псевдо-артист копился в истории,
// становясь затравкой для рекомендаций. Сам разбор общий с SoundCloud — там та
// же беда с перезаливами (см. internal/trackmeta).
func splitBingTitle(raw string) (artist, title string) {
	return trackmeta.SplitArtistTitle(stripTitleSuffixes(raw))
}

// stripTitleSuffixes срезает хвосты «- YouTube Music» и т.п., не разбирая
// исполнителя. Используется, когда исполнитель уже известен (Topic-канал) и
// нужно лишь очистить название трека.
func stripTitleSuffixes(raw string) string {
	title := strings.TrimSpace(raw)
	for _, suffix := range bingTitleSuffixes {
		if trimmed := strings.TrimSuffix(title, suffix); trimmed != title {
			return strings.TrimSpace(trimmed)
		}
	}
	return title
}

// isTopicChannel сообщает, что канал — автогенерированный «Артист - Topic».
// Такой канал заводит сам лейбл, и его имя и есть настоящий исполнитель.
func isTopicChannel(channel string) bool {
	c := strings.TrimSpace(channel)
	return strings.HasSuffix(c, " - Topic") || strings.HasSuffix(c, " - Тема")
}

// resolveArtistTitle определяет настоящего исполнителя и чистое название трека
// из имени канала и заголовка YouTube. Логика такая:
//   - «Артист - Topic» — автогенерированный канал лейбла: сам является
//     исполнителем, заголовок берём как название (лишь чистим хвосты).
//   - Обычный канал-заливщик (GAZ LIVE, Uharmony и т.п.) с именем артиста не
//     связан — настоящий исполнитель сидит в заголовке «Артист - Название»,
//     поэтому парсим его оттуда, игнорируя канал. Раньше это ломало выдачу:
//     в «Ведущий исполнитель» попадал заливщик, а не искомый артист.
//   - Если заголовок без разделителя — падаем обратно на имя канала.
func resolveArtistTitle(channel, rawTitle string) (artist, title string) {
	if isTopicChannel(channel) {
		return normalizeChannelName(channel), cleanTrackTitle(stripTitleSuffixes(rawTitle))
	}
	if a, t := splitBingTitle(rawTitle); a != "" {
		return a, cleanTrackTitle(t)
	}
	return normalizeChannelName(channel), cleanTrackTitle(stripTitleSuffixes(rawTitle))
}

// cleanTrackTitle убирает из названия трека чисто оформительские скобочные
// хвосты YouTube («(Official Video)», «[Official Audio]», «(Lyric Video)»,
// «(HD)», «(Клип)» и т.п.), не трогая значимые пометки версии. Это чистит и
// отображение, и совпадение при поиске (в заголовке остаётся суть трека).
// Реализация общая с SoundCloud — см. internal/trackmeta.
func cleanTrackTitle(raw string) string {
	return trackmeta.CleanTitle(raw)
}

func tracksFromBing(results []websearch.Result) []domain.Track {
	tracks := make([]domain.Track, 0, len(results))
	// Bing может отдать одно и то же видео несколькими ссылками (с разными
	// query-параметрами) — дедуплицируем по id, иначе выдача дублируется.
	seen := make(map[string]bool, len(results))
	for _, item := range results {
		if !strings.Contains(item.URL, "/watch?v=") {
			continue
		}
		parsedURL, err := url.Parse(item.URL)
		if err != nil {
			continue
		}
		videoID := parsedURL.Query().Get("v")
		if videoID == "" || seen[videoID] {
			continue
		}
		// У Bing нет имени канала — исполнителя берём из заголовка.
		artist, title := resolveArtistTitle("", item.Title)
		if title == "" {
			continue
		}
		seen[videoID] = true
		var artists []string
		if artist != "" {
			artists = []string{artist}
		}
		tracks = append(tracks, domain.Track{
			ID:           videoID,
			Service:      domain.ServiceYouTube,
			Title:        title,
			Artists:      artists,
			ArtworkURL:   thumbnailURL(videoID),
			ExternalURL:  "https://music.youtube.com/watch?v=" + videoID,
			PlayableKind: domain.PlayableStream,
		})
	}
	return tracks
}

// thumbnailURL возвращает обложку видео по его id. YouTube отдаёт превью
// по стабильному пути i.ytimg.com/vi/<id>/..., поэтому обложка есть всегда
// (в отличие от парсинга страницы, где её часто нет). hqdefault (480x360)
// существует для ЛЮБОГО видео, включая старые и приватные превью — в отличие
// от mqdefault, который у части роликов отсутствует и давал «пустую» заглушку.
func thumbnailURL(videoID string) string {
	if videoID == "" {
		return ""
	}
	return "https://i.ytimg.com/vi/" + videoID + "/hqdefault.jpg"
}

func (s *Service) searchByYtDlp(ctx context.Context, query string) ([]domain.Track, error) {
	cmd := exec.CommandContext(ctx, "yt-dlp", "--no-warnings", "--skip-download", "--flat-playlist", "--encoding", "utf-8", "--print", "%(id)s|%(title)s|%(uploader)s|%(webpage_url)s|%(duration)s", "ytsearch10:"+query)
	// На Windows Python по умолчанию пишет в пайп в кодировке локали (cp1251),
	// из-за чего кириллица в названиях превращается в «ромбики». Форсируем UTF-8.
	cmd.Env = append(os.Environ(), "PYTHONIOENCODING=utf-8", "PYTHONUTF8=1")
	// Прячем консольное окно, иначе оно мигает при каждом поиске.
	hideConsole(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("yt-dlp search failed: %w: %s", err, strings.TrimSpace(string(out)))
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	tracks := make([]domain.Track, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 4 {
			continue
		}
		videoID := strings.TrimSpace(parts[0])
		title := strings.TrimSpace(parts[1])
		externalURL := strings.TrimSpace(parts[3])
		if videoID == "" || title == "" {
			continue
		}
		// Заливщик надёжен лишь для «Артист - Topic»; иначе исполнителя берём из
		// заголовка «Артист - Название», а не из имени канала-перезаливщика.
		artist, cleanTitle := resolveArtistTitle(parts[2], title)
		title = cleanTitle
		var artists []string
		if artist != "" {
			artists = []string{artist}
		}
		if externalURL == "" {
			externalURL = "https://music.youtube.com/watch?v=" + videoID
		}
		// %(duration)s в flat-playlist приходит числом секунд (или «NA»).
		durationMs := 0
		if len(parts) >= 5 {
			if sec, perr := strconv.ParseFloat(strings.TrimSpace(parts[4]), 64); perr == nil && sec > 0 {
				durationMs = int(sec * 1000)
			}
		}
		tracks = append(tracks, domain.Track{
			ID:           videoID,
			Service:      domain.ServiceYouTube,
			Title:        title,
			Artists:      artists,
			DurationMs:   durationMs,
			ArtworkURL:   thumbnailURL(videoID),
			ExternalURL:  externalURL,
			PlayableKind: domain.PlayableStream,
		})
	}
	return tracks, nil
}
