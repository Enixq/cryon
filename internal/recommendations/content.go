// Контентная похожесть на тегах (задача 39).
//
// До этого «похожесть» бралась только из Last.fm artist.getsimilar — попарной
// близости затравка→кандидат. Она не учитывает жанровый профиль пользователя в
// целом: кандидат, похожий на одного любимого артиста, но чуждый общему вкусу,
// получал такой же вес, как жанрово «свой».
//
// Здесь артист представляется вектором тегов (жанры/настроения из Last.fm,
// компоненты — «сила» тега), а вкус пользователя — суммой векторов его любимых
// артистов, взвешенной их весом в профиле. Близость кандидата к вкусу — косинус
// между их TF-IDF-векторами. TF-IDF гасит вездесущие теги («rock», «seen live»)
// и поднимает различающие — этого нет ни в одном из референсных репозиториев
// (все они берут сырые счётчики), это наш вклад поверх заимствованной идеи
// «косинус по тегам» (репозитории CarterHand/ftroeman/alirezatajfar).
//
// Слой полностью деградирует: нет ключа Last.fm или тегов → косинусы нулевые,
// ранжирование возвращается к прежнему поведению на sim.Match.

package recommendations

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"sync"
	"time"

	"Cryon2/internal/services/lastfm"
)

// tagWeight — насколько контентная (теговая) близость домешивается к базовому
// весу кандидата. Множитель, а не слагаемое: итоговый вес умножается на
// (1 + tagWeight*cos), поэтому при отсутствии тегов (cos=0) вес не меняется, а
// жанрово «свои» кандидаты получают до +tagWeight к весу. Подобрано умеренно:
// близость Last.fm остаётся ведущим сигналом, теги лишь доворачивают.
const tagWeight = 0.6

// artistTagLimit — сколько верхних тегов берём на артиста. Хвост тегов Last.fm
// шумный (единичные пользовательские метки), поэтому ограничиваемся значимыми.
const artistTagLimit = 15

// tagVector — разреженный вектор тегов артиста: тег → вес (счётчик Last.fm или
// уже перевзвешенный TF-IDF).
type tagVector map[string]float64

// tagVectorFrom строит вектор из тегов Last.fm. Ключ нормализуется (нижний
// регистр, без пробелов по краям); нулевой count заменяется на 1, чтобы тег без
// счётчика всё же присутствовал в векторе.
func tagVectorFrom(tags []lastfm.Tag) tagVector {
	if len(tags) == 0 {
		return nil
	}
	v := make(tagVector, len(tags))
	for _, t := range tags {
		name := strings.ToLower(strings.TrimSpace(t.Name))
		if name == "" {
			continue
		}
		w := float64(t.Count)
		if w <= 0 {
			w = 1
		}
		v[name] += w
	}
	return v
}

// cosine — косинусная близость двух векторов тегов (0..1). Пустой вектор с любой
// стороны даёт 0.
func cosine(a, b tagVector) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	// Скалярное произведение обходим по меньшему вектору.
	small, large := a, b
	if len(large) < len(small) {
		small, large = large, small
	}
	var dot float64
	for tag, wa := range small {
		if wb, ok := large[tag]; ok {
			dot += wa * wb
		}
	}
	if dot == 0 {
		return 0
	}
	return dot / (norm(a) * norm(b))
}

// norm — евклидова норма вектора.
func norm(v tagVector) float64 {
	var sum float64
	for _, w := range v {
		sum += w * w
	}
	return math.Sqrt(sum)
}

// buildIDF считает inverse document frequency по корпусу векторов: idf(tag) =
// log(1 + N/df), где df — в скольких артистах встречается тег. Сглаживание «1+»
// держит idf положительным даже для вездесущего тега (df=N → log2 ≈ 0.69), а
// редкий тег (df=1) при большом N получает заметно больший вес.
func buildIDF(docs []tagVector) map[string]float64 {
	if len(docs) == 0 {
		return map[string]float64{}
	}
	df := map[string]int{}
	for _, d := range docs {
		for tag := range d {
			df[tag]++
		}
	}
	n := float64(len(docs))
	idf := make(map[string]float64, len(df))
	for tag, c := range df {
		idf[tag] = math.Log(1 + n/float64(c))
	}
	return idf
}

// applyIDF перевзвешивает вектор: tf(tag) * idf(tag). Теги без idf (не из
// корпуса) отбрасываются.
func applyIDF(v tagVector, idf map[string]float64) tagVector {
	if len(v) == 0 {
		return nil
	}
	out := make(tagVector, len(v))
	for tag, tf := range v {
		if w, ok := idf[tag]; ok && w > 0 {
			out[tag] = tf * w
		}
	}
	return out
}

// tagProfile — теговый портрет вкуса и близости кандидатов к нему. Строится один
// раз на сборку подборки: собирает теги затравок и кандидатов, считает по ним
// TF-IDF и косинус каждого артиста к профилю пользователя.
type tagProfile struct {
	// scoreByArtist — близость артиста (нижний регистр) к вкусу, 0..1.
	scoreByArtist map[string]float64
}

// affinity возвращает теговую близость артиста к вкусу (0, если тегов нет).
func (p *tagProfile) affinity(artist string) float64 {
	if p == nil {
		return 0
	}
	return p.scoreByArtist[strings.ToLower(strings.TrimSpace(artist))]
}

// artistTagsCached — ArtistTags с кэшем в SQLite (TTL сутки), как
// similarArtistsCached/topTracksCached.
func (e *Engine) artistTagsCached(ctx context.Context, artist string) []lastfm.Tag {
	key := "lastfm:tags:" + strings.ToLower(strings.TrimSpace(artist))
	if e.store != nil {
		if raw, ok, _ := e.store.RecoCacheGet(ctx, key); ok {
			var cached []lastfm.Tag
			if json.Unmarshal([]byte(raw), &cached) == nil {
				return cached
			}
		}
	}
	res, err := e.lastfm.ArtistTags(ctx, artist, artistTagLimit)
	if err != nil {
		return nil
	}
	if e.store != nil {
		if data, mErr := json.Marshal(res); mErr == nil {
			_ = e.store.RecoCacheSet(ctx, key, string(data), 24*time.Hour)
		}
	}
	return res
}

// buildTagProfile строит теговый портрет вкуса и близость к нему всех
// кандидатов. seeds — затравочные (любимые) артисты с весами; candidates —
// уникальные имена артистов-кандидатов. Теги обоих множеств тянутся волнами
// (searchConcurrency), результаты кэшируются. Возвращает nil-безопасный
// профиль: при отсутствии тегов все близости будут 0.
func (e *Engine) buildTagProfile(ctx context.Context, seeds []artistWeight, candidates []string) *tagProfile {
	prof := &tagProfile{scoreByArtist: map[string]float64{}}
	if e == nil || !e.lastfm.Available() {
		return prof
	}

	// Собираем уникальный список имён (затравки + кандидаты) для запросов тегов.
	order := make([]string, 0, len(seeds)+len(candidates))
	seen := map[string]bool{}
	addName := func(name string) {
		key := strings.ToLower(strings.TrimSpace(name))
		if key == "" || seen[key] {
			return
		}
		seen[key] = true
		order = append(order, name)
	}
	for _, s := range seeds {
		addName(s.name)
	}
	for _, c := range candidates {
		addName(c)
	}
	if len(order) == 0 {
		return prof
	}

	// Теги по волнам: каждый запрос независим, но держим общий предел
	// параллелизма, чтобы не ловить 429 от Last.fm.
	vecs := make([]tagVector, len(order))
	for start := 0; start < len(order); start += searchConcurrency {
		end := start + searchConcurrency
		if end > len(order) {
			end = len(order)
		}
		var wg sync.WaitGroup
		for i := start; i < end; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				vecs[i] = tagVectorFrom(e.artistTagsCached(ctx, order[i]))
			}(i)
		}
		wg.Wait()
	}

	byName := make(map[string]tagVector, len(order))
	corpus := make([]tagVector, 0, len(order))
	for i, name := range order {
		if len(vecs[i]) == 0 {
			continue
		}
		byName[strings.ToLower(strings.TrimSpace(name))] = vecs[i]
		corpus = append(corpus, vecs[i])
	}
	if len(corpus) == 0 {
		return prof
	}

	idf := buildIDF(corpus)

	// Портрет вкуса: сумма TF-IDF-векторов затравок, взвешенная весом артиста в
	// профиле. Более любимый артист сильнее задаёт жанровый центр.
	user := tagVector{}
	for _, s := range seeds {
		vec := byName[strings.ToLower(strings.TrimSpace(s.name))]
		if len(vec) == 0 {
			continue
		}
		w := s.weight
		if w <= 0 {
			w = 1
		}
		for tag, tf := range applyIDF(vec, idf) {
			user[tag] += tf * w
		}
	}
	if len(user) == 0 {
		return prof
	}

	// Близость каждого артиста (и затравок, и кандидатов) к портрету вкуса.
	for name, vec := range byName {
		prof.scoreByArtist[name] = cosine(applyIDF(vec, idf), user)
	}
	return prof
}
