package recommendations

import (
	"math"
	"testing"

	"Cryon2/internal/services/lastfm"
)

func TestTagVectorFrom(t *testing.T) {
	v := tagVectorFrom([]lastfm.Tag{
		{Name: "Rock", Count: 100},
		{Name: "  indie ", Count: 50},
		{Name: "noCount", Count: 0}, // нулевой count → вес 1
		{Name: "", Count: 10},       // пустое имя отбрасывается
	})
	if len(v) != 3 {
		t.Fatalf("ожидали 3 тега, got %d (%v)", len(v), v)
	}
	if v["rock"] != 100 || v["indie"] != 50 {
		t.Fatalf("нормализация/счётчики неверны: %v", v)
	}
	if v["nocount"] != 1 {
		t.Fatalf("нулевой count должен стать 1, got %v", v["nocount"])
	}
	if tagVectorFrom(nil) != nil {
		t.Fatalf("пустой вход → nil")
	}
}

func TestCosine(t *testing.T) {
	a := tagVector{"rock": 1, "indie": 1}
	// Идентичные векторы → 1.
	if got := cosine(a, tagVector{"rock": 1, "indie": 1}); math.Abs(got-1) > 1e-9 {
		t.Fatalf("идентичные векторы должны дать 1, got %v", got)
	}
	// Ортогональные (без общих тегов) → 0.
	if got := cosine(a, tagVector{"jazz": 1}); got != 0 {
		t.Fatalf("без общих тегов должно быть 0, got %v", got)
	}
	// Частичное пересечение → строго между 0 и 1.
	got := cosine(a, tagVector{"rock": 1, "jazz": 1})
	if got <= 0 || got >= 1 {
		t.Fatalf("частичное пересечение должно быть в (0,1), got %v", got)
	}
	// Пустой вектор с любой стороны → 0.
	if cosine(a, nil) != 0 || cosine(nil, a) != 0 {
		t.Fatalf("пустой вектор → 0")
	}
}

func TestBuildIDFAndApply(t *testing.T) {
	// "common" есть у всех троих, "rare" — у одного. idf(rare) > idf(common).
	docs := []tagVector{
		{"common": 1, "rare": 1},
		{"common": 1},
		{"common": 1},
	}
	idf := buildIDF(docs)
	if idf["rare"] <= idf["common"] {
		t.Fatalf("редкий тег должен иметь больший idf: rare=%v common=%v", idf["rare"], idf["common"])
	}
	if idf["common"] <= 0 {
		t.Fatalf("idf вездесущего тега должен оставаться положительным (сглаживание), got %v", idf["common"])
	}

	// applyIDF перевзвешивает и отбрасывает теги вне корпуса.
	got := applyIDF(tagVector{"common": 2, "rare": 3, "unknown": 5}, idf)
	if _, ok := got["unknown"]; ok {
		t.Fatalf("тег вне корпуса должен отбрасываться")
	}
	if math.Abs(got["common"]-2*idf["common"]) > 1e-9 {
		t.Fatalf("common: ожидали tf*idf, got %v", got["common"])
	}
	if math.Abs(got["rare"]-3*idf["rare"]) > 1e-9 {
		t.Fatalf("rare: ожидали tf*idf, got %v", got["rare"])
	}

	if buildIDF(nil) == nil {
		t.Fatalf("buildIDF(nil) должен вернуть пустую карту, не nil")
	}
	if applyIDF(nil, idf) != nil {
		t.Fatalf("applyIDF(nil) → nil")
	}
}

func TestTagProfileAffinity(t *testing.T) {
	var nilProf *tagProfile
	if nilProf.affinity("x") != 0 {
		t.Fatalf("nil-профиль должен давать 0 без паники")
	}
	p := &tagProfile{scoreByArtist: map[string]float64{"radiohead": 0.8}}
	if p.affinity("  Radiohead ") != 0.8 {
		t.Fatalf("affinity должна нормализовать имя, got %v", p.affinity(" Radiohead "))
	}
	if p.affinity("unknown") != 0 {
		t.Fatalf("неизвестный артист → 0")
	}
}
