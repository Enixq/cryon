package soundcloud

import (
	"context"
	"os"
	"testing"
	"time"

	"Cryon2/internal/domain"
)

// TestLiveSearch — интеграционная проверка живого поиска SoundCloud через
// api-v2 с автоопределением client_id. Ходит в сеть, поэтому включается только
// при CRYON_SC_IT=1 (в CI/оффлайне пропускается).
func TestLiveSearch(t *testing.T) {
	if os.Getenv("CRYON_SC_IT") != "1" {
		t.Skip("integration: set CRYON_SC_IT=1 to run")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	svc := New("") // без ручного client_id — проверяем автоопределение
	tracks, err := svc.Search(ctx, "Jahmal TGK")
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(tracks) == 0 {
		t.Fatalf("ожидались треки из SoundCloud, получено 0")
	}
	t.Logf("SoundCloud нашёл %d треков; первый: %q — %v", len(tracks), tracks[0].Title, tracks[0].Artists)
	for i, tr := range tracks {
		if i >= 5 {
			break
		}
		t.Logf("  %d. %q — %v (id=%s)", i+1, tr.Title, tr.Artists, tr.ID)
	}
}

// TestLiveStaleClientID — проверяет самовосстановление: с заведомо неверным
// ручным client_id поиск должен переоткрыть свежий ключ и всё равно вернуть
// треки, а не молча провалиться в пустой Bing-фолбэк.
func TestLiveStaleClientID(t *testing.T) {
	if os.Getenv("CRYON_SC_IT") != "1" {
		t.Skip("integration: set CRYON_SC_IT=1 to run")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	svc := New("0000000000000000000000000000dead") // невалидный, но правдоподобный
	tracks, err := svc.Search(ctx, "Jahmal TGK")
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(tracks) == 0 {
		t.Fatalf("ожидалось самовосстановление client_id и непустой результат")
	}
	t.Logf("после отказа устаревшего client_id SoundCloud нашёл %d треков", len(tracks))
}

// TestLiveResolveAlbum — интеграционная проверка резолва трек-листа альбома
// на SoundCloud (используется агрегатором как приоритетный, нецензурный
// источник). Включается только при CRYON_SC_IT=1.
func TestLiveResolveAlbum(t *testing.T) {
	if os.Getenv("CRYON_SC_IT") != "1" {
		t.Skip("integration: set CRYON_SC_IT=1 to run")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	svc := New("")
	tracks, err := svc.ResolveAlbum(ctx, "Jahmal TGK", "БУДНИ")
	if err != nil {
		t.Fatalf("ResolveAlbum error: %v", err)
	}
	if len(tracks) < 3 {
		t.Fatalf("ожидался трек-лист альбома, получено %d треков", len(tracks))
	}
	for _, tr := range tracks {
		if tr.ID == "" || tr.Title == "" || tr.PlayableKind != domain.PlayableStream {
			t.Fatalf("битый трек альбома: %+v", tr)
		}
	}
	t.Logf("SoundCloud вернул %d треков альбома; первый: %q — %v", len(tracks), tracks[0].Title, tracks[0].Artists)
}
