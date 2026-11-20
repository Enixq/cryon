package youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"Cryon2/internal/domain"
)

// YouTube Music exposes artist/release/playlist catalog data through the same
// public InnerTube endpoint used by its web client.  This path is deliberately
// independent from yt-dlp so it works in the Android APK.
const (
	catalogKey     = innertubeWebKey
	catalogClient  = "WEB_REMIX"
	catalogVersion = "1.20230724.00.00"
	catalogUA      = innertubeWebUserAgent
)

func (s *Service) catalogCall(ctx context.Context, endpoint string, body map[string]any) (any, error) {
	payload, err := json.Marshal(map[string]any{
		"context": map[string]any{"client": map[string]any{
			"clientName": catalogClient, "clientVersion": catalogVersion,
			"hl": "ru", "gl": "RU",
		}},
		"browseId": body["browseId"],
		"query":    body["query"],
		"params":   body["params"],
	})
	if err != nil {
		return nil, err
	}
	url := "https://music.youtube.com/youtubei/v1/" + endpoint + "?key=" + catalogKey + "&prettyPrint=false"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Origin", "https://music.youtube.com")
	req.Header.Set("Referer", "https://music.youtube.com/")
	req.Header.Set("User-Agent", catalogUA)
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("youtube music %s returned HTTP %d", endpoint, resp.StatusCode)
	}
	var out any
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func walkCatalog(value any, visit func(map[string]any)) {
	switch v := value.(type) {
	case map[string]any:
		visit(v)
		for _, child := range v {
			walkCatalog(child, visit)
		}
	case []any:
		for _, child := range v {
			walkCatalog(child, visit)
		}
	}
}

func catalogText(value any) string {
	var out []string
	walkCatalog(value, func(m map[string]any) {
		if runs, ok := m["runs"].([]any); ok {
			for _, raw := range runs {
				if run, ok := raw.(map[string]any); ok {
					if text, ok := run["text"].(string); ok && strings.TrimSpace(text) != "" {
						out = append(out, text)
					}
				}
			}
		}
	})
	return strings.TrimSpace(strings.Join(out, " "))
}

func catalogThumb(value any) string {
	best := ""
	walkCatalog(value, func(m map[string]any) {
		if list, ok := m["thumbnails"].([]any); ok {
			for _, raw := range list {
				if thumb, ok := raw.(map[string]any); ok {
					if url, ok := thumb["url"].(string); ok && url != "" {
						best = url
					}
				}
			}
		}
	})
	return best
}

func catalogItemTitle(value any) string {
	return catalogItemColumn(value, 0)
}

func catalogItemColumn(value any, index int) string {
	if item, ok := value.(map[string]any); ok {
		if columns, ok := item["flexColumns"].([]any); ok && len(columns) > index {
			if column, ok := columns[index].(map[string]any); ok {
				if renderer, ok := column["musicResponsiveListItemFlexColumnRenderer"].(map[string]any); ok {
					if title := catalogText(renderer["text"]); title != "" {
						return title
					}
				}
			}
		}
	}
	return ""
}

func catalogHeaderTitle(value any) string {
	var title string
	walkCatalog(value, func(m map[string]any) {
		if title != "" {
			return
		}
		for _, key := range []string{"musicImmersiveHeaderRenderer", "musicResponsiveHeaderRenderer"} {
			if header, ok := m[key].(map[string]any); ok {
				if candidate := catalogText(header["title"]); candidate != "" {
					title = candidate
					return
				}
			}
		}
	})
	return title
}

func catalogEndpointIDs(value any, prefixes ...string) []string {
	seen := map[string]bool{}
	ids := []string{}
	walkCatalog(value, func(m map[string]any) {
		if endpoint, ok := m["browseEndpoint"].(map[string]any); ok {
			id, _ := endpoint["browseId"].(string)
			for _, prefix := range prefixes {
				if strings.HasPrefix(id, prefix) && !seen[id] {
					seen[id] = true
					ids = append(ids, id)
				}
			}
		}
	})
	return ids
}

func catalogVideoID(value any) string {
	var id string
	walkCatalog(value, func(m map[string]any) {
		if id != "" {
			return
		}
		if endpoint, ok := m["watchEndpoint"].(map[string]any); ok {
			id, _ = endpoint["videoId"].(string)
		}
		if endpoint, ok := m["watchPlaylistEndpoint"].(map[string]any); ok && id == "" {
			id, _ = endpoint["videoId"].(string)
		}
	})
	return id
}

func firstBrowseID(value any, prefixes ...string) string {
	ids := catalogEndpointIDs(value, prefixes...)
	if len(ids) > 0 {
		return ids[0]
	}
	return ""
}

func parseCatalogDuration(value string) int {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0
	}
	total := 0
	for _, part := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			return 0
		}
		total = total*60 + n
	}
	return total * 1000
}

func (s *Service) catalogBrowse(ctx context.Context, id string) (any, error) {
	return s.catalogCall(ctx, "browse", map[string]any{"browseId": id})
}

func (s *Service) searchCatalog(ctx context.Context, query, params string) (any, error) {
	return s.catalogCall(ctx, "search", map[string]any{"query": query, "params": params})
}

func (s *Service) getArtistInnerTube(ctx context.Context, name string) (domain.ArtistInfo, error) {
	result, err := s.searchCatalog(ctx, name, "")
	if err != nil {
		return domain.ArtistInfo{}, err
	}
	artistID := firstBrowseID(result, "UC")
	if artistID == "" {
		return domain.ArtistInfo{}, nil
	}
	page, err := s.catalogBrowse(ctx, artistID)
	if err != nil {
		return domain.ArtistInfo{}, err
	}
	info := domain.ArtistInfo{Name: name, Service: domain.ServiceYouTube}
	info.ArtworkURL = catalogThumb(page)
	seenTracks := map[string]bool{}
	seenAlbums := map[string]bool{}
	walkCatalog(page, func(m map[string]any) {
		if _, ok := m["musicResponsiveListItemRenderer"]; !ok {
			return
		}
		item := m["musicResponsiveListItemRenderer"]
		videoID := catalogVideoID(item)
		if videoID != "" && !seenTracks[videoID] && len(info.TopTracks) < 20 {
			seenTracks[videoID] = true
			info.TopTracks = append(info.TopTracks, domain.Track{
				ID: videoID, Service: domain.ServiceYouTube, Title: catalogItemTitle(item),
				Artists: []string{name}, ArtworkURL: catalogThumb(item),
				ExternalURL:  "https://music.youtube.com/watch?v=" + videoID,
				PlayableKind: domain.PlayableStream,
			})
		}
		albumID := firstBrowseID(item, "MPREb_")
		if albumID == "" || seenAlbums[albumID] {
			return
		}
		seenAlbums[albumID] = true
		albumTitle := catalogItemTitle(item)
		kind := "album"
		lower := strings.ToLower(albumTitle)
		if strings.Contains(lower, "single") || strings.Contains(lower, "сингл") {
			kind = "single"
		}
		info.Albums = append(info.Albums, domain.Album{ID: albumID, Service: domain.ServiceYouTube, Title: albumTitle, Artist: name, ArtworkURL: catalogThumb(item), Kind: kind, ExternalURL: "https://music.youtube.com/browse/" + albumID})
	})
	// Resolve the release cards to obtain real track counts and cache their tracks.
	for i := range info.Albums {
		tracks, title, cover, err := s.catalogAlbumTracks(ctx, info.Albums[i].ID, name)
		if err != nil || len(tracks) == 0 {
			continue
		}
		info.Albums[i].TrackCount = len(tracks)
		if title != "" {
			info.Albums[i].Title = title
		}
		if cover != "" {
			info.Albums[i].ArtworkURL = cover
		}
		s.cacheAlbum(info.Albums[i].ID, tracks)
		if info.Albums[i].Kind == "single" || (info.Albums[i].Kind == "album" && len(tracks) <= 4) {
			info.Singles = append(info.Singles, info.Albums[i])
		}
	}
	// Remove short releases from the album list after classifying them.
	filtered := info.Albums[:0]
	for _, album := range info.Albums {
		if album.Kind == "single" || (album.Kind == "album" && album.TrackCount > 0 && album.TrackCount <= 4) {
			continue
		}
		filtered = append(filtered, album)
	}
	info.Albums = filtered
	return info, nil
}

func (s *Service) catalogAlbumTracks(ctx context.Context, id, artist string) ([]domain.Track, string, string, error) {
	page, err := s.catalogBrowse(ctx, id)
	if err != nil {
		return nil, "", "", err
	}
	title := catalogHeaderTitle(page)
	cover := catalogThumb(page)
	seen := map[string]bool{}
	tracks := []domain.Track{}
	walkCatalog(page, func(m map[string]any) {
		item, ok := m["musicResponsiveListItemRenderer"]
		if !ok {
			return
		}
		videoID := catalogVideoID(item)
		if videoID == "" || seen[videoID] {
			return
		}
		seen[videoID] = true
		trackArtist := catalogItemColumn(item, 1)
		if trackArtist == "" {
			trackArtist = artist
		}
		tracks = append(tracks, domain.Track{ID: videoID, Service: domain.ServiceYouTube, Title: catalogItemTitle(item), Artists: []string{trackArtist}, Album: title, ArtworkURL: catalogThumb(item), ExternalURL: "https://music.youtube.com/watch?v=" + videoID, PlayableKind: domain.PlayableStream})
	})
	return tracks, title, cover, nil
}

func (s *Service) SearchPlaylists(ctx context.Context, query string, limit int) ([]domain.Playlist, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	result, err := s.searchCatalog(ctx, strings.TrimSpace(query), "EgIQAw==")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := []domain.Playlist{}
	walkCatalog(result, func(m map[string]any) {
		if len(out) >= limit {
			return
		}
		if _, ok := m["musicCardShelfRenderer"]; !ok {
			return
		}
		card := m["musicCardShelfRenderer"]
		id := firstBrowseID(card, "VL", "PL")
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		cardMap, _ := card.(map[string]any)
		name := catalogText(cardMap["title"])
		if name == "" {
			name = catalogItemTitle(card)
		}
		out = append(out, domain.Playlist{ID: id, Service: domain.ServiceYouTube, Name: name, ArtworkURL: catalogThumb(card), ExternalURL: "https://music.youtube.com/playlist?list=" + strings.TrimPrefix(id, "VL")})
	})
	return out, nil
}

func (s *Service) PlaylistTracks(ctx context.Context, playlistID string) ([]domain.Track, error) {
	page, err := s.catalogBrowse(ctx, playlistID)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	tracks := []domain.Track{}
	walkCatalog(page, func(m map[string]any) {
		item, ok := m["musicResponsiveListItemRenderer"]
		if !ok {
			return
		}
		id := catalogVideoID(item)
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		text := catalogItemTitle(item)
		artist := catalogItemColumn(item, 1)
		if artist == "" {
			artist = "YouTube Music"
		}
		tracks = append(tracks, domain.Track{ID: id, Service: domain.ServiceYouTube, Title: text, Artists: []string{artist}, ArtworkURL: catalogThumb(item), ExternalURL: "https://music.youtube.com/watch?v=" + id, PlayableKind: domain.PlayableStream})
	})
	return tracks, nil
}
