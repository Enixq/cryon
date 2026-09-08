package recommendations

import (
	"context"
	"testing"

	"Cryon2/internal/domain"
)

// RadarSeeds без Last.fm возвращает только знакомых артистов профиля: по убыванию
// веса и с дедупом без учёта регистра. Похожих без ключа Last.fm не добавляет.
func TestRadarSeedsFamiliarOnly(t *testing.T) {
	e := &Engine{
		favFn: func(context.Context) ([]domain.Track, error) {
			return []domain.Track{
				tr("Aurora", "a1", domain.ServiceYandex),
				tr("Aurora", "a2", domain.ServiceYandex), // тот же артист — дедуп
				tr("Boards", "b1", domain.ServiceYandex),
			}, nil
		},
	}

	got := e.RadarSeeds(context.Background(), 12)
	// Aurora весит больше (два трека), поэтому идёт первой; дубль схлопнут.
	want := []string{"Aurora", "Boards"}
	if !equalStrings(got, want) {
		t.Fatalf("засев по знакомым: got %v, want %v", got, want)
	}
}

// Пустой профиль → nil: засевать нечего, радар остаётся редакционным.
func TestRadarSeedsEmptyProfile(t *testing.T) {
	e := &Engine{}
	if got := e.RadarSeeds(context.Background(), 12); got != nil {
		t.Fatalf("без вкусов ожидали nil, got %v", got)
	}
}

// Лимит ограничивает общее число затравок.
func TestRadarSeedsRespectsLimit(t *testing.T) {
	e := &Engine{
		favFn: func(context.Context) ([]domain.Track, error) {
			return []domain.Track{
				tr("A", "1", domain.ServiceYandex),
				tr("B", "2", domain.ServiceYandex),
				tr("C", "3", domain.ServiceYandex),
			}, nil
		},
	}
	if got := e.RadarSeeds(context.Background(), 2); len(got) != 2 {
		t.Fatalf("ожидали 2 затравки, got %d (%v)", len(got), got)
	}
}
