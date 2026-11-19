package core

// Эмпирическая проверка прокси так, как это делает реальный <audio>:
// probe → чтение начала → перемотка в середину → чтение хвоста.
// CRYON_PROXY_IT=1 go test . -run TestProxyLikeBrowser -v

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// browserVideoIDs — реальные треки, на которых воспроизведение «то играет, то нет».
var browserVideoIDs = []string{
	"dQw4w9WgXcQ", // Rick Astley
	"k6BTsnVcxS4", // Jahmal — Именно Ты
	"4t_knf7UVzY", // JAHMAL — Именно Ты
	"y5lBx1nbwos", // Jahmal TGK
	"OPf0YbXqDm0", // Mark Ronson
}

func TestProxyLikeBrowser(t *testing.T) {
	if os.Getenv("CRYON_PROXY_IT") != "1" {
		t.Skip("живая проверка сети: CRYON_PROXY_IT=1")
	}
	app := NewApp()
	app.ctx = context.Background()
	srv := httptest.NewServer(&localAssetHandler{app: app})
	defer srv.Close()
	client := &http.Client{Timeout: 90 * time.Second}

	fetch := func(id, rng string) (int, int) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/stream/youtube/"+id, nil)
		if rng != "" {
			req.Header.Set("Range", rng)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Logf("    req(%q) ERROR %v", rng, err)
			return 0, 0
		}
		defer resp.Body.Close()
		n, _ := io.Copy(io.Discard, resp.Body)
		return resp.StatusCode, int(n)
	}

	for _, id := range browserVideoIDs {
		t.Run(id, func(t *testing.T) {
			// 1) probe — браузер узнаёт длительность
			s1, n1 := fetch(id, "bytes=0-1")
			// 2) старт: читаем начало (первые ~2 МиБ), затем обрываем как при перемотке
			s2, n2 := fetch(id, "bytes=0-")
			// 3) перемотка в середину
			s3, n3 := fetch(id, "bytes=2000000-")
			// 4) хвост
			s4, n4 := fetch(id, "bytes=6000000-")
			t.Logf("  probe=%d/%dB start=%d/%dB seek=%d/%dB tail=%d/%dB",
				s1, n1, s2, n2, s3, n3, s4, n4)
			// Прокрутка начала должна отдать много байт; перемотка и хвост — непустые.
			if n2 == 0 {
				t.Errorf("старт не отдал данных (status=%d)", s2)
			}
			if n3 == 0 {
				t.Errorf("перемотка не отдала данных (status=%d)", s3)
			}
			if n4 == 0 {
				t.Errorf("хвост не отдал данных (status=%d)", s4)
			}
		})
	}
}
