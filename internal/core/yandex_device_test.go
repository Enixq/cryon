package core

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"Cryon2/internal/domain"
	"Cryon2/internal/services/yandex"
)

type deviceRoundTripper func(*http.Request) (*http.Response, error)

func (f deviceRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestYandexDeviceFlow(t *testing.T) {
	old := yandexDeviceHTTP
	defer func() { yandexDeviceHTTP = old }()
	authorized := false
	yandexDeviceHTTP = &http.Client{Transport: deviceRoundTripper(func(r *http.Request) (*http.Response, error) {
		var body string
		status := 200
		switch r.URL.Path {
		case "/device/code":
			body = `{"device_code":"private-code","user_code":"shown-code","expires_in":300,"interval":5}`
		case "/token":
			if !authorized {
				status = 400
				body = `{"error":"authorization_pending"}`
			} else {
				body = `{"access_token":"private-token"}`
			}
		default:
			t.Fatalf("unexpected request %s", r.URL.Path)
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	service := yandex.New("")
	app := &App{registry: map[domain.ServiceID]domain.MusicService{domain.ServiceYandex: service}}
	call := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		app.APIHandler().ServeHTTP(rec, req)
		return rec
	}
	start := call("/v1/auth/yandex/device/start")
	if start.Code != 200 || !strings.Contains(start.Body.String(), "shown-code") || strings.Contains(start.Body.String(), "private-code") {
		t.Fatalf("unexpected start: %d %s", start.Code, start.Body.String())
	}
	pending := call("/v1/auth/yandex/device/poll")
	if pending.Code != 200 || service.HasToken() {
		t.Fatal("pending authorization connected account")
	}
	authorized = true
	done := call("/v1/auth/yandex/device/poll")
	if done.Code != 200 || !service.HasToken() || strings.Contains(done.Body.String(), "private-token") {
		t.Fatalf("unexpected completion: %d %s", done.Code, done.Body.String())
	}
	if again := call("/v1/auth/yandex/device/poll"); again.Code != 410 {
		t.Fatalf("completed code reused: %d", again.Code)
	}
}
