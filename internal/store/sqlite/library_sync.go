package sqlite

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"Cryon2/internal/domain"
	"Cryon2/internal/store"
)

func portableTrack(track domain.Track) (domain.Track, bool) {
	switch track.Service {
	case domain.ServiceYandex, domain.ServiceYouTube, domain.ServiceSoundCloud, domain.ServiceVK, domain.ServiceSpotify:
	default:
		return domain.Track{}, false
	}
	if track.ID == "" || len(track.ID) > 2048 || len(track.Title) > 1024 || len(track.Artists) > 20 {
		return domain.Track{}, false
	}
	// Only explicit catalogue metadata travels; local paths and credentials do not.
	result := domain.Track{ID: track.ID, Service: track.Service, Title: track.Title, Artists: track.Artists, Album: track.Album, DurationMs: track.DurationMs, PlayableKind: track.PlayableKind}
	publicURL := func(value string) string {
		uri, err := url.Parse(value)
		if err != nil || uri.Host == "" || uri.User != nil || len(value) > 4096 || (uri.Scheme != "https" && uri.Scheme != "http") {
			return ""
		}
		return value
	}
	result.ArtworkURL, result.ExternalURL = publicURL(track.ArtworkURL), publicURL(track.ExternalURL)
	return result, true
}
func syncClock(ctx context.Context, tx *sql.Tx) (string, int64, error) {
	var origin string
	var counter int64
	err := tx.QueryRowContext(ctx, `SELECT origin,counter FROM library_clock WHERE id=1`).Scan(&origin, &counter)
	if err == sql.ErrNoRows {
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			return "", 0, err
		}
		origin = hex.EncodeToString(random[:])
		_, err = tx.ExecContext(ctx, `INSERT INTO library_clock(id,origin,counter) VALUES(1,?,0)`, origin)
	}
	return origin, counter, err
}
func recordLibraryChange(ctx context.Context, tx *sql.Tx, track domain.Track, deleted bool) error {
	track, ok := portableTrack(track)
	if !ok {
		return nil
	}
	origin, counter, err := syncClock(ctx, tx)
	if err != nil {
		return err
	}
	counter++
	data, err := json.Marshal(track)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO library_entries(service,track_id,counter,origin,deleted,track_json) VALUES(?,?,?,?,?,?) ON CONFLICT(service,track_id) DO UPDATE SET counter=excluded.counter,origin=excluded.origin,deleted=excluded.deleted,track_json=excluded.track_json`, track.Service, track.ID, counter, origin, boolInt(deleted), string(data))
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE library_clock SET counter=? WHERE id=1`, counter)
	return err
}

func (s *Store) LibrarySyncSnapshot(ctx context.Context) ([]store.LibraryEntry, error) {
	favorites, err := s.FavoriteList(ctx)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	origin, counter, err := syncClock(ctx, tx)
	if err != nil {
		return nil, err
	}
	for _, favorite := range favorites {
		track, ok := portableTrack(favorite)
		if !ok {
			continue
		}
		data, _ := json.Marshal(track)
		// Bootstrap v0.2.25 favourites once; tombstones always take precedence.
		_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO library_entries(service,track_id,counter,origin,deleted,track_json) VALUES(?,?,1,?,0,?)`, track.Service, track.ID, origin, string(data))
		if err != nil {
			return nil, err
		}
	}
	if counter < 1 {
		if _, err = tx.ExecContext(ctx, `UPDATE library_clock SET counter=1 WHERE id=1`); err != nil {
			return nil, err
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT counter,origin,deleted,track_json FROM library_entries ORDER BY service,track_id`)
	if err != nil {
		return nil, err
	}
	entries := []store.LibraryEntry{}
	for rows.Next() {
		var entry store.LibraryEntry
		var data string
		if err = rows.Scan(&entry.Counter, &entry.Origin, &entry.Deleted, &data); err != nil {
			rows.Close()
			return nil, err
		}
		if err = json.Unmarshal([]byte(data), &entry.Track); err != nil {
			rows.Close()
			return nil, err
		}
		entries = append(entries, entry)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return entries, nil
}

func (s *Store) LibrarySyncMerge(ctx context.Context, entries []store.LibraryEntry) (int, error) {
	if len(entries) > 20000 {
		return 0, fmt.Errorf("слишком большая библиотека")
	}
	// Bootstrap before merging so an incoming snapshot cannot erase old favourites.
	if _, err := s.LibrarySyncSnapshot(ctx); err != nil {
		return 0, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	_, clock, err := syncClock(ctx, tx)
	if err != nil {
		return 0, err
	}
	changes := 0
	for _, entry := range entries {
		track, ok := portableTrack(entry.Track)
		if !ok || entry.Counter <= 0 || entry.Counter > 1<<53 || len(entry.Origin) != 32 || strings.TrimSpace(entry.Origin) == "" {
			return 0, fmt.Errorf("неверная запись библиотеки")
		}
		if _, err := hex.DecodeString(entry.Origin); err != nil {
			return 0, fmt.Errorf("неверное устройство библиотеки")
		}
		var oldCounter int64
		var oldOrigin string
		err := tx.QueryRowContext(ctx, `SELECT counter,origin FROM library_entries WHERE service=? AND track_id=?`, track.Service, track.ID).Scan(&oldCounter, &oldOrigin)
		if err != nil && err != sql.ErrNoRows {
			return 0, err
		}
		clock = max(clock, entry.Counter)
		if entry.Counter < oldCounter || (entry.Counter == oldCounter && entry.Origin <= oldOrigin) {
			continue
		}
		data, _ := json.Marshal(track)
		_, err = tx.ExecContext(ctx, `INSERT INTO library_entries(service,track_id,counter,origin,deleted,track_json) VALUES(?,?,?,?,?,?) ON CONFLICT(service,track_id) DO UPDATE SET counter=excluded.counter,origin=excluded.origin,deleted=excluded.deleted,track_json=excluded.track_json`, track.Service, track.ID, entry.Counter, entry.Origin, boolInt(entry.Deleted), string(data))
		if err != nil {
			return 0, err
		}
		if entry.Deleted {
			_, err = tx.ExecContext(ctx, `DELETE FROM favorites WHERE service=? AND track_id=?`, track.Service, track.ID)
		} else {
			artists, _ := json.Marshal(track.Artists)
			_, err = tx.ExecContext(ctx, `INSERT INTO favorites(added_at,service,track_id,title,artists_json,album,duration_ms,artwork_url,external_url,playable_kind) VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(service,track_id) DO UPDATE SET title=excluded.title,artists_json=excluded.artists_json,album=excluded.album,duration_ms=excluded.duration_ms,artwork_url=excluded.artwork_url,external_url=excluded.external_url,playable_kind=excluded.playable_kind`, time.Now().UnixMilli(), track.Service, track.ID, track.Title, string(artists), track.Album, track.DurationMs, track.ArtworkURL, track.ExternalURL, track.PlayableKind)
		}
		if err != nil {
			return 0, err
		}
		changes++
	}
	if _, err = tx.ExecContext(ctx, `UPDATE library_clock SET counter=? WHERE id=1`, clock); err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return changes, nil
}
