package recommendations

import (
	"Cryon2/internal/domain"
	"Cryon2/internal/store"
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"
)

type dailyCache struct {
	*fakeSettings
	cache map[string]string
}

func (s *dailyCache) RecoCacheGet(_ context.Context, k string) (string, bool, error) {
	v, ok := s.cache[k]
	return v, ok, nil
}
func (s *dailyCache) RecoCacheSet(_ context.Context, k, v string, _ time.Duration) error {
	s.cache[k] = v
	return nil
}

func TestDailyMixKeepsFavoritesAfterListeningAndAcrossReloads(t *testing.T) {
	ctx := context.Background()
	favs := []domain.Track{}
	for artist := 0; artist < 10; artist++ {
		for song := 0; song < 4; song++ {
			favs = append(favs, domain.Track{ID: fmt.Sprintf("%d-%d", artist, song), Service: domain.ServiceYandex, Title: fmt.Sprintf("Song %d", song), Artists: []string{fmt.Sprintf("Artist %d", artist)}, DurationMs: 180000})
		}
	}
	cache := &dailyCache{fakeSettings: newFakeSettings(), cache: map[string]string{}}
	var history []store.HistoryRow
	searches := 0
	e := &Engine{store: cache, favFn: func(context.Context) ([]domain.Track, error) { return favs, nil }, histFn: func(context.Context, int) ([]store.HistoryRow, error) { return history, nil }, search: func(context.Context, string) ([]domain.Track, error) { searches++; return nil, nil }}
	first, err := e.AutoMix(ctx, MixDaily, 30)
	if err != nil || len(first) != 30 {
		t.Fatalf("mix: %d %v", len(first), err)
	}
	for _, track := range first {
		history = append(history, store.HistoryRow{Track: track, PlayedAtMs: time.Now().UnixMilli()})
	}
	before := searches
	second, err := e.AutoMix(ctx, MixDaily, 30)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("listening drained/reordered mix: %d %v", len(second), err)
	}
	if searches != before {
		t.Fatal("cached mix unnecessarily rebuilt")
	}
	if _, err = e.SetFeedback(ctx, first[0].Artists[0], first[0].Title, -1); err != nil {
		t.Fatal(err)
	}
	third, err := e.AutoMix(ctx, MixDaily, 30)
	if err != nil || len(third) != 30 {
		t.Fatalf("dislike shrank mix: %d %v", len(third), err)
	}
	for _, track := range third {
		if trackKey(track) == trackKey(first[0]) {
			t.Fatal("disliked song returned")
		}
	}
}

func TestDailyMixFillsSmallLibraryWithOneArtist(t *testing.T) {
	favorites := []domain.Track{}
	for song := range 12 {
		favorites = append(favorites, domain.Track{ID: fmt.Sprint(song), Service: domain.ServiceLocal,
			Title: fmt.Sprintf("Song %d", song), Artists: []string{"Artist"}, DurationMs: 180000})
	}
	e := &Engine{favFn: func(context.Context) ([]domain.Track, error) { return favorites, nil },
		search: func(context.Context, string) ([]domain.Track, error) { return nil, nil }}
	mix, err := e.AutoMix(context.Background(), MixDaily, 10)
	if err != nil || len(mix) != 10 {
		t.Fatalf("artist diversity unexpectedly shrank mix: %d %v", len(mix), err)
	}
}
