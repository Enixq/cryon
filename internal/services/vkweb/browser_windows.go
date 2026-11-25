//go:build windows

package vkweb

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

func BrowserSupported() bool { return true }
func browserPath() string {
	for _, base := range []string{os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramFiles"), os.Getenv("LOCALAPPDATA")} {
		for _, suffix := range []string{"Microsoft/Edge/Application/msedge.exe", "Google/Chrome/Application/chrome.exe"} {
			candidate := filepath.Join(base, filepath.FromSlash(suffix))
			if base != "" {
				if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
					return candidate
				}
			}
		}
	}
	return ""
}

// A disposable profile contains only this explicit VK login. Other browser
// profiles are never inspected. Browser-owned 2FA/captcha works without Selenium.
func ConnectBrowser(ctx context.Context) (Session, error) {
	binary := browserPath()
	if binary == "" {
		return Session{}, &Error{Stage: "browser", Kind: "browser_missing"}
	}
	profile, err := os.MkdirTemp("", "cryon-vk-login-*")
	if err != nil {
		return Session{}, &Error{Stage: "browser"}
	}
	defer os.RemoveAll(profile)
	cmd := exec.Command(binary, "--user-data-dir="+profile, "--remote-debugging-port=0", "--remote-debugging-address=127.0.0.1", "--no-first-run", "--no-default-browser-check", "--disable-sync", "--app=https://vk.ru/audio")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	if err = cmd.Start(); err != nil {
		return Session{}, &Error{Stage: "browser"}
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	var conn *websocket.Conn
	defer func() {
		if conn != nil {
			_ = conn.WriteJSON(map[string]any{"id": 999999, "method": "Browser.close"})
			_ = conn.Close()
		}
		select {
		case <-done:
			return
		case <-time.After(3 * time.Second):
		}
		kill := exec.Command("taskkill.exe", "/PID", strconv.Itoa(cmd.Process.Pid), "/T", "/F")
		kill.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
		_ = kill.Run()
		select {
		case <-done:
		case <-time.After(time.Second):
		}
	}()
	var address string
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if raw, e := os.ReadFile(filepath.Join(profile, "DevToolsActivePort")); e == nil {
			address = debuggerAddress(string(raw))
			if address != "" {
				break
			}
		}
		select {
		case <-ctx.Done():
			return Session{}, ctx.Err()
		case <-done:
			return Session{}, &Error{Stage: "browser", Kind: "browser_closed"}
		case <-time.After(200 * time.Millisecond):
		}
	}
	if address == "" {
		return Session{}, &Error{Stage: "browser"}
	}
	conn, _, err = websocket.DefaultDialer.DialContext(ctx, address, nil)
	if err != nil {
		return Session{}, &Error{Stage: "browser"}
	}
	conn.SetReadLimit(2 << 20)
	closed := make(chan struct{})
	defer close(closed)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-closed:
		}
	}()
	var sequence int
	call := func(method string, out any) error {
		sequence++
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		if conn.WriteJSON(map[string]any{"id": sequence, "method": method}) != nil {
			return &Error{Stage: "browser"}
		}
		for {
			var result struct {
				ID     int             `json:"id"`
				Result json.RawMessage `json:"result"`
				Error  json.RawMessage `json:"error"`
			}
			if err := conn.ReadJSON(&result); err != nil {
				if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
					return &Error{Stage: "browser", Kind: "browser_timeout"}
				}
				return &Error{Stage: "browser", Kind: "browser_closed"}
			}
			if result.ID != sequence {
				continue
			}
			if len(result.Error) > 0 || json.Unmarshal(result.Result, out) != nil {
				return &Error{Stage: "browser"}
			}
			return nil
		}
	}
	var version struct {
		UserAgent string `json:"userAgent"`
	}
	if err = call("Browser.getVersion", &version); err != nil {
		return Session{}, err
	}
	for {
		var result struct {
			Cookies []BrowserCookie `json:"cookies"`
		}
		if err = call("Storage.getCookies", &result); err != nil {
			if ctx.Err() != nil {
				return Session{}, ctx.Err()
			}
			return Session{}, err
		}
		session := Session{Cookies: FormatCookies(result.Cookies), UserAgent: version.UserAgent}
		if session.Valid() {
			return session, nil
		}
		select {
		case <-ctx.Done():
			return Session{}, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

func debuggerAddress(raw string) string {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	if len(lines) != 2 {
		return ""
	}
	port, err := strconv.Atoi(strings.TrimSpace(lines[0]))
	path := strings.TrimSpace(lines[1])
	if err != nil || port < 1 || port > 65535 || !strings.HasPrefix(path, "/devtools/browser/") || strings.ContainsAny(path, "?# \r") {
		return ""
	}
	return fmt.Sprintf("ws://127.0.0.1:%d%s", port, path)
}
