package main

import "testing"

func TestParseStreamPath(t *testing.T) {
	cases := []struct {
		name        string
		escaped     string
		wantService string
		wantID      string
		wantOK      bool
	}{
		{
			name:        "обычный числовой id",
			escaped:     "/stream/soundcloud/123456",
			wantService: "soundcloud",
			wantID:      "123456",
			wantOK:      true,
		},
		{
			name:        "id видео YouTube",
			escaped:     "/stream/youtube/dQw4w9WgXcQ",
			wantService: "youtube",
			wantID:      "dQw4w9WgXcQ",
			wantOK:      true,
		},
		{
			// Главный смысл escaped-пути: id-ссылка со слэшами доезжает целиком.
			name:        "id-ссылка со слэшами",
			escaped:     "/stream/soundcloud/https%3A%2F%2Fsoundcloud.com%2Fbasta%2F8800",
			wantService: "soundcloud",
			wantID:      "https://soundcloud.com/basta/8800",
			wantOK:      true,
		},
		{
			name:        "id с пробелом и кириллицей",
			escaped:     "/stream/local/%D0%91%D0%B0%D1%81%D1%82%D0%B0%20-%208800.mp3",
			wantService: "local",
			wantID:      "Баста - 8800.mp3",
			wantOK:      true,
		},
		{name: "чужой префикс", escaped: "/local/123"},
		{name: "нет id", escaped: "/stream/youtube"},
		{name: "пустой id", escaped: "/stream/youtube/"},
		{name: "пустой сервис", escaped: "/stream//123"},
		{name: "битая escape-последовательность", escaped: "/stream/youtube/%zz"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service, id, ok := parseStreamPath(tc.escaped)
			if ok != tc.wantOK {
				t.Fatalf("parseStreamPath(%q) ok = %v, ожидалось %v", tc.escaped, ok, tc.wantOK)
			}
			if !tc.wantOK {
				return
			}
			if service != tc.wantService || id != tc.wantID {
				t.Fatalf("parseStreamPath(%q) = (%q, %q), ожидалось (%q, %q)",
					tc.escaped, service, id, tc.wantService, tc.wantID)
			}
		})
	}
}
