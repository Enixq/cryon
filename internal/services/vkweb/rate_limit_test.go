package vkweb

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestWebsite429StopsRequestsAndRecovers(t *testing.T) {
	var calls atomic.Int32
	var blocked atomic.Bool
	blocked.Store(true)
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/audio" {
			fmt.Fprint(w, `{"uid":123}`)
			return
		}
		calls.Add(1)
		if blocked.Load() {
			w.Header().Set("Retry-After", "12")
			w.WriteHeader(429)
			return
		}
		writePayload(w, nil, map[string]any{"playlists": []any{}})
	})
	for i := 0; i < 4; i++ {
		_, err := s.Search(context.Background(), "Song")
		var limited *Error
		if !errors.As(err, &limited) || limited.HTTP != 429 || limited.RetryAfter <= 0 || limited.RetryAfter > 12*time.Second {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 || !s.HasToken() {
		t.Fatal("site limit caused duplicate requests or revoked login")
	}
	blocked.Store(false)
	s.mu.Lock()
	s.cooldownUntil = time.Time{}
	s.mu.Unlock()
	s.next = time.Time{}
	s.searchNext = time.Time{}
	if _, err := s.Search(context.Background(), "Song"); err != nil || calls.Load() != 2 {
		t.Fatal("could not recover", err)
	}
}
func TestConcurrentStreamRequestsDeduplicateAndIsolateAccounts(t *testing.T) {
	var calls atomic.Int32
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/audio" {
			fmt.Fprint(w, `{"uid":123}`)
			return
		}
		calls.Add(1)
		writePayload(w, []any{fixtureAudio(1, "https://cdn.test/file.mp3", 180, nil)})
	})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			target, _, err := s.ResolveStream(context.Background(), "site_-3_1_action_url")
			if err != nil || target != "https://cdn.test/file.mp3" {
				t.Error(target, err)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal("duplicate reloads", calls.Load())
	}
	s.mu.Lock()
	s.cooldownUntil = time.Now().Add(time.Minute)
	s.mu.Unlock()
	if _, _, err := s.ResolveStream(context.Background(), "site_-3_1_action_url"); err != nil {
		t.Fatal("cache lost during site cooldown", err)
	}
	s.SetSession(Session{Cookies: "remixsid=another"})
	if _, _, err := s.ResolveStream(context.Background(), "site_-3_1_action_url"); err == nil || calls.Load() != 1 {
		t.Fatal("previous account cache reused")
	}
}
func TestCancelledSearchDoesNotSendWebsiteRequest(t *testing.T) {
	var calls atomic.Int32
	s := testService(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); fmt.Fprint(w, `{"uid":123}`) })
	s.next = time.Now().Add(time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := s.Search(ctx, "Song")
	if !errors.Is(err, context.DeadlineExceeded) || calls.Load() != 0 {
		t.Fatal("cancelled search sent request", err)
	}
}
func TestAccountReplacementDiscardsInflightSearch(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/audio" {
			fmt.Fprint(w, `{"uid":123}`)
			return
		}
		close(started)
		<-release
		writePayload(w, nil, map[string]any{"playlists": []any{}})
	})
	result := make(chan error, 1)
	go func() { _, err := s.Search(context.Background(), "Song"); result <- err }()
	<-started
	s.SetSession(Session{Cookies: "remixsid=another"})
	close(release)
	if err := <-result; err == nil {
		t.Fatal("old account response returned after replacement")
	}
}
