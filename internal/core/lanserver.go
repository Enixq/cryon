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

// startLANServer exposes the narrow device-sync surface to the local network.
// Catalog, credentials, file access and reflective RPC stay on the local app.
func (a *App) startLANServer() {
	startDeviceDiscoveryResponder()
	mux := http.NewServeMux()
	handle := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			setCORS(w)
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next(w, r)
		}
	}
	mux.HandleFunc("/v1/system/health", handle(a.handleAPIHealth))
	mux.HandleFunc("/v1/devices/state", handle(a.handleAPIDeviceState))
	mux.HandleFunc(libraryExchangePath, handle(a.handleAPILibraryExchange))

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
	hdr.Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
	hdr.Set("Access-Control-Allow-Headers", "Content-Type, Range")
	hdr.Set("Access-Control-Allow-Private-Network", "true")
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
