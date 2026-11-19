package core

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestYouTubeProxyCombinesSegmentsAndSupportsSeeking(t *testing.T) {
	data := bytes.Repeat([]byte("audio-data"), 70000)
	var requests atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "audio/webm")
		w.Header().Set("Accept-Ranges", "bytes")
		http.ServeContent(w, r, "audio.webm", time.Time{}, bytes.NewReader(data))
	}))
	defer upstream.Close()
	const id = "segments-test"
	streamURLCache.Lock()
	streamURLCache.m["youtube\x00"+id] = cachedStreamURL{url: upstream.URL, at: time.Now()}
	streamURLCache.Unlock()
	defer invalidateStreamURL("youtube", id)
	defer cleanupStreamFile("youtube", id)
	server := httptest.NewServer(&localAssetHandler{app: &App{}})
	defer server.Close()
	for _, interval := range []struct{ start, end int }{{0, -1}, {12345, -1}, {12345, 400000}} {
		start, end := interval.start, interval.end
		req, err := http.NewRequest(http.MethodGet, server.URL+"/stream/youtube/"+id, nil)
		if err != nil {
			t.Fatal(err)
		}
		if start > 0 {
			if end < 0 {
				req.Header.Set("Range", fmt.Sprintf("bytes=%d-", start))
			} else {
				req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
			}
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		last := len(data) - 1
		if end >= 0 && end < last {
			last = end
		}
		if err != nil || !bytes.Equal(body, data[start:last+1]) {
			t.Fatalf("start=%d bytes=%d err=%v", start, len(body), err)
		}
		if start == 0 && resp.StatusCode != http.StatusOK {
			t.Fatalf("full status=%d", resp.StatusCode)
		}
		if start > 0 && (resp.StatusCode != http.StatusPartialContent || resp.Header.Get("Content-Range") != fmt.Sprintf("bytes %d-%d/%d", start, last, len(data))) {
			t.Fatalf("seek status=%d range=%q", resp.StatusCode, resp.Header.Get("Content-Range"))
		}
	}
	if requests.Load() != 1 {
		t.Fatalf("ожидался 1 upstream-запрос (скачать целиком), получен %d", requests.Load())
	}
}
