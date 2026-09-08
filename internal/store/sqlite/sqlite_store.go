package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"Cryon2/internal/domain"
	"Cryon2/internal/logging"
	"Cryon2/internal/store"
)

// Store — реализация store.Store на SQLite.
// Используется чистый Go-драйвер modernc.org/sqlite (без CGO),
// чтобы сборка .exe не требовала C-компилятора.
type Store struct {
	dbPath string
	db     *sql.DB
}

var _ store.Store = (*Store)(nil)

// New подготавливает путь к БД в каталоге данных приложения.
func New(appName string) (*Store, error) {
	dir, err := logging.DataDir(appName)
	if err != nil {
		return nil, err
	}
	return &Store{
		dbPath: filepath.Join(dir, "cryon.db"),
	}, nil
}

func (s *Store) Init(ctx context.Context) error {
	if s.db != nil {
		return nil
	}
	db, err := sql.Open("sqlite", s.dbPath)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(0)

	s.db = db
	if err := s.migrate(ctx); err != nil {
		_ = db.Close()
		s.db = nil
		return err
	}
	return nil
}

func (s *Store) Close() error {
	if s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	return err
}

func (s *Store) migrate(ctx context.Context) error {
	if s.db == nil {
		return errors.New("sqlite store не инициализирован")
	}

	stmts := []string{
		`CREATE TABLE IF NOT EXISTS favorites (
			row_id INTEGER PRIMARY KEY AUTOINCREMENT,
			added_at INTEGER NOT NULL,
			service TEXT NOT NULL,
			track_id TEXT NOT NULL,
			title TEXT NOT NULL,
			artists_json TEXT NOT NULL,
			album TEXT,
			duration_ms INTEGER,
			artwork_url TEXT,
			external_url TEXT,
			playable_kind TEXT NOT NULL,
			UNIQUE(service, track_id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_favorites_added_at ON favorites(added_at);`,

		`CREATE TABLE IF NOT EXISTS tokens (
			service TEXT PRIMARY KEY,
			access_token TEXT,
			refresh_token TEXT,
			expires_at INTEGER,
			scope TEXT,
			token_type TEXT,
			updated_at INTEGER NOT NULL
		);`,

		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT,
			updated_at INTEGER NOT NULL
		);`,

		`CREATE TABLE IF NOT EXISTS local_folders (
			path TEXT PRIMARY KEY,
			added_at INTEGER NOT NULL
		);`,

		`CREATE TABLE IF NOT EXISTS local_tracks (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			artists_json TEXT NOT NULL,
			album TEXT,
			file_path TEXT NOT NULL
		);`,

		`CREATE TABLE IF NOT EXISTS history (
			row_id INTEGER PRIMARY KEY AUTOINCREMENT,
			played_at INTEGER NOT NULL,
			service TEXT NOT NULL,
			track_id TEXT NOT NULL,
			title TEXT NOT NULL,
			artists_json TEXT NOT NULL,
			album TEXT,
			duration_ms INTEGER,
			artwork_url TEXT,
			external_url TEXT,
			playable_kind TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_history_played_at ON history(played_at);`,

		`CREATE TABLE IF NOT EXISTS playlists (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			description TEXT,
			created_at INTEGER NOT NULL
		);`,

		`CREATE TABLE IF NOT EXISTS playlist_tracks (
			row_id INTEGER PRIMARY KEY AUTOINCREMENT,
			playlist_id TEXT NOT NULL,
			added_at INTEGER NOT NULL,
			service TEXT NOT NULL,
			track_id TEXT NOT NULL,
			title TEXT NOT NULL,
			artists_json TEXT NOT NULL,
			album TEXT,
			duration_ms INTEGER,
			artwork_url TEXT,
			external_url TEXT,
			playable_kind TEXT NOT NULL,
			UNIQUE(playlist_id, service, track_id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_playlist_tracks_playlist ON playlist_tracks(playlist_id, added_at);`,

		// Кэш ответов внешних API рекомендаций (Last.fm). Ключ — произвольная
		// строка запроса, value — сериализованный JSON, expires_at — TTL.
		`CREATE TABLE IF NOT EXISTS reco_cache (
			cache_key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			expires_at INTEGER NOT NULL
		);`,

		// Оповещения (колокольчик). read=0/1, created_at — unix ms.
		`CREATE TABLE IF NOT EXISTS notifications (
			id TEXT PRIMARY KEY,
			created_at INTEGER NOT NULL,
			kind TEXT NOT NULL,
			title TEXT NOT NULL,
			message TEXT,
			read INTEGER NOT NULL DEFAULT 0
		);`,
		`CREATE INDEX IF NOT EXISTS idx_notifications_created_at ON notifications(created_at);`,
		`CREATE TABLE IF NOT EXISTS accounts (
			id TEXT PRIMARY KEY,
			login TEXT NOT NULL UNIQUE COLLATE NOCASE,
			email TEXT NOT NULL UNIQUE COLLATE NOCASE,
			password_hash TEXT NOT NULL,
			created_at INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS account_session (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			account_id TEXT,
			is_guest INTEGER NOT NULL DEFAULT 0,
			updated_at INTEGER NOT NULL
		);`,
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for _, stmt := range stmts {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("миграция: %w", err)
		}
	}
	return tx.Commit()
}

func (s *Store) AccountRegister(ctx context.Context, login, email, passwordHash string) (store.Account, error) {
	login, email = strings.TrimSpace(login), strings.TrimSpace(email)
	if login == "" || email == "" || passwordHash == "" {
		return store.Account{}, errors.New("заполните логин, email и пароль")
	}
	account := store.Account{ID: fmt.Sprintf("acc_%d", time.Now().UnixNano()), Login: login, Email: email}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO accounts(id, login, email, password_hash, created_at) VALUES (?, ?, ?, ?, ?)`, account.ID, login, email, passwordHash, time.Now().UnixMilli()); err != nil {
		return store.Account{}, fmt.Errorf("создать аккаунт: %w", err)
	}
	return account, s.AccountActivate(ctx, account.ID)
}

func (s *Store) AccountLogin(ctx context.Context, login string) (store.Account, string, error) {
	var account store.Account
	var hash string
	err := s.db.QueryRowContext(ctx, `SELECT id, login, email, password_hash FROM accounts WHERE login = ? COLLATE NOCASE`, strings.TrimSpace(login)).Scan(&account.ID, &account.Login, &account.Email, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return store.Account{}, "", errors.New("аккаунт не найден")
	}
	return account, hash, err
}

func (s *Store) AccountActivate(ctx context.Context, accountID string) error {
	return s.setSession(ctx, accountID, false)
}
func (s *Store) AccountSetGuest(ctx context.Context) (store.Account, error) {
	account := store.Account{ID: "guest", Login: "Гость", IsGuest: true}
	return account, s.setSession(ctx, "", true)
}
func (s *Store) AccountCurrent(ctx context.Context) (store.Account, bool, error) {
	var id sql.NullString
	var guest int
	err := s.db.QueryRowContext(ctx, `SELECT account_id, is_guest FROM account_session WHERE id = 1`).Scan(&id, &guest)
	if errors.Is(err, sql.ErrNoRows) {
		return store.Account{}, false, nil
	}
	if err != nil {
		return store.Account{}, false, err
	}
	if guest != 0 {
		return store.Account{ID: "guest", Login: "Гость", IsGuest: true}, true, nil
	}
	if !id.Valid || id.String == "" {
		return store.Account{}, false, nil
	}
	var account store.Account
	err = s.db.QueryRowContext(ctx, `SELECT id, login, email FROM accounts WHERE id = ?`, id.String).Scan(&account.ID, &account.Login, &account.Email)
	if errors.Is(err, sql.ErrNoRows) {
		return store.Account{}, false, nil
	}
	return account, err == nil, err
}
func (s *Store) AccountLogout(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM account_session WHERE id = 1`)
	return err
}
func (s *Store) setSession(ctx context.Context, accountID string, guest bool) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO account_session(id, account_id, is_guest, updated_at) VALUES (1, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET account_id=excluded.account_id, is_guest=excluded.is_guest, updated_at=excluded.updated_at`, accountID, boolInt(guest), time.Now().UnixMilli())
	return err
}
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func (s *Store) FavoriteAdd(ctx context.Context, t domain.Track) error {
	if s.db == nil {
		return errors.New("sqlite store не инициализирован")
	}
	artistsJSON, err := json.Marshal(t.Artists)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(
		ctx,
		`INSERT INTO favorites
		 (added_at, service, track_id, title, artists_json, album, duration_ms, artwork_url, external_url, playable_kind)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(service, track_id) DO NOTHING;`,
		time.Now().UnixMilli(),
		string(t.Service),
		t.ID,
		t.Title,
		string(artistsJSON),
		nullIfEmpty(t.Album),
		intOrNull(t.DurationMs),
		nullIfEmpty(t.ArtworkURL),
		nullIfEmpty(t.ExternalURL),
		string(t.PlayableKind),
	)
	return err
}

func (s *Store) FavoriteRemove(ctx context.Context, service domain.ServiceID, trackID string) error {
	if s.db == nil {
		return errors.New("sqlite store не инициализирован")
	}
	_, err := s.db.ExecContext(
		ctx,
		`DELETE FROM favorites WHERE service = ? AND track_id = ?;`,
		string(service),
		trackID,
	)
	return err
}

func (s *Store) FavoriteList(ctx context.Context) ([]domain.Track, error) {
	if s.db == nil {
		return nil, errors.New("sqlite store не инициализирован")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT service, track_id, title, artists_json, album, duration_ms, artwork_url, external_url, playable_kind
		FROM favorites
		ORDER BY added_at DESC
		LIMIT 500;
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]domain.Track, 0, 64)
	for rows.Next() {
		var (
			service      string
			trackID      string
			title        string
			artistsJSON  string
			album        sql.NullString
			durationMs   sql.NullInt64
			artworkURL   sql.NullString
			externalURL  sql.NullString
			playableKind string
		)
		if err := rows.Scan(&service, &trackID, &title, &artistsJSON, &album, &durationMs, &artworkURL, &externalURL, &playableKind); err != nil {
			return nil, err
		}
		var artists []string
		_ = json.Unmarshal([]byte(artistsJSON), &artists)

		out = append(out, domain.Track{
			ID:           trackID,
			Service:      domain.ServiceID(service),
			Title:        title,
			Artists:      artists,
			Album:        album.String,
			DurationMs:   int(durationMs.Int64),
			ArtworkURL:   artworkURL.String,
			ExternalURL:  externalURL.String,
			PlayableKind: domain.PlayableKind(playableKind),
		})
	}
	return out, rows.Err()
}

func (s *Store) TokenGet(ctx context.Context, serviceID domain.ServiceID) (store.Token, bool, error) {
	if s.db == nil {
		return store.Token{}, false, errors.New("sqlite store не инициализирован")
	}
	var (
		accessToken  sql.NullString
		refreshToken sql.NullString
		expiresAt    sql.NullInt64
		scope        sql.NullString
		tokenType    sql.NullString
	)
	err := s.db.QueryRowContext(
		ctx,
		`SELECT access_token, refresh_token, expires_at, scope, token_type
		 FROM tokens WHERE service = ?;`,
		string(serviceID),
	).Scan(&accessToken, &refreshToken, &expiresAt, &scope, &tokenType)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return store.Token{}, false, nil
		}
		return store.Token{}, false, err
	}
	return store.Token{
		AccessToken:  accessToken.String,
		RefreshToken: refreshToken.String,
		ExpiresAtMs:  expiresAt.Int64,
		Scope:        scope.String,
		TokenType:    tokenType.String,
	}, true, nil
}

func (s *Store) TokenUpsert(ctx context.Context, serviceID domain.ServiceID, token store.Token) error {
	if s.db == nil {
		return errors.New("sqlite store не инициализирован")
	}
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO tokens(service, access_token, refresh_token, expires_at, scope, token_type, updated_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(service) DO UPDATE SET
		   access_token=excluded.access_token,
		   refresh_token=excluded.refresh_token,
		   expires_at=excluded.expires_at,
		   scope=excluded.scope,
		   token_type=excluded.token_type,
		   updated_at=excluded.updated_at;`,
		string(serviceID),
		nullIfEmpty(token.AccessToken),
		nullIfEmpty(token.RefreshToken),
		intOrNull64(token.ExpiresAtMs),
		nullIfEmpty(token.Scope),
		nullIfEmpty(token.TokenType),
		time.Now().UnixMilli(),
	)
	return err
}

func (s *Store) SettingGet(ctx context.Context, key string) (string, bool, error) {
	if s.db == nil {
		return "", false, errors.New("sqlite store не инициализирован")
	}
	var v sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?;`, key).Scan(&v)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	return v.String, true, nil
}

func (s *Store) SettingSet(ctx context.Context, key string, value string) error {
	if s.db == nil {
		return errors.New("sqlite store не инициализирован")
	}
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO settings(key, value, updated_at)
		 VALUES(?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET
		   value=excluded.value,
		   updated_at=excluded.updated_at;`,
		key,
		value,
		time.Now().UnixMilli(),
	)
	return err
}

// RecoCacheGet возвращает закэшированное значение, если оно ещё не истекло.
// Просроченные записи трактуются как отсутствующие (и лениво удаляются).
func (s *Store) RecoCacheGet(ctx context.Context, key string) (string, bool, error) {
	if s.db == nil {
		return "", false, errors.New("sqlite store не инициализирован")
	}
	var (
		value     string
		expiresAt int64
	)
	err := s.db.QueryRowContext(ctx, `SELECT value, expires_at FROM reco_cache WHERE cache_key = ?;`, key).
		Scan(&value, &expiresAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	if time.Now().UnixMilli() >= expiresAt {
		_, _ = s.db.ExecContext(ctx, `DELETE FROM reco_cache WHERE cache_key = ?;`, key)
		return "", false, nil
	}
	return value, true, nil
}

// RecoCacheSet сохраняет значение с TTL. При ttl <= 0 берётся 24 часа.
func (s *Store) RecoCacheSet(ctx context.Context, key string, value string, ttl time.Duration) error {
	if s.db == nil {
		return errors.New("sqlite store не инициализирован")
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	expiresAt := time.Now().Add(ttl).UnixMilli()
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO reco_cache(cache_key, value, expires_at)
		 VALUES(?, ?, ?)
		 ON CONFLICT(cache_key) DO UPDATE SET
		   value=excluded.value,
		   expires_at=excluded.expires_at;`,
		key,
		value,
		expiresAt,
	)
	return err
}

func (s *Store) LocalFolderAdd(ctx context.Context, path string) error {
	if s.db == nil {
		return errors.New("sqlite store не инициализирован")
	}
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO local_folders(path, added_at) VALUES(?, ?)
		 ON CONFLICT(path) DO NOTHING;`,
		path,
		time.Now().UnixMilli(),
	)
	return err
}

func (s *Store) LocalFolderRemove(ctx context.Context, path string) error {
	if s.db == nil {
		return errors.New("sqlite store не инициализирован")
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM local_folders WHERE path = ?;`, path)
	return err
}

func (s *Store) LocalFolderList(ctx context.Context) ([]string, error) {
	if s.db == nil {
		return nil, errors.New("sqlite store не инициализирован")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT path FROM local_folders ORDER BY added_at ASC;`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]string, 0, 8)
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) LocalTrackReplaceAll(ctx context.Context, tracks []store.LocalTrackRow) error {
	if s.db == nil {
		return errors.New("sqlite store не инициализирован")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM local_tracks;`); err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO local_tracks(id, title, artists_json, album, file_path) VALUES(?, ?, ?, ?, ?);`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, t := range tracks {
		artistsJSON, mErr := json.Marshal(t.Artists)
		if mErr != nil {
			return mErr
		}
		if _, err := stmt.ExecContext(ctx, t.ID, t.Title, string(artistsJSON), nullIfEmpty(t.Album), t.FilePath); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) LocalTrackList(ctx context.Context) ([]store.LocalTrackRow, error) {
	if s.db == nil {
		return nil, errors.New("sqlite store не инициализирован")
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, title, artists_json, album, file_path FROM local_tracks ORDER BY title COLLATE NOCASE ASC;`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]store.LocalTrackRow, 0, 64)
	for rows.Next() {
		var (
			id          string
			title       string
			artistsJSON string
			album       sql.NullString
			filePath    string
		)
		if err := rows.Scan(&id, &title, &artistsJSON, &album, &filePath); err != nil {
			return nil, err
		}
		var artists []string
		_ = json.Unmarshal([]byte(artistsJSON), &artists)
		out = append(out, store.LocalTrackRow{
			ID:       id,
			Title:    title,
			Artists:  artists,
			Album:    album.String,
			FilePath: filePath,
		})
	}
	return out, rows.Err()
}

// --- История прослушивания ---

func (s *Store) HistoryAdd(ctx context.Context, t domain.Track) error {
	if s.db == nil {
		return errors.New("sqlite store не инициализирован")
	}
	artistsJSON, err := json.Marshal(t.Artists)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(
		ctx,
		`INSERT INTO history
		 (played_at, service, track_id, title, artists_json, album, duration_ms, artwork_url, external_url, playable_kind)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`,
		time.Now().UnixMilli(),
		string(t.Service),
		t.ID,
		t.Title,
		string(artistsJSON),
		nullIfEmpty(t.Album),
		intOrNull(t.DurationMs),
		nullIfEmpty(t.ArtworkURL),
		nullIfEmpty(t.ExternalURL),
		string(t.PlayableKind),
	)
	return err
}

func (s *Store) HistoryList(ctx context.Context, limit int) ([]store.HistoryRow, error) {
	if s.db == nil {
		return nil, errors.New("sqlite store не инициализирован")
	}
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT played_at, service, track_id, title, artists_json, album, duration_ms, artwork_url, external_url, playable_kind
		FROM history
		ORDER BY played_at DESC
		LIMIT ?;
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]store.HistoryRow, 0, 64)
	for rows.Next() {
		var (
			playedAt     int64
			service      string
			trackID      string
			title        string
			artistsJSON  string
			album        sql.NullString
			durationMs   sql.NullInt64
			artworkURL   sql.NullString
			externalURL  sql.NullString
			playableKind string
		)
		if err := rows.Scan(&playedAt, &service, &trackID, &title, &artistsJSON, &album, &durationMs, &artworkURL, &externalURL, &playableKind); err != nil {
			return nil, err
		}
		var artists []string
		_ = json.Unmarshal([]byte(artistsJSON), &artists)
		out = append(out, store.HistoryRow{
			PlayedAtMs: playedAt,
			Track: domain.Track{
				ID:           trackID,
				Service:      domain.ServiceID(service),
				Title:        title,
				Artists:      artists,
				Album:        album.String,
				DurationMs:   int(durationMs.Int64),
				ArtworkURL:   artworkURL.String,
				ExternalURL:  externalURL.String,
				PlayableKind: domain.PlayableKind(playableKind),
			},
		})
	}
	return out, rows.Err()
}

func (s *Store) HistoryClear(ctx context.Context) error {
	if s.db == nil {
		return errors.New("sqlite store не инициализирован")
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM history;`)
	return err
}

// --- Оповещения ---

func (s *Store) NotificationAdd(ctx context.Context, n store.NotificationRow) error {
	if s.db == nil {
		return errors.New("sqlite store не инициализирован")
	}
	read := 0
	if n.Read {
		read = 1
	}
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO notifications (id, created_at, kind, title, message, read)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO NOTHING;`,
		n.ID,
		n.CreatedAtMs,
		n.Kind,
		n.Title,
		nullIfEmpty(n.Message),
		read,
	)
	return err
}

func (s *Store) NotificationList(ctx context.Context, limit int) ([]store.NotificationRow, error) {
	if s.db == nil {
		return nil, errors.New("sqlite store не инициализирован")
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, created_at, kind, title, message, read
		FROM notifications
		ORDER BY created_at DESC
		LIMIT ?;
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]store.NotificationRow, 0, 32)
	for rows.Next() {
		var (
			id        string
			createdAt int64
			kind      string
			title     string
			message   sql.NullString
			read      int
		)
		if err := rows.Scan(&id, &createdAt, &kind, &title, &message, &read); err != nil {
			return nil, err
		}
		out = append(out, store.NotificationRow{
			ID:          id,
			CreatedAtMs: createdAt,
			Kind:        kind,
			Title:       title,
			Message:     message.String,
			Read:        read != 0,
		})
	}
	return out, rows.Err()
}

func (s *Store) NotificationMarkAllRead(ctx context.Context) error {
	if s.db == nil {
		return errors.New("sqlite store не инициализирован")
	}
	_, err := s.db.ExecContext(ctx, `UPDATE notifications SET read = 1 WHERE read = 0;`)
	return err
}

func (s *Store) NotificationRemove(ctx context.Context, id string) error {
	if s.db == nil {
		return errors.New("sqlite store не инициализирован")
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM notifications WHERE id = ?;`, id)
	return err
}

func (s *Store) NotificationClear(ctx context.Context) error {
	if s.db == nil {
		return errors.New("sqlite store не инициализирован")
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM notifications;`)
	return err
}

// --- Пользовательские плейлисты ---

func (s *Store) PlaylistCreate(ctx context.Context, title, description string) (domain.UserPlaylist, error) {
	if s.db == nil {
		return domain.UserPlaylist{}, errors.New("sqlite store не инициализирован")
	}
	now := time.Now().UnixMilli()
	id := fmt.Sprintf("pl_%d", now)
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO playlists(id, title, description, created_at) VALUES(?, ?, ?, ?);`,
		id, title, nullIfEmpty(description), now,
	)
	if err != nil {
		return domain.UserPlaylist{}, err
	}
	return domain.UserPlaylist{
		ID:          id,
		Title:       title,
		Description: description,
		TrackCount:  0,
		CreatedAtMs: now,
	}, nil
}

func (s *Store) PlaylistList(ctx context.Context) ([]domain.UserPlaylist, error) {
	if s.db == nil {
		return nil, errors.New("sqlite store не инициализирован")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.id, p.title, p.description, p.created_at,
		       (SELECT COUNT(*) FROM playlist_tracks pt WHERE pt.playlist_id = p.id) AS track_count
		FROM playlists p
		ORDER BY p.created_at DESC;
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]domain.UserPlaylist, 0, 16)
	for rows.Next() {
		var (
			id          string
			title       string
			description sql.NullString
			createdAt   int64
			trackCount  int
		)
		if err := rows.Scan(&id, &title, &description, &createdAt, &trackCount); err != nil {
			return nil, err
		}
		out = append(out, domain.UserPlaylist{
			ID:          id,
			Title:       title,
			Description: description.String,
			TrackCount:  trackCount,
			CreatedAtMs: createdAt,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Догружаем до 4 обложек первых треков каждого плейлиста для коллажа.
	// Отдельными запросами (после закрытия основного курсора), чтобы не
	// усложнять основной SELECT оконными функциями.
	for i := range out {
		covers, err := s.playlistCoverURLs(ctx, out[i].ID, 4)
		if err != nil {
			return nil, err
		}
		out[i].CoverURLs = covers
	}
	return out, nil
}

// playlistCoverURLs возвращает до limit непустых обложек первых треков
// плейлиста (в порядке добавления). Для коллажа на карточке плейлиста.
func (s *Store) playlistCoverURLs(ctx context.Context, playlistID string, limit int) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT artwork_url FROM playlist_tracks
		WHERE playlist_id = ? AND artwork_url IS NOT NULL AND artwork_url <> ''
		ORDER BY added_at ASC, row_id ASC
		LIMIT ?;
	`, playlistID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]string, 0, limit)
	for rows.Next() {
		var url string
		if err := rows.Scan(&url); err != nil {
			return nil, err
		}
		out = append(out, url)
	}
	return out, rows.Err()
}

func (s *Store) PlaylistUpdate(ctx context.Context, id, title, description string) error {
	if s.db == nil {
		return errors.New("sqlite store не инициализирован")
	}
	_, err := s.db.ExecContext(
		ctx,
		`UPDATE playlists SET title = ?, description = ? WHERE id = ?;`,
		title, nullIfEmpty(description), id,
	)
	return err
}

func (s *Store) PlaylistDelete(ctx context.Context, id string) error {
	if s.db == nil {
		return errors.New("sqlite store не инициализирован")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM playlist_tracks WHERE playlist_id = ?;`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM playlists WHERE id = ?;`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) PlaylistTracks(ctx context.Context, playlistID string) ([]domain.Track, error) {
	if s.db == nil {
		return nil, errors.New("sqlite store не инициализирован")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT service, track_id, title, artists_json, album, duration_ms, artwork_url, external_url, playable_kind
		FROM playlist_tracks
		WHERE playlist_id = ?
		ORDER BY added_at ASC;
	`, playlistID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]domain.Track, 0, 64)
	for rows.Next() {
		var (
			service      string
			trackID      string
			title        string
			artistsJSON  string
			album        sql.NullString
			durationMs   sql.NullInt64
			artworkURL   sql.NullString
			externalURL  sql.NullString
			playableKind string
		)
		if err := rows.Scan(&service, &trackID, &title, &artistsJSON, &album, &durationMs, &artworkURL, &externalURL, &playableKind); err != nil {
			return nil, err
		}
		var artists []string
		_ = json.Unmarshal([]byte(artistsJSON), &artists)
		out = append(out, domain.Track{
			ID:           trackID,
			Service:      domain.ServiceID(service),
			Title:        title,
			Artists:      artists,
			Album:        album.String,
			DurationMs:   int(durationMs.Int64),
			ArtworkURL:   artworkURL.String,
			ExternalURL:  externalURL.String,
			PlayableKind: domain.PlayableKind(playableKind),
		})
	}
	return out, rows.Err()
}

func (s *Store) PlaylistAddTrack(ctx context.Context, playlistID string, t domain.Track) error {
	if s.db == nil {
		return errors.New("sqlite store не инициализирован")
	}
	artistsJSON, err := json.Marshal(t.Artists)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(
		ctx,
		`INSERT INTO playlist_tracks
		 (playlist_id, added_at, service, track_id, title, artists_json, album, duration_ms, artwork_url, external_url, playable_kind)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(playlist_id, service, track_id) DO NOTHING;`,
		playlistID,
		time.Now().UnixMilli(),
		string(t.Service),
		t.ID,
		t.Title,
		string(artistsJSON),
		nullIfEmpty(t.Album),
		intOrNull(t.DurationMs),
		nullIfEmpty(t.ArtworkURL),
		nullIfEmpty(t.ExternalURL),
		string(t.PlayableKind),
	)
	return err
}

func (s *Store) PlaylistRemoveTrack(ctx context.Context, playlistID string, service domain.ServiceID, trackID string) error {
	if s.db == nil {
		return errors.New("sqlite store не инициализирован")
	}
	_, err := s.db.ExecContext(
		ctx,
		`DELETE FROM playlist_tracks WHERE playlist_id = ? AND service = ? AND track_id = ?;`,
		playlistID, string(service), trackID,
	)
	return err
}

// PlaylistReorder переписывает added_at треков по возрастанию в порядке keys,
// сохраняя относительный порядок отображения (PlaylistTracks сортирует по
// added_at ASC). Треки не из keys получают более поздние метки и уходят в
// конец. Всё в одной транзакции.
func (s *Store) PlaylistReorder(ctx context.Context, playlistID string, keys []store.PlaylistTrackKey) error {
	if s.db == nil {
		return errors.New("sqlite store не инициализирован")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// База отсчёта — фиксированная, чтобы порядок был детерминирован и не
	// пересекался со «старыми» added_at при последующих добавлениях.
	base := time.Now().UnixMilli()
	for i, k := range keys {
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE playlist_tracks SET added_at = ? WHERE playlist_id = ? AND service = ? AND track_id = ?;`,
			base+int64(i), playlistID, string(k.Service), k.TrackID,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// ResetAllData очищает все пользовательские данные в одной транзакции.
// Таблицы миграции остаются (структура), удаляется только содержимое.
// NOTE: добавлено koda — для сброса к «чистому аккаунту» перед передачей
// приложения другому пользователю.
func (s *Store) ResetAllData(ctx context.Context) error {
	if s.db == nil {
		return errors.New("sqlite store не инициализирован")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	tables := []string{
		"history",
		"favorites",
		"playlist_tracks",
		"playlists",
		"local_tracks",
		"local_folders",
		"settings",
		"tokens",
		"reco_cache",
		"notifications",
	}
	for _, t := range tables {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s;`, t)); err != nil {
			return fmt.Errorf("сброс таблицы %s: %w", t, err)
		}
	}
	return tx.Commit()
}

func intOrNull(v int) any {
	if v == 0 {
		return nil
	}
	return v
}

func intOrNull64(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}
