package vkweb

import (
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"Cryon2/internal/domain"
)

type audio struct {
	ID, OwnerID                       int64
	Duration                          int
	Title, Artist, URL, Hashes, Cover string
	Artists                           []string
	Flags                             struct {
		Restricted        json.RawMessage `json:"is_restricted"`
		ContentRestricted json.RawMessage `json:"content_restricted"`
		Streams           []struct {
			Type string `json:"type"`
			URL  string `json:"url"`
		} `json:"audio_streams"`
	}
}

func rawString(raw json.RawMessage) string {
	var value string
	_ = json.Unmarshal(raw, &value)
	return value
}
func rawInt(raw json.RawMessage) int64 {
	var value json.Number
	if json.Unmarshal(raw, &value) != nil {
		return 0
	}
	n, _ := strconv.ParseInt(string(value), 10, 64)
	return n
}
func parseAudioList(raw json.RawMessage) ([]audio, error) {
	var list []json.RawMessage
	if json.Unmarshal(raw, &list) != nil {
		return nil, fmt.Errorf("invalid audio list")
	}
	out := []audio{}
	for _, item := range list {
		var a []json.RawMessage
		if json.Unmarshal(item, &a) != nil || len(a) < 15 {
			continue
		}
		track := audio{ID: rawInt(a[0]), OwnerID: rawInt(a[1]), URL: rawString(a[2]), Title: html.UnescapeString(rawString(a[3])), Artist: html.UnescapeString(rawString(a[4])), Duration: int(rawInt(a[5])), Hashes: rawString(a[13]), Cover: rawString(a[14])}
		flags := a[12]
		if text := rawString(flags); text != "" {
			flags = []byte(text)
		}
		_ = json.Unmarshal(flags, &track.Flags)
		for _, index := range []int{17, 18} {
			if len(a) <= index {
				continue
			}
			var artists []struct {
				Name string `json:"name"`
			}
			if json.Unmarshal(a[index], &artists) == nil {
				for _, artist := range artists {
					if name := strings.TrimSpace(html.UnescapeString(artist.Name)); name != "" {
						track.Artists = append(track.Artists, name)
					}
				}
			}
		}
		out = append(out, track)
	}
	return out, nil
}

var htmlTags = regexp.MustCompile(`<[^>]*>`)

func searchAudio(raw json.RawMessage) ([]audio, error) {
	type block struct {
		Title string          `json:"title"`
		List  json.RawMessage `json:"list"`
	}
	var data struct {
		Playlists []block `json:"playlists"`
		Playlist  *block  `json:"playlist"`
	}
	if json.Unmarshal(raw, &data) != nil || (data.Playlists == nil && data.Playlist == nil) {
		return nil, fmt.Errorf("invalid search section")
	}
	best := []audio{}
	for _, p := range data.Playlists {
		if len(p.List) == 0 || string(p.List) == "null" {
			continue
		}
		list, err := parseAudioList(p.List)
		if err != nil || len(list) == 0 {
			continue
		}
		title := strings.ToLower(strings.TrimSpace(html.UnescapeString(htmlTags.ReplaceAllString(p.Title, ""))))
		if title == "все треки" || title == "all tracks" {
			return list, nil
		}
		if title != "мои треки" && title != "my tracks" && len(list) > len(best) {
			best = list
		}
	}
	if len(best) == 0 && data.Playlist != nil && len(data.Playlist.List) > 0 {
		return parseAudioList(data.Playlist.List)
	}
	return best, nil
}
func restricted(raw json.RawMessage) bool {
	v := strings.TrimSpace(string(raw))
	return v != "" && v != "false" && v != "0" && v != "null"
}
func (a audio) isRestricted() bool {
	return restricted(a.Flags.Restricted) || restricted(a.Flags.ContentRestricted)
}
func validURL(target string) bool {
	u, e := url.Parse(target)
	return e == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil
}

var expiryPath = regexp.MustCompile(`/expires/(\d+)`)

func streamExpiry(target string) time.Time {
	expires := time.Now().Add(50 * time.Minute)
	u, err := url.Parse(target)
	if err != nil {
		return time.Time{}
	}
	value := u.Query().Get("expires")
	if value == "" {
		if match := expiryPath.FindStringSubmatch(u.Path); len(match) == 2 {
			value = match[1]
		}
	}
	if stamp, err := strconv.ParseInt(value, 10, 64); err == nil && stamp > 0 {
		// VK's CDN links use both Unix seconds and milliseconds.
		date := time.Unix(stamp, 0)
		if stamp > 100000000000 {
			date = time.UnixMilli(stamp)
		}
		date = date.Add(-time.Minute)
		if date.Before(expires) {
			expires = date
		}
	}
	return expires
}
func (a audio) stream(uid int64) (string, string) {
	if a.isRestricted() {
		return "", ""
	}
	for _, kind := range []string{"mp3", "aac", "hls_ts", "hls"} {
		for _, stream := range a.Flags.Streams {
			if stream.Type != kind || !validURL(stream.URL) || !streamExpiry(stream.URL).After(time.Now()) {
				continue
			}
			mime := "audio/mpeg"
			if kind == "aac" {
				mime = "audio/aac"
			}
			if strings.HasPrefix(kind, "hls") {
				mime = "application/x-mpegURL"
			}
			return stream.URL, mime
		}
	}
	target := decodeAudioURL(a.URL, uid)
	if !validURL(target) || !streamExpiry(target).After(time.Now()) {
		return "", ""
	}
	u, _ := url.Parse(target)
	mime := "audio/mpeg"
	if strings.Contains(strings.ToLower(u.Path), ".m3u8") {
		mime = "application/x-mpegURL"
	}
	return target, mime
}
func (a audio) trackID() string {
	if a.ID == 0 || a.OwnerID == 0 {
		return ""
	}
	hashes := strings.Split(a.Hashes, "/")
	if len(hashes) < 6 || hashes[2] == "" || hashes[5] == "" {
		return ""
	}
	id := fmt.Sprintf("site_%d_%d_%s_%s", a.OwnerID, a.ID, hashes[2], hashes[5])
	if !siteTrackID.MatchString(id) {
		return ""
	}
	return id
}
func (a audio) track() domain.Track {
	artists := a.Artists
	if len(artists) == 0 {
		artists = []string{a.Artist}
	}
	cover := ""
	for _, candidate := range strings.Split(a.Cover, ",") {
		candidate = html.UnescapeString(strings.TrimSpace(candidate))
		if strings.HasPrefix(candidate, "//") {
			candidate = "https:" + candidate
		}
		if strings.HasPrefix(candidate, "http://") {
			candidate = "https://" + strings.TrimPrefix(candidate, "http://")
		}
		if validURL(candidate) {
			cover = candidate
		}
	}
	return domain.Track{ID: a.trackID(), Service: domain.ServiceVK, Title: a.Title, Artists: artists, DurationMs: a.Duration * 1000, ArtworkURL: cover, PlayableKind: domain.PlayableStream, ExternalURL: fmt.Sprintf("https://vk.ru/audio%d_%d", a.OwnerID, a.ID)}
}
