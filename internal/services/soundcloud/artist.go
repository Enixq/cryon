package soundcloud

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"Cryon2/internal/domain"
)

var _ domain.ArtistBrowser = (*Service)(nil)

// GetArtist is a best-effort SoundCloud catalog path. SoundCloud profiles do
// not expose a canonical discography like Yandex/YouTube Music, so playlists
// and sets owned by the matching user are presented as releases. This keeps
// the result tied to the actual artist profile instead of inventing albums
// from unrelated search tracks.
func (s *Service) GetArtist(ctx context.Context, name string) (domain.ArtistInfo, error) {
	name = strings.TrimSpace(name)
	info := domain.ArtistInfo{Name: name, Service: domain.ServiceSoundCloud}
	if name == "" {
		return info, nil
	}
	clientID, err := s.ensureClientID(ctx)
	if err != nil {
		return info, nil
	}
	user, err := s.findUser(ctx, name, clientID)
	if err != nil || user.ID == 0 {
		return info, nil
	}
	if user.Username != "" {
		info.Name = user.Username
	}
	info.ArtworkURL = normalizeArtwork(user.AvatarURL, s.ensureArtworkHost(ctx))
	if tracks, err := s.apiSearch(ctx, name, clientID); err == nil {
		for _, track := range tracks {
			if len(info.TopTracks) >= 20 {
				break
			}
			info.TopTracks = append(info.TopTracks, track)
		}
	}
	playlists, err := s.userPlaylists(ctx, user.ID, clientID)
	if err != nil {
		return info, nil
	}
	for _, playlist := range playlists {
		if playlist.ID == 0 || strings.TrimSpace(playlist.Title) == "" {
			continue
		}
		tracks := s.expandPlaylistTracks(ctx, &playlist, clientID)
		if len(tracks) == 0 {
			continue
		}
		kind := "album"
		if len(tracks) <= 4 {
			kind = "ep"
		}
		info.Albums = append(info.Albums, domain.Album{
			ID: strconv.FormatInt(playlist.ID, 10), Service: domain.ServiceSoundCloud,
			Title: playlist.Title, Artist: info.Name, Kind: kind,
			TrackCount: len(tracks), ArtworkURL: normalizeArtwork(playlist.ArtworkURL, s.ensureArtworkHost(ctx)),
			ExternalURL: playlist.Permalink,
		})
		if kind == "ep" {
			info.Singles = append(info.Singles, info.Albums[len(info.Albums)-1])
			info.Albums = info.Albums[:len(info.Albums)-1]
		}
	}
	return info, nil
}

func (s *Service) AlbumTracks(ctx context.Context, albumID string) ([]domain.Track, error) {
	return s.PlaylistTracks(ctx, albumID)
}

func (s *Service) findUser(ctx context.Context, name, clientID string) (struct {
	ID        int64
	Username  string
	AvatarURL string
}, error) {
	u, _ := url.Parse("https://api-v2.soundcloud.com/search/users")
	q := u.Query()
	q.Set("q", name)
	q.Set("client_id", clientID)
	q.Set("limit", "10")
	u.RawQuery = q.Encode()
	body, err := s.fetchText(ctx, u.String())
	if err != nil {
		return struct {
			ID        int64
			Username  string
			AvatarURL string
		}{}, err
	}
	var response struct {
		Collection []struct {
			ID        int64  `json:"id"`
			Username  string `json:"username"`
			AvatarURL string `json:"avatar_url"`
		} `json:"collection"`
	}
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		return struct {
			ID        int64
			Username  string
			AvatarURL string
		}{}, err
	}
	want := strings.ToLower(strings.TrimSpace(name))
	chosen := response.Collection
	for _, candidate := range response.Collection {
		if strings.EqualFold(candidate.Username, want) {
			chosen = []struct {
				ID        int64  `json:"id"`
				Username  string `json:"username"`
				AvatarURL string `json:"avatar_url"`
			}{candidate}
			break
		}
	}
	if len(chosen) == 0 {
		return struct {
			ID        int64
			Username  string
			AvatarURL string
		}{}, nil
	}
	return struct {
		ID        int64
		Username  string
		AvatarURL string
	}{ID: chosen[0].ID, Username: chosen[0].Username, AvatarURL: chosen[0].AvatarURL}, nil
}

func (s *Service) userPlaylists(ctx context.Context, userID int64, clientID string) ([]scPlaylist, error) {
	endpoint := "https://api-v2.soundcloud.com/users/" + strconv.FormatInt(userID, 10) + "/playlists?client_id=" + url.QueryEscape(clientID) + "&limit=50"
	body, err := s.fetchText(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	var response struct {
		Collection []scPlaylist `json:"collection"`
	}
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		return nil, err
	}
	return response.Collection, nil
}
