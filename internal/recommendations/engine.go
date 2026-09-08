// Package recommendations — движок авто-предложек поверх мульти-сервисной
// библиотеки. Профиль вкусов строится из локальной истории и избранного
// (чистый Go), а «похожесть» артистов берётся из Last.fm — он знает артистов
// во всех источниках, поэтому рекомендации работают cross-service без обучения
// собственной модели.
//
// Уровни (см. plan.md, п.12):
//   - Уровень 1 — контентный фильтр: веса артистов из истории/избранного →
//     ранжирование кандидатов (частота, свежесть, исключение прослушанного).
//   - Fallback без Last.fm-ключа: те же любимые артисты через обычный поиск,
//     чтобы рекомендации работали хотя бы базово оффлайн.
package recommendations

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"Cryon2/internal/domain"
	"Cryon2/internal/logging"
	"Cryon2/internal/services/lastfm"
	"Cryon2/internal/store"
)

// SearchFunc — поиск трека по всем источникам (обычно App.SearchAll).
// Движок использует его, чтобы найти реальные проигрываемые треки по названиям
// от Last.fm.
type SearchFunc func(ctx context.Context, query string) ([]domain.Track, error)

// Engine — движок рекомендаций.
type Engine struct {
	store  store.Store
	lastfm *lastfm.Client
	search SearchFunc
	favFn  func(ctx context.Context) ([]domain.Track, error)
	histFn func(ctx context.Context, limit int) ([]store.HistoryRow, error)
	// Плейлисты — третий локальный сигнал вкуса наряду с избранным и историей:
	// добавить трек в свой плейлист это осознанный выбор, не случайное
	// прослушивание.
	plFn       func(ctx context.Context) ([]domain.UserPlaylist, error)
	plTracksFn func(ctx context.Context, playlistID string) ([]domain.Track, error)
}

// New собирает движок. store и search обязательны; lastfm может быть без ключа
// (тогда работает оффлайн-фолбэк).
func New(st store.Store, lf *lastfm.Client, search SearchFunc) *Engine {
	e := &Engine{store: st, lastfm: lf, search: search}
	if st != nil {
		e.favFn = st.FavoriteList
		e.histFn = st.HistoryList
		e.plFn = st.PlaylistList
		e.plTracksFn = st.PlaylistTracks
	}
	return e
}

// OnlineAvailable сообщает, доступен ли онлайн-движок (задан ли ключ Last.fm).
func (e *Engine) OnlineAvailable() bool {
	return e != nil && e.lastfm.Available()
}

// SetLastFMKey применяет ключ Last.fm в рантайме (из настроек). Пустая строка
// возвращает движок в оффлайн-режим.
func (e *Engine) SetLastFMKey(apiKey string) {
	if e == nil || e.lastfm == nil {
		return
	}
	e.lastfm.SetAPIKey(apiKey)
}

// sourcePrefs — насколько пользователь на самом деле слушает каждый источник.
// Одна и та же песня находится сразу в нескольких сервисах, и раньше движок
// брал просто самый релевантный результат поиска. Из-за этого рекомендации
// приходили из случайного сервиса: у человека подключён Yandex с прямым
// mp3-потоком, а трек подсовывался из SoundCloud. Теперь при прочих равных
// выбирается тот источник, из которого пользователь действительно слушает.
type sourcePrefs map[domain.ServiceID]float64

// pickCandidate выбирает лучший проигрываемый трек из результатов поиска для
// рекомендаций. Внутри отобранного множества сохраняется исходный порядок
// релевантности поиска (found[0] — самый релевантный), поэтому возвращаем
// первый подходящий.
func (e *Engine) pickCandidate(found []domain.Track) (domain.Track, bool) {
	return e.pickCandidateFor(found, nil)
}

// pickCandidateFor — pickCandidate с учётом предпочитаемых источников. Порог в
// prefs намеренно грубый: если у одного из кандидатов источник заметно «ближе»
// пользователю, берём его, иначе остаёмся на самом релевантном результате
// поиска. Без prefs (nil) поведение полностью совпадает с pickCandidate.
//
// YouTube Music из рекомендаций исключён полностью (см. plan.md, §30.2): его
// музыку можно искать и слушать, но в подборки и радио она не попадает — выдача
// YT шумная (каверы, лайвы, «topic»-дубликаты) и засоряет профиль тем, что
// пользователь на самом деле не слушает.
func (e *Engine) pickCandidateFor(found []domain.Track, prefs sourcePrefs) (domain.Track, bool) {
	if len(found) == 0 {
		return domain.Track{}, false
	}
	best := domain.Track{}
	haveBest := false
	bestScore := 0.0
	for i, t := range found {
		if t.Service == domain.ServiceYouTube {
			continue
		}
		// Релевантность поиска — основа порядка: чем дальше в выдаче, тем меньше
		// базовый вес. Предпочтение источника только доворачивает выбор. Индекс i
		// берётся из исходной выдачи (с пропущенными YT), поэтому позиционный вес
		// не-YT кандидатов не смещается.
		score := 1.0/float64(i+1) + prefs[t.Service]
		if !haveBest || score > bestScore {
			best, bestScore, haveBest = t, score, true
		}
	}
	return best, haveBest
}

// orderCandidates упорядочивает список кандидатов для многоэлементных подборок
// (ещё треки того же/любимого артиста). Треки YouTube Music из рекомендаций
// исключаются полностью (см. pickCandidateFor). Порядок остальных сохраняется.
func (e *Engine) orderCandidates(found []domain.Track) []domain.Track {
	return e.orderCandidatesFor(found, nil)
}

// orderCandidatesFor — orderCandidates с приоритетом привычных источников.
// Сортировка стабильная, поэтому с пустыми prefs порядок выдачи поиска
// сохраняется полностью (за вычетом отброшенных YT-треков).
func (e *Engine) orderCandidatesFor(found []domain.Track, prefs sourcePrefs) []domain.Track {
	if len(found) == 0 {
		return found
	}
	out := make([]domain.Track, 0, len(found))
	for _, t := range found {
		if t.Service == domain.ServiceYouTube {
			continue
		}
		out = append(out, t)
	}
	if len(prefs) > 0 {
		sort.SliceStable(out, func(i, j int) bool { return prefs[out[i].Service] > prefs[out[j].Service] })
	}
	return out
}

// artistWeight — накопленный вес артиста в профиле вкусов.
type artistWeight struct {
	name       string
	weight     float64
	lastPlayed int64
}

// taste — профиль вкусов, собранный из всех локальных сигналов сразу:
// избранное, пользовательские плейлисты, история и явные оценки рекомендаций.
type taste struct {
	// artists — любимые исполнители по убыванию веса (затравки для Last.fm).
	artists []artistWeight
	// seen — что уже слушали/лайкали: такое не рекомендуем повторно. Сюда же
	// попадают треки с дизлайком.
	seen map[string]bool
	// sources — насколько пользователь пользуется каждым источником. Нужен,
	// чтобы рекомендованный трек приходил из привычного сервиса.
	sources sourcePrefs
	// blockedArtists — исполнители, набравшие достаточно дизлайков.
	blockedArtists map[string]bool
	// feedbackRev — версия оценок, входит в ключ кэша подборки: после оценки
	// подборка должна пересобраться, не дожидаясь истечения TTL.
	feedbackRev int
}

// buildTaste строит профиль вкусов из всех локальных сигналов. Избранное весит
// больше истории, треки из своих плейлистов — между ними, свежие прослушивания
// получают надбавку за свежесть. Явные оценки («нравится»/«не нравится» на
// карточке рекомендации) усиливают или полностью исключают исполнителя.
func (e *Engine) buildTaste(ctx context.Context) taste {
	weights := map[string]*artistWeight{}
	seenTrackKeys := map[string]bool{}
	sources := sourcePrefs{}

	add := func(name string, w float64, playedAt int64) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		key := strings.ToLower(name)
		cur, ok := weights[key]
		if !ok {
			cur = &artistWeight{name: name}
			weights[key] = cur
		}
		cur.weight += w
		if playedAt > cur.lastPlayed {
			cur.lastPlayed = playedAt
		}
	}

	// Избранное — сильный сигнал вкуса.
	if e.favFn != nil {
		if favs, err := e.favFn(ctx); err == nil {
			for _, t := range favs {
				seenTrackKeys[trackKey(t)] = true
				sources[t.Service] += 3.0
				for _, a := range t.Artists {
					add(a, 3.0, 0)
				}
			}
		}
	}

	// Пользовательские плейлисты — тоже осознанный выбор, но слабее избранного.
	// Треки плейлистов не помечаются как seen: собранный руками плейлист не
	// означает «уже надоело», а вот рекомендовать ровно эти треки смысла нет,
	// поэтому в seen они всё же попадают — иначе подборка повторяет плейлист.
	if e.plFn != nil && e.plTracksFn != nil {
		if lists, err := e.plFn(ctx); err == nil {
			for i, pl := range lists {
				// Плейлистов у пользователя может быть много, а каждый — отдельный
				// запрос к SQLite. Профиль строится на каждую сборку подборки,
				// поэтому ограничиваемся первыми двадцатью.
				if i >= 20 {
					break
				}
				items, tErr := e.plTracksFn(ctx, pl.ID)
				if tErr != nil {
					continue
				}
				for _, t := range items {
					seenTrackKeys[trackKey(t)] = true
					sources[t.Service] += 2.0
					for _, a := range t.Artists {
						add(a, 2.0, 0)
					}
				}
			}
		}
	}

	// История — частота прослушивания + свежесть.
	if e.histFn != nil {
		if rows, err := e.histFn(ctx, 300); err == nil {
			now := time.Now().UnixMilli()
			for _, r := range rows {
				seenTrackKeys[trackKey(r.Track)] = true
				// Свежие прослушивания весят больше (спад за ~30 дней).
				ageDays := float64(now-r.PlayedAtMs) / float64(24*3600*1000)
				recency := 1.0
				if ageDays > 0 {
					recency = 1.0 / (1.0 + ageDays/30.0)
				}
				sources[r.Track.Service] += recency
				for _, a := range r.Track.Artists {
					add(a, 1.0*recency, r.PlayedAtMs)
				}
			}
		}
	}

	// Явные оценки рекомендаций.
	fb := e.loadFeedback(ctx)
	blocked := map[string]bool{}
	for key, score := range fb.Tracks {
		if score < 0 {
			// Дизлайкнутый трек не должен возвращаться ни в одной подборке.
			seenTrackKeys[key] = true
		}
	}
	for name, score := range fb.Artists {
		// Имя в оценках нормализовано (нижний регистр). Для затравки Last.fm
		// берём исходное написание, если оно сохранилось.
		display := name
		if saved := strings.TrimSpace(fb.Names[name]); saved != "" {
			display = saved
		}
		switch {
		case score <= artistBlockScore:
			// Набралось достаточно дизлайков — исполнитель уходит из подборок
			// целиком, включая затравки.
			blocked[name] = true
			delete(weights, name)
		case score > 0:
			// Лайк рекомендации — самый прямой сигнал вкуса, сильнее избранного:
			// пользователь одобрил именно предложенное, а не то, что уже знал.
			add(display, 4.0*float64(score), 0)
		}
	}

	out := make([]artistWeight, 0, len(weights))
	for _, w := range weights {
		out = append(out, *w)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].weight != out[j].weight {
			return out[i].weight > out[j].weight
		}
		return out[i].lastPlayed > out[j].lastPlayed
	})

	return taste{
		artists:        out,
		seen:           seenTrackKeys,
		sources:        normalizeSources(sources),
		blockedArtists: blocked,
		feedbackRev:    fb.Rev,
	}
}

// normalizeSources приводит веса источников к диапазону 0..0.6. Верхняя граница
// подобрана так, чтобы предпочтение источника могло переставить кандидатов
// внутри выдачи поиска, но не перебило разницу между первым и явно нерелевантным
// результатом (базовый вес позиции — 1/(i+1)).
func normalizeSources(in sourcePrefs) sourcePrefs {
	peak := 0.0
	for _, v := range in {
		if v > peak {
			peak = v
		}
	}
	if peak <= 0 {
		return sourcePrefs{}
	}
	out := make(sourcePrefs, len(in))
	for svc, v := range in {
		out[svc] = 0.6 * v / peak
	}
	return out
}

// PersonalizeReleases превращает «Радар новинок» из редакционной витрины в
// подборку под вкус: релизы исполнителей, которых пользователь слушает
// (familiar), и артистов, ПОХОЖИХ на них по Last.fm (similar), поднимаются
// наверх, а всё остальное — editorial-мейнстрим, не имеющий отношения к вкусу, —
// отсеивается. Раньше метод только пересортировывал список, поэтому чужой
// редакционный поток (шансон/поп из ленты сервиса) оставался в радаре, просто
// внизу — ровно на это и жаловался пользователь.
//
// Деградация без потери работоспособности:
//   - нет вкусов (профиль пуст) → радар остаётся как есть, персонализировать
//     нечего;
//   - вкусы есть, но после фильтра ничего не осталось (например, ключа Last.fm
//     нет, а редакционная лента вообще не пересекается со знакомыми артистами) →
//     возвращаем пересортированный полный список, чтобы секция не была пустой.
//
// Профиль строится по локальным данным (SQLite); similar-артисты берутся из
// кэша Last.fm (TTL сутки), поэтому повторные сборки радара сетевых запросов не
// делают.
func (e *Engine) PersonalizeReleases(ctx context.Context, in []domain.Track) []domain.Track {
	if e == nil || len(in) == 0 {
		return in
	}
	profile := e.buildTaste(ctx)
	if len(profile.artists) == 0 && len(profile.blockedArtists) == 0 {
		// Вкусы ещё не набраны — радар остаётся как есть.
		return in
	}

	// Абсолютные веса профиля зависят от объёма истории, поэтому нормируем к
	// 0..1 по максимуму: важен только относительный порядок «знакомости».
	familiar := make(map[string]float64, len(profile.artists))
	if len(profile.artists) > 0 {
		if peak := profile.artists[0].weight; peak > 0 {
			for _, a := range profile.artists {
				familiar[strings.ToLower(strings.TrimSpace(a.name))] = a.weight / peak
			}
		}
	}

	// Похожие артисты для верхних затравок профиля. Дают «широту вкуса»: новинки
	// артистов, которых пользователь ещё не слушал, но которые близки любимым.
	// Ограничиваем число затравок, чтобы при холодном кэше не сделать десятки
	// запросов к Last.fm; каждый ответ кэшируется на сутки.
	const maxSeeds = 12
	seeds := profile.artists
	if len(seeds) > maxSeeds {
		seeds = seeds[:maxSeeds]
	}
	similar := make(map[string]float64, len(seeds)*8)
	// Similar-артистов запрашиваем только когда Last.fm доступен: без ключа
	// SimilarArtists всё равно вернёт ошибку, а обращаться к кэшу store смысла
	// нет. Именно наличие similar-набора решает, фильтровать ли радар (ниже).
	if e.lastfm != nil && len(seeds) > 0 {
		perSeed := make([][]lastfm.Artist, len(seeds))
		var wg sync.WaitGroup
		for i, s := range seeds {
			wg.Add(1)
			go func(i int, name string) {
				defer wg.Done()
				perSeed[i] = e.similarArtistsCached(ctx, name, 8)
			}(i, s.name)
		}
		wg.Wait()
		for _, arts := range perSeed {
			for _, a := range arts {
				key := strings.ToLower(strings.TrimSpace(a.Name))
				if key == "" || familiar[key] > 0 {
					continue
				}
				// Близость Last.fm (0..1) масштабируем ниже знакомых артистов:
				// знакомый всегда важнее просто похожего.
				if score := 0.5 * a.Match; score > similar[key] {
					similar[key] = score
				}
			}
		}
	}

	type scored struct {
		track domain.Track
		score float64
	}
	relevant := make([]scored, 0, len(in))
	reordered := make([]scored, 0, len(in))
	for _, t := range in {
		key := artistKey(t)
		if profile.blockedArtists[key] {
			continue
		}
		famScore := familiar[key]
		simScore := similar[key]
		full := famScore + simScore + profile.sources[t.Service]
		reordered = append(reordered, scored{track: t, score: full})
		if famScore > 0 || simScore > 0 {
			relevant = append(relevant, scored{track: t, score: full})
		}
	}

	// Фильтруем радар до релевантных исполнителей ТОЛЬКО когда есть сигнал
	// «похожести» от Last.fm: тогда можно уверенно отбросить редакционный
	// мейнстрим, оставив знакомых и близких артистов. Без Last.fm сигнала для
	// фильтрации нет — просто пересортировываем полный список (как раньше),
	// иначе радар схлопнулся бы до горстки уже знакомых артистов. Пустой
	// relevant при живом Last.fm — тоже повод не оставлять секцию пустой.
	pick := reordered
	if len(similar) > 0 && len(relevant) > 0 {
		pick = relevant
	}
	sort.SliceStable(pick, func(i, j int) bool { return pick[i].score > pick[j].score })

	out := make([]domain.Track, len(pick))
	for i, s := range pick {
		out[i] = s.track
	}
	return out
}

// RadarSeeds возвращает имена исполнителей для «засева» Радара новинок по вкусу:
// верхние знакомые артисты профиля плюс похожие на них (Last.fm, из кэша). Порядок
// значимый — сначала знакомые (по убыванию веса), затем похожие; дедуп без учёта
// регистра, заблокированные (дизлайкнутые) артисты исключены. Пустой список,
// если вкусы ещё не набраны: тогда засевать нечего и радар остаётся редакционным.
//
// В отличие от PersonalizeReleases, которая лишь пересортировывает готовую ленту,
// эти имена нужны, чтобы адаптеры-источники (domain.ArtistReleaser) сходили за
// свежими релизами именно этих артистов. Похожих берём только при живом Last.fm:
// без него сеять можно лишь тем, кого пользователь уже слушает.
func (e *Engine) RadarSeeds(ctx context.Context, maxSeeds int) []string {
	if e == nil {
		return nil
	}
	profile := e.buildTaste(ctx)
	if len(profile.artists) == 0 {
		return nil
	}
	if maxSeeds <= 0 {
		maxSeeds = 12
	}

	seen := map[string]bool{}
	out := make([]string, 0, maxSeeds)
	// add возвращает false, когда список заполнен — сигнал прекратить засев.
	add := func(name string) bool {
		name = strings.TrimSpace(name)
		if name == "" {
			return len(out) < maxSeeds
		}
		key := strings.ToLower(name)
		if seen[key] || profile.blockedArtists[key] {
			return len(out) < maxSeeds
		}
		seen[key] = true
		out = append(out, name)
		return len(out) < maxSeeds
	}

	// Ядро засева — верхние знакомые артисты. Их же используем как затравки для
	// похожих, поэтому ограничиваем отдельно от общего лимита.
	const maxFamiliar = 8
	familiar := profile.artists
	if len(familiar) > maxFamiliar {
		familiar = familiar[:maxFamiliar]
	}
	for _, a := range familiar {
		if !add(a.name) {
			return out
		}
	}

	// Похожие артисты добавляют широты (новинки тех, кого пользователь ещё не
	// слушал, но кто близок любимым). Только при доступном Last.fm — иначе
	// similarArtistsCached вернёт пусто и запрос к сети смысла не имеет.
	if e.lastfm != nil {
		for _, a := range familiar {
			for _, sim := range e.similarArtistsCached(ctx, a.name, 4) {
				if !add(sim.Name) {
					return out
				}
			}
		}
	}
	return out
}

// artistBlockScore — при какой сумме оценок исполнитель исключается из
// подборок целиком. Один дизлайк — это «не этот трек», два и больше — уже
// «не этот исполнитель».
const artistBlockScore = -2

// feedbackSettingKey — ключ настройки, в которой лежат оценки рекомендаций.
// Оценки — это несколько десятков пар «ключ→оценка», поэтому отдельная таблица
// не нужна: храним JSON в существующем key-value хранилище настроек и не
// добавляем миграцию схемы.
const feedbackSettingKey = "reco.feedback"

// feedback — явные оценки рекомендаций.
type feedback struct {
	// Tracks — оценка конкретного трека по ключу «исполнитель|название».
	Tracks map[string]int `json:"tracks"`
	// Artists — накопленная оценка исполнителя (сумма оценок его треков).
	Artists map[string]int `json:"artists"`
	// Names — исходное написание имени по нормализованному ключу. Лайкнутый
	// артист попадает в профиль только через оценки, а его имя уходит затравкой
	// в Last.fm и в логи, поэтому храним его как есть, а не в нижнем регистре.
	Names map[string]string `json:"names,omitempty"`
	// Rev — счётчик изменений: попадает в ключ кэша подборки, поэтому оценка
	// сразу пересобирает рекомендации, не дожидаясь TTL.
	Rev int `json:"rev"`
}

// feedbackArtistKey/feedbackTrackKey — та же нормализация, что и в trackKey,
// чтобы ключи из UI и из профиля совпадали.
func feedbackArtistKey(artist string) string {
	return strings.ToLower(strings.TrimSpace(artist))
}

func feedbackTrackKey(artist, title string) string {
	return strings.ToLower(strings.TrimSpace(artist) + "|" + strings.TrimSpace(title))
}

// loadFeedback читает оценки. Ошибки и битый JSON трактуются как «оценок нет»:
// рекомендации должны работать и без них.
func (e *Engine) loadFeedback(ctx context.Context) feedback {
	fb := feedback{Tracks: map[string]int{}, Artists: map[string]int{}, Names: map[string]string{}}
	if e == nil || e.store == nil {
		return fb
	}
	raw, ok, err := e.store.SettingGet(ctx, feedbackSettingKey)
	if err != nil || !ok || strings.TrimSpace(raw) == "" {
		return fb
	}
	var parsed feedback
	if json.Unmarshal([]byte(raw), &parsed) != nil {
		return fb
	}
	if parsed.Tracks == nil {
		parsed.Tracks = map[string]int{}
	}
	if parsed.Artists == nil {
		parsed.Artists = map[string]int{}
	}
	if parsed.Names == nil {
		parsed.Names = map[string]string{}
	}
	return parsed
}

// SetFeedback сохраняет оценку рекомендации: score > 0 — «нравится», score < 0 —
// «не нравится», score == 0 — снять оценку. Возвращает актуальную карту оценок
// по трекам, чтобы UI не делал второй запрос.
//
// Оценка трека переносится и на исполнителя: накопленный минус исключает его из
// подборок (см. artistBlockScore), плюс — усиливает в профиле вкусов.
func (e *Engine) SetFeedback(ctx context.Context, artist, title string, score int) (map[string]int, error) {
	if e == nil || e.store == nil {
		return map[string]int{}, nil
	}
	tKey := feedbackTrackKey(artist, title)
	if tKey == "|" {
		return e.FeedbackState(ctx)
	}
	if score > 1 {
		score = 1
	}
	if score < -1 {
		score = -1
	}

	fb := e.loadFeedback(ctx)
	prev := fb.Tracks[tKey]
	if prev == score {
		return fb.Tracks, nil
	}
	if score == 0 {
		delete(fb.Tracks, tKey)
	} else {
		fb.Tracks[tKey] = score
	}
	// Вес исполнителя двигаем на разницу — повторный клик по той же кнопке не
	// накапливает оценку, а смена «нравится» на «не нравится» сдвигает сразу на
	// два шага.
	if aKey := feedbackArtistKey(artist); aKey != "" {
		fb.Artists[aKey] += score - prev
		if fb.Artists[aKey] == 0 {
			delete(fb.Artists, aKey)
			delete(fb.Names, aKey)
		} else {
			if fb.Names == nil {
				fb.Names = map[string]string{}
			}
			fb.Names[aKey] = strings.TrimSpace(artist)
		}
	}
	fb.Rev++

	data, err := json.Marshal(fb)
	if err != nil {
		return fb.Tracks, err
	}
	if err := e.store.SettingSet(ctx, feedbackSettingKey, string(data)); err != nil {
		return fb.Tracks, err
	}
	return fb.Tracks, nil
}

// FeedbackState отдаёт оценки по трекам для подсветки кнопок в UI.
func (e *Engine) FeedbackState(ctx context.Context) (map[string]int, error) {
	if e == nil {
		return map[string]int{}, nil
	}
	return e.loadFeedback(ctx).Tracks, nil
}

// recoTTL — срок жизни готовой подборки в reco_cache. Пересборка подборки —
// это десятки поисков по всем источникам, поэтому результат кэшируется целиком,
// а не только ответы Last.fm. Компромисс: треки, прослушанные за последние
// recoTTL, могут ещё попадать в выдачу (обычно их и так отфильтрует профиль на
// следующей пересборке).
const recoTTL = 3 * time.Hour

// searchConcurrency — сколько кандидатов Last.fm ищется в источниках
// одновременно. Каждый поиск сам веером идёт по ~6 сервисам, поэтому держим
// число небольшим: иначе внешние API отвечают 429.
const searchConcurrency = 4

// profileSignature — стабильный отпечаток вкусов для ключа кэша подборки.
// Веса артистов меняются после каждого прослушивания, поэтому берём только
// порядок топовых имён: он меняется заметно реже, но реагирует на смену вкуса.
func profileSignature(seeds []artistWeight) string {
	names := make([]string, 0, len(seeds))
	for _, s := range seeds {
		names = append(names, strings.ToLower(strings.TrimSpace(s.name)))
	}
	return strings.Join(names, ",")
}

// recoCacheKey учитывает всё, что меняет выдачу: профиль, лимит, доступность
// онлайн-режима и версию оценок пользователя. Смена любого из них даёт новый
// ключ, поэтому отдельная инвалидация кэша не нужна. Версия v3 — с полным
// исключением YouTube Music из кандидатов (записи v2 могли содержать YT-треки).
func (e *Engine) recoCacheKey(t taste, seeds []artistWeight, limit int) string {
	mode := "offline"
	if e.lastfm.Available() {
		mode = "online"
	}
	return "reco:v3:" + mode + ":fb" + strconv.Itoa(t.feedbackRev) +
		":n" + strconv.Itoa(limit) + ":" + profileSignature(seeds)
}

// Recommendations возвращает до limit рекомендованных треков: похожие артисты
// (Last.fm) к любимым, найденные в реальных источниках, с исключением уже
// прослушанного. При отсутствии ключа Last.fm — оффлайн-фолбэк по своим
// артистам. Готовая подборка кэшируется на recoTTL: главный экран запрашивает
// рекомендации и обе авто-подборки сразу, а каждая пересборка — это десятки
// сетевых поисков.
func (e *Engine) Recommendations(ctx context.Context, limit int) ([]domain.Track, error) {
	if limit <= 0 {
		limit = 20
	}
	profile := e.buildTaste(ctx)
	if len(profile.artists) == 0 {
		return []domain.Track{}, nil
	}

	// Берём топ любимых артистов как «затравку».
	seeds := profile.artists
	if len(seeds) > 5 {
		seeds = seeds[:5]
	}

	cacheKey := e.recoCacheKey(profile, seeds, limit)
	if e.store != nil {
		if raw, ok, _ := e.store.RecoCacheGet(ctx, cacheKey); ok {
			var cached []domain.Track
			if json.Unmarshal([]byte(raw), &cached) == nil && len(cached) > 0 {
				return cached, nil
			}
		}
	}

	var tracks []domain.Track
	if e.lastfm.Available() {
		tracks = e.onlineRecommendations(ctx, profile, seeds, limit)
		if len(tracks) == 0 {
			logging.L().Debug("recommendations: онлайн-движок пуст, откат в оффлайн")
		}
	}
	if len(tracks) == 0 {
		tracks = e.offlineRecommendations(ctx, profile, seeds, limit)
	}

	if len(tracks) > 0 && e.store != nil {
		if data, err := json.Marshal(tracks); err == nil {
			_ = e.store.RecoCacheSet(ctx, cacheKey, string(data), recoTTL)
		}
	}
	return tracks, nil
}

// RelatedTracks возвращает до limit треков, похожих на конкретный seed —
// основа «радио»/умной очереди. Похожесть берётся из Last.fm similar tracks
// (по паре артист+название seed'а), а найденные названия ищутся в источниках.
// Без ключа Last.fm — фолбэк: ещё треки того же артиста.
func (e *Engine) RelatedTracks(ctx context.Context, seed domain.Track, limit int) ([]domain.Track, error) {
	if limit <= 0 {
		limit = 15
	}
	artist := ""
	if len(seed.Artists) > 0 {
		artist = seed.Artists[0]
	}
	if strings.TrimSpace(artist) == "" && strings.TrimSpace(seed.Title) == "" {
		return []domain.Track{}, nil
	}

	seen := map[string]bool{trackKey(seed): true}
	out := make([]domain.Track, 0, limit)

	// Радио тоже подчиняется профилю: трек берём из привычного пользователю
	// источника, а заблокированных исполнителей не подставляем. Уже
	// прослушанное здесь, в отличие от подборок, не исключаем — «похожее на
	// этот трек» вполне может быть знакомым.
	profile := e.buildTaste(ctx)

	if e.lastfm.Available() {
		similar := e.similarTracksCached(ctx, artist, seed.Title, limit*2)
		// Поиск волнами: радио дозаполняет очередь в момент, когда трек
		// закончился, поэтому последовательные limit*2 поисков давали слышимую
		// паузу. Порядок похожести сохраняется — волна разбирается по индексам.
		for start := 0; start < len(similar) && len(out) < limit; start += searchConcurrency {
			end := start + searchConcurrency
			if end > len(similar) {
				end = len(similar)
			}
			batch := similar[start:end]
			results := make([][]domain.Track, len(batch))
			var wg sync.WaitGroup
			for i, sim := range batch {
				query := strings.TrimSpace(sim.Artist + " " + sim.Name)
				if query == "" {
					continue
				}
				wg.Add(1)
				go func(i int, query string) {
					defer wg.Done()
					found, err := e.search(ctx, query)
					if err != nil {
						return
					}
					results[i] = found
				}(i, query)
			}
			wg.Wait()

			for _, found := range results {
				if len(out) >= limit {
					break
				}
				if len(found) == 0 {
					continue
				}
				best, ok := e.pickCandidateFor(found, profile.sources)
				if !ok {
					continue
				}
				if profile.blockedArtists[artistKey(best)] {
					continue
				}
				key := trackKey(best)
				if seen[key] {
					continue
				}
				seen[key] = true
				out = append(out, best)
			}
		}
		if len(out) > 0 {
			return out, nil
		}
	}

	// Фолбэк: ещё треки того же артиста.
	if strings.TrimSpace(artist) != "" {
		found, err := e.search(ctx, artist)
		if err == nil {
			for _, t := range e.orderCandidatesFor(found, profile.sources) {
				if len(out) >= limit {
					break
				}
				if profile.blockedArtists[artistKey(t)] {
					continue
				}
				key := trackKey(t)
				if seen[key] {
					continue
				}
				seen[key] = true
				out = append(out, t)
			}
		}
	}
	return out, nil
}

// similarTracksCached — SimilarTracks с кэшем в SQLite (TTL сутки).
func (e *Engine) similarTracksCached(ctx context.Context, artist, title string, limit int) []lastfm.Track {
	key := "lastfm:similartracks:" + strings.ToLower(artist+"|"+title)
	if e.store != nil {
		if raw, ok, _ := e.store.RecoCacheGet(ctx, key); ok {
			var cached []lastfm.Track
			if json.Unmarshal([]byte(raw), &cached) == nil {
				return cached
			}
		}
	}
	res, err := e.lastfm.SimilarTracks(ctx, artist, title, limit)
	if err != nil {
		logging.L().Debug("recommendations: similar tracks не удалось", "artist", artist, "title", title, "err", err)
		return nil
	}
	if e.store != nil {
		if data, mErr := json.Marshal(res); mErr == nil {
			_ = e.store.RecoCacheSet(ctx, key, string(data), 24*time.Hour)
		}
	}
	return res
}

// onlineRecommendations: Last.fm similar artists → их топ-треки → поиск в
// источниках. Кэш ответов Last.fm живёт в SQLite (сутки), готовая подборка —
// в reco_cache на recoTTL (см. Recommendations). Профиль t здесь нужен трижды:
// исключить уже слышанное (t.seen), выбрать источник кандидата по привычкам
// пользователя (t.sources) и отбросить заблокированных исполнителей.
func (e *Engine) onlineRecommendations(ctx context.Context, t taste, seeds []artistWeight, limit int) []domain.Track {
	// Собираем кандидатов-названий (artist + track) из похожих артистов.
	var cands []scoredCand
	seedSet := map[string]bool{}
	for _, s := range seeds {
		seedSet[strings.ToLower(s.name)] = true
	}

	// Фаза 1: похожие артисты для всех затравок сразу. Один запрос на затравку,
	// поэтому параллелим целиком.
	similarBySeed := make([][]lastfm.Artist, len(seeds))
	var wgSimilar sync.WaitGroup
	for i, seed := range seeds {
		wgSimilar.Add(1)
		go func(i int, name string) {
			defer wgSimilar.Done()
			similarBySeed[i] = e.similarArtistsCached(ctx, name, 8)
		}(i, seed.name)
	}
	wgSimilar.Wait()

	// Фаза 2: топ-треки похожих артистов. Разные затравки часто дают одних и тех
	// же похожих — дедуплицируем имена, чтобы не запрашивать один артист дважды.
	// Раньше обе фазы шли строго последовательно: ~45 запросов к Last.fm подряд.
	uniqueArtists := make([]string, 0, len(seeds)*8)
	indexByArtist := map[string]int{}
	for _, similar := range similarBySeed {
		for _, sim := range similar {
			key := strings.ToLower(sim.Name)
			if seedSet[key] {
				continue
			}
			// Заблокированному исполнителю даже топ-треки не запрашиваем: это
			// экономит обращения к Last.fm, а не только фильтрует выдачу.
			if t.blockedArtists[strings.TrimSpace(key)] {
				continue
			}
			if _, ok := indexByArtist[key]; ok {
				continue
			}
			indexByArtist[key] = len(uniqueArtists)
			uniqueArtists = append(uniqueArtists, sim.Name)
		}
	}
	topByArtist := make([][]lastfm.Track, len(uniqueArtists))
	for start := 0; start < len(uniqueArtists); start += searchConcurrency {
		end := start + searchConcurrency
		if end > len(uniqueArtists) {
			end = len(uniqueArtists)
		}
		var wgTop sync.WaitGroup
		for i := start; i < end; i++ {
			wgTop.Add(1)
			go func(i int) {
				defer wgTop.Done()
				topByArtist[i] = e.topTracksCached(ctx, uniqueArtists[i], 3)
			}(i)
		}
		wgTop.Wait()
	}

	// Контентный слой (задача 39): теговый портрет вкуса и близость к нему
	// каждого артиста-кандидата. Строится по тем же уникальным именам, для
	// которых уже тянулись топ-треки, плюс затравки. При отсутствии ключа/тегов
	// все близости нулевые и ранжирование остаётся прежним.
	tagProf := e.buildTagProfile(ctx, seeds, uniqueArtists)

	// Фаза 3: веса. Порядок обхода — как раньше (по затравкам, затем по похожим),
	// поэтому при равных score выдача остаётся стабильной.
	for si, seed := range seeds {
		for _, sim := range similarBySeed[si] {
			// Не рекомендуем самих затравочных артистов.
			if seedSet[strings.ToLower(sim.Name)] {
				continue
			}
			// Исполнитель с накопленными дизлайками отбрасывается ещё до поиска:
			// это экономит запросы к источникам, а не только фильтрует выдачу.
			if t.blockedArtists[strings.ToLower(strings.TrimSpace(sim.Name))] {
				continue
			}
			idx, ok := indexByArtist[strings.ToLower(sim.Name)]
			if !ok {
				continue
			}
			// Теговая близость артиста-кандидата к профилю вкуса домешивается
			// множителем: жанрово «свой» кандидат поднимается, чуждый — остаётся
			// на базовой близости Last.fm.
			tagBoost := 1.0 + tagWeight*tagProf.affinity(sim.Name)
			for i, tr := range topByArtist[idx] {
				if t.blockedArtists[strings.ToLower(strings.TrimSpace(tr.Artist))] {
					continue
				}
				// Чем выше близость артиста и позиция трека — тем больше вес.
				score := seed.weight * (sim.Match + 0.1) * (1.0 / float64(i+1)) * tagBoost
				cands = append(cands, scoredCand{artist: tr.Artist, title: tr.Name, score: score})
			}
		}
	}

	return e.resolveCandidates(ctx, cands, t, limit)
}

// scoredCand — кандидат-название (артист + трек) с весом релевантности, ещё не
// найденный в источниках. Общий тип для всех генераторов кандидатов
// (рекомендации, 2-хоповые «Открытия недели»).
type scoredCand struct {
	artist string
	title  string
	score  float64
}

// resolveCandidates превращает взвешенные названия в реальные проигрываемые
// треки: сортирует по весу, волнами ищет в источниках (searchConcurrency),
// выбирает лучший играбельный результат с учётом привычных источников,
// исключает уже слышанное/заблокированное и ограничивает число треков на
// исполнителя. Выделено из onlineRecommendations, чтобы тем же конвейером
// пользовался 2-хоповый обход «Открытий недели» (см. discovery.go).
func (e *Engine) resolveCandidates(ctx context.Context, cands []scoredCand, t taste, limit int) []domain.Track {
	if limit <= 0 || len(cands) == 0 {
		return []domain.Track{}
	}
	seen := t.seen

	sort.SliceStable(cands, func(i, j int) bool { return cands[i].score > cands[j].score })

	// Собираем кандидатов-треки, затем прогоняем через кап по артистам, чтобы
	// подборка не зацикливалась на одном исполнителе (см. plan.md, п.30).
	//
	// Поиск идёт волнами по searchConcurrency кандидатов: раньше это был строго
	// последовательный цикл — до 120 поисков подряд, каждый веером по всем
	// источникам, из-за чего первая сборка подборки занимала минуты. Порядок
	// результатов по-прежнему строго по score: волна разбирается по индексам.
	//
	// Запас по кандидатам нужен для разнообразия (часть отсеет кап по артистам),
	// но не двукратный: capPerArtist добавляет «лишние» треки обратно, если
	// слотов не хватило, поэтому сверх limit+половины смысла набирать нет.
	target := limit + limit/2
	if target < limit+4 {
		target = limit + 4
	}
	found := make([]domain.Track, 0, target)
	usedKeys := map[string]bool{}
	for start := 0; start < len(cands) && len(found) < target; start += searchConcurrency {
		end := start + searchConcurrency
		if end > len(cands) {
			end = len(cands)
		}
		batch := cands[start:end]
		results := make([][]domain.Track, len(batch))
		var wg sync.WaitGroup
		for i, c := range batch {
			query := strings.TrimSpace(c.artist + " " + c.title)
			if query == "" {
				continue
			}
			wg.Add(1)
			go func(i int, query string) {
				defer wg.Done()
				res, err := e.search(ctx, query)
				if err != nil {
					return
				}
				results[i] = res
			}(i, query)
		}
		wg.Wait()

		for _, res := range results {
			if len(found) >= target {
				break
			}
			if len(res) == 0 {
				continue
			}
			best, ok := e.pickCandidateFor(res, t.sources)
			if !ok {
				continue
			}
			// Поиск по «артист + название» может вернуть кавер или сборник другого
			// исполнителя — проверяем блок ещё и по фактическому результату.
			if t.blockedArtists[artistKey(best)] {
				continue
			}
			key := trackKey(best)
			if seen[key] || usedKeys[key] {
				continue
			}
			usedKeys[key] = true
			found = append(found, best)
		}
	}
	return capPerArtist(found, limit, artistCap)
}

// artistCap — сколько треков одного исполнителя допускается в подборке, пока
// хватает других. Держим низко для разнообразия, но не 1: у любимого артиста
// пара треков подряд — это нормально.
const artistCap = 2

// capPerArtist ограничивает число треков на исполнителя значением maxPerArtist, сохраняя
// исходный порядок релевантности. Если после ограничения набралось меньше
// limit, добавляет отложенные («лишние») треки — лучше показать больше от
// одного артиста, чем отдать пустые слоты.
func capPerArtist(in []domain.Track, limit, maxPerArtist int) []domain.Track {
	if limit <= 0 {
		return []domain.Track{}
	}
	out := make([]domain.Track, 0, limit)
	overflow := make([]domain.Track, 0)
	counts := map[string]int{}
	for _, t := range in {
		if len(out) >= limit {
			break
		}
		a := artistKey(t)
		if a != "" && counts[a] >= maxPerArtist {
			overflow = append(overflow, t)
			continue
		}
		counts[a]++
		out = append(out, t)
	}
	for _, t := range overflow {
		if len(out) >= limit {
			break
		}
		out = append(out, t)
	}
	return out
}

// artistKey — нормализованный первый исполнитель трека для группировки.
func artistKey(t domain.Track) string {
	if len(t.Artists) == 0 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(t.Artists[0]))
}

// offlineRecommendations: без Last.fm ищем ещё треки любимых артистов в
// источниках (чистый локальный сигнал, один сетевой запрос на артиста).
func (e *Engine) offlineRecommendations(ctx context.Context, t taste, seeds []artistWeight, limit int) []domain.Track {
	if len(seeds) == 0 {
		return []domain.Track{}
	}
	seen := t.seen
	// Без Last.fm единственный сигнал — сами любимые артисты, поэтому кап на
	// артиста здесь выше: иначе от 2-3 seed'ов не набрать limit треков.
	perArtist := limit/len(seeds) + 1
	// Запросы по затравкам независимы — выполняем их сразу все (их не больше 5).
	// Разбор идёт в порядке затравок, поэтому выдача остаётся стабильной.
	foundBySeed := make([][]domain.Track, len(seeds))
	var wg sync.WaitGroup
	for i, seed := range seeds {
		wg.Add(1)
		go func(i int, name string) {
			defer wg.Done()
			found, err := e.search(ctx, name)
			if err != nil {
				return
			}
			foundBySeed[i] = found
		}(i, seed.name)
	}
	wg.Wait()

	out := make([]domain.Track, 0, limit)
	usedKeys := map[string]bool{}
	for _, found := range foundBySeed {
		if len(out) >= limit {
			break
		}
		taken := 0
		for _, tr := range e.orderCandidatesFor(found, t.sources) {
			if len(out) >= limit || taken >= perArtist {
				break
			}
			if t.blockedArtists[artistKey(tr)] {
				continue
			}
			key := trackKey(tr)
			if seen[key] || usedKeys[key] {
				continue
			}
			usedKeys[key] = true
			out = append(out, tr)
			taken++
		}
	}
	return out
}

// similarArtistsCached — SimilarArtists с кэшем в SQLite (TTL сутки).
func (e *Engine) similarArtistsCached(ctx context.Context, artist string, limit int) []lastfm.Artist {
	key := "lastfm:similar:" + strings.ToLower(artist)
	if e.store != nil {
		if raw, ok, _ := e.store.RecoCacheGet(ctx, key); ok {
			var cached []lastfm.Artist
			if json.Unmarshal([]byte(raw), &cached) == nil {
				return cached
			}
		}
	}
	res, err := e.lastfm.SimilarArtists(ctx, artist, limit)
	if err != nil {
		logging.L().Debug("recommendations: similar artists не удалось", "artist", artist, "err", err)
		return nil
	}
	if e.store != nil {
		if data, mErr := json.Marshal(res); mErr == nil {
			_ = e.store.RecoCacheSet(ctx, key, string(data), 24*time.Hour)
		}
	}
	return res
}

// topTracksCached — TopArtistTracks с кэшем в SQLite (TTL сутки).
func (e *Engine) topTracksCached(ctx context.Context, artist string, limit int) []lastfm.Track {
	key := "lastfm:toptracks:" + strings.ToLower(artist)
	if e.store != nil {
		if raw, ok, _ := e.store.RecoCacheGet(ctx, key); ok {
			var cached []lastfm.Track
			if json.Unmarshal([]byte(raw), &cached) == nil {
				return cached
			}
		}
	}
	res, err := e.lastfm.TopArtistTracks(ctx, artist, limit)
	if err != nil {
		logging.L().Debug("recommendations: top tracks не удалось", "artist", artist, "err", err)
		return nil
	}
	if e.store != nil {
		if data, mErr := json.Marshal(res); mErr == nil {
			_ = e.store.RecoCacheSet(ctx, key, string(data), 24*time.Hour)
		}
	}
	return res
}

// MixKind — тип авто-подборки.
type MixKind string

const (
	// MixDaily — «Микс дня»: любимое и близкое к вкусу, стабильно в течение суток.
	MixDaily MixKind = "daily"
	// MixWeekly — «Открытия недели»: уклон в новых артистов (похожие, но не из
	// топа профиля), стабильно в течение недели.
	MixWeekly MixKind = "weekly"
)

// AutoMix возвращает авто-подборку заданного типа. Подборка детерминирована в
// пределах периода (день/неделя): один и тот же набор в течение суток/недели,
// затем обновляется сам. Порядок перемешивается псевдослучайно от seed'а
// периода, чтобы подборка не выглядела как обычный список рекомендаций.
func (e *Engine) AutoMix(ctx context.Context, kind MixKind, limit int) ([]domain.Track, error) {
	if limit <= 0 {
		limit = 20
	}
	seed := periodSeed(kind)

	// «Открытия недели»: сначала пробуем настоящий 2-хоповый обход графа похожих
	// артистов (задача 40) — он достаёт незнакомое, куда прямой similar к любимым
	// не дотягивается. Если обход недоступен (нет ключа Last.fm) или пуст —
	// откатываемся на менее знакомый срез обычных рекомендаций (прежнее
	// поведение), чтобы секция не осталась пустой.
	if kind == MixWeekly {
		if disco := e.discoveryMix(ctx, limit*2); len(disco) > 0 {
			shuffled := deterministicShuffle(disco, seed)
			if len(shuffled) > limit {
				shuffled = shuffled[:limit]
			}
			return shuffled, nil
		}
	}

	// Пул кандидатов берём с запасом, затем детерминированно отбираем limit.
	pool, err := e.Recommendations(ctx, limit*3)
	if err != nil {
		return nil, err
	}
	if len(pool) == 0 {
		return []domain.Track{}, nil
	}

	// «Открытия недели» (фолбэк) отдают приоритет менее знакомым артистам: те,
	// кого нет в топе профиля вкусов, идут первыми.
	if kind == MixWeekly {
		profile := e.buildTaste(ctx).artists
		top := make(map[string]bool, len(profile))
		topN := profile
		if len(topN) > 8 {
			topN = topN[:8]
		}
		for _, w := range topN {
			top[strings.ToLower(w.name)] = true
		}
		sort.SliceStable(pool, func(i, j int) bool {
			return !isFamiliar(pool[i], top) && isFamiliar(pool[j], top)
		})
	}

	shuffled := deterministicShuffle(pool, seed)
	if len(shuffled) > limit {
		shuffled = shuffled[:limit]
	}
	return shuffled, nil
}

// isFamiliar — есть ли первый артист трека в множестве топ-артистов профиля.
func isFamiliar(t domain.Track, top map[string]bool) bool {
	if len(t.Artists) == 0 {
		return false
	}
	return top[strings.ToLower(strings.TrimSpace(t.Artists[0]))]
}

// periodSeed возвращает стабильный в пределах периода seed: номер дня для
// дневного микса и номер недели (ISO) для недельного.
func periodSeed(kind MixKind) int64 {
	now := time.Now()
	if kind == MixWeekly {
		year, week := now.ISOWeek()
		return int64(year)*100 + int64(week)
	}
	return int64(now.Year())*1000 + int64(now.YearDay())
}

// deterministicShuffle перемешивает копию среза детерминированно от seed'а
// (LCG + Fisher–Yates). Один seed → один порядок.
func deterministicShuffle(in []domain.Track, seed int64) []domain.Track {
	out := make([]domain.Track, len(in))
	copy(out, in)
	// Числовой генератор (константы из Numerical Recipes LCG).
	state := uint64(seed)*2862933555777941757 + 3037000493
	next := func(n int) int {
		state = state*6364136223846793005 + 1442695040888963407
		return int((state >> 33) % uint64(n))
	}
	for i := len(out) - 1; i > 0; i-- {
		j := next(i + 1)
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// trackKey — ключ дедупликации трека между источниками: по названию+артисту
// (нижний регистр), а не по service-id, чтобы одна песня из разных сервисов
// не дублировалась.
func trackKey(t domain.Track) string {
	artist := ""
	if len(t.Artists) > 0 {
		artist = t.Artists[0]
	}
	return strings.ToLower(strings.TrimSpace(artist) + "|" + strings.TrimSpace(t.Title))
}
