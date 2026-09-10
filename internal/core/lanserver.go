//go:build cryonlan

package core
import (
	"fmt"
	"net"
	"net/http"
	"time"

	"Cryon2/internal/logging"
)

// lanServerPort — фиксированный порт LAN-сервера для доступа с телефона.
const lanServerPort = 8899

// startLANServer поднимает второй HTTP-сервер на всех интерфейсах (0.0.0.0),
// чтобы приложение на телефоне (Capacitor WebView в той же Wi-Fi-сети) вызывало
// те же методы App по HTTP, а звук тянуло через уже существующий прокси
// /stream. Компилируется ТОЛЬКО со сборочным тегом `cryonlan`
// (`wails build -tags cryonlan`); в обычной десктоп-сборке действует no-op
// заглушка из lanserver_stub.go — так этот код физически не влияет на рабочий
// десктоп-билд.
//
// ЛЕГАСИ: этот режим (телефон → ПК по локальной сети) заменён на автономный
// встроенный сервер на устройстве (mobileserver.go, тег `cryonmobile`), который
// работает и через мобильную сеть, и без включённого ПК. Оставлен для отладки в
// доверенной домашней сети. Рефлексивный RPC-мост (serveRPC) — общий, см.
// rpcbridge.go.
//
// Безопасность: сервер отдаёт данные локального App; ключи/секреты остаются на
// ПК и по сети НЕ передаются (телефон хранит только введённый адрес). Поднимать
// только в доверенной домашней сети.
func (a *App) startLANServer() {
	h := &localAssetHandler{app: a}
	mux := http.NewServeMux()

	mux.HandleFunc("/api/call", func(w http.ResponseWriter, r *http.Request) {
		setCORS(w)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodPost {
			writeRPCError(w, "только POST", http.StatusMethodNotAllowed)
			return
		}
		a.serveRPC(w, r)
	})

	// Аудио и локальные файлы — переиспользуем существующий обработчик, добавив
	// заголовки CORS (кросс-доменный <audio> + Web Audio-граф эквалайзера).
	streamHandler := func(w http.ResponseWriter, r *http.Request) {
		setCORS(w)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	}
	mux.HandleFunc("/local/", streamHandler)
	mux.HandleFunc("/stream/", streamHandler)

	srv := &http.Server{
		Addr:              fmt.Sprintf("0.0.0.0:%d", lanServerPort),
		Handler:           mux,
		ReadHeaderTimeout: 15 * time.Second,
	}
	logLANAddresses()
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logging.L().Error("LAN-сервер остановлен с ошибкой", "err", err)
		}
	}()
}

// setCORS разрешает запросы со страницы Capacitor (иной origin) и выставляет
// заголовки, нужные для кросс-доменного потокового <audio> с перемоткой.
func setCORS(w http.ResponseWriter) {
	hdr := w.Header()
	hdr.Set("Access-Control-Allow-Origin", "*")
	hdr.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	hdr.Set("Access-Control-Allow-Headers", "Content-Type, Range")
	hdr.Set("Access-Control-Expose-Headers", "Content-Length, Content-Range, Accept-Ranges")
}

// logLANAddresses печатает в лог IPv4-адреса машины с портом — их пользователь
// вводит на телефоне (Настройки → Сервер Cryon).
func logLANAddresses() {
	logging.L().Info("LAN-сервер Cryon запущен — введите один из адресов на телефоне (Настройки → «Сервер Cryon»)")
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		logging.L().Error("LAN-сервер: не удалось получить сетевые адреса", "err", err)
		return
	}
	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok || ipnet.IP.IsLoopback() {
			continue
		}
		if ip4 := ipnet.IP.To4(); ip4 != nil {
			logging.L().Info("LAN-адрес", "url", fmt.Sprintf("http://%s:%d", ip4.String(), lanServerPort))
		}
	}
}
