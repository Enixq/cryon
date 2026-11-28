package recommendations

import (
	"Cryon2/internal/domain"
	"Cryon2/internal/intelligence"
	"context"
	"sync"
)

type originContextKey struct{}
type originRecorder struct {
	sync.Mutex
	rows    map[string][]intelligence.CandidateOrigin
	channel intelligence.Channel
}

func recordOrigin(ctx context.Context, track, anchor domain.Track) {
	recordAnchorOrigin(ctx, track, string(anchor.Service)+":"+anchor.ID, anchor.Artists...)
}
func recordAnchorOrigin(ctx context.Context, track domain.Track, anchorID string, artists ...string) {
	recorder, _ := ctx.Value(originContextKey{}).(*originRecorder)
	if recorder == nil {
		return
	}
	recorder.Lock()
	defer recorder.Unlock()
	key := string(track.Service) + ":" + track.ID
	recorder.rows[key] = append(recorder.rows[key], intelligence.CandidateOrigin{Channel: recorder.channel, AnchorID: anchorID, AnchorArtists: append([]string{}, artists...)})
}
func (e *Engine) ExecuteDJTool(ctx context.Context, step intelligence.SearchStep, seed domain.Track) ([]domain.Track, map[string][]intelligence.CandidateOrigin, error) {
	recorder := &originRecorder{rows: map[string][]intelligence.CandidateOrigin{}, channel: step.Channel}
	tracks, err := e.ExecuteDJStep(context.WithValue(ctx, originContextKey{}, recorder), step, seed)
	recorder.Lock()
	defer recorder.Unlock()
	origins := map[string][]intelligence.CandidateOrigin{}
	for key, rows := range recorder.rows {
		origins[key] = append([]intelligence.CandidateOrigin{}, rows...)
	}
	return tracks, origins, err
}
