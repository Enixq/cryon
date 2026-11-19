package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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

// streamProxyClient — клиент для прокачки аудиопотока. Без общего таймаута: тело
// потока читается всё время воспроизведения трека, и таймаут его бы обрывал.
var streamProxyClient = &http.Client{
	Timeout: 0,
	// Редиректы googlevideo/сервисов проходим сами — заголовок Range должен
	// уехать и на конечный хост.
	CheckRedirect: func(*http.Request, []*http.Request) error { return nil },
}

// streamDownloadClient — клиент скачивания потока в кэш. Фазовые таймауты, а не
// общий: трек может качаться долго на медленном соединении, но зависнуть на
// одном чанке он не должен. ResponseHeaderTimeout ловит «соединение есть, а
// байты не идут».
var streamDownloadClient = &http.Client{
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 25 * time.Second,
		ExpectContinueTimeout: 2 * time.Second,
		MaxIdleConns:          8,
		IdleConnTimeout:       90 * time.Second,
	},
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

// streamFileCache — кэш ПОЛНОСТЬЮ скачанных потоков в локальные файлы.
//
// Зачем: потоковая раздача чанками (как было раньше) обрывала трек на середине,
// если любой чанк после первого получал 403/503 от googlevideo (rate-limit,
// протухание ссылки, троттлинг). Браузер видел обрезанный файл и молча
// останавливал воспроизведение — пользователь получал «то играет, то нет».
// Скачивание всего файла в кэш один раз (с ретраями и докачкой по Range) и
// последующая отдача через http.ServeContent делает перемотку и воспроизведение
// независимыми от нестабильности CDN: данные локальны и всегда полны.
type cachedStreamFile struct {
	path string
	size int64
	at   time.Time
}

var streamFileCache = struct {
	sync.Mutex
	m    map[string]cachedStreamFile
	infl map[string]*streamFileCall
}{m: map[string]cachedStreamFile{}, infl: map[string]*streamFileCall{}}

// streamFileCall дедуплицирует одновременные загрузки одного потока: при старте
// трека <audio> шлёт несколько Range-запросов почти одновременно, и каждый не
// должен запускать отдельное скачивание.
type streamFileCall struct {
	done chan struct{}
	path string
	size int64
	err  error
}

// streamFileTTL — сколько держим скачанный файл. Долго: перескачивать трек при
// повторном прослушивании в рамках сеанса незачем.
const streamFileTTL = 2 * time.Hour

// streamFileMaxBytes — предохранитель от скачивания гигантских/livestream-видео.
const streamFileMaxBytes = 200 << 20

// streamDownloadDir — каталог для кэша скачанных потоков.
func streamDownloadDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	dir := filepath.Join(base, "Cryon2", "streams")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

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

// serveRemoteStream отдаёт аудиопоток внешнего сервиса через собственный
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
//
// Стратегия раздачи. Раньше поток стримился чанками по 256КБ прямо из
// googlevideo. Любой чанк после первого, получивший 403/503 (rate-limit,
// троттлинг, протухание ссылки), обрывал раздачу посередине: браузер видел
// неполный файл и останавливал воспроизведение — отсюда «то играет, то нет».
// Теперь файл скачивается в локальный кэш ЦЕЛИКОМ (с ретраями и докачкой по
// Range), а затем отдаётся через http.ServeContent. Данные локальны и всегда
// полны, поэтому перемотка и воспроизведение больше не зависят от капризов
// CDN. Цена — задержка на полное скачивание трека (единицы секунд), что для
// музыки предпочтительнее срывов на середине.
func (h *localAssetHandler) serveRemoteStream(w http.ResponseWriter, r *http.Request) {
	service, trackID, ok := parseStreamPath(r.URL.EscapedPath())
	if !ok {
		http.NotFound(w, r)
		return
	}

	file, contentType, err := h.ensureStreamFile(r.Context(), service, trackID)
	if err != nil {
		if r.Context().Err() == nil {
			logging.L().Warn("прокси потока: не удалось скачать", "service", service, "track", trackID, "err", err)
			http.Error(w, "поток недоступен", http.StatusBadGateway)
		}
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		http.Error(w, "кэш недоступен", http.StatusInternalServerError)
		return
	}
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	http.ServeContent(w, r, "", info.ModTime(), file)
}

// ensureStreamFile возвращает открытый файл скачанного потока и его Content-Type,
// при необходимости скачивая поток в кэш (с дедупликацией одновременных
// запросов). Файл остаётся открытым для чтения вызывающим кодом.
func (h *localAssetHandler) ensureStreamFile(ctx context.Context, service, trackID string) (*os.File, string, error) {
	key := service + "\x00" + trackID

	// Быстрый путь: файл уже в кэше.
	streamFileCache.Lock()
	if hit, ok := streamFileCache.m[key]; ok && time.Since(hit.at) < streamFileTTL {
		streamFileCache.Unlock()
		if f, err := os.Open(hit.path); err == nil {
			if ct := readStreamFileContentType(hit.path); ct != "" {
				return f, ct, nil
			}
			f.Close()
		}
		// Файл пропал (очистка ОС) — перескачиваем.
		delete(streamFileCache.m, key)
	} else if ok {
		delete(streamFileCache.m, key)
	}
	// Дедупликация: присоединяемся к идущему скачиванию.
	if call, infl := streamFileCache.infl[key]; infl {
		streamFileCache.Unlock()
		select {
		case <-call.done:
		case <-ctx.Done():
			return nil, "", ctx.Err()
		}
		streamFileCache.Lock()
		hit := streamFileCache.m[key]
		streamFileCache.Unlock()
		if call.err != nil {
			return nil, "", call.err
		}
		f, err := os.Open(hit.path)
		if err != nil {
			return nil, "", err
		}
		return f, readStreamFileContentType(hit.path), nil
	}
	call := &streamFileCall{done: make(chan struct{})}
	streamFileCache.infl[key] = call
	streamFileCache.Unlock()

	call.path, call.size, call.err = h.downloadStreamToFile(ctx, service, trackID, key)

	streamFileCache.Lock()
	delete(streamFileCache.infl, key)
	if call.err == nil {
		streamFileCache.m[key] = cachedStreamFile{path: call.path, size: call.size, at: time.Now()}
		h.pruneStreamFileCacheLocked()
	}
	streamFileCache.Unlock()
	close(call.done)

	if call.err != nil {
		return nil, "", call.err
	}
	f, err := os.Open(call.path)
	if err != nil {
		return nil, "", err
	}
	return f, readStreamFileContentType(call.path), nil
}

// downloadStreamToFile скачивает поток в файл внутри кэша. Устойчив к
// прерываниям: при обрыве на середине докачивает остаток по Range с нового
// разрешённого URL, а не начинает с нуля и не отдаёт обрезанный файл.
func (h *localAssetHandler) downloadStreamToFile(ctx context.Context, service, trackID, key string) (string, int64, error) {
	dir, err := streamDownloadDir()
	if err != nil {
		return "", 0, err
	}
	sum := sha256.Sum256([]byte(key))
	base := hex.EncodeToString(sum[:])[:24]
	finalPath := filepath.Join(dir, base)
	partPath := finalPath + ".part"

	const maxDownloadAttempts = 5
	var contentType string
	// forceNext=true требует свежей ссылки: ставим только после 403/410
	// (источник отверг именно ссылку). При 5xx/сетевом сбое ссылка цела —
	// переиспользуем её из кэша, не устраивая дорогой повторный резолв.
	forceNext := false
	for attempt := 1; attempt <= maxDownloadAttempts; attempt++ {
		if ctx.Err() != nil {
			break
		}
		// Сколько уже скачано — точка докачки.
		var have int64
		if fi, err := os.Stat(partPath); err == nil {
			have = fi.Size()
		}

		streamURL, err := h.resolveStreamURL(service, trackID, forceNext)
		if err != nil {
			// Не удалось получить ссылку — чистим partial и пробуем снова.
			_ = os.Remove(partPath)
			have = 0
			h.backoff(ctx, attempt)
			continue
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, streamURL, nil)
		if err != nil {
			return "", 0, err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		if service == "youtube" {
			req.Header.Set("Referer", "https://www.youtube.com/")
			req.Header.Set("Origin", "https://www.youtube.com")
		}
		if have > 0 {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", have))
		}

		resp, err := streamDownloadClient.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			_ = os.Remove(partPath)
			h.backoff(ctx, attempt)
			continue
		}

		// 403/410 — ссылка протухла: инвалидируем кэш URL, убираем partial
		// (докачка по протухшей ссылке невозможна) и повторяем с новой ссылкой.
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusGone {
			resp.Body.Close()
			invalidateStreamURL(service, trackID)
			_ = os.Remove(partPath)
			have = 0
			forceNext = true
			h.backoff(ctx, attempt)
			continue
		}
		if resp.StatusCode >= 500 || resp.StatusCode == http.StatusRequestTimeout || resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			h.backoff(ctx, attempt)
			continue
		}
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
			resp.Body.Close()
			return "", 0, fmt.Errorf("источник вернул HTTP %d", resp.StatusCode)
		}
		if contentType == "" {
			contentType = resp.Header.Get("Content-Type")
		}

		// Если сервер не принял Range (дал 200 вместо 206), а у нас уже есть
		// хвост — начинаем файл заново, иначе допишем дубликаты.
		shouldTruncate := have > 0 && resp.StatusCode == http.StatusOK
		f, err := h.appendOrWriteFile(partPath, have, shouldTruncate)
		if err != nil {
			resp.Body.Close()
			return "", 0, err
		}
		written, err := h.copyWithLimit(f, resp.Body, streamFileMaxBytes)
		closeErr := f.Close()
		resp.Body.Close()
		if err == nil {
			err = closeErr
		}
		if written >= streamFileMaxBytes {
			return "", 0, fmt.Errorf("поток превышает допустимый размер")
		}

		// Определяем полную длину и проверяем полноту файла.
		expected := expectedStreamLength(resp, have, shouldTruncate)
		fi, statErr := os.Stat(partPath)
		if err == nil && statErr == nil && (expected <= 0 || fi.Size() >= expected) {
			// Скачивание завершено.
			if rmErr := os.Remove(finalPath); rmErr != nil && !os.IsNotExist(rmErr) {
				return "", 0, rmErr
			}
			if err := os.Rename(partPath, finalPath); err != nil {
				return "", 0, err
			}
			h.writeStreamFileContentType(finalPath, contentType)
			return finalPath, fi.Size(), nil
		}
		if err != nil && ctx.Err() != nil {
			break
		}
		// Прервано/неполно — оставим partial для докачки на следующем заходе.
		h.backoff(ctx, attempt)
	}
	_ = os.Remove(partPath)
	if ctx.Err() != nil {
		return "", 0, ctx.Err()
	}
	return "", 0, fmt.Errorf("не удалось скачать поток за %d попыток", maxDownloadAttempts)
}

// appendOrWriteFile открывает partial-файл для докачки с позиции have.
func (h *localAssetHandler) appendOrWriteFile(partPath string, have int64, truncate bool) (*os.File, error) {
	if truncate || have <= 0 {
		f, err := os.OpenFile(partPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, err
		}
		return f, nil
	}
	f, err := os.OpenFile(partPath, os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	if _, err := f.Seek(have, io.SeekStart); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

// copyWithLimit копирует поток, останавливаясь на streamFileMaxBytes.
func (h *localAssetHandler) copyWithLimit(dst io.Writer, src io.Reader, limit int64) (int64, error) {
	return io.Copy(dst, io.LimitReader(src, limit))
}

// expectedStreamLength вычисляет ожидаемый полный размер файла.
func expectedStreamLength(resp *http.Response, have int64, truncated bool) int64 {
	if truncated {
		have = 0
	}
	if resp.StatusCode == http.StatusPartialContent {
		_, _, total, ok := parseContentRangeTotal(resp.Header.Get("Content-Range"))
		if ok {
			return total
		}
	}
	if resp.ContentLength > 0 {
		return have + resp.ContentLength
	}
	return 0
}

// backoff — пауза перед повторной попыткой; учтён уход клиента.
func (h *localAssetHandler) backoff(ctx context.Context, attempt int) {
	select {
	case <-time.After(time.Duration(attempt*300) * time.Millisecond):
	case <-ctx.Done():
	}
}

// pruneStreamFileCacheLocked чистит кэш файлов по TTL и предельному размеру.
// Вызывается под streamFileCache.Lock.
func (h *localAssetHandler) pruneStreamFileCacheLocked() {
	const maxCachedFiles = 64
	stale := time.Now().Add(-streamFileTTL)
	for k, v := range streamFileCache.m {
		if v.at.Before(stale) {
			_ = os.Remove(v.path)
			delete(streamFileCache.m, k)
		}
	}
	for len(streamFileCache.m) > maxCachedFiles {
		var oldestKey string
		var oldestAt time.Time
		first := true
		for k, v := range streamFileCache.m {
			if first || v.at.Before(oldestAt) {
				oldestKey, oldestAt, first = k, v.at, false
			}
		}
		if oldestKey == "" {
			break
		}
		_ = os.Remove(streamFileCache.m[oldestKey].path)
		delete(streamFileCache.m, oldestKey)
	}
}

// contentTypeName — имя файла, где держим Content-Type скачанного потока.
func contentTypeSidecar(path string) string { return path + ".type" }

func (h *localAssetHandler) writeStreamFileContentType(path, contentType string) {
	if contentType == "" {
		return
	}
	_ = os.WriteFile(contentTypeSidecar(path), []byte(contentType), 0o600)
}

func readStreamFileContentType(path string) string {
	b, err := os.ReadFile(contentTypeSidecar(path))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// parseContentRangeTotal разбирает «bytes 0-99/12345» и возвращает total.
func parseContentRangeTotal(header string) (int64, int64, int64, bool) {
	if !strings.HasPrefix(header, "bytes ") {
		return 0, 0, 0, false
	}
	spec := strings.TrimPrefix(header, "bytes ")
	rangePart, totalPart, found := strings.Cut(spec, "/")
	if !found {
		return 0, 0, 0, false
	}
	total, err := strconv.ParseInt(strings.TrimSpace(totalPart), 10, 64)
	if err != nil {
		return 0, 0, 0, false
	}
	first, last, found := strings.Cut(rangePart, "-")
	if !found {
		return 0, 0, total, true
	}
	f, err1 := strconv.ParseInt(strings.TrimSpace(first), 10, 64)
	l, err2 := strconv.ParseInt(strings.TrimSpace(last), 10, 64)
	if err1 != nil || err2 != nil {
		return 0, 0, total, true
	}
	return f, l, total, true
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
