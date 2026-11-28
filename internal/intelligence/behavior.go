package intelligence

import "math"

type BehaviorWeights struct {
	Like               float64 `json:"like"`
	Dislike            float64 `json:"dislike"`
	Replay             float64 `json:"replay"`
	Completion         float64 `json:"completion"`
	CompletionFraction float64 `json:"completionFraction"`
	HalfLifeDays       float64 `json:"halfLifeDays"`
}

func DefaultBehaviorWeights() BehaviorWeights { return BehaviorWeights{3, -3, 1.5, .4, .8, 30} }
func (w BehaviorWeights) Valid() bool {
	for _, v := range []float64{w.Like, w.Dislike, w.Replay, w.Completion, w.CompletionFraction, w.HalfLifeDays} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return w.Like >= 0 && w.Like <= 6 && w.Dislike <= 0 && w.Dislike >= -6 && w.Replay >= 0 && w.Replay <= 3 && w.Completion >= 0 && w.Completion <= 2 && w.CompletionFraction >= .5 && w.CompletionFraction <= 1 && w.HalfLifeDays >= 1 && w.HalfLifeDays <= 365
}
func (w BehaviorWeights) Signal(e FeedbackEvent) float64 {
	switch e.Kind {
	case FeedbackLike:
		return w.Like
	case FeedbackDislike:
		return w.Dislike
	case FeedbackReplay:
		return w.Replay
	case FeedbackCompleted:
		if e.DurationMS > 0 && e.PlayedMS > 0 && float64(e.PlayedMS)/float64(e.DurationMS) >= w.CompletionFraction {
			return w.Completion
		}
	}
	// A lone skip, a transport error, or a mere start has no dislike weight.
	return 0
}
