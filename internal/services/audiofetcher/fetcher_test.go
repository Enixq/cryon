package audiofetcher

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kkdai/youtube/v2"
)

// parseTestRange разбирает Range вида «bytes=start-end» и ограничивает его
// размером виртуального файла — как это делает корректный HTTP-сервер. Проверка
// резолвера намеренно не зависит от точного размера контрольного блока.
func parseTestRange(header string, total int64) (int64, int64, bool) {
	if !strings.HasPrefix(header, "bytes=") {
		return 0, 0, false
	}
	startText, endText, found := strings.Cut(strings.TrimPrefix(header, "bytes="), "-")
	if !found || startText == "" {
		return 0, 0, false
	}
	start, err := strconv.ParseInt(startText, 10, 64)
	if err != nil || start < 0 || start >= total {
		return 0, 0, false
	}
	end := total - 1
	if endText != "" {
		if parsed, err := strconv.ParseInt(endText, 10, 64); err == nil && parsed < end {
			end = parsed
		}
	}
	return start, end, true
}

// serveTestRange отвечает на Range-запрос по виртуальному файлу размером total.
func serveTestRange(w http.ResponseWriter, r *http.Request, total int64) {
	start, end, ok := parseTestRange(r.Header.Get("Range"), total)
	if !ok {
		http.Error(w, "некорректный Range", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, total))
	w.WriteHeader(http.StatusPartialContent)
	_, _ = w.Write([]byte(strings.Repeat("x", int(end-start+1))))
}

func TestRaceAudioStreamsReturnsWorkingResolverWithoutWaitingForFailure(t *testing.T) {
	started := time.Now()
	stream, err := raceAudioStreams(context.Background(), []audioStreamAttempt{
		{
			name:    "blocked",
			timeout: time.Second,
			resolve: func(ctx context.Context) (*AudioStream, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			},
		},
		{
			name:    "working",
			timeout: time.Second,
			resolve: func(context.Context) (*AudioStream, error) {
				return &AudioStream{URL: "https://audio.example/stream"}, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("raceAudioStreams: %v", err)
	}
	if stream.URL == "" {
		t.Fatal("working stream was not returned")
	}
	if time.Since(started) > 200*time.Millisecond {
		t.Fatal("race waited for the blocked resolver")
	}
}

func TestSelectYouTubeAudioFormatTriesAnotherWhenFirstLacksTail(t *testing.T) {
	const total = 2097152
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// opus отдаётся только в первых 1 МиБ — имитация «превью»-ограничения
		// googlevideo, из-за которого трек начинался и замолкал на середине.
		if r.URL.Path == "/opus" {
			start, _, ok := parseTestRange(r.Header.Get("Range"), total)
			if !ok || start >= 1<<20 {
				w.WriteHeader(http.StatusForbidden)
				return
			}
		}
		serveTestRange(w, r, total)
	}))
	defer server.Close()
	formats := []youtube.Format{
		{ItagNo: 140, Bitrate: 128000, MimeType: "audio/mp4"},
		{ItagNo: 251, Bitrate: 160000, MimeType: "audio/webm"},
	}
	var tried []int
	stream, err := selectYouTubeAudioFormat(context.Background(), formats, func(ctx context.Context, format *youtube.Format) (*AudioStream, error) {
		tried = append(tried, format.ItagNo)
		if format.ItagNo == 251 {
			return &AudioStream{URL: server.URL + "/opus"}, nil
		}
		return &AudioStream{URL: server.URL + "/aac"}, nil
	})
	if err != nil || stream == nil || !strings.HasSuffix(stream.URL, "/aac") || len(tried) != 2 || tried[0] != 251 || tried[1] != 140 {
		t.Fatalf("stream=%+v tried=%v err=%v", stream, tried, err)
	}
}

func TestVerifiedResolverSkipsForbiddenAndEmptyStreams(t *testing.T) {
	const total = 2097152
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/forbidden":
			w.WriteHeader(http.StatusForbidden)
		case "/empty":
			w.Header().Set("Content-Type", "audio/mpeg")
		case "/audio":
			serveTestRange(w, r, total)
		case "/prefix":
			// Источник отдаёт только начало файла — перемотка невозможна.
			start, _, ok := parseTestRange(r.Header.Get("Range"), total)
			if !ok || start != 0 {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			serveTestRange(w, r, total)
		}
	}))
	defer server.Close()
	resolve := func(path string) func(context.Context) (*AudioStream, error) {
		return verifyResolver(func(context.Context) (*AudioStream, error) { return &AudioStream{URL: server.URL + path}, nil })
	}
	stream, err := raceAudioStreams(context.Background(), []audioStreamAttempt{
		{name: "forbidden", timeout: time.Second, resolve: resolve("/forbidden")},
		{name: "empty", timeout: time.Second, resolve: resolve("/empty")},
		{name: "prefix", timeout: time.Second, resolve: resolve("/prefix")},
		{name: "working", timeout: time.Second, resolve: resolve("/audio")},
	})
	if err != nil || stream == nil || !strings.HasSuffix(stream.URL, "/audio") {
		t.Fatalf("working stream not selected: %+v, %v", stream, err)
	}
}

// TestVerifiedResolverToleratesTransientTailFailure — googlevideo отвечает 503
// на дальний чанк при перегрузке/лимитировании. Раньше это отбраковывало
// заведомо рабочий поток, и трек «не воспроизводился» хотя ссылка была годной.
// Теперь transient-ошибка хвоста не убивает резолвер.
func TestVerifiedResolverToleratesTransientTailFailure(t *testing.T) {
	const total = 8 << 20
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start, _, ok := parseTestRange(r.Header.Get("Range"), total)
		if !ok {
			http.Error(w, "bad range", http.StatusBadRequest)
			return
		}
		if start >= total-2*streamProbeBytes {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		serveTestRange(w, r, total)
	}))
	defer server.Close()

	stream, err := verifyResolver(func(context.Context) (*AudioStream, error) {
		return &AudioStream{URL: server.URL}, nil
	})(context.Background())
	if err != nil {
		t.Fatalf("transient 503 на хвосте отбраковал рабочий поток: %v", err)
	}
	if stream == nil || stream.URL == "" {
		t.Fatal("поток не возвращён")
	}
}

// TestVerifiedResolverRejectsForbiddenTail — 403 на хвосте означает, что
// источник отдаёт только начало файла (классика ANDROID_VR-ссылок до 1 МиБ).
// Такой поток обязан быть отбракован, иначе трек замолкнет при перемотке.
func TestVerifiedResolverRejectsForbiddenTail(t *testing.T) {
	const total = 8 << 20
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start, _, ok := parseTestRange(r.Header.Get("Range"), total)
		if !ok {
			http.Error(w, "bad range", http.StatusBadRequest)
			return
		}
		if start >= 1<<20 {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		serveTestRange(w, r, total)
	}))
	defer server.Close()

	if _, err := verifyResolver(func(context.Context) (*AudioStream, error) {
		return &AudioStream{URL: server.URL}, nil
	})(context.Background()); err == nil {
		t.Fatal("поток, обрывающийся на 1 МиБ, принят как рабочий")
	}
}

func TestRaceAudioStreamsReturnsAttemptErrors(t *testing.T) {
	_, err := raceAudioStreams(context.Background(), []audioStreamAttempt{
		{name: "first", timeout: time.Second, resolve: func(context.Context) (*AudioStream, error) { return nil, errors.New("first failure") }},
		{name: "second", timeout: time.Second, resolve: func(context.Context) (*AudioStream, error) { return nil, errors.New("second failure") }},
	})
	if err == nil {
		t.Fatal("expected resolver error")
	}
}

// TestPickBestInnerTubeProgressiveSelectsItag18 — ANDROID InnerTube отдаёт
// прогрессивный формат (video+audio) с прямым URL; аудио-only форматы
// шифрованы sigCipher (URL пустой). pickBestInnerTubeProgressive должен выбрать
// itag=18.
func TestPickBestInnerTubeProgressiveSelectsItag18(t *testing.T) {
	formats := []innertubeFormat{
		{URL: "https://example/itag18", MimeType: `video/mp4; codecs="avc1.42001E, mp4a.40.2"`, Bitrate: 500000},
		{URL: "", MimeType: `video/mp4; codecs="avc1.42001E, mp4a.40.2"`, Bitrate: 800000}, // без URL — sigCipher
	}
	best := pickBestInnerTubeProgressive(formats)
	if best == nil {
		t.Fatal("pickBestInnerTubeProgressive вернул nil")
	}
	if best.URL != "https://example/itag18" {
		t.Fatalf("itag=%q, want itag18", best.URL)
	}
}

// TestPickBestInnerTubeAudioSkipsEncrypted — аудио-only форматы ANDROID имеют
// пустой URL (sigCipher). pickBestInnerTubeAudio должен их пропустить.
func TestPickBestInnerTubeAudioSkipsEncrypted(t *testing.T) {
	formats := []innertubeFormat{
		{URL: "", MimeType: `audio/mp4; codecs="mp4a.40.2"`, Bitrate: 128000},
		{URL: "", MimeType: `audio/webm; codecs="opus"`, Bitrate: 160000},
	}
	if best := pickBestInnerTubeAudio(formats); best != nil {
		t.Fatalf("nil ожидался, получен %+v", best)
	}
}
