package vkweb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/encoding/charmap"
)

func testService(t *testing.T, handler http.HandlerFunc) *Service {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	s := New()
	s.siteBase = server.URL
	s.SetSession(Session{Cookies: "p=fixture; remixsid=fixture", UserAgent: "test-browser"})
	s.http.Transport = server.Client().Transport
	s.http.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if strings.HasPrefix(r.URL.String(), server.URL+"/") && len(via) < 5 {
			return nil
		}
		return http.ErrUseLastResponse
	}
	return s
}
func fixtureAudio(id int, target string, duration int, flags any) []any {
	a := make([]any, 32)
	a[0] = id
	a[1] = -3
	a[2] = target
	a[3] = "Song &amp; title"
	a[4] = "Artist"
	a[5] = duration
	a[12] = flags
	a[13] = "a/b/action/d/e/url"
	a[14] = "//cdn.test/small.jpg,https://cdn.test/large.jpg"
	a[17] = []any{map[string]any{"name": "Основной"}}
	a[18] = []any{map[string]any{"name": "Приглашённый"}}
	return a
}
func writePayload(w http.ResponseWriter, inner ...any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	fmt.Fprint(w, "<!--")
	_ = json.NewEncoder(w).Encode(map[string]any{"payload": []any{0, inner}})
}
func TestWebsiteRedirectCookiesSearchReloadAndCache(t *testing.T) {
	searches, reloads := 0, 0
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "test-browser" {
			t.Error("lost browser user agent")
		}
		switch r.URL.Path {
		case "/audio":
			c, _ := r.Cookie("refreshed")
			if c == nil {
				http.SetCookie(w, &http.Cookie{Name: "refreshed", Value: "yes", Path: "/", Secure: true})
				http.Redirect(w, r, "/audio", 302)
				return
			}
			fmt.Fprint(w, `<script>window.vk = {"id":123}</script>`)
		case "/al_audio.php":
			if c, _ := r.Cookie("refreshed"); c == nil {
				t.Error("redirect cookies not retained")
			}
			if r.Method != "POST" || r.Header.Get("X-Requested-With") != "XMLHttpRequest" || !strings.HasSuffix(r.Header.Get("Referer"), "/audio") {
				t.Error("website request headers missing")
			}
			_ = r.ParseForm()
			if r.Form.Get("access_token") != "" {
				t.Error("obsolete API token sent")
			}
			switch r.Form.Get("act") {
			case "section":
				searches++
				if r.Form.Get("owner_id") != "123" || r.Form.Get("q") != "Song" {
					t.Error("search parameters missing")
				}
				data := map[string]any{"playlists": []any{
					map[string]any{"title": "Мои треки", "list": []any{fixtureAudio(10, "", 180, nil)}},
					map[string]any{"title": "Все треки", "list": []any{fixtureAudio(1, "", 180, nil), fixtureAudio(1, "", 180, nil), fixtureAudio(2, "", 30, nil), fixtureAudio(3, "", 180, map[string]any{"is_restricted": 1})}},
				}}
				encoded, _ := json.Marshal(map[string]any{"payload": []any{0, []any{nil, data}}})
				body, _ := charmap.Windows1251.NewEncoder().Bytes(encoded)
				w.Header().Set("Content-Type", "application/json; charset=windows-1251")
				_, _ = w.Write(append([]byte("<!--"), body...))
			case "reload_audio":
				reloads++
				if r.Form.Get("ids") != "-3_1_action_url" {
					t.Error("reload identifier lost")
				}
				writePayload(w, []any{fixtureAudio(1, "https://cdn.test/audio.mp3", 180, nil)})
			default:
				t.Error("unexpected website action")
			}
		default:
			t.Error("obsolete API or unexpected path")
		}
	})
	ctx := context.Background()
	if err := s.ValidateToken(ctx); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s.Session().Cookies, "refreshed=yes") {
		t.Fatal("updated session not exportable")
	}
	tracks, err := s.Search(ctx, "Song")
	if err != nil || len(tracks) != 1 {
		t.Fatal(tracks, err)
	}
	track := tracks[0]
	if track.ID != "site_-3_1_action_url" || track.Title != "Song & title" || strings.Join(track.Artists, "/") != "Основной/Приглашённый" || track.ArtworkURL != "https://cdn.test/large.jpg" {
		t.Fatal(track)
	}
	tracks[0].Artists[0] = "mutated"
	again, err := s.Search(ctx, "Song")
	if err != nil || again[0].Artists[0] != "Основной" || searches != 1 {
		t.Fatal("search cache not isolated", err)
	}
	target, mime, err := s.ResolveStream(ctx, track.ID)
	if err != nil || target != "https://cdn.test/audio.mp3" || mime != "audio/mpeg" {
		t.Fatal(target, mime, err)
	}
	_, _, err = s.ResolveStream(ctx, track.ID)
	if err != nil || reloads != 1 {
		t.Fatal("reload cache not reused", err)
	}
}
func TestSearchDirectStreamsAvoidReloadAndPreferMP3(t *testing.T) {
	searchCalls := 0
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/audio" {
			fmt.Fprint(w, `{"uid":123}`)
			return
		}
		_ = r.ParseForm()
		if r.Form.Get("act") != "section" {
			t.Error("direct stream unnecessarily reloaded")
			return
		}
		searchCalls++
		flags := map[string]any{"audio_streams": []any{map[string]any{"type": "hls", "url": "https://cdn.test/no-extension"}, map[string]any{"type": "mp3", "url": "https://cdn.test/file.mp3"}}}
		writePayload(w, nil, map[string]any{"playlist": map[string]any{"list": []any{fixtureAudio(1, "", 180, flags)}}})
	})
	tracks, err := s.Search(context.Background(), "Song")
	if err != nil || len(tracks) != 1 {
		t.Fatal(tracks, err)
	}
	target, mime, err := s.ResolveStream(context.Background(), tracks[0].ID)
	if err != nil || target != "https://cdn.test/file.mp3" || mime != "audio/mpeg" || searchCalls != 1 {
		t.Fatal(target, mime, err)
	}
}
func TestWebsiteErrorsWithoutSecrets(t *testing.T) {
	for _, test := range []struct {
		body   string
		status int
		kind   string
		code   int
	}{
		{`private-token<html>login</html>`, 200, "json", 0},
		{`{"payload":[3,["private-token"]]}`, 200, "session", 3},
		{`{"payload":[8,["private-token"]]}`, 200, "", 8},
		{`{"payload":["bogus",[]]}`, 200, "json", 0},
		{"private-token", 503, "", 0},
	} {
		t.Run(fmt.Sprint(test.status, test.kind, test.code), func(t *testing.T) {
			s := testService(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/audio" {
					fmt.Fprint(w, `{"uid":123}`)
					return
				}
				w.WriteHeader(test.status)
				fmt.Fprint(w, test.body)
			})
			_, err := s.Search(context.Background(), "Song")
			var known *Error
			if !errors.As(err, &known) || known.Stage != "section" || known.Kind != test.kind || known.Code != test.code || strings.Contains(err.Error(), "private-token") {
				t.Fatal(err)
			}
		})
	}
}
func TestDisconnectedLegacyAndEmptySearch(t *testing.T) {
	s := New()
	tracks, err := s.Search(context.Background(), "Song")
	if err != nil || len(tracks) != 0 {
		t.Fatal(tracks, err)
	}
	for _, id := range []string{"api_1_2", "web_1_2", "site_1_2_bad!_hash"} {
		_, _, err = s.ResolveStream(context.Background(), id)
		var known *Error
		if !errors.As(err, &known) || known.Kind != "legacy" {
			t.Fatal(err)
		}
	}
	calls := 0
	s = testService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/audio" {
			fmt.Fprint(w, `{"uid":123}`)
			return
		}
		calls++
		writePayload(w, nil, map[string]any{"playlists": []any{map[string]any{"title": "Все треки", "list": []any{}}}})
	})
	for i := 0; i < 2; i++ {
		tracks, err = s.Search(context.Background(), "missing")
		if err != nil || len(tracks) != 0 {
			t.Fatal(tracks, err)
		}
	}
	if calls != 1 || time.Until(s.searches["missing"].expires) > 31*time.Second {
		t.Fatal("empty search not briefly cached")
	}
}
func TestRedirectScope(t *testing.T) {
	for _, target := range []string{"https://login.vk.ru/", "https://vk.com/login", "https://evil.test/", "https://vk.ru.evil.test/", "http://vk.ru/", "https://user@vk.ru/"} {
		r, _ := http.NewRequest("GET", target, nil)
		allowed := target == "https://login.vk.ru/" || target == "https://vk.com/login"
		if (allowedRedirect(r, nil) == nil) != allowed {
			t.Fatal(target)
		}
	}
}
func TestSessionIsolationAndEncryption(t *testing.T) {
	first := Session{Cookies: "p=first; remixsid=first", UserAgent: "browser"}
	encoded, err := EncodeSession(first)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeSession(encoded)
	if err != nil || decoded != first {
		t.Fatal("session roundtrip failed")
	}
	s := New()
	s.SetSession(first)
	sig := s.SignMedia("site_1_2_a_b", "https://cdn.test/key")
	if !s.VerifyMedia("site_1_2_a_b", "https://cdn.test/key", sig) || s.VerifyMedia("site_1_3_a_b", "https://cdn.test/key", sig) {
		t.Fatal("signature not bound to track")
	}
	s.SetSession(Session{Cookies: "p=second; remixsid=second"})
	if s.VerifyMedia("site_1_2_a_b", "https://cdn.test/key", sig) {
		t.Fatal("previous account media restored")
	}
	bad := first
	bad.Cookies += "\r\nInjected: secret"
	if bad.Valid() {
		t.Fatal("header injection accepted")
	}
	if _, err = DecodeSession("not-a-session"); err == nil {
		t.Fatal("invalid session accepted")
	}
	if !(Session{Cookies: "remixsid=only"}).Valid() {
		t.Fatal("website sid incorrectly requires API exchange cookie")
	}
	cookies := FormatCookies([]BrowserCookie{{Name: "p", Value: "foreign", Domain: "evil.test"}, {Name: "p", Value: "old", Domain: ".vk.com"}, {Name: "p", Value: "current", Domain: ".vk.ru"}, {Name: "remixsid", Value: "id", Domain: ".vk.ru"}})
	if cookies != "p=current; remixsid=id" {
		t.Fatal(cookies)
	}
}

func TestAndroidSessionUsesDesktopPlayerIdentity(t *testing.T) {
	const mobileUA = "Mozilla/5.0 (Linux; Android 16; HONOR; wv) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/157.0.8103.50 Mobile Safari/537.36"
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		ua := r.Header.Get("User-Agent")
		if strings.Contains(ua, "Mobile") || strings.Contains(ua, "Android") || !strings.Contains(ua, "Chrome/157.0.8103.50") {
			fmt.Fprint(w, `<html>mobile login page</html>`)
			return
		}
		if r.URL.Path == "/audio" {
			fmt.Fprint(w, `{"uid":123}`)
			return
		}
		writePayload(w, nil, map[string]any{"playlists": []any{}})
	})
	transport, redirect := s.http.Transport, s.http.CheckRedirect
	s.SetSession(Session{Cookies: "remixsid=fixture; remixnsid=mobile-session", UserAgent: mobileUA})
	s.http.Transport, s.http.CheckRedirect = transport, redirect
	if err := s.ValidateToken(context.Background()); err != nil {
		t.Fatal("Android identity reached the mobile login page", err)
	}
	if _, err := s.Search(context.Background(), "Song"); err != nil {
		t.Fatal(err)
	}
	if ua := s.Session().UserAgent; ua == mobileUA || !strings.Contains(ua, "Windows NT") || !strings.Contains(ua, "Chrome/157.0.8103.50") {
		t.Fatal("normalised browser identity not persisted")
	}
	if websiteUserAgent("desktop-browser") != "desktop-browser" {
		t.Fatal("existing desktop identity changed")
	}
	bad := Session{Cookies: "remixsid=fixture", UserAgent: mobileUA + "\r\nInjected: secret"}
	s.SetSession(bad)
	if s.HasToken() {
		t.Fatal("normalisation accepted a header injection")
	}
}
