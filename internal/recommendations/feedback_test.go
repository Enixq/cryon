package recommendations

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"Cryon2/internal/domain"
	"Cryon2/internal/store"
)

// fakeSettings — минимальное key-value хранилище для тестов оценок. Интерфейс
// store.Store большой, а движку от него здесь нужны только SettingGet/SettingSet,
// поэтому остальное берётся у встроенного (nil) интерфейса и не вызывается.
type fakeSettings struct {
	store.Store
	values map[string]string
}

func newFakeSettings() *fakeSettings {
	return &fakeSettings{values: map[string]string{}}
}

func (f *fakeSettings) SettingGet(_ context.Context, key string) (string, bool, error) {
	v, ok := f.values[key]
	return v, ok, nil
}

func (f *fakeSettings) SettingSet(_ context.Context, key string, value string) error {
	f.values[key] = value
	return nil
}

func TestNormalizeSources(t *testing.T) {
	got := normalizeSources(sourcePrefs{
		domain.ServiceYandex:     10,
		domain.ServiceSoundCloud: 5,
	})
	if got[domain.ServiceYandex] != 0.6 {
		t.Fatalf("максимум должен стать 0.6, got %v", got[domain.ServiceYandex])
	}
	if got[domain.ServiceSoundCloud] != 0.3 {
		t.Fatalf("половина максимума должна стать 0.3, got %v", got[domain.ServiceSoundCloud])
	}
	if len(normalizeSources(sourcePrefs{})) != 0 {
		t.Fatalf("пустой вход должен дать пустой результат")
	}
	// Отрицательных весов быть не может, но нулевой максимум не должен делить на 0.
	if len(normalizeSources(sourcePrefs{domain.ServiceLocal: 0})) != 0 {
		t.Fatalf("нулевой максимум должен дать пустой результат")
	}
}

func TestPickCandidateForPrefersHabitualSource(t *testing.T) {
	e := &Engine{}
	found := []domain.Track{
		tr("A", "sc", domain.ServiceSoundCloud),
		tr("A", "ya", domain.ServiceYandex),
	}

	// Без предпочтений — самый релевантный результат поиска.
	if best, ok := e.pickCandidateFor(found, nil); !ok || best.Title != "sc" {
		t.Fatalf("без prefs ожидали sc, got %v ok=%v", best.Title, ok)
	}

	// Пользователь слушает Yandex — вторая позиция перебивает первую.
	prefs := normalizeSources(sourcePrefs{domain.ServiceYandex: 10})
	if best, ok := e.pickCandidateFor(found, prefs); !ok || best.Title != "ya" {
		t.Fatalf("с prefs ожидали ya, got %v ok=%v", best.Title, ok)
	}

	// Предпочтение не должно вытаскивать явно нерелевантный результат из хвоста:
	// вес позиции 1/(i+1) на первых местах больше максимума prefs (0.6).
	long := []domain.Track{
		tr("A", "sc", domain.ServiceSoundCloud),
		tr("A", "sp", domain.ServiceSpotify),
		tr("A", "ya", domain.ServiceYandex),
	}
	if best, ok := e.pickCandidateFor(long, prefs); !ok || best.Title != "sc" {
		t.Fatalf("prefs не должны поднимать третий результат, got %v ok=%v", best.Title, ok)
	}
}

func TestOrderCandidatesForPrefersHabitualSource(t *testing.T) {
	e := &Engine{}
	found := []domain.Track{
		tr("A", "yt1", domain.ServiceYouTube),
		tr("A", "sc1", domain.ServiceSoundCloud),
		tr("A", "ya1", domain.ServiceYandex),
		tr("A", "sc2", domain.ServiceSoundCloud),
	}
	prefs := normalizeSources(sourcePrefs{domain.ServiceYandex: 10, domain.ServiceSoundCloud: 5})
	got := titles(e.orderCandidatesFor(found, prefs))
	// Yandex вперёд, порядок внутри одного источника сохраняется, YT исключён.
	want := []string{"ya1", "sc1", "sc2"}
	if !equalStrings(got, want) {
		t.Fatalf("порядок по предпочтениям: got %v, want %v", got, want)
	}
}

func TestBuildTasteUsesAllLocalSignals(t *testing.T) {
	now := time.Now().UnixMilli()
	e := &Engine{
		favFn: func(context.Context) ([]domain.Track, error) {
			return []domain.Track{tr("Fav", "f1", domain.ServiceYandex)}, nil
		},
		plFn: func(context.Context) ([]domain.UserPlaylist, error) {
			return []domain.UserPlaylist{{ID: "p1", Title: "Мой"}}, nil
		},
		plTracksFn: func(_ context.Context, id string) ([]domain.Track, error) {
			if id != "p1" {
				t.Fatalf("неожиданный плейлист %q", id)
			}
			return []domain.Track{tr("Pl", "p1t", domain.ServiceSpotify)}, nil
		},
		histFn: func(context.Context, int) ([]store.HistoryRow, error) {
			return []store.HistoryRow{
				{Track: tr("Hist", "h1", domain.ServiceSoundCloud), PlayedAtMs: now},
			}, nil
		},
	}

	got := e.buildTaste(context.Background())

	// Порядок весов: избранное (3) > плейлист (2) > история (~1).
	want := []string{"Fav", "Pl", "Hist"}
	names := make([]string, len(got.artists))
	for i, a := range got.artists {
		names[i] = a.name
	}
	if !equalStrings(names, want) {
		t.Fatalf("порядок артистов профиля: got %v, want %v", names, want)
	}

	// Все три трека считаются знакомыми и не должны попадать в подборки повторно.
	for _, key := range []string{"fav|f1", "pl|p1t", "hist|h1"} {
		if !got.seen[key] {
			t.Fatalf("трек %q должен быть в seen", key)
		}
	}

	// Источники: Yandex — максимум (3), значит 0.6; Spotify (2) и SoundCloud (~1) ниже.
	if got.sources[domain.ServiceYandex] != 0.6 {
		t.Fatalf("Yandex должен быть предпочтительным источником, got %v", got.sources)
	}
	if got.sources[domain.ServiceSpotify] <= got.sources[domain.ServiceSoundCloud] {
		t.Fatalf("плейлист должен весить больше истории: %v", got.sources)
	}
}

func TestSetFeedbackAffectsProfile(t *testing.T) {
	ctx := context.Background()
	st := newFakeSettings()
	e := &Engine{
		store: st,
		favFn: func(context.Context) ([]domain.Track, error) {
			return []domain.Track{tr("Keep", "k1", domain.ServiceYandex)}, nil
		},
	}

	// «Не нравится» дважды по разным трекам одного артиста — артист блокируется.
	if _, err := e.SetFeedback(ctx, "Bad", "b1", -1); err != nil {
		t.Fatalf("SetFeedback: %v", err)
	}
	if _, err := e.SetFeedback(ctx, "Bad", "b2", -1); err != nil {
		t.Fatalf("SetFeedback: %v", err)
	}
	profile := e.buildTaste(ctx)
	if !profile.blockedArtists["bad"] {
		t.Fatalf("после двух дизлайков артист должен блокироваться: %v", profile.blockedArtists)
	}
	if !profile.seen["bad|b1"] || !profile.seen["bad|b2"] {
		t.Fatalf("дизлайкнутые треки должны попадать в seen")
	}

	// «Нравится» добавляет артиста в профиль сильнее избранного.
	if _, err := e.SetFeedback(ctx, "Loved", "l1", 1); err != nil {
		t.Fatalf("SetFeedback: %v", err)
	}
	profile = e.buildTaste(ctx)
	if len(profile.artists) == 0 || profile.artists[0].name != "Loved" {
		t.Fatalf("лайкнутый артист должен быть первым в профиле: %+v", profile.artists)
	}

	// Повторный клик по той же кнопке снимает оценку и разблокирует артиста.
	if _, err := e.SetFeedback(ctx, "Bad", "b1", 0); err != nil {
		t.Fatalf("SetFeedback: %v", err)
	}
	if _, err := e.SetFeedback(ctx, "Bad", "b2", 0); err != nil {
		t.Fatalf("SetFeedback: %v", err)
	}
	profile = e.buildTaste(ctx)
	if profile.blockedArtists["bad"] {
		t.Fatalf("после снятия оценок артист не должен быть заблокирован")
	}

	// Rev растёт на каждое изменение — иначе кэш подборки не пересобрался бы.
	var saved feedback
	if err := json.Unmarshal([]byte(st.values[feedbackSettingKey]), &saved); err != nil {
		t.Fatalf("сохранённые оценки не разбираются: %v", err)
	}
	if saved.Rev != 5 {
		t.Fatalf("ожидали Rev=5 после пяти изменений, got %d", saved.Rev)
	}

	// Ключ кэша меняется вместе с версией оценок.
	first := e.recoCacheKey(taste{feedbackRev: 1}, nil, 20)
	second := e.recoCacheKey(taste{feedbackRev: 2}, nil, 20)
	if first == second {
		t.Fatalf("ключ кэша должен зависеть от версии оценок")
	}
}

func TestPersonalizeReleases(t *testing.T) {
	ctx := context.Background()
	st := newFakeSettings()
	e := &Engine{
		store: st,
		favFn: func(context.Context) ([]domain.Track, error) {
			return []domain.Track{tr("Known", "k1", domain.ServiceYandex)}, nil
		},
	}
	if _, err := e.SetFeedback(ctx, "Bad", "b1", -1); err != nil {
		t.Fatalf("SetFeedback: %v", err)
	}
	if _, err := e.SetFeedback(ctx, "Bad", "b2", -1); err != nil {
		t.Fatalf("SetFeedback: %v", err)
	}

	// Все релизы из одного сервиса, поэтому бонус источника одинаковый и на
	// порядок влияет только знакомство с исполнителем.
	in := []domain.Track{
		tr("Fresh", "n1", domain.ServiceSpotify),
		tr("Bad", "n2", domain.ServiceSpotify),
		tr("Known", "n3", domain.ServiceSpotify),
		tr("Other", "n4", domain.ServiceSpotify),
	}
	got := titles(e.PersonalizeReleases(ctx, in))
	// Знакомый исполнитель — наверх, заблокированный — исключён, остальные
	// сохраняют исходный порядок (сортировка стабильная).
	want := []string{"n3", "n1", "n4"}
	if !equalStrings(got, want) {
		t.Fatalf("радар под профиль: got %v, want %v", got, want)
	}

	// Пустой профиль (вкусы не набраны) не должен менять радар.
	empty := &Engine{}
	if untouched := titles(empty.PersonalizeReleases(ctx, in)); !equalStrings(untouched, titles(in)) {
		t.Fatalf("без профиля порядок должен сохраняться: got %v", untouched)
	}
}
