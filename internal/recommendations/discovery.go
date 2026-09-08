// 2-хоповый обход графа похожих артистов для «Открытий недели» (задача 40).
//
// «Открытия недели» раньше были просто менее знакомым срезом обычных
// рекомендаций — а те строятся на 1 хопе (затравки → похожие артисты). Настоящее
// открытие лежит дальше: артисты, похожие на похожих, которых прямой similar к
// любимым не достаёт. Здесь граф обходится на 2 хопа с затуханием вклада, а
// знакомые (уже в профиле вкуса) и заблокированные артисты отсекаются на каждом
// шаге — на выходе только незнакомое, но жанрово родственное (теговый буст из
// §39 держит открытия в границах вкуса).
//
// Стоимость обхода кэшируется дважды: ответы Last.fm — в reco_cache на сутки
// (общий кэш с рекомендациями), а готовая недельная подборка — на неделю с
// ключом от профиля, поэтому в течение недели обход не повторяется.

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
	"Cryon2/internal/services/lastfm"
)

const (
	discoveryHop1Limit = 8   // похожих артистов на затравку (хоп 1)
	discoveryHop2Limit = 6   // похожих артистов на «мост» (хоп 2)
	discoveryBridgeCap = 20  // сколько артистов хопа 1 раскрываем во 2-й хоп
	discoveryArtistCap = 40  // сколько лучших артистов-открытий берём в поиск
	discoveryTopTracks = 2   // топ-треков на артиста-открытие
	discoverySeedCap   = 6   // затравок для обхода
	discoveryHopDecay  = 0.5 // затухание вклада на каждый хоп дальше от вкуса
	discoveryWeekTTL   = 7 * 24 * time.Hour
)

// discoveryMix собирает «Открытия недели» 2-хоповым обходом. Возвращает пустой
// срез, если онлайн-движок недоступен или обход ничего не дал — вызывающий код
// (AutoMix) в этом случае откатывается на обычный пул рекомендаций.
func (e *Engine) discoveryMix(ctx context.Context, limit int) []domain.Track {
	if e == nil || !e.lastfm.Available() || limit <= 0 {
		return nil
	}
	profile := e.buildTaste(ctx)
	if len(profile.artists) == 0 {
		return nil
	}
	seeds := profile.artists
	if len(seeds) > discoverySeedCap {
		seeds = seeds[:discoverySeedCap]
	}

	// Недельный кэш готовой подборки: ключ от профиля + версии оценок + номера
	// недели, поэтому обход не повторяется в течение недели, но пересобирается
	// при смене вкуса.
	cacheKey := "reco:discovery:v1:w" + strconv.FormatInt(periodSeed(MixWeekly), 10) +
		":fb" + strconv.Itoa(profile.feedbackRev) + ":n" + strconv.Itoa(limit) +
		":" + profileSignature(seeds)
	if e.store != nil {
		if raw, ok, _ := e.store.RecoCacheGet(ctx, cacheKey); ok {
			var cached []domain.Track
			if json.Unmarshal([]byte(raw), &cached) == nil && len(cached) > 0 {
				return cached
			}
		}
	}

	cands := e.discoveryCandidates(ctx, profile, seeds)
	if len(cands) == 0 {
		return nil
	}
	tracks := e.resolveCandidates(ctx, cands, profile, limit)
	if len(tracks) > 0 && e.store != nil {
		if data, err := json.Marshal(tracks); err == nil {
			_ = e.store.RecoCacheSet(ctx, cacheKey, string(data), discoveryWeekTTL)
		}
	}
	return tracks
}

// discoveryCandidates выполняет собственно обход графа и возвращает взвешенные
// названия (артист + трек) для последующего поиска в источниках.
func (e *Engine) discoveryCandidates(ctx context.Context, t taste, seeds []artistWeight) []scoredCand {
	// Знакомые артисты (весь профиль, не только затравки) — не «открытия».
	familiar := make(map[string]bool, len(t.artists))
	for _, a := range t.artists {
		familiar[strings.ToLower(strings.TrimSpace(a.name))] = true
	}

	// scores — накопленный вес артиста-открытия; names — исходное написание для
	// поиска. record отбрасывает знакомых и заблокированных.
	scores := map[string]float64{}
	names := map[string]string{}
	record := func(name string, score float64) {
		key := strings.ToLower(strings.TrimSpace(name))
		if key == "" || familiar[key] || t.blockedArtists[key] || score <= 0 {
			return
		}
		scores[key] += score
		if _, ok := names[key]; !ok {
			names[key] = strings.TrimSpace(name)
		}
	}

	// Хоп 1: похожие на затравки (параллельно — по запросу на затравку). Каждый
	// незнакомый похожий уже кандидат-открытие; он же — «мост» во 2-й хоп.
	type bridge struct {
		name   string
		weight float64
	}
	similarBySeed := make([][]lastfm.Artist, len(seeds))
	var wg1 sync.WaitGroup
	for i, s := range seeds {
		wg1.Add(1)
		go func(i int, name string) {
			defer wg1.Done()
			similarBySeed[i] = e.similarArtistsCached(ctx, name, discoveryHop1Limit)
		}(i, s.name)
	}
	wg1.Wait()

	bridgeByKey := map[string]*bridge{}
	for si, s := range seeds {
		for _, sim := range similarBySeed[si] {
			w1 := s.weight * (sim.Match + 0.1)
			record(sim.Name, w1)
			// Мостом может быть и знакомый артист (хороший проводник вглубь), но
			// не заблокированный.
			key := strings.ToLower(strings.TrimSpace(sim.Name))
			if key == "" || t.blockedArtists[key] {
				continue
			}
			if b, ok := bridgeByKey[key]; ok {
				if w1 > b.weight {
					b.weight = w1
				}
			} else {
				bridgeByKey[key] = &bridge{name: sim.Name, weight: w1}
			}
		}
	}

	// Лучшие мосты по весу — их и раскрываем во 2-й хоп (бюджет запросов).
	bridges := make([]bridge, 0, len(bridgeByKey))
	for _, b := range bridgeByKey {
		bridges = append(bridges, *b)
	}
	sort.SliceStable(bridges, func(i, j int) bool { return bridges[i].weight > bridges[j].weight })
	if len(bridges) > discoveryBridgeCap {
		bridges = bridges[:discoveryBridgeCap]
	}

	// Хоп 2: похожие на мосты (волнами против 429). Вклад затухает на discoveryHopDecay.
	hop2 := make([][]lastfm.Artist, len(bridges))
	for start := 0; start < len(bridges); start += searchConcurrency {
		end := start + searchConcurrency
		if end > len(bridges) {
			end = len(bridges)
		}
		var wg2 sync.WaitGroup
		for i := start; i < end; i++ {
			wg2.Add(1)
			go func(i int) {
				defer wg2.Done()
				hop2[i] = e.similarArtistsCached(ctx, bridges[i].name, discoveryHop2Limit)
			}(i)
		}
		wg2.Wait()
	}
	for i, b := range bridges {
		for _, sim := range hop2[i] {
			record(sim.Name, b.weight*(sim.Match+0.1)*discoveryHopDecay)
		}
	}

	if len(scores) == 0 {
		return nil
	}

	// Лучшие артисты-открытия по накопленному весу.
	type scoredArtist struct {
		key   string
		score float64
	}
	ranked := make([]scoredArtist, 0, len(scores))
	for key, sc := range scores {
		ranked = append(ranked, scoredArtist{key: key, score: sc})
	}
	sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].score > ranked[j].score })
	if len(ranked) > discoveryArtistCap {
		ranked = ranked[:discoveryArtistCap]
	}

	// Топ-треки открытий (волнами) + теговый буст (жанровая близость к вкусу
	// удерживает открытия в границах вкуса, а не уводит в случайный шум).
	discoNames := make([]string, len(ranked))
	for i, ra := range ranked {
		discoNames[i] = names[ra.key]
	}
	tagProf := e.buildTagProfile(ctx, seeds, discoNames)

	topByArtist := make([][]lastfm.Track, len(ranked))
	for start := 0; start < len(ranked); start += searchConcurrency {
		end := start + searchConcurrency
		if end > len(ranked) {
			end = len(ranked)
		}
		var wgT sync.WaitGroup
		for i := start; i < end; i++ {
			wgT.Add(1)
			go func(i int) {
				defer wgT.Done()
				topByArtist[i] = e.topTracksCached(ctx, discoNames[i], discoveryTopTracks)
			}(i)
		}
		wgT.Wait()
	}

	var cands []scoredCand
	for i, ra := range ranked {
		tagBoost := 1.0 + tagWeight*tagProf.affinity(discoNames[i])
		for j, tr := range topByArtist[i] {
			if t.blockedArtists[strings.ToLower(strings.TrimSpace(tr.Artist))] {
				continue
			}
			score := ra.score * (1.0 / float64(j+1)) * tagBoost
			cands = append(cands, scoredCand{artist: tr.Artist, title: tr.Name, score: score})
		}
	}
	return cands
}
