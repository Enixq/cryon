package vkweb

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestDecodeWebsiteURLs(t *testing.T) {
	// Fixed vectors generated from the reviewed NK Player JavaScript algorithm.
	const target = "https://cdn.test/track.mp3?key=O0+xyz"
	for _, encoded := range []string{
		"ENL4kZbppxLLAZ8ZCgOUA2nHCNqVDhnLDc5UzgmVlZPZChrOAa#DG",
		"BOfbD306zMzQA3uUqwX6qwzbEwHQCI5ODYS/CMXgzZDwzuvgrW#CGS3",
		"AhjJExqRAY5ZChq6md1OzgvJDc8/Cc9LDgSVBJn5t214ENnHlG#CWSXmJmOnq",
		"CI8VlNq6CcTOC2HOyY5LpxKWBNrWzxPRm3HJzd8Vt3nOBwT5yq#AqS5odC2ntqZmJe",
		"iZ8/oZHXzgq0lYvLpY44p2q/os00iguMo3HOic4YDGr7ydmYmq#EaTl",
	} {
		got := decodeAudioURL("https://vk.ru/audio_api_unavailable.mp3?extra="+encoded, 123456789)
		if got != target {
			t.Fatalf("decoded wrong URL for %s: %q", encoded, got)
		}
	}
	if decodeAudioURL(target, 0) != target {
		t.Fatal("direct URL changed")
	}
	for _, bad := range []string{"http://cdn.test/audio.mp3", "https://user:secret@cdn.test/a", "https://vk.ru/audio_api_unavailable.mp3?extra=a#bad", "https://vk.ru/audio_api_unavailable.mp3", "file:///etc/passwd"} {
		if decodeAudioURL(bad, 123) != "" {
			t.Fatal("unsafe or invalid URL accepted")
		}
	}
}
func TestSearchSelectsRelevantBlockAndHandlesFallbacks(t *testing.T) {
	for _, test := range []struct {
		data any
		want int
	}{
		{map[string]any{"playlists": []any{map[string]any{"title": "Все треки", "list": []any{fixtureAudio(1, "", 180, nil)}}, map[string]any{"title": "Найдено в тексте", "list": []any{fixtureAudio(2, "", 180, nil), fixtureAudio(3, "", 180, nil)}}}}, 1},
		{map[string]any{"playlists": []any{map[string]any{"title": "Мои треки", "list": []any{fixtureAudio(2, "", 180, nil), fixtureAudio(3, "", 180, nil)}}, map[string]any{"title": "Результаты", "list": []any{fixtureAudio(1, "", 180, nil)}}}}, 1},
		{map[string]any{"playlist": map[string]any{"list": []any{fixtureAudio(1, "", 180, nil)}}}, 1},
		{map[string]any{"playlists": []any{}}, 0},
		{map[string]any{"playlists": []any{map[string]any{"title": "Все треки", "list": []any{}}, map[string]any{"title": "Найдено в тексте", "list": []any{fixtureAudio(1, "", 180, nil)}}}}, 1},
	} {
		raw, _ := json.Marshal(test.data)
		list, err := searchAudio(raw)
		if err != nil || len(list) != test.want {
			t.Fatal(list, err)
		}
		if len(list) > 0 && list[0].ID != 1 {
			t.Fatal("unrelated block selected")
		}
	}
	if _, err := searchAudio([]byte(`{"format_changed":true}`)); err == nil {
		t.Fatal("format change hidden as empty results")
	}
}
func TestStreamKindsAndExpiry(t *testing.T) {
	flags := map[string]any{"audio_streams": []any{
		map[string]any{"type": "mp3", "url": "https://cdn.test/expired?expires=1000"},
		map[string]any{"type": "hls_ts", "url": "https://cdn.test/no-extension"},
	}}
	encodedFlags, _ := json.Marshal(flags)
	raw, _ := json.Marshal([]any{fixtureAudio(1, "", 180, string(encodedFlags))})
	a, err := parseAudioList(raw)
	if err != nil || len(a) != 1 {
		t.Fatal(err)
	}
	target, mime := a[0].stream(123)
	if target != "https://cdn.test/no-extension" || !strings.Contains(mime, "mpegURL") {
		t.Fatal(target, mime)
	}
	seconds := time.Now().Add(4 * time.Minute).Unix()
	for _, stamp := range []int64{seconds, seconds * 1000} {
		date := streamExpiry("https://cdn.test/expires/" + jsonNumber(stamp))
		if time.Until(date) < 2*time.Minute || time.Until(date) > 4*time.Minute {
			t.Fatal("wrong CDN expiry units")
		}
	}
}
func jsonNumber(n int64) string { raw, _ := json.Marshal(n); return string(raw) }
