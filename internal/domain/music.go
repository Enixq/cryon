package domain

import (
	"strings"
	"time"
	"unicode"
)

// Unknown duration stays searchable; known previews and long mixes do not.
func IsSongDuration(ms int) bool { return ms == 0 || ms > 30_000 && ms < 20*60_000 }

func MusicKey(s string) string {
	return strings.Map(func(r rune) rune {
		r = unicode.ToLower(r)
		if r == 'ё' {
			r = 'е'
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return -1
	}, s)
}

func SameRecording(t Track, artist, title string) bool {
	if MusicKey(t.Title) != MusicKey(title) || MusicKey(artist) == "" {
		return false
	}
	for _, a := range t.Artists {
		if MusicKey(a) == MusicKey(artist) {
			return true
		}
	}
	return false
}

// Only a complete calendar date can establish a release in the last week.
func RecentRelease(date string, now time.Time) bool {
	if len(date) < 10 {
		return false
	}
	d, err := time.Parse("2006-01-02", date[:10])
	if err != nil {
		return false
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return !d.Before(today.AddDate(0, 0, -7)) && !d.After(today)
}

func RecentTracks(tracks []Track, now time.Time) []Track {
	out := make([]Track, 0, len(tracks))
	for _, t := range tracks {
		if RecentRelease(t.ReleaseDate, now) && IsSongDuration(t.DurationMs) {
			out = append(out, t)
		}
	}
	return out
}
