package audiofetcher

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"runtime"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/proxy"
)

// На Android системный VPN-туннель фильтрует трафик по UID приложения.
// VPN-клиенты (HappProxy/Remnawave) в режиме «антизапрет» могут не включать
// Cryon в туннель — тогда youtube.com/googlevideo.com резолвятся напрямую из
// РФ и блокируются, из-за чего аудиопоток не получается получить вообще.
//
// Обход: тот же HappProxy поднимает на петле локальный SOCKS5/HTTP-прокси
// (обычно 127.0.0.1:10808). Соединение на 127.0.0.1 НЕ проходит через
// VPN-маршрутизацию и не зависит от UID, а наружу его гонит уже сам HappProxy
// через зарубежный сервер. Поэтому запросы к YouTube/Google мы проксируем на
// этот порт, а весь остальной трафик (Piped, SoundCloud, обложки и т.п.) —
// оставляем напрямую, чтобы ничего не сломать.
//
// На десктопе (Windows) ничего не меняем: там системный VPN включает весь
// трафик, и локальный прокси не нужен.
var localProxyCandidates = []string{
	"127.0.0.1:10808",
	"127.0.0.1:1080",
	"127.0.0.1:10809",
}

var (
	localProxyMu    sync.Mutex
	localProxyAddr  string
	localProxyFound bool
)

// detectedLocalProxy однократно опрашивает известные порты локального
// SOCKS5-прокси HappProxy. Найденный адрес кэшируется навсегда; если прокси не
// запущен — проверка повторится при следующем обращении (чтобы подхватить
// HappProxy, включённый уже после старта приложения).
func detectedLocalProxy() string {
	if runtime.GOOS != "android" {
		return ""
	}
	localProxyMu.Lock()
	defer localProxyMu.Unlock()
	if localProxyFound {
		return localProxyAddr
	}
	for _, addr := range localProxyCandidates {
		d := net.Dialer{Timeout: 400 * time.Millisecond}
		conn, err := d.Dial("tcp", addr)
		if err == nil {
			_ = conn.Close()
			localProxyAddr = addr
			localProxyFound = true
			return addr
		}
	}
	return ""
}

// isGoogleHost сообщает, относится ли хост к YouTube/Google (их блокируют в РФ,
// и именно их нужно вести через локальный прокси).
func isGoogleHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, suffix := range []string{"youtube.com", "googlevideo.com", "ytimg.com", "ggpht.com"} {
		if host == suffix || strings.HasSuffix(host, "."+suffix) {
			return true
		}
	}
	return false
}

// googleDialContext возвращает DialContext, который проксирует обращения к
// YouTube/Google через локальный SOCKS5 HappProxy (когда он запущен), а прочие
// хосты соединяет напрямую через base. Если прокси недоступен — ведёт себя
// ровно как base.DialContext.
func googleDialContext(base *net.Dialer) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(addr)
		if err != nil || !isGoogleHost(host) {
			return base.DialContext(ctx, network, addr)
		}
		proxyAddr := detectedLocalProxy()
		if proxyAddr == "" {
			return base.DialContext(ctx, network, addr)
		}
		dialer, derr := proxy.SOCKS5("tcp", proxyAddr, nil, base)
		if derr == nil {
			if cd, ok := dialer.(proxy.ContextDialer); ok {
				if conn, err := cd.DialContext(ctx, network, addr); err == nil {
					return conn, nil
				}
			} else if conn, err := dialer.Dial(network, addr); err == nil {
				return conn, nil
			}
		}
		// Some Android VPN clients expose an HTTP CONNECT proxy on the same
		// local port instead of SOCKS5. Try that protocol before falling back
		// to the direct route (which is blocked for YouTube in many regions).
		if conn, err := dialHTTPConnect(ctx, base, proxyAddr, addr); err == nil {
			return conn, nil
		}
		return base.DialContext(ctx, network, addr)
	}
}

// dialHTTPConnect establishes a tunnel through a local HTTP proxy. The
// returned connection is intentionally left unencrypted; net/http performs
// the TLS handshake for the target host after this function returns.
func dialHTTPConnect(ctx context.Context, base *net.Dialer, proxyAddr, targetAddr string) (net.Conn, error) {
	conn, err := base.DialContext(ctx, "tcp", proxyAddr)
	if err != nil {
		return nil, err
	}
	closeOnError := true
	defer func() {
		if closeOnError {
			_ = conn.Close()
		}
	}()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	if _, err := fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", targetAddr, targetAddr); err != nil {
		return nil, err
	}
	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	parts := strings.Fields(line)
	if len(parts) < 2 || parts[0] != "HTTP/1.1" && parts[0] != "HTTP/1.0" || parts[1] != "200" {
		return nil, fmt.Errorf("HTTP proxy CONNECT failed: %s", strings.TrimSpace(line))
	}
	for {
		line, err = reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(line) == "" {
			break
		}
	}
	_ = conn.SetDeadline(time.Time{})
	closeOnError = false
	return conn, nil
}

// GoogleDialContext — экспортируемая обёртка над googleDialContext для пакета
// core (скачивание потока в кэш идёт отдельным HTTP-клиентом там).
func GoogleDialContext(base *net.Dialer) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return googleDialContext(base)
}
