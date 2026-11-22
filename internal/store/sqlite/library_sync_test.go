package sqlite

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"Cryon2/internal/domain"
)

func TestLibrarySyncOfflineMergeDeletionAndRestart(t *testing.T) {
	ctx := context.Background()
	makeStore := func(path string) *Store {
		db := &Store{dbPath: path}
		if err := db.Init(ctx); err != nil {
			t.Fatal(err)
		}
		return db
	}
	pc := makeStore(filepath.Join(t.TempDir(), "pc.db"))
	defer pc.Close()
	phonePath := filepath.Join(t.TempDir(), "phone.db")
	phone := makeStore(phonePath)
	defer func() { phone.Close() }()
	song := func(id string) domain.Track {
		return domain.Track{ID: id, Service: domain.ServiceYandex, Title: id, Artists: []string{"Artist"}, PlayableKind: domain.PlayableStream}
	}
	exchange := func() {
		left, err := pc.LibrarySyncSnapshot(ctx)
		if err != nil {
			t.Fatal(err)
		}
		right, err := phone.LibrarySyncSnapshot(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pc.LibrarySyncMerge(ctx, right); err != nil {
			t.Fatal(err)
		}
		if _, err = phone.LibrarySyncMerge(ctx, left); err != nil {
			t.Fatal(err)
		}
	}
	if err := pc.FavoriteAdd(ctx, song("pc")); err != nil {
		t.Fatal(err)
	}
	if err := phone.FavoriteAdd(ctx, song("phone")); err != nil {
		t.Fatal(err)
	}
	exchange()
	exchange()
	left, _ := pc.FavoriteList(ctx)
	right, _ := phone.FavoriteList(ctx)
	if len(left) != 2 || len(right) != 2 {
		t.Fatal("offline additions were lost")
	}
	stale, _ := phone.LibrarySyncSnapshot(ctx)
	if err := pc.FavoriteRemove(ctx, domain.ServiceYandex, "phone"); err != nil {
		t.Fatal(err)
	}
	exchange()
	phone.Close()
	phone = makeStore(phonePath)
	if _, err := pc.LibrarySyncMerge(ctx, stale); err != nil {
		t.Fatal(err)
	}
	if _, err := phone.LibrarySyncMerge(ctx, stale); err != nil {
		t.Fatal(err)
	}
	left, _ = pc.FavoriteList(ctx)
	right, _ = phone.FavoriteList(ctx)
	if len(left) != 1 || len(right) != 1 || left[0].ID != "pc" || right[0].ID != "pc" {
		t.Fatal("stale snapshot resurrected a removed favourite")
	}
	if err := phone.FavoriteAdd(ctx, song("phone")); err != nil {
		t.Fatal(err)
	}
	exchange()
	leftSnapshot, _ := pc.LibrarySyncSnapshot(ctx)
	rightSnapshot, _ := phone.LibrarySyncSnapshot(ctx)
	if !reflect.DeepEqual(leftSnapshot, rightSnapshot) {
		t.Fatal("devices did not converge after re-adding a favourite")
	}
}

func TestLibrarySyncNeverTransfersLocalFiles(t *testing.T) {
	ctx := context.Background()
	db := &Store{dbPath: filepath.Join(t.TempDir(), "library.db")}
	if err := db.Init(ctx); err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.FavoriteAdd(ctx, domain.Track{Service: domain.ServiceLocal, ID: `C:\private\song.mp3`, Title: "Local", PlayableKind: domain.PlayableStream}); err != nil {
		t.Fatal(err)
	}
	entries, err := db.LibrarySyncSnapshot(ctx)
	if err != nil || len(entries) != 0 {
		t.Fatal("local file leaked into sync")
	}
}
