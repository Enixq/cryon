package recommendations

import (
	"context"
	"testing"
)

// Движок без ключа Last.fm и без стора: обход графа должен деградировать в
// пустой результат без паники (nil-приёмники lastfm/store безопасны).
func TestDiscoveryMixGraceful(t *testing.T) {
	e := &Engine{}
	if got := e.discoveryMix(context.Background(), 10); got != nil {
		t.Fatalf("без Last.fm обход должен вернуть nil, got %v", got)
	}
	if got := e.discoveryMix(context.Background(), 0); got != nil {
		t.Fatalf("limit<=0 → nil")
	}
}

// AutoMix обоих типов на пустом движке возвращает пустую подборку без ошибки и
// без паники (недельный путь откатывается с обхода на пустой пул рекомендаций).
func TestAutoMixEmptyEngine(t *testing.T) {
	e := &Engine{}
	for _, kind := range []MixKind{MixDaily, MixWeekly} {
		got, err := e.AutoMix(context.Background(), kind, 10)
		if err != nil {
			t.Fatalf("%s: неожиданная ошибка %v", kind, err)
		}
		if len(got) != 0 {
			t.Fatalf("%s: ожидали пустую подборку, got %d", kind, len(got))
		}
	}
}
