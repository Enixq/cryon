package core

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"Cryon2/internal/services/audiofetcher"
)

func TestLiveYouTubeFullStreamAndSeek(t *testing.T) {
	if os.Getenv("CRYON_YOUTUBE_LIVE_IT") != "1" {
		t.Skip("live playback test is disabled")
	}
	app := &App{ctx: context.Background()}
	server := httptest.NewServer(&localAssetHandler{app: app})
	defer server.Close()
	videoIDs := []string{"dQw4w9WgXcQ", "9bZkp7q19f0", "kJQP7kiw5Fk"}
	if selected := os.Getenv("CRYON_YOUTUBE_VIDEO_ID"); selected != "" {
		videoIDs = []string{selected}
	}
	for _, videoID := range videoIDs {
		t.Run(videoID, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			stream, err := audiofetcher.GetYouTubeAudioStream(ctx, videoID)
			if err != nil {
				t.Fatal(err)
			}
			streamURLCache.Lock()
			streamURLCache.m["youtube\x00"+videoID] = cachedStreamURL{url: stream.URL, at: time.Now()}
			streamURLCache.Unlock()
			defer invalidateStreamURL("youtube", videoID)
			localURL := server.URL + "/stream/youtube/" + videoID
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, localURL, nil)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			count, err := io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if err != nil || count != resp.ContentLength {
				t.Fatalf("stream incomplete: received=%d expected=%d error=%v", count, resp.ContentLength, err)
			}
			req, err = http.NewRequestWithContext(ctx, http.MethodGet, localURL, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Range", "bytes=1048576-1049599")
			resp, err = http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			count, err = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if err != nil || resp.StatusCode != http.StatusPartialContent || count != 1024 {
				t.Fatalf("seek failed: status=%d received=%d error=%v", resp.StatusCode, count, err)
			}
		})
	}
}
