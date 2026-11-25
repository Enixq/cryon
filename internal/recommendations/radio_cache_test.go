package recommendations

import (
	"Cryon2/internal/domain"
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

type radioCacheStore struct {
	*fakeSettings
	cache map[string]string
}

func (s *radioCacheStore) RecoCacheGet(_ context.Context, key string) (string, bool, error) {
	v, ok := s.cache[key]
	return v, ok, nil
}
func (s *radioCacheStore) RecoCacheSet(_ context.Context, key, value string, _ time.Duration) error {
	s.cache[key] = value
	return nil
}

func TestWarmRadioAvoidsSourceRequestsAndKeepsPool(t *testing.T) {
	ctx := context.Background()
	st := &radioCacheStore{fakeSettings: newFakeSettings(), cache: map[string]string{}}
	seed := tr("Seed", "Seed", domain.ServiceYandex)
	seed.ID = "seed"
	pool := []domain.Track{}
	for i := 0; i < 20; i++ {
		track := tr(fmt.Sprint("Artist", i), fmt.Sprint("Song", i), domain.ServiceYandex)
		track.ID = fmt.Sprint(i)
		pool = append(pool, track)
	}
	key := "radio:v1:yandex:seed:ad50"
	raw, _ := json.Marshal(pool)
	st.cache[key] = string(raw)
	calls := 0
	e := &Engine{store: st, search: func(context.Context, string) ([]domain.Track, error) { calls++; return nil, nil }}
	first, err := e.RelatedTracks(ctx, seed, 6)
	if err != nil || len(first) != 6 || calls != 0 {
		t.Fatal("warm radio searched sources", len(first), calls, err)
	}
	second, err := e.RelatedTracks(ctx, seed, 6, first...)
	if err != nil || len(second) != 6 || calls != 0 {
		t.Fatal("pool shrank or radio searched again", len(second), calls, err)
	}
	var retained []domain.Track
	_ = json.Unmarshal([]byte(st.cache[key]), &retained)
	if len(retained) != 20 {
		t.Fatal("cached pool was truncated", len(retained))
	}
}
