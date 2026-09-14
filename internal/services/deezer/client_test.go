package deezer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// newTestClient направляет клиент на локальный httptest-сервер, чтобы проверять
// разбор ответов без сети.
func newTestClient(base string) *Client {
	return &Client{httpClient: &http.Client{Timeout: 5 * time.Second}, baseURL: base}
}

// TestSimilarArtistsParsing — оффлайн: /search/artist → /artist/{id}/related,
// синтез Match из ранга (верхний ≈1.0, дальше убывает).
func TestSimilarArtistsParsing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/search/artist"):
			_, _ = w.Write([]byte(`{"data":[{"id":27,"name":"Daft Punk"}]}`))
		case r.URL.Path == "/artist/27/related":
			_, _ = w.Write([]byte(`{"data":[{"id":1,"name":"Justice"},{"id":2,"name":"Cassius"},{"id":3,"name":""}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	got, err := c.SimilarArtists(context.Background(), "Daft Punk", 10)
	if err != nil {
		t.Fatalf("SimilarArtists: %v", err)
	}
	// Пустое имя отфильтровано → 2 артиста.
	if len(got) != 2 {
		t.Fatalf("ожидали 2 артистов (пустое имя отсеяно), got %d: %+v", len(got), got)
	}
	if got[0].Name != "Justice" || got[1].Name != "Cassius" {
		t.Fatalf("неожиданный порядок/имена: %+v", got)
	}
	// Match убывает по рангу и лежит в (0..1].
	if !(got[0].Match > got[1].Match) {
		t.Fatalf("Match должен убывать по рангу: %v затем %v", got[0].Match, got[1].Match)
	}
	if got[0].Match <= 0 || got[0].Match > 1 {
		t.Fatalf("Match вне (0..1]: %v", got[0].Match)
	}
}

// TestTopArtistTracksParsing — оффлайн: /search/artist → /artist/{id}/top,
// подстановка имени артиста в трек и фолбэк на имя-затравку при пустом.
func TestTopArtistTracksParsing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/search/artist"):
			_, _ = w.Write([]byte(`{"data":[{"id":42,"name":"Radiohead"}]}`))
		case r.URL.Path == "/artist/42/top":
			_, _ = w.Write([]byte(`{"data":[{"title":"Creep","artist":{"name":"Radiohead"}},{"title":"No Surprises","artist":{"name":""}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	got, err := c.TopArtistTracks(context.Background(), "Radiohead", 10)
	if err != nil {
		t.Fatalf("TopArtistTracks: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ожидали 2 трека, got %d: %+v", len(got), got)
	}
	if got[0].Title != "Creep" || got[0].Artist != "Radiohead" {
		t.Fatalf("первый трек неверный: %+v", got[0])
	}
	// Пустое имя артиста в ответе → подставляется имя-затравка.
	if got[1].Artist != "Radiohead" {
		t.Fatalf("ожидали фолбэк имени артиста, got %q", got[1].Artist)
	}
}

// TestChartTracksParsing — оффлайн: /chart, вложенный tracks.data.
func TestChartTracksParsing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/chart" {
			_, _ = w.Write([]byte(`{"tracks":{"data":[{"title":"Hit","artist":{"name":"Star"}}]},"artists":{"data":[{"id":7,"name":"Star"}]}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	tracks, err := c.ChartTracks(context.Background(), 10)
	if err != nil {
		t.Fatalf("ChartTracks: %v", err)
	}
	if len(tracks) != 1 || tracks[0].Title != "Hit" || tracks[0].Artist != "Star" {
		t.Fatalf("неверный разбор чарта: %+v", tracks)
	}
	artists, err := c.ChartArtists(context.Background(), 10)
	if err != nil {
		t.Fatalf("ChartArtists: %v", err)
	}
	if len(artists) != 1 || artists[0].Name != "Star" {
		t.Fatalf("неверный разбор чарта артистов: %+v", artists)
	}
}

// TestDeezerErrorObject — Deezer отвечает 200 OK с top-level "error"; клиент
// должен вернуть ошибку, а не молча пустой результат.
func TestDeezerErrorObject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"error":{"type":"Exception","message":"Quota limit exceeded","code":4}}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	if _, err := c.SimilarArtists(context.Background(), "Whoever", 5); err == nil {
		t.Fatal("ожидали ошибку при top-level error, got nil")
	}
}

// TestUnknownArtistEmpty — /search/artist без результатов → пустой список без
// ошибки (не паникуем, как и остальные адаптеры).
func TestUnknownArtistEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	got, err := c.SimilarArtists(context.Background(), "Nonexistent Artist ZZZ", 5)
	if err != nil {
		t.Fatalf("неизвестный артист не должен давать ошибку: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("ожидали пусто, got %+v", got)
	}
}

func TestRankMatch(t *testing.T) {
	if m := rankMatch(0, 1); m != 1.0 {
		t.Fatalf("единственный элемент → 1.0, got %v", m)
	}
	if m := rankMatch(0, 10); m <= rankMatch(9, 10) {
		t.Fatalf("верхний ранг должен быть больше нижнего: %v vs %v", m, rankMatch(9, 10))
	}
	if m := rankMatch(0, 10); m <= 0 || m > 1 {
		t.Fatalf("Match вне (0..1]: %v", m)
	}
}

// TestSimilarArtistsLive — интеграционная проверка живого API Deezer. Ходит в
// сеть, поэтому включается только при CRYON_DEEZER_IT=1 (в CI/оффлайне
// пропускается), как youtube/soundcloud live-тесты.
func TestSimilarArtistsLive(t *testing.T) {
	if os.Getenv("CRYON_DEEZER_IT") != "1" {
		t.Skip("integration: set CRYON_DEEZER_IT=1 to run")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	c := New()
	similar, err := c.SimilarArtists(ctx, "Daft Punk", 10)
	if err != nil {
		t.Fatalf("SimilarArtists: %v", err)
	}
	if len(similar) == 0 {
		t.Fatal("ожидали непустой список похожих артистов")
	}
	t.Logf("похожих на Daft Punk: %d; первый: %q (match %.3f)", len(similar), similar[0].Name, similar[0].Match)

	top, err := c.TopArtistTracks(ctx, "Daft Punk", 5)
	if err != nil {
		t.Fatalf("TopArtistTracks: %v", err)
	}
	if len(top) == 0 {
		t.Fatal("ожидали непустой топ треков")
	}
	t.Logf("топ-треков: %d; первый: %q — %q", len(top), top[0].Title, top[0].Artist)
}
