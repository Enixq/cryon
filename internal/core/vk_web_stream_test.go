package core

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"Cryon2/internal/domain"
	"Cryon2/internal/services/vkweb"
)

type vkMediaTransport func(*http.Request) (*http.Response, error)

func (f vkMediaTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestVKProxyPreservesEncryptedHLSAndRanges(t *testing.T) {
	key := []byte("0123456789abcdef")
	plain := []byte("audio-frame-test")
	encrypted := make([]byte, len(plain))
	block, _ := aes.NewCipher(key)
	cipher.NewCBCEncrypter(block, make([]byte, 16)).CryptBlocks(encrypted, plain)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "" {
			t.Error("website cookies leaked to CDN")
		}
		switch r.URL.Path {
		case "/playlist":
			_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-KEY:METHOD=AES-128,URI=\"key\",IV=0x00000000000000000000000000000000\n#EXTINF:3,\nsegment\n#EXT-X-ENDLIST\n"))
		case "/key":
			_, _ = w.Write(key)
		case "/segment":
			if r.Header.Get("Range") == "bytes=0-15" {
				w.Header().Set("Content-Range", "bytes 0-15/16")
				w.WriteHeader(206)
			}
			_, _ = w.Write(encrypted)
		}
	}))
	defer upstream.Close()
	base, _ := url.Parse(upstream.URL)
	old := streamDownloadClient
	defer func() { streamDownloadClient = old }()
	streamDownloadClient = &http.Client{Transport: vkMediaTransport(func(r *http.Request) (*http.Response, error) {
		clone := r.Clone(r.Context())
		clone.URL.Host = base.Host
		clone.URL.Scheme = "http"
		return http.DefaultTransport.RoundTrip(clone)
	})}
	provider := vkweb.New()
	provider.SetSession(vkweb.Session{Cookies: "p=fixture; remixsid=fixture"})
	app := &App{registry: map[domain.ServiceID]domain.MusicService{domain.ServiceVK: provider}}
	id := "web_1_2"
	target := "https://cdn.test/playlist"
	path := vkStreamPath + "?" + url.Values{"id": {id}, "u": {base64.RawURLEncoding.EncodeToString([]byte(target))}, "sig": {provider.SignMedia(id, target)}}.Encode()
	rec := httptest.NewRecorder()
	app.handleVKWebStream(rec, httptest.NewRequest("GET", path, nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "METHOD=AES-128") {
		t.Fatal(rec.Code, rec.Body.String())
	}
	lines := strings.Split(rec.Body.String(), "\n")
	matches := playlistURI.FindStringSubmatch(lines[1])
	if len(matches) != 2 {
		t.Fatal(lines)
	}
	keyRec := httptest.NewRecorder()
	app.handleVKWebStream(keyRec, httptest.NewRequest("GET", matches[1], nil))
	if keyRec.Code != 200 || keyRec.Body.String() != string(key) {
		t.Fatal(keyRec.Code)
	}
	segRec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", lines[3], nil)
	req.Header.Set("Range", "bytes=0-15")
	app.handleVKWebStream(segRec, req)
	if segRec.Code != 206 || segRec.Body.String() != string(encrypted) || segRec.Header().Get("Content-Range") == "" {
		t.Fatal(segRec.Code)
	}
	decoded := make([]byte, 16)
	cipher.NewCBCDecrypter(block, make([]byte, 16)).CryptBlocks(decoded, segRec.Body.Bytes())
	if string(decoded) != string(plain) {
		t.Fatal("HLS AES data damaged")
	}
	bad := httptest.NewRecorder()
	app.handleVKWebStream(bad, httptest.NewRequest("GET", path+"x", nil))
	if bad.Code != 403 {
		t.Fatal("accepted invalid signature", bad.Code)
	}
	provider.SetSession(vkweb.Session{})
	bad = httptest.NewRecorder()
	app.handleVKWebStream(bad, httptest.NewRequest("GET", path, nil))
	if bad.Code != 401 {
		t.Fatal("accepted disconnected session", bad.Code)
	}
}

func TestVKPlaylistRejectsPrivateTargets(t *testing.T) {
	provider := vkweb.New()
	provider.SetSession(vkweb.Session{Cookies: "p=id; remixsid=id"})
	for _, target := range []string{"https://127.0.0.1/", "https://192.168.1.1/", "file:///x", "https://user:secret@cdn.test/"} {
		if _, err := rewriteVKPlaylist("#EXTM3U\n"+target, "https://cdn.test/playlist", "web_1_2", provider); err == nil {
			t.Fatal("accepted target", target)
		}
	}
}
