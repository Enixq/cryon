package core

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// cleanupStreamFile убирает скачанный файл и сайдкарон из кэша после теста.
func cleanupStreamFile(service, id string) {
	key := service + "\x00" + id
	streamFileCache.Lock()
	if hit, ok := streamFileCache.m[key]; ok {
		_ = os.Remove(hit.path)
		_ = os.Remove(contentTypeSidecar(hit.path))
		delete(streamFileCache.m, key)
	}
	streamFileCache.Unlock()
}

// TestStreamProxyRetriesTemporaryFailure — transient 503 на первой попытке
// скачивания не должен убивать трек: вторая попытка той же ссылкой докачивает,
// а клиент получает корректный Range из полного локального файла.
func TestStreamProxyRetriesTemporaryFailure(t *testing.T) {
	attempts := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("audio"))
	}))
	defer upstream.Close()

	const service, id = "youtube", "retry-test"
	streamURLCache.Lock()
	streamURLCache.m[service+"\x00"+id] = cachedStreamURL{url: upstream.URL, at: time.Now()}
	streamURLCache.Unlock()
	defer func() {
		invalidateStreamURL(service, id)
		cleanupStreamFile(service, id)
	}()

	req := httptest.NewRequest(http.MethodGet, "/stream/"+service+"/"+id, nil)
	req.Header.Set("Range", "bytes=0-")
	response := httptest.NewRecorder()
	(&localAssetHandler{}).serveRemoteStream(response, req)

	if response.Code != http.StatusPartialContent || response.Body.String() != "audio" {
		t.Fatalf("status=%d body=%q attempts=%d", response.Code, response.Body.String(), attempts)
	}
	if attempts < 2 {
		t.Fatalf("ретрай transient не сработал: attempts=%d", attempts)
	}
}

// TestStreamProxyServesRangeFromCache — скачанный файл отдаёт перемотку нативно
// (http.ServeContent), без повторного обращения к источнику.
func TestStreamProxyServesRangeFromCache(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "audio/mpeg")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ABCDEFGHIJ"))
	}))
	defer upstream.Close()

	const service, id = "youtube", "range-cache"
	streamURLCache.Lock()
	streamURLCache.m[service+"\x00"+id] = cachedStreamURL{url: upstream.URL, at: time.Now()}
	streamURLCache.Unlock()
	defer func() {
		invalidateStreamURL(service, id)
		cleanupStreamFile(service, id)
	}()

	r1 := httptest.NewRequest(http.MethodGet, "/stream/"+service+"/"+id, nil)
	r1.Header.Set("Range", "bytes=0-")
	rec1 := httptest.NewRecorder()
	(&localAssetHandler{}).serveRemoteStream(rec1, r1)
	if !strings.HasPrefix(rec1.Body.String(), "ABCDEFGHIJ") {
		t.Fatalf("полный поток не отдан: body=%q", rec1.Body.String())
	}

	r2 := httptest.NewRequest(http.MethodGet, "/stream/"+service+"/"+id, nil)
	r2.Header.Set("Range", "bytes=3-5")
	rec2 := httptest.NewRecorder()
	(&localAssetHandler{}).serveRemoteStream(rec2, r2)
	if rec2.Code != http.StatusPartialContent || rec2.Body.String() != "DEF" {
		t.Fatalf("перемотка: status=%d body=%q", rec2.Code, rec2.Body.String())
	}
	if calls != 1 {
		t.Fatalf("источник дёрнут %d раз(а), ожидался 1 (второй — из кэша)", calls)
	}
}
