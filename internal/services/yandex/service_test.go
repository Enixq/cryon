package yandex

import (
	"encoding/json"
	"testing"

	"Cryon2/internal/domain"
)

func TestYaTrackToDomain(t *testing.T) {
	raw := `{
		"id": 42,
		"title": "Песня",
		"durationMs": 200000,
		"coverUri": "avatars.yandex.net/get-music/x/%%",
		"available": true,
		"artists": [{"name": "A"}, {"name": "B"}],
		"albums": [{"id": 99, "title": "Альбом"}]
	}`
	var yt yaTrack
	if err := json.Unmarshal([]byte(raw), &yt); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	tr := yt.toDomain()
	if tr.ID != "42:99" {
		t.Errorf("ID = %q, want 42:99 (track:album)", tr.ID)
	}
	if tr.Service != domain.ServiceYandex {
		t.Errorf("Service = %q, want yandex", tr.Service)
	}
	if tr.Album != "Альбом" {
		t.Errorf("Album = %q", tr.Album)
	}
	if len(tr.Artists) != 2 {
		t.Errorf("Artists = %v", tr.Artists)
	}
	if tr.DurationMs != 200000 {
		t.Errorf("DurationMs = %d", tr.DurationMs)
	}
	if tr.PlayableKind != domain.PlayableStream {
		t.Errorf("доступный трек должен быть stream, got %q", tr.PlayableKind)
	}
	if tr.ArtworkURL != "https://avatars.yandex.net/get-music/x/400x400" {
		t.Errorf("ArtworkURL = %q", tr.ArtworkURL)
	}
}

func TestYaTrackUnavailableIsExternal(t *testing.T) {
	raw := `{"id": 1, "title": "t", "available": false, "albums": [{"id": 2, "title": "al"}]}`
	var yt yaTrack
	if err := json.Unmarshal([]byte(raw), &yt); err != nil {
		t.Fatal(err)
	}
	if yt.toDomain().PlayableKind != domain.PlayableExternal {
		t.Error("недоступный трек должен быть external_only")
	}
}

func TestParseYandexSearchPage(t *testing.T) {
	html := `<html><body><a href="/album/10/track/20">Свет</a><a href="https://music.yandex.ru/track/21">Тьма</a></body></html>`
	got := parseYandexSearchPage(html)
	if len(got) != 2 {
		t.Fatalf("parseYandexSearchPage() вернул %d результатов, want 2", len(got))
	}
	if got[0].ExternalURL != "https://music.yandex.ru/album/10/track/20" {
		t.Fatalf("первый URL = %q, want yandex album/track url", got[0].ExternalURL)
	}
	if got[0].Title != "Свет" {
		t.Fatalf("первый title = %q, want Свет", got[0].Title)
	}
}
