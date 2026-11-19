package soundcloud

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestResolveAvailableTranscodingFallsBackToHLS(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/progressive":
			_, _ = fmt.Fprintf(w, `{"url":%q}`, server.URL+"/blocked")
		case "/hls":
			_, _ = fmt.Fprintf(w, `{"url":%q}`, server.URL+"/playlist")
		case "/blocked":
			w.WriteHeader(http.StatusForbidden)
		case "/playlist":
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			_, _ = w.Write([]byte("#EXTM3U"))
		}
	}))
	defer server.Close()
	progressive := transcoding{URL: server.URL + "/progressive"}
	progressive.Format.Protocol = "progressive"
	hls := transcoding{URL: server.URL + "/hls"}
	hls.Format.Protocol = "hls"
	service := &Service{httpClient: server.Client()}
	chosen, streamURL, err := service.resolveAvailableTranscoding(context.Background(), []transcoding{progressive, hls}, "test")
	if err != nil || chosen == nil || chosen.Format.Protocol != "hls" || streamURL != server.URL+"/playlist" {
		t.Fatalf("fallback: candidate=%+v url=%q err=%v", chosen, streamURL, err)
	}
}

type rewriteTransport struct {
	target *url.URL
	base   http.RoundTripper
}

func (transport rewriteTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	copy := request.Clone(request.Context())
	copy.URL.Scheme = transport.target.Scheme
	copy.URL.Host = transport.target.Host
	return transport.base.RoundTrip(copy)
}

func TestResolveStreamRefreshesRejectedClientID(t *testing.T) {
	const stale = "staleClientId123456789012345"
	const fresh = "freshClientId123456789012345"
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = fmt.Fprintf(w, `{"clientId":%q}`, fresh)
		case "/tracks/123":
			if r.URL.Query().Get("client_id") != fresh {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = fmt.Fprintf(w, `{"duration":180000,"media":{"transcodings":[{"url":%q,"format":{"protocol":"progressive","mime_type":"audio/mpeg"}}]}}`, server.URL+"/transcoding")
		case "/transcoding":
			_, _ = fmt.Fprintf(w, `{"url":%q}`, server.URL+"/audio")
		case "/audio":
			_, _ = w.Write([]byte("audio"))
		}
	}))
	defer server.Close()
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	service := &Service{clientID: stale, httpClient: &http.Client{Transport: rewriteTransport{target: target, base: http.DefaultTransport}}}
	service.artworkOnce.Do(func() {})
	stream, err := service.ResolveStream(context.Background(), "123")
	if err != nil || stream == nil || stream.URL != server.URL+"/audio" {
		t.Fatalf("stream=%+v err=%v", stream, err)
	}
}
