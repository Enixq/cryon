package core

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"Cryon2/internal/domain"
	"Cryon2/internal/store"
	"Cryon2/internal/store/sqlite"
)

func TestLibraryExchangeRequiresCorrectKeyAndDirection(t *testing.T) {
	key := strings.Repeat("ab", 32)
	entries := []store.LibraryEntry{{Track: domain.Track{ID: "song", Service: domain.ServiceYandex}, Counter: 1, Origin: strings.Repeat("a", 32)}}
	envelope, err := sealLibrary(key, "request", entries)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := openLibrary(key, "request", envelope)
	if err != nil || len(decoded) != 1 || decoded[0].Track.ID != "song" {
		t.Fatal("valid pairing failed")
	}
	if _, err = openLibrary(strings.Repeat("cd", 32), "request", envelope); err == nil {
		t.Fatal("wrong pairing key accepted")
	}
	if _, err = openLibrary(key, "response", envelope); err == nil {
		t.Fatal("reflected request accepted as response")
	}
	envelope.Data = "AAAA" + envelope.Data[4:]
	if _, err = openLibrary(key, "request", envelope); err == nil {
		t.Fatal("modified packet accepted")
	}
}

func TestLibrarySyncTwoWayHTTPExchangeAndDeletion(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	ctx := context.Background()
	newApp := func(name string) *App {
		db, err := sqlite.New(name)
		if err != nil {
			t.Fatal(err)
		}
		if err = db.Init(ctx); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		return &App{ctx: ctx, store: db}
	}
	pc, phone := newApp("pc"), newApp("phone")
	key := strings.Repeat("ab", 32)
	if err := pc.saveLibraryConfig(ctx, librarySyncConfig{Enabled: true, Key: key}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(pc.handleAPILibraryExchange))
	defer server.Close()
	if err := phone.saveLibraryConfig(ctx, librarySyncConfig{Enabled: true, PeerURL: server.URL, PeerKey: key}); err != nil {
		t.Fatal(err)
	}
	song := func(id string) domain.Track {
		return domain.Track{ID: id, Service: domain.ServiceYandex, Title: id, Artists: []string{"Artist"}, PlayableKind: domain.PlayableStream}
	}
	if err := pc.store.FavoriteAdd(ctx, song("pc")); err != nil {
		t.Fatal(err)
	}
	if err := phone.store.FavoriteAdd(ctx, song("phone")); err != nil {
		t.Fatal(err)
	}
	if err := phone.syncLibraryOnce(ctx); err != nil {
		t.Fatal(err)
	}
	for _, app := range []*App{pc, phone} {
		favorites, err := app.store.FavoriteList(ctx)
		if err != nil || len(favorites) != 2 {
			t.Fatalf("two-way exchange lost additions: %d %v", len(favorites), err)
		}
	}
	if err := phone.store.FavoriteRemove(ctx, domain.ServiceYandex, "pc"); err != nil {
		t.Fatal(err)
	}
	if err := phone.syncLibraryOnce(ctx); err != nil {
		t.Fatal(err)
	}
	for _, app := range []*App{pc, phone} {
		favorites, err := app.store.FavoriteList(ctx)
		if err != nil || len(favorites) != 1 || favorites[0].ID != "phone" {
			t.Fatal("deletion did not propagate")
		}
	}
	_ = phone.saveLibraryConfig(ctx, librarySyncConfig{Enabled: true, PeerURL: server.URL, PeerKey: strings.Repeat("cd", 32)})
	if err := phone.syncLibraryOnce(ctx); err == nil {
		t.Fatal("unpaired device exchanged library")
	}
}

func TestLibraryPeerStaysInsidePrivateNetwork(t *testing.T) {
	for _, target := range []string{"http://8.8.8.8:8899", "http://192.168.1.3:8899/path", "http://user:secret@192.168.1.3:8899", "http://192.168.1.3:8899?token=secret", "file:///tmp/library"} {
		if validLibraryPeer(target) {
			t.Fatalf("unexpected peer accepted: %s", target)
		}
	}
	if !validLibraryPeer("http://192.168.1.3:8899") {
		t.Fatal("private peer rejected")
	}
}
