//go:build cryonmobile

package core
import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"Cryon2/internal/logging"
)

// АВТОНОМНЫЙ встроенный сервер на устройстве (Android). Заменяет отвергнутую
// схему «телефон → ПК по локальной сети» (lanserver.go): работает и через
// мобильную сеть, и без включённого ПК, и приложением можно поделиться.
//
// Идея. Тот же Go-бэкенд (App) поднимается ВНУТРИ телефона через gomobile bind,
// слушает 127.0.0.1 на случайном порту и отдаёт: собранный SPA (index.html +
// ассеты), рефлексивный RPC (/api/call, общий serveRPC из rpcbridge.go), поток
// событий (/api/events, SSE) и аудио/локальные файлы (/local, /stream — тот же
// localAssetHandler). WebView грузит http://127.0.0.1:<порт>/ — фронтенд и API
// оказываются в ОДНОМ origin, поэтому CORS не нужен.
//
// Тег `cryonmobile` держит код вне обычной десктоп-сборки. После выноса App в
// биндируемый пакет (см. plan-android.md, «Извлечение») эти же типы переезжают
// в пакет `mobile`, а gomobile-функция Start()/Stop() вызывает startMobileServer.
// Пока — «слепой» seed, компилируемый и проверяемый через
// `go build -tags cryonmobile ./...`, физически не влияющий на рабочий билд.
//
// Безопасность: сервер слушает ТОЛЬКО петлю 127.0.0.1 (недоступен из сети).
// Ключи/секреты в APK не зашиваются — только пользовательский ввод в настройках.

// sseEnvelope — формат сообщения SSE, совпадающий с ожиданиями httpBridge.ts
// (`{event, data}` → EventsOn(name, ...data)).
type sseEnvelope struct {
	Event string        `json:"event"`
	Data  []interface{} `json:"data"`
}

// sseHub рассылает события backend всем подключённым WebView-клиентам (обычно
// один). Замена Wails EventsEmit на устройстве.
type sseHub struct {
	mu      sync.Mutex
	clients map[chan []byte]struct{}
}

func newSSEHub() *sseHub {
	return &sseHub{clients: make(map[chan []byte]struct{})}
}

func (h *sseHub) add() chan []byte {
	ch := make(chan []byte, 32)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *sseHub) remove(ch chan []byte) {
	h.mu.Lock()
	if _, ok := h.clients[ch]; ok {
		delete(h.clients, ch)
		close(ch)
	}
	h.mu.Unlock()
}

func (h *sseHub) broadcast(payload []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients {
		select {
		case ch <- payload:
		default:
			// Клиент не успевает читать — пропускаем событие, чтобы Emit из
			// backend никогда не блокировался (события идемпотентны: фронтенд
			// перечитывает состояние по ним, а не накапливает дельты).
		}
	}
}

// serveEvents — обработчик /api/events: держит SSE-соединение и стримит события.
func (h *sseHub) serveEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	hdr := w.Header()
	hdr.Set("Content-Type", "text/event-stream")
	hdr.Set("Cache-Control", "no-cache")
	hdr.Set("Connection", "keep-alive")

	ch := h.add()
	defer h.remove(ch)

	// Начальный комментарий — EventSource считает соединение открытым сразу.
	fmt.Fprint(w, ": ok\n\n")
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		}
	}
}

// sseHost — реализация platformHost для устройства: события уходят в SSE-хаб,
// «открыть ссылку» просим сделать фронтенд (у backend нет своего браузера),
// нативных диалогов выбора файла/папки нет (мобайл сканирует медиатеку сам).
type sseHost struct{ hub *sseHub }

func (h *sseHost) Emit(event string, data ...interface{}) {
	if data == nil {
		data = []interface{}{}
	}
	payload, err := json.Marshal(sseEnvelope{Event: event, Data: data})
	if err != nil {
		return
	}
	h.hub.broadcast(payload)
}

func (h *sseHost) OpenURL(url string) {
	payload, err := json.Marshal(sseEnvelope{Event: "browser:open", Data: []interface{}{url}})
	if err != nil {
		return
	}
	h.hub.broadcast(payload)
}

func (h *sseHost) PickFile(_, _, _ string) (string, error) { return "", nil }
func (h *sseHost) PickDirectory(string) (string, error)    { return "", nil }

// injectBaseURL вставляет в <head> index.html скрипт, задающий адрес backend
// фронтенду (window.__CRYON_BASE__). Берём location.origin — страница и API в
// одном origin, поэтому порт знать не нужно. Скрипт добавляется ПЕРЕД бандлом
// (сразу после <head>), чтобы installBridge увидел адрес до запуска React.
func injectBaseURL(html []byte) []byte {
	const snippet = "<script>window.__CRYON_BASE__=location.origin;</script>"
	s := string(html)
	if i := strings.Index(s, "<head>"); i >= 0 {
		cut := i + len("<head>")
		return []byte(s[:cut] + snippet + s[cut:])
	}
	// Нестандартный index.html — ставим скрипт в самое начало (тоже сработает).
	return append([]byte(snippet), html...)
}

// startMobileServer поднимает встроенный сервер на 127.0.0.1:<случайный порт>
// и возвращает базовый URL (его WebView грузит) и функцию остановки. spa —
// корень собранного фронтенда (index.html + ассеты), обычно fs.Sub(assets,
// "frontend/dist"). Побочно назначает a.platform = sseHost, поэтому события
// backend начинают уходить в WebView через SSE.
func (a *App) startMobileServer(spa fs.FS) (string, func(), error) {
	hub := newSSEHub()
	a.platform = &sseHost{hub: hub}

	assetHandler := &localAssetHandler{app: a}
	mux := http.NewServeMux()

	mux.HandleFunc("/api/call", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeRPCError(w, "только POST", http.StatusMethodNotAllowed)
			return
		}
		a.serveRPC(w, r)
	})
	mux.HandleFunc("/api/events", hub.serveEvents)
	mux.Handle("/local/", assetHandler)
	mux.Handle("/stream/", assetHandler)

	// SPA: статический файл, если он есть в сборке; иначе — index.html (SPA на
	// HashRouter, серверный фолбэк нужен только для «/» и неизвестных путей).
	indexHTML, err := fs.ReadFile(spa, "index.html")
	if err != nil {
		return "", nil, fmt.Errorf("встроенный сервер: не найден index.html фронтенда: %w", err)
	}
	index := injectBaseURL(indexHTML)
	fileServer := http.FileServer(http.FS(spa))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if name := strings.TrimPrefix(r.URL.Path, "/"); name != "" {
			if f, err := spa.Open(name); err == nil {
				_ = f.Close()
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(index)
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, fmt.Errorf("встроенный сервер: не удалось открыть порт: %w", err)
	}
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 15 * time.Second}
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			logging.L().Error("встроенный сервер остановлен с ошибкой", "err", err)
		}
	}()

	baseURL := "http://" + ln.Addr().String()
	logging.L().Info("встроенный сервер Cryon запущен", "url", baseURL)
	stop := func() { _ = srv.Close() }
	return baseURL, stop, nil
}
