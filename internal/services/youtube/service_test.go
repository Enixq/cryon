package youtube

import "testing"

func TestResolveArtistTitle(t *testing.T) {
	cases := []struct {
		name       string
		channel    string
		rawTitle   string
		wantArtist string
		wantTitle  string
	}{
		{
			name:       "topic-канал — исполнитель это сам канал",
			channel:    "Баста - Topic",
			rawTitle:   "8800",
			wantArtist: "Баста",
			wantTitle:  "8800",
		},
		{
			name:       "topic-канал чистит хвост YouTube Music",
			channel:    "Jahmal TGK - Topic",
			rawTitle:   "БУДНИ - YouTube Music",
			wantArtist: "Jahmal TGK",
			wantTitle:  "БУДНИ",
		},
		{
			name:       "заливщик — исполнитель из заголовка, а не из канала",
			channel:    "GAZ LIVE",
			rawTitle:   "Баста - 8800",
			wantArtist: "Баста",
			wantTitle:  "8800",
		},
		{
			name:       "заливщик + совместка в заголовке",
			channel:    "Uharmony",
			rawTitle:   "Баста, AMCHI - Буду всегда",
			wantArtist: "Баста, AMCHI",
			wantTitle:  "Буду всегда",
		},
		{
			name:       "заголовок без разделителя — падаем на имя канала",
			channel:    "БастаVEVO",
			rawTitle:   "Сансара",
			wantArtist: "БастаVEVO",
			wantTitle:  "Сансара",
		},
		{
			name:       "нет ни канала, ни разделителя (Bing)",
			channel:    "",
			rawTitle:   "Просто трек",
			wantArtist: "",
			wantTitle:  "Просто трек",
		},
		{
			name:       "канал YouTube Music не считается исполнителем",
			channel:    "YouTube Music",
			rawTitle:   "Название без артиста",
			wantArtist: "",
			wantTitle:  "Название без артиста",
		},
		{
			name:       "чистит оформительский хвост (Official Video)",
			channel:    "GAZ LIVE",
			rawTitle:   "Баста - Медлячок (Official Video)",
			wantArtist: "Баста",
			wantTitle:  "Медлячок",
		},
		{
			name:       "сохраняет значимую пометку версии",
			channel:    "Miyagi - Topic",
			rawTitle:   "Kosandra (Slowed + Reverb)",
			wantArtist: "Miyagi",
			wantTitle:  "Kosandra (Slowed + Reverb)",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			artist, title := resolveArtistTitle(c.channel, c.rawTitle)
			if artist != c.wantArtist || title != c.wantTitle {
				t.Errorf("resolveArtistTitle(%q, %q) = (%q, %q), want (%q, %q)",
					c.channel, c.rawTitle, artist, title, c.wantArtist, c.wantTitle)
			}
		})
	}
}

func TestCleanTrackTitle(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Медлячок (Official Video)", "Медлячок"},
		{"Song [Official Audio]", "Song"},
		{"Track (Official Music Video)", "Track"},
		{"Track (Lyric Video)", "Track"},
		{"Track (HD)", "Track"},
		{"Клип - Название (Клип)", "Клип - Название"},
		{"Song (Audio) [4K]", "Song"},
		// Значимые пометки версии сохраняются целиком.
		{"Kosandra (Slowed + Reverb)", "Kosandra (Slowed + Reverb)"},
		{"Track (Live)", "Track (Live)"},
		{"Track (prod. Metro)", "Track (prod. Metro)"},
		{"Track (Remix)", "Track (Remix)"},
		{"Обычное название", "Обычное название"},
	}
	for _, c := range cases {
		if got := cleanTrackTitle(c.in); got != c.want {
			t.Errorf("cleanTrackTitle(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseISO8601Duration(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"PT3M45S", 225000},
		{"PT1H2M3S", 3723000},
		{"PT45S", 45000},
		{"PT1H", 3600000},
		{"P1DT1H", 90000000},
		{"", 0},
		{"garbage", 0},
	}
	for _, c := range cases {
		if got := parseISO8601Duration(c.in); got != c.want {
			t.Errorf("parseISO8601Duration(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestParseYouTubeSearchPageEscapedVideoWithContextRenderer(t *testing.T) {
	page := `<script>var ytInitialData = '\x7b\x22contents\x22:\x7b\x22videoWithContextRenderer\x22:\x7b\x22videoId\x22:\x22abc123\x22,\x22headline\x22:\x7b\x22runs\x22:[\x7b\x22text\x22:\x22Artist - Song\x22\x7d]\x7d,\x22shortBylineText\x22:\x7b\x22runs\x22:[\x7b\x22text\x22:\x22Artist\x22\x7d]\x7d,\x22lengthText\x22:\x7b\x22simpleText\x22:\x223:21\x22\x7d\x7d\x7d\x7d';</script>`
	tracks, err := parseYouTubeSearchPage(page)
	if err != nil {
		t.Fatalf("parseYouTubeSearchPage() error = %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("len(tracks) = %d, want 1", len(tracks))
	}
	track := tracks[0]
	if track.ID != "abc123" || track.Title != "Song" || len(track.Artists) != 1 || track.Artists[0] != "Artist" || track.DurationMs != 201000 {
		t.Fatalf("unexpected track: %#v", track)
	}
}

func TestParseYouTubeSearchPage(t *testing.T) {
	page := `<script>var ytInitialData = {"contents":{"videoRenderer":{"videoId":"dQw4w9WgXcQ","title":{"runs":[{"text":"Rick Astley - Never Gonna Give You Up"}]},"ownerText":{"runs":[{"text":"Rick Astley"}]},"lengthText":{"simpleText":"3:33"}}}};</script>`
	tracks, err := parseYouTubeSearchPage(page)
	if err != nil {
		t.Fatalf("parseYouTubeSearchPage() error = %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("len(tracks) = %d, want 1", len(tracks))
	}
	track := tracks[0]
	if track.ID != "dQw4w9WgXcQ" || track.Title != "Never Gonna Give You Up" || len(track.Artists) != 1 || track.Artists[0] != "Rick Astley" || track.DurationMs != 213000 {
		t.Fatalf("unexpected track: %#v", track)
	}
}
