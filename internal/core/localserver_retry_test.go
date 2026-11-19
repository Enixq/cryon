package core

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestStreamProxyRetriesTemporaryFailureAndKeepsRange(t *testing.T) {
	attempts := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if r.Header.Get("Range") != "bytes=42-262185" {
			t.Errorf("range lost on attempt %d: %q", attempts, r.Header.Get("Range"))
		}
		if attempts == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Range", "bytes 42-46/47")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("audio"))
	}))
	defer upstream.Close()
	const service, id = "youtube", "retry-test"
	streamURLCache.Lock()
	streamURLCache.m[service+"\x00"+id] = cachedStreamURL{url: upstream.URL, at: time.Now()}
	streamURLCache.Unlock()
	defer invalidateStreamURL(service, id)
	req := httptest.NewRequest(http.MethodGet, "/stream/"+service+"/"+id, nil)
	req.Header.Set("Range", "bytes=42-")
	response := httptest.NewRecorder()
	(&localAssetHandler{}).serveRemoteStream(response, req)
	if response.Code != http.StatusPartialContent || response.Body.String() != "audio" || attempts != 2 {
		t.Fatalf("status=%d body=%q attempts=%d", response.Code, response.Body.String(), attempts)
	}
}
