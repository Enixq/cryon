package vk

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"Cryon2/internal/domain"
	"Cryon2/internal/services/lyrics"
	"github.com/oklookat/govkm"
	"github.com/oklookat/govkm/schema"
	"github.com/oklookat/vantuz"
)

type Service struct {
	mu        sync.RWMutex
	token     string
	transport http.RoundTripper
}

func New(token string) *Service         { return &Service{token: strings.TrimSpace(token)} }
func (s *Service) ID() domain.ServiceID { return domain.ServiceVK }
func (s *Service) SetToken(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.token = strings.TrimSpace(token)
}
func (s *Service) HasToken() bool { s.mu.RLock(); defer s.mu.RUnlock(); return s.token != "" }
func (s *Service) client() *govkm.Client {
	s.mu.RLock()
	token := s.token
	s.mu.RUnlock()
	httpClient := vantuz.C().SetAuthorization("Bearer " + token)
	httpClient.SetClient(&http.Client{Timeout: 12 * time.Second, Transport: s.transport})
	httpClient.SetUserAgent("okhttp/5.0.0-alpha.2")
	httpClient.SetGlobalHeader("X-App-Id", "android").SetGlobalHeader("X-Client-Version", "10477")
	// govkm.New performs an unbounded network call; construct only its client.
	return &govkm.Client{Http: httpClient}
}
func (s *Service) ValidateToken(ctx context.Context) error {
	if !s.HasToken() {
		return fmt.Errorf("ВК Музыка не подключена")
	}
	_, err := s.client().UserInfo(ctx)
	if err != nil {
		return fmt.Errorf("ВК Музыка не приняла токен или недоступна")
	}
	return nil
}
func usable(track schema.Track) bool {
	return !track.IsRestricted && track.Permissions.Permit && track.APIID != "" && (validStream(track.File) || validStream(track.Hls))
}
func validStream(target string) bool {
	uri, err := url.Parse(target)
	return err == nil && uri.Host != "" && (uri.Scheme == "https" || uri.Scheme == "http")
}
func mapTrack(track schema.Track) domain.Track {
	artists := []string{}
	for _, artist := range track.Artists {
		if artist.Name != "" {
			artists = append(artists, artist.Name)
		}
	}
	if len(artists) == 0 {
		name := track.ArtistDisplayName
		if name == "" {
			name = track.Artist.Name
		}
		artists = []string{name}
	}
	result := domain.Track{ID: track.APIID.String(), Service: domain.ServiceVK, Title: track.Name, Artists: artists, DurationMs: track.Duration * 1000, ArtworkURL: track.Cover.URL, PlayableKind: domain.PlayableStream}
	if track.Album != nil {
		result.Album = track.Album.Name
	}
	if track.Counts != nil {
		result.PlayCount = track.Counts.Play
	}
	if track.VkAudioID != "" {
		result.ExternalURL = "https://vk.com/audio" + track.VkAudioID.String()
	}
	return result
}
func (s *Service) Search(ctx context.Context, query string) ([]domain.Track, error) {
	if !s.HasToken() || strings.TrimSpace(query) == "" {
		return []domain.Track{}, nil
	}
	result, err := s.client().SearchTrack(ctx, query, 30, 0)
	if err != nil {
		return nil, fmt.Errorf("поиск ВК Музыки недоступен")
	}
	out := []domain.Track{}
	for _, track := range result.Data.Tracks {
		if usable(track) && domain.IsSongDuration(track.Duration*1000) {
			out = append(out, mapTrack(track))
		}
	}
	return out, nil
}

type StreamInfo struct {
	URL, MimeType string
	DurationMs    int
}

func (s *Service) track(ctx context.Context, id string) (*schema.Track, error) {
	if !s.HasToken() {
		return nil, fmt.Errorf("ВК Музыка не подключена")
	}
	result, err := s.client().Track(ctx, schema.ID(id))
	if err != nil || result == nil || result.Data.Track == nil {
		return nil, fmt.Errorf("трек ВК Музыки недоступен")
	}
	return result.Data.Track, nil
}
func (s *Service) ResolveStream(ctx context.Context, id string) (*StreamInfo, error) {
	track, err := s.track(ctx, id)
	if err != nil {
		return nil, err
	}
	if !usable(*track) {
		return nil, fmt.Errorf("ВК Музыка ограничила доступ к треку")
	}
	info := &StreamInfo{DurationMs: track.Duration * 1000}
	if validStream(track.File) {
		info.URL, info.MimeType = track.File, "audio/mpeg"
	} else {
		info.URL, info.MimeType = track.Hls, "application/x-mpegURL"
	}
	return info, nil
}
func (s *Service) TrackLyrics(ctx context.Context, id string) (lyrics.Result, error) {
	track, err := s.track(ctx, id)
	if err != nil {
		return lyrics.Result{}, err
	}
	result := lyrics.Result{Provider: "ВК Музыка"}
	if track.Lyrics == nil {
		return result, nil
	}
	var plain, synced strings.Builder
	for _, interval := range track.Lyrics.Intervals {
		if strings.TrimSpace(interval.Line) == "" {
			continue
		}
		plain.WriteString(interval.Line + "\n")
		ms := interval.Begin
		fmt.Fprintf(&synced, "[%02d:%02d.%02d]%s\n", ms/60000, (ms/1000)%60, (ms%1000)/10, interval.Line)
	}
	result.Plain, result.Synced = strings.TrimSpace(plain.String()), strings.TrimSpace(synced.String())
	result.Found = result.Plain != ""
	return result, nil
}
