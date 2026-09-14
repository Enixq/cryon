package recommendations

import (
	"context"
	"fmt"
	"testing"

	"Cryon2/internal/domain"
	"Cryon2/internal/store"
)

func artistNames(ws []artistWeight) []string {
	out := make([]string, len(ws))
	for i, w := range ws {
		out[i] = w.name
	}
	return out
}

// Холодный старт: пользователь только просканировал локальную папку (ни
// избранного, ни истории, ни плейлистов). Профиль всё равно должен наполниться
// артистами из локальной библиотеки, а сами локальные треки — попасть в seen,
// чтобы их не рекомендовали обратно.
func TestBuildTasteSeedsFromLocalLibrary(t *testing.T) {
	e := &Engine{
		localFn: func(context.Context) ([]store.LocalTrackRow, error) {
			return []store.LocalTrackRow{
				{Title: "l1", Artists: []string{"Local A"}},
				{Title: "l2", Artists: []string{"Local A"}},
				{Title: "l3", Artists: []string{"Local B"}},
			}, nil
		},
	}

	got := e.buildTaste(context.Background())

	if len(got.artists) != 2 {
		t.Fatalf("ожидали 2 локальных артиста в профиле, got %v", artistNames(got.artists))
	}
	haveA, haveB := false, false
	for _, a := range got.artists {
		switch a.name {
		case "Local A":
			haveA = true
		case "Local B":
			haveB = true
		}
		// Локальный вклад — плоский вес 1.5 на артиста (не на трек), чтобы
		// большая библиотека не раздувала профиль.
		if a.weight != 1.5 {
			t.Fatalf("локальный артист %q должен весить 1.5, got %v", a.name, a.weight)
		}
	}
	if !haveA || !haveB {
		t.Fatalf("оба локальных артиста должны быть в профиле, got %v", artistNames(got.artists))
	}
	for _, key := range []string{"local a|l1", "local a|l2", "local b|l3"} {
		if !got.seen[key] {
			t.Fatalf("локальный трек %q должен быть в seen", key)
		}
	}
}

// Локальная библиотека — сигнал слабее избранного и ограничена top-50 артистов
// по числу треков, чтобы 10k-библиотека не перебила осознанный выбор.
func TestLocalLibraryRanksBelowFavoritesAndCaps(t *testing.T) {
	// 60 локальных артистов: L00 — 60 треков, L59 — 1 трек (убывающе).
	rows := make([]store.LocalTrackRow, 0, 60*61/2)
	for k := 0; k < 60; k++ {
		name := fmt.Sprintf("L%02d", k)
		for n := 0; n < 60-k; n++ {
			rows = append(rows, store.LocalTrackRow{Title: fmt.Sprintf("%s-t%d", name, n), Artists: []string{name}})
		}
	}
	e := &Engine{
		favFn: func(context.Context) ([]domain.Track, error) {
			return []domain.Track{tr("Fav", "f1", domain.ServiceYandex)}, nil
		},
		localFn: func(context.Context) ([]store.LocalTrackRow, error) {
			return rows, nil
		},
	}

	got := e.buildTaste(context.Background())

	// Избранное (вес 3) обгоняет любой локальный сигнал (вес 1.5).
	if len(got.artists) == 0 || got.artists[0].name != "Fav" {
		t.Fatalf("избранное должно быть первым, got %v", artistNames(got.artists))
	}
	// Кап: не больше 50 локальных артистов (+ 1 избранный).
	localCount := 0
	kept := map[string]bool{}
	for _, a := range got.artists {
		if a.name != "Fav" {
			localCount++
			kept[a.name] = true
		}
	}
	if localCount != 50 {
		t.Fatalf("ожидали ровно 50 локальных артистов после капа, got %d", localCount)
	}
	// Отсекаются артисты с наименьшим числом треков (L50..L59).
	if !kept["L00"] {
		t.Fatal("самый представленный локальный артист (L00) должен остаться")
	}
	if kept["L59"] {
		t.Fatal("наименее представленный локальный артист (L59) должен быть отсечён капом")
	}
}
