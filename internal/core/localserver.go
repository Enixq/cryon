package core

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"Cryon2/internal/logging"
)

// localAssetHandler отдаёт локальные аудиофайлы фронтенду по HTTP-пути
// вида /local/<trackID>. Это нужно для фолбэка HTML5 <audio>: WebView2
// не проигрывает файлы по абсолютному пути file://, но легко играет их
// с внутреннего ассет-сервера Wails (тот же origin).
//
// http.ServeContent сам выставляет Content-Type и поддерживает Range,
// поэтому перемотка по треку работает корректно.
type localAssetHandler struct {
	app *App
}

func (h *localAssetHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, streamPrefix) {
		h.serveRemoteStream(w, r)
		return
	}
	const prefix = "/local/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		http.NotFound(w, r)
		return
	}
	trackID := strings.TrimPrefix(r.URL.Path, prefix)
	if trackID == "" {
		http.NotFound(w, r)
		return
	}

	path, ok := h.app.local.FilePath(trackID)
	if !ok {
		http.Error(w, "локальный трек не найден", http.StatusNotFound)
		return
	}

	f, err := os.Open(path)
	if err != nil {
		logging.L().Warn("не удалось открыть локальный файл", "path", path, "err", err)
		http.Error(w, "не удалось открыть файл", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		http.Error(w, "не удалось прочитать файл", http.StatusInternalServerError)
		return
	}

	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
}

// streamPrefix — путь внутреннего прокси удалённых аудиопотоков:
// /stream/<service>/<trackID>.
const streamPrefix = "/stream/"

// streamProxyClient — клиент для прокачки аудио. Без общего таймаута: тело
// потока читается всё время воспроизведения трека, и таймаут его бы обрывал.
var streamProxyClient = &http.Client{
	Timeout: 0,
	// Редиректы googlevideo/сервисов проходим сами — заголовок Range должен
	// уехать и на конечный хост.
	CheckRedirect: func(*http.Request, []*http.Request) error { return nil },
}

// streamProxyHeaders — заголовки запроса, которые имеет смысл передать наверх.
// Accept-Encoding намеренно НЕ пробрасываем: иначе Go-транспорт отдал бы тело
// как есть (возможно, gzip), а Content-Encoding обратно мы не копируем — браузер
// принял бы сжатые байты за аудио. Без него транспорт сам прозрачно разожмёт
// (для уже сжатых медиапотоков это в любом случае no-op).
var streamProxyHeaders = []string{"Range", "If-Range", "Accept", "Referer", "Origin"}

// streamProxyResponseHeaders — заголовки ответа, нужные <audio> для перемотки
// и корректного декодирования.
var streamProxyResponseHeaders = []string{
	"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges",
	"Last-Modified", "ETag", "Cache-Control",
}

// streamURLTTL — сколько держим разрешённую ссылку на поток. <audio> тянет трек
// множеством Range-запросов, и разрешать поток заново на каждый из них нельзя:
// для YouTube это разбор страницы видео с расшифровкой подписи (секунды), для
// SoundCloud — два обращения к api-v2. Ссылки живут заметно дольше (googlevideo
// — часы), так что короткий TTL безопасен, а истёкшую ссылку ловим по 403/410
// от апстрима и переоткрываем.
const streamURLTTL = 5 * time.Minute

// streamURLCacheMax — верхняя граница числа записей в кэше ссылок. Кэш живёт
// весь сеанс; без границы он рос бы по записи на каждый прослушанный трек. При
// достижении предела чистим протухшее, а если и это не помогло (все ссылки
// свежие) — вытесняем самые старые. Одновременно играет один трек, так что
// реально нужно единицы записей — предел с большим запасом.
const streamURLCacheMax = 256

// streamURLCache — кэш разрешённых ссылок на потоки, общий для всех запросов
// прокси.
var streamURLCache = struct {
	sync.Mutex
	m map[string]cachedStreamURL
}{m: map[string]cachedStreamURL{}}

type cachedStreamURL struct {
	url string
	at  time.Time
}

// streamResolveGroup дедуплицирует одновременные резолвы одного и того же
// потока. При старте трека <audio> во WebView2 шлёт несколько Range-запросов
// почти одновременно; на холодном кэше без дедупликации каждый запускал бы
// полный разбор (для YouTube — загрузка страницы видео с расшифровкой подписи,
// секунды). Здесь первый запрос по ключу резолвит, а остальные ждут его
// результат — аналог golang.org/x/sync/singleflight, но без новой зависимости.
var streamResolveGroup = struct {
	sync.Mutex
	calls map[string]*streamResolveCall
}{calls: map[string]*streamResolveCall{}}

type streamResolveCall struct {
	done chan struct{}
	url  string
	err  error
}

// resolveStreamURL возвращает ссылку на поток трека, по возможности из кэша.
// force=true игнорирует кэш (апстрим отверг прежнюю ссылку).
func (h *localAssetHandler) resolveStreamURL(service, trackID string, force bool) (string, error) {
	key := service + "\x00" + trackID

	if !force {
		streamURLCache.Lock()
		hit, ok := streamURLCache.m[key]
		streamURLCache.Unlock()
		if ok && time.Since(hit.at) < streamURLTTL {
			return hit.url, nil
		}
	}

	// Дедупликация: если резолв этого потока уже идёт, ждём его результат, а не
	// запускаем второй тяжёлый разбор параллельно. Резолв всегда обращается к
	// апстриму за свежей ссылкой, поэтому присоединиться к идущему безопасно и
	// на force-пути (переоткрытие после 403/410).
	streamResolveGroup.Lock()
	if call, ok := streamResolveGroup.calls[key]; ok {
		streamResolveGroup.Unlock()
		<-call.done
		return call.url, call.err
	}
	call := &streamResolveCall{done: make(chan struct{})}
	streamResolveGroup.calls[key] = call
	streamResolveGroup.Unlock()
	// Снимаем запись и будим ожидающих через defer — даже если резолв запаникует,
	// последователи не зависнут на call.done (получат пустой результат и мягко
	// свалятся в 502, а не в вечное ожидание).
	defer func() {
		streamResolveGroup.Lock()
		delete(streamResolveGroup.calls, key)
		streamResolveGroup.Unlock()
		close(call.done)
	}()

	call.url, call.err = h.doResolveStreamURL(service, trackID, key)
	return call.url, call.err
}

// doResolveStreamURL выполняет собственно (дорогой) резолв ссылки и кладёт её в
// кэш. Вызывается строго под дедупликацией streamResolveGroup. Ошибки не
// кэшируются: запись в streamResolveGroup удаляется после завершения, поэтому
// следующий запрос повторит резолв заново.
func (h *localAssetHandler) doResolveStreamURL(service, trackID, key string) (string, error) {
	stream, err := h.app.GetAudioStream(service, trackID)
	if err != nil {
		return "", err
	}
	if stream == nil || stream.URL == "" {
		return "", errNoStreamURL
	}

	streamURLCache.Lock()
	// Чистим кэш только когда он разросся до предела — обычный резолв за полный
	// проход по map не платит (раньше проход шёл на каждый промах).
	if len(streamURLCache.m) >= streamURLCacheMax {
		// Сначала выбрасываем протухшее.
		for k, v := range streamURLCache.m {
			if time.Since(v.at) >= streamURLTTL {
				delete(streamURLCache.m, k)
			}
		}
		// Если всё ещё у предела (все ссылки свежие) — вытесняем самую старую,
		// пока не уйдём под границу, чтобы map не рос бесконечно.
		for len(streamURLCache.m) >= streamURLCacheMax {
			var oldestKey string
			var oldestAt time.Time
			first := true
			for k, v := range streamURLCache.m {
				if first || v.at.Before(oldestAt) {
					oldestKey, oldestAt, first = k, v.at, false
				}
			}
			delete(streamURLCache.m, oldestKey)
		}
	}
	streamURLCache.m[key] = cachedStreamURL{url: stream.URL, at: time.Now()}
	streamURLCache.Unlock()
	return stream.URL, nil
}

// invalidateStreamURL убирает ссылку из кэша, чтобы следующий запрос переоткрыл её.
func invalidateStreamURL(service, trackID string) {
	streamURLCache.Lock()
	delete(streamURLCache.m, service+"\x00"+trackID)
	streamURLCache.Unlock()
}

var errNoStreamURL = errors.New("пустая ссылка на поток")

// parseStreamPath разбирает путь /stream/<service>/<trackID>.
//
// На вход идёт ИМЕННО escaped-путь (r.URL.EscapedPath): id трека сам может
// содержать «/» — у SoundCloud из веб-поиска это целая ссылка на трек, — и
// фронтенд шлёт его через encodeURIComponent. В r.URL.Path слэш уже
// раскодирован, поэтому разбор по нему разъехался бы на первом же таком id.
func parseStreamPath(escapedPath string) (service, trackID string, ok bool) {
	rest := strings.TrimPrefix(escapedPath, streamPrefix)
	if rest == escapedPath {
		return "", "", false
	}
	rawService, rawID, found := strings.Cut(rest, "/")
	if !found || rawService == "" || rawID == "" {
		return "", "", false
	}
	service, err := url.PathUnescape(rawService)
	if err != nil || service == "" {
		return "", "", false
	}
	trackID, err = url.PathUnescape(rawID)
	if err != nil || trackID == "" {
		return "", "", false
	}
	return service, trackID, true
}

// serveRemoteStream проксирует аудиопоток внешнего сервиса через собственный
// ассет-сервер приложения.
//
// Зачем прокси, а не прямая ссылка в <audio src>. Прямые ссылки внешних
// сервисов во WebView2 воспроизводятся ненадёжно: googlevideo отвечает 403 на
// часть запросов вебвью (например, на HEAD и на запросы с чужим Origin), а сам
// поток вообще-то отдаётся — тот же URL из процесса приложения возвращает 206.
// Пользователь при этом видел просто «трек не начинается» без единой ошибки,
// потому что фронтенд глушил отказ <audio>. Через прокси поток становится
// same-origin: перемотка (Range) работает, отказы видны в логе, а Web
// Audio-граф эквалайзера не получает «испорченный» кросс-доменный источник
// (иначе он играет тишину).
func (h *localAssetHandler) serveRemoteStream(w http.ResponseWriter, r *http.Request) {
	service, trackID, ok := parseStreamPath(r.URL.EscapedPath())
	if !ok {
		http.NotFound(w, r)
		return
	}

	start, requestedEnd, segmented, requestedRange := youtubeSegmentedRange(service, r.Header.Get("Range"))
	upstreamRequest := r
	if segmented {
		upstreamRequest = r.Clone(r.Context())
		upstreamRequest.Header = r.Header.Clone()
		firstEnd := start + youtubeStreamChunkSize - 1
		if requestedEnd >= 0 && requestedEnd < firstEnd {
			firstEnd = requestedEnd
		}
		upstreamRequest.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, firstEnd))
	}
	resp, err := h.fetchStreamResponse(upstreamRequest, service, trackID)
	if err != nil {
		if r.Context().Err() == nil {
			http.Error(w, "поток недоступен", http.StatusBadGateway)
		}
		return
	}
	defer resp.Body.Close()

	if segmented && resp.StatusCode == http.StatusPartialContent {
		var first, last, total int64
		if _, err := fmt.Sscanf(resp.Header.Get("Content-Range"), "bytes %d-%d/%d", &first, &last, &total); err == nil && first == start && last >= first && last < total && (requestedEnd < 0 || last <= requestedEnd) {
			finalEnd := total - 1
			if requestedEnd >= 0 && requestedEnd < finalEnd {
				finalEnd = requestedEnd
			}
			for _, name := range streamProxyResponseHeaders {
				if v := resp.Header.Get(name); v != "" {
					w.Header().Set(name, v)
				}
			}
			w.Header().Set("Accept-Ranges", "bytes")
			w.Header().Set("Content-Length", strconv.FormatInt(finalEnd-start+1, 10))
			if requestedRange {
				w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, finalEnd, total))
				w.WriteHeader(http.StatusPartialContent)
			} else {
				w.Header().Del("Content-Range")
				w.WriteHeader(http.StatusOK)
			}
			if _, err := io.Copy(w, resp.Body); err != nil {
				return
			}
			for next := last + 1; next <= finalEnd; {
				end := next + youtubeStreamChunkSize - 1
				if end > finalEnd {
					end = finalEnd
				}
				segmentRequest := r.Clone(r.Context())
				segmentRequest.Header = r.Header.Clone()
				segmentRequest.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", next, end))
				segment, err := h.fetchStreamResponse(segmentRequest, service, trackID)
				if err != nil {
					return
				}
				var segmentStart, segmentEnd, segmentTotal int64
				_, parseErr := fmt.Sscanf(segment.Header.Get("Content-Range"), "bytes %d-%d/%d", &segmentStart, &segmentEnd, &segmentTotal)
				if segment.StatusCode != http.StatusPartialContent || parseErr != nil || segmentStart != next || segmentEnd < next || segmentEnd > end || segmentTotal != total {
					segment.Body.Close()
					return
				}
				_, copyErr := io.Copy(w, segment.Body)
				segment.Body.Close()
				if copyErr != nil {
					return
				}
				next = segmentEnd + 1
			}
			return
		}
		http.Error(w, "некорректный ответ источника", http.StatusBadGateway)
		return
	}

	if resp.StatusCode >= 400 {
		logging.L().Warn("прокси потока: апстрим отказал", "service", service, "status", resp.StatusCode)
	}

	for _, name := range streamProxyResponseHeaders {
		if v := resp.Header.Get(name); v != "" {
			w.Header().Set(name, v)
		}
	}
	if w.Header().Get("Accept-Ranges") == "" {
		w.Header().Set("Accept-Ranges", "bytes")
	}
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		logging.L().Debug("прокси потока: копирование прервано", "service", service, "err", err)
	}
}

const youtubeStreamChunkSize int64 = 1 << 18

func youtubeSegmentedRange(service, header string) (int64, int64, bool, bool) {
	if service != "youtube" {
		return 0, 0, false, false
	}
	if header == "" {
		return 0, -1, true, false
	}
	if !strings.HasPrefix(header, "bytes=") {
		return 0, 0, false, false
	}
	startText, endText, ok := strings.Cut(strings.TrimPrefix(header, "bytes="), "-")
	if !ok || startText == "" {
		return 0, 0, false, false
	}
	start, err := strconv.ParseInt(startText, 10, 64)
	if err != nil || start < 0 || start > (1<<63-1)-youtubeStreamChunkSize {
		return 0, 0, false, false
	}
	if endText == "" {
		return start, -1, true, true
	}
	end, err := strconv.ParseInt(endText, 10, 64)
	if err != nil || end < start || end-start < youtubeStreamChunkSize {
		return 0, 0, false, false
	}
	return start, end, true, true
}

func (h *localAssetHandler) fetchStreamResponse(r *http.Request, service, trackID string) (*http.Response, error) {
	const maxStreamAttempts = 3
	var resp *http.Response
	force := false
	for attempt := 1; ; attempt++ {
		streamURL, err := h.resolveStreamURL(service, trackID, force)
		if err != nil {
			logging.L().Warn("прокси потока: не удалось получить ссылку", "service", service, "attempt", attempt, "err", err)
			return nil, err
		}
		resp, err = h.fetchUpstream(r, streamURL, service)
		if err != nil {
			if r.Context().Err() != nil {
				return nil, r.Context().Err()
			}
			var urlError *url.Error
			if errors.As(err, &urlError) {
				err = urlError.Err
			}
			logging.L().Warn("прокси потока: апстрим недоступен", "service", service, "attempt", attempt, "err", err)
			if attempt >= maxStreamAttempts {
				return nil, err
			}
		} else if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusGone {
			resp.Body.Close()
			if attempt >= maxStreamAttempts {
				logging.L().Warn("прокси потока: апстрим отвергает ссылку после ретраев", "service", service, "status", resp.StatusCode, "attempts", attempt)
				return nil, fmt.Errorf("апстрим отверг поток: HTTP %d", resp.StatusCode)
			}
			invalidateStreamURL(service, trackID)
			force = true
		} else if attempt < maxStreamAttempts && (resp.StatusCode == http.StatusRequestTimeout || resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500) {
			resp.Body.Close()
			force = false
		} else {
			break
		}
		select {
		case <-time.After(time.Duration(attempt*250) * time.Millisecond):
		case <-r.Context().Done():
			// Клиент ушёл (переключил трек/перемотал) — не спим впустую.
			return nil, r.Context().Err()
		}
	}
	return resp, nil
}

// fetchUpstream выполняет запрос к источнику потока, передавая наверх
// заголовки клиента (в первую очередь Range — от него зависит перемотка).
func (h *localAssetHandler) fetchUpstream(r *http.Request, streamURL, service string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, streamURL, nil)
	if err != nil {
		return nil, err
	}
	for _, name := range streamProxyHeaders {
		if v := r.Header.Get(name); v != "" {
			req.Header.Set(name, v)
		}
	}
	// Свой User-Agent: часть CDN отвечает 403 на пустой.
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	if service == "youtube" {
		if req.Header.Get("Referer") == "" {
			req.Header.Set("Referer", "https://www.youtube.com/")
		}
		if req.Header.Get("Origin") == "" {
			req.Header.Set("Origin", "https://www.youtube.com")
		}
	}
	return streamProxyClient.Do(req)
}
