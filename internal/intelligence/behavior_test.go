package intelligence

import "testing"

func TestBehaviorUsesFractionAndIgnoresAmbiguousSignals(t *testing.T) {
	w := DefaultBehaviorWeights()
	for _, duration := range []int{30000, 300000} {
		if w.Signal(FeedbackEvent{Kind: FeedbackCompleted, DurationMS: duration, PlayedMS: duration * 9 / 10}) != w.Completion {
			t.Fatal("completion should scale by fraction")
		}
		if w.Signal(FeedbackEvent{Kind: FeedbackCompleted, DurationMS: duration, PlayedMS: duration / 5}) != 0 {
			t.Fatal("short observation treated as completion")
		}
	}
	for _, kind := range []FeedbackKind{FeedbackSkip, FeedbackPlaybackError, FeedbackStarted} {
		if w.Signal(FeedbackEvent{Kind: kind}) != 0 {
			t.Fatal("ambiguous signal became dislike")
		}
	}
	w.CompletionFraction = .95
	if w.Signal(FeedbackEvent{Kind: FeedbackCompleted, DurationMS: 100, PlayedMS: 90}) != 0 {
		t.Fatal("configured fraction ignored")
	}
}
