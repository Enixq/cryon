package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestExpiredCacheRemainsAvailableForOfflineFallback(t *testing.T) {
	ctx := context.Background()
	db := &Store{dbPath: filepath.Join(t.TempDir(), "cache.db")}
	if err := db.Init(ctx); err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.RecoCacheSet(ctx, "key", "saved", time.Millisecond)
	time.Sleep(3 * time.Millisecond)
	if _, ok, _ := db.RecoCacheGet(ctx, "key"); ok {
		t.Fatal("fresh read used expired value")
	}
	if raw, ok, err := db.RecoCachePeek(ctx, "key"); err != nil || !ok || raw != "saved" {
		t.Fatal("offline value deleted")
	}
}
