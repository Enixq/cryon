package core

import (
	"context"
	"fmt"
	"strings"
	"time"

	"Cryon2/internal/domain"
)

// SearchPlaylists searches a source's public playlist catalog. It is separate
// from local user playlists, which are stored in SQLite.
func (a *App) SearchPlaylists(service, query string) ([]domain.Playlist, error) {
	serviceID := domain.ServiceID(strings.TrimSpace(service))
	svc, ok := a.registry[serviceID]
	if !ok {
		return nil, fmt.Errorf("неизвестный источник: %s", service)
	}
	browser, ok := svc.(domain.PlaylistBrowser)
	if !ok {
		return []domain.Playlist{}, nil
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return browser.SearchPlaylists(ctx, query, 20)
}

// GetPlaylistTracks returns a public playlist's tracks. The ID is always
// interpreted by the selected source adapter, so IDs from different sources
// cannot collide.
func (a *App) GetPlaylistTracks(service, playlistID string) ([]domain.Track, error) {
	serviceID := domain.ServiceID(strings.TrimSpace(service))
	svc, ok := a.registry[serviceID]
	if !ok {
		return nil, fmt.Errorf("неизвестный источник: %s", service)
	}
	browser, ok := svc.(domain.PlaylistBrowser)
	if !ok {
		return []domain.Track{}, nil
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return browser.PlaylistTracks(ctx, playlistID)
}
