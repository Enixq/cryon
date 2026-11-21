package domain

import (
	"testing"
	"time"
)

func TestRecentReleaseUsesExactCalendarDates(t *testing.T) {
	now := time.Date(2026, 11, 22, 13, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		date string
		want bool
	}{{"2026-11-21", true}, {"2026-11-15T00:00:00+03:00", true}, {"2026-11-14", false}, {"2026-11-23", false}, {"2026", false}, {"2026-11", false}, {"2026-02-30", false}, {"", false}} {
		if got := RecentRelease(tc.date, now); got != tc.want {
			t.Errorf("%s: got %v", tc.date, got)
		}
	}
}

func TestSongDurationBoundaries(t *testing.T) {
	for _, tc := range []struct {
		ms   int
		want bool
	}{{0, true}, {30_000, false}, {30_001, true}, {1199999, true}, {1200000, false}, {3600000, false}, {-1, false}} {
		if got := IsSongDuration(tc.ms); got != tc.want {
			t.Errorf("%d: %v", tc.ms, got)
		}
	}
}
