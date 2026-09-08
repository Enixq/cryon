package trackmeta

import "testing"

func TestSplitArtistTitle(t *testing.T) {
	cases := []struct {
		name       string
		raw        string
		wantArtist string
		wantTitle  string
	}{
		{"обычный дефис", "Баста - 8800", "Баста", "8800"},
		{"длинное тире", "Гуф — Ice Baby", "Гуф", "Ice Baby"},
		{"среднее тире", "Каспийский Груз – 18", "Каспийский Груз", "18"},
		{"интерпункт (YouTube Music)", "Basta · Sunshine", "Basta", "Sunshine"},
		{"без разделителя", "8800", "", "8800"},
		{"слева только украшения", "Official Video - 8800", "", "Official Video - 8800"},
		{"пустая правая часть", "Баста - ", "", "Баста -"},
		{"разделитель в начале", "- 8800", "", "- 8800"},
		{"первый разделитель побеждает", "Баста - 8800 - Live", "Баста", "8800 - Live"},
		{"пробелы по краям", "  Баста - 8800  ", "Баста", "8800"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			artist, title := SplitArtistTitle(tc.raw)
			if artist != tc.wantArtist || title != tc.wantTitle {
				t.Fatalf("SplitArtistTitle(%q) = (%q, %q), ожидалось (%q, %q)",
					tc.raw, artist, title, tc.wantArtist, tc.wantTitle)
			}
		})
	}
}

func TestCleanTitle(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"8800 (Official Video)", "8800"},
		{"8800 [Official Audio]", "8800"},
		{"Мама (Клип)", "Мама"},
		{"Урбан (HD)", "Урбан"},
		{"Песня (Official Video) [4K]", "Песня"},
		// Значимые пометки версии остаются: они меняют сам трек.
		{"8800 (Slowed + Reverb)", "8800 (Slowed + Reverb)"},
		{"Ice Baby (Live)", "Ice Baby (Live)"},
		{"Мама (Remix)", "Мама (Remix)"},
		{"Трек (prod. Metro)", "Трек (prod. Metro)"},
		{"  8800  ", "8800"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := CleanTitle(tc.raw); got != tc.want {
			t.Errorf("CleanTitle(%q) = %q, ожидалось %q", tc.raw, got, tc.want)
		}
	}
}
