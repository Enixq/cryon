package recommendations

import (
	"Cryon2/internal/domain"
	"context"
	"hash/fnv"
)

type acquisitionKey struct{}

// AcquisitionContext rotates anchors per job, not the final playlist. All
// candidates still need a real provider relation and local musical admission.
func (e *Engine) AcquisitionContext(ctx context.Context, key string) context.Context {
	return context.WithValue(ctx, acquisitionKey{}, key)
}
func djAnchors(ctx context.Context, tracks []domain.Track, limit int) []domain.Track {
	if len(tracks) <= limit {
		return tracks
	}
	key, _ := ctx.Value(acquisitionKey{}).(string)
	if key == "" {
		return tracks[:limit]
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	offset := int(h.Sum32() % uint32(len(tracks)))
	out := make([]domain.Track, 0, limit)
	for i := 0; i < limit; i++ {
		out = append(out, tracks[(offset+i*len(tracks)/limit)%len(tracks)])
	}
	return out
}
