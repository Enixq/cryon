package main

// Временная живая проверка прокси потоков: CRYON_PROXY_IT=1 go test . -run TestLiveStreamProxy -v
// Гейт по переменной окружения — сеть в CI недоступна.

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestLiveStreamProxy(t *testing.T) {
	if os.Getenv("CRYON_PROXY_IT") != "1" {
		t.Skip("живая проверка сети")
	}
	app := NewApp()
	app.ctx = context.Background()

	srv := httptest.NewServer(&localAssetHandler{app: app})
	defer srv.Close()

	for _, tc := range []struct{ name, path string }{
		{"youtube", "/stream/youtube/dQw4w9WgXcQ"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, srv.URL+tc.path, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Range", "bytes=0-65535")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("запрос к прокси: %v", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			t.Logf("status=%d type=%q len=%s range=%q получено=%d байт",
				resp.StatusCode, resp.Header.Get("Content-Type"),
				resp.Header.Get("Content-Length"), resp.Header.Get("Content-Range"), len(body))
			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
				t.Fatalf("прокси вернул %d, ожидался 200/206", resp.StatusCode)
			}
			if len(body) == 0 {
				t.Fatal("пустое тело потока")
			}
		})
	}
}
