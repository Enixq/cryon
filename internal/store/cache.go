package store

import "context"

// PeekCache is an optional stale read. It never changes fresh-read semantics.
func PeekCache(ctx context.Context, s Store, key string) (string, bool) {
	if reader, ok := s.(interface {
		RecoCachePeek(context.Context, string) (string, bool, error)
	}); ok {
		raw, found, _ := reader.RecoCachePeek(ctx, key)
		return raw, found
	}
	return "", false
}
