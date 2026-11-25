//go:build windows

package vkweb

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// Exercises the installed Chromium/CDP stack with synthetic cookies, without
// navigating to VK or touching the user's existing browser profiles.
func TestInstalledBrowserCDPCookies(t *testing.T) {
	binary := browserPath()
	if binary == "" {
		t.Skip("Edge/Chrome is not installed")
	}
	profile := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "--headless=new", "--disable-gpu", "--no-first-run", "--user-data-dir="+profile, "--remote-debugging-port=0", "about:blank")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	defer func() {
		select {
		case <-done:
		case <-time.After(4 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
	}()
	address := ""
	for address == "" {
		if raw, err := os.ReadFile(filepath.Join(profile, "DevToolsActivePort")); err == nil {
			address = debuggerAddress(string(raw))
		}
		select {
		case <-ctx.Done():
			t.Fatal("browser CDP endpoint did not start")
		case <-done:
			t.Fatal("browser exited early")
		case <-time.After(100 * time.Millisecond):
		}
	}
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, address, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	defer func() { _ = conn.WriteJSON(map[string]any{"id": 99, "method": "Browser.close"}) }()
	call := func(id int, method string, params any, out any) {
		t.Helper()
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		if err := conn.WriteJSON(map[string]any{"id": id, "method": method, "params": params}); err != nil {
			t.Fatal(err)
		}
		for {
			var result struct {
				ID     int             `json:"id"`
				Result json.RawMessage `json:"result"`
				Error  json.RawMessage `json:"error"`
			}
			if err := conn.ReadJSON(&result); err != nil {
				t.Fatal(err)
			}
			if result.ID != id {
				continue
			}
			if len(result.Error) > 0 {
				t.Fatal("CDP operation failed", method)
			}
			if err := json.Unmarshal(result.Result, out); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	var empty map[string]any
	call(1, "Storage.setCookies", map[string]any{"cookies": []map[string]any{{"name": "p", "value": "fixture", "domain": ".vk.ru", "path": "/", "secure": true, "httpOnly": true}, {"name": "remixsid", "value": "fixture", "domain": ".vk.ru", "path": "/", "secure": true, "httpOnly": true}}}, &empty)
	var result struct {
		Cookies []BrowserCookie `json:"cookies"`
	}
	call(2, "Storage.getCookies", map[string]any{}, &result)
	if !(Session{Cookies: FormatCookies(result.Cookies)}).Valid() {
		t.Fatal("could not obtain HttpOnly session cookies through CDP")
	}
}

func TestDebuggerAddressRemainsLoopback(t *testing.T) {
	if !strings.HasPrefix(debuggerAddress("12345\n/devtools/browser/id\n"), "ws://127.0.0.1:12345/") {
		t.Fatal("invalid normal endpoint")
	}
	for _, raw := range []string{"0\n/devtools/browser/id", "99999\n/devtools/browser/id", "12345\nhttps://evil.test/", "12345\n/devtools/browser/id?bad"} {
		if debuggerAddress(raw) != "" {
			t.Fatal("accepted malformed endpoint")
		}
	}
}
