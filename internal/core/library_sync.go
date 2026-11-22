package core

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"Cryon2/internal/store"
)

type librarySyncStore interface {
	LibrarySyncSnapshot(context.Context) ([]store.LibraryEntry, error)
	LibrarySyncMerge(context.Context, []store.LibraryEntry) (int, error)
}
type librarySyncConfig struct {
	Enabled bool   `json:"enabled"`
	PeerURL string `json:"peerUrl"`
	PeerKey string `json:"peerKey"`
	Key     string `json:"key"`
}
type libraryEnvelope struct {
	Nonce string `json:"nonce"`
	Data  string `json:"data"`
}

const libraryConfigKey = "library.sync.config"
const libraryExchangePath = "/v1/library/sync/exchange"
const libraryMaxPlaintext = 20 << 20

func libraryAEAD(key string) (cipher.AEAD, error) {
	data, err := hex.DecodeString(key)
	if err != nil || len(data) != 32 {
		return nil, fmt.Errorf("неверный ключ сопряжения")
	}
	block, err := aes.NewCipher(data)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
func sealLibrary(key, direction string, entries []store.LibraryEntry) (libraryEnvelope, error) {
	aead, err := libraryAEAD(key)
	if err != nil {
		return libraryEnvelope{}, err
	}
	data, err := json.Marshal(entries)
	if err != nil {
		return libraryEnvelope{}, err
	}
	if len(data) > libraryMaxPlaintext || len(entries) > 20000 {
		return libraryEnvelope{}, fmt.Errorf("слишком большая библиотека")
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return libraryEnvelope{}, err
	}
	return libraryEnvelope{base64.StdEncoding.EncodeToString(nonce), base64.StdEncoding.EncodeToString(aead.Seal(nil, nonce, data, []byte("CryonLibrarySync/v1/"+direction)))}, nil
}
func openLibrary(key, direction string, envelope libraryEnvelope) ([]store.LibraryEntry, error) {
	aead, err := libraryAEAD(key)
	if err != nil {
		return nil, err
	}
	nonce, err := base64.StdEncoding.DecodeString(envelope.Nonce)
	if err != nil || len(nonce) != aead.NonceSize() {
		return nil, fmt.Errorf("неверный пакет сопряжения")
	}
	data, err := base64.StdEncoding.DecodeString(envelope.Data)
	if err != nil || len(data) > libraryMaxPlaintext+aead.Overhead() {
		return nil, fmt.Errorf("неверный пакет библиотеки")
	}
	data, err = aead.Open(nil, nonce, data, []byte("CryonLibrarySync/v1/"+direction))
	if err != nil {
		return nil, fmt.Errorf("ключ сопряжения не совпадает")
	}
	var entries []store.LibraryEntry
	if err = json.Unmarshal(data, &entries); err != nil || len(entries) > 20000 {
		return nil, fmt.Errorf("неверная библиотека")
	}
	return entries, nil
}
func (a *App) libraryConfig(ctx context.Context) (librarySyncConfig, error) {
	var config librarySyncConfig
	if a.store == nil {
		return config, fmt.Errorf("хранилище недоступно")
	}
	raw, _, err := a.store.SettingGet(ctx, libraryConfigKey)
	if err != nil {
		return config, err
	}
	if raw != "" {
		err = json.Unmarshal([]byte(raw), &config)
	}
	return config, err
}
func (a *App) saveLibraryConfig(ctx context.Context, config librarySyncConfig) error {
	data, err := json.Marshal(config)
	if err != nil {
		return err
	}
	return a.store.SettingSet(ctx, libraryConfigKey, string(data))
}
func validLibraryPeer(target string) bool {
	uri, err := url.Parse(target)
	if err != nil || uri.Scheme != "http" || uri.User != nil || uri.RawQuery != "" || uri.Fragment != "" || (uri.Path != "" && uri.Path != "/") || uri.Port() == "" {
		return false
	}
	ip := net.ParseIP(uri.Hostname())
	return ip != nil && (ip.IsPrivate() || ip.IsLoopback()) && !ip.IsUnspecified() && !ip.IsMulticast()
}

func (a *App) handleAPILibrarySyncConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.librarySyncConfigMu.Lock()
		defer a.librarySyncConfigMu.Unlock()
	}
	config, err := a.libraryConfig(r.Context())
	if err != nil {
		writeAPIAppError(w, err)
		return
	}
	if r.Method == http.MethodGet {
		last, _, _ := a.store.SettingGet(r.Context(), "library.sync.last")
		a.librarySyncStatusMu.Lock()
		message := a.librarySyncMessage
		a.librarySyncStatusMu.Unlock()
		writeAPIJSON(w, 200, map[string]any{"enabled": config.Enabled, "peerUrl": config.PeerURL, "paired": config.PeerKey != "", "lastSync": last, "message": message})
		return
	}
	if r.Method == http.MethodPut {
		var input struct {
			Enabled bool   `json:"enabled"`
			PeerURL string `json:"peerUrl"`
			PeerKey string `json:"peerKey"`
		}
		if decodeAPIJSON(w, r, &input) != nil {
			return
		}
		input.PeerURL = strings.TrimRight(strings.TrimSpace(input.PeerURL), "/")
		if input.PeerURL != "" && !validLibraryPeer(input.PeerURL) {
			writeAPIError(w, 400, "укажите локальный адрес http://192.168.1.50:8899")
			return
		}
		if input.PeerKey != "" {
			if _, err := libraryAEAD(strings.TrimSpace(input.PeerKey)); err != nil {
				writeAPIAppError(w, err)
				return
			}
			config.PeerKey = strings.TrimSpace(input.PeerKey)
		}
		if input.PeerURL == "" {
			config.PeerKey = ""
		}
		if input.PeerURL != "" && config.PeerKey == "" {
			writeAPIError(w, 400, "введите ключ сопряжения другого устройства")
			return
		}
		config.Enabled, config.PeerURL = input.Enabled, input.PeerURL
		if err = a.saveLibraryConfig(r.Context(), config); err != nil {
			writeAPIAppError(w, err)
			return
		}
		writeAPIJSON(w, 200, map[string]bool{"saved": true})
		return
	}
	if r.Method != http.MethodPost {
		writeAPIError(w, 405, "требуется GET, PUT или POST")
		return
	}
	var input struct {
		Action string `json:"action"`
	}
	if decodeAPIJSON(w, r, &input) != nil {
		return
	}
	if input.Action == "pairing-key" || input.Action == "rotate-key" {
		// Only an explicit local user action reveals a persistent pairing secret.
		config, err = a.libraryConfig(r.Context())
		if err != nil {
			writeAPIAppError(w, err)
			return
		}
		if config.Key == "" || input.Action == "rotate-key" {
			var key [32]byte
			if _, err = rand.Read(key[:]); err != nil {
				writeAPIAppError(w, err)
				return
			}
			config.Key = hex.EncodeToString(key[:])
			if err = a.saveLibraryConfig(r.Context(), config); err != nil {
				writeAPIAppError(w, err)
				return
			}
		}
		writeAPIJSON(w, 200, map[string]string{"key": config.Key})
		return
	}
	if input.Action != "sync" {
		writeAPIError(w, 400, "неизвестное действие")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	if err = a.syncLibraryOnce(ctx); err != nil {
		writeAPIAppError(w, err)
		return
	}
	a.librarySyncStatusMu.Lock()
	a.librarySyncMessage = ""
	a.librarySyncStatusMu.Unlock()
	writeAPIJSON(w, 200, map[string]bool{"synced": true})
}

func (a *App) handleAPILibraryExchange(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, 405, "требуется POST")
		return
	}
	config, err := a.libraryConfig(r.Context())
	if err != nil || !config.Enabled || config.Key == "" {
		writeAPIError(w, 403, "синхронизация библиотеки отключена")
		return
	}
	var envelope libraryEnvelope
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<20)).Decode(&envelope) != nil {
		writeAPIError(w, 400, "неверный пакет")
		return
	}
	entries, err := openLibrary(config.Key, "request", envelope)
	if err != nil {
		writeAPIError(w, 403, "ключ сопряжения не совпадает")
		return
	}
	storage, ok := a.store.(librarySyncStore)
	if !ok {
		writeAPIError(w, 503, "синхронизация недоступна")
		return
	}
	changes, err := storage.LibrarySyncMerge(r.Context(), entries)
	if err != nil {
		writeAPIAppError(w, err)
		return
	}
	snapshot, err := storage.LibrarySyncSnapshot(r.Context())
	if err != nil {
		writeAPIAppError(w, err)
		return
	}
	encrypted, err := sealLibrary(config.Key, "response", snapshot)
	if err != nil {
		writeAPIAppError(w, err)
		return
	}
	if changes > 0 && a.platform != nil {
		a.platform.Emit(favoritesChangedEvent)
	}
	_ = a.store.SettingSet(r.Context(), "library.sync.last", time.Now().UTC().Format(time.RFC3339))
	writeAPIJSON(w, 200, encrypted)
}

func (a *App) syncLibraryOnce(ctx context.Context) error {
	a.librarySyncMu.Lock()
	defer a.librarySyncMu.Unlock()
	config, err := a.libraryConfig(ctx)
	if err != nil {
		return err
	}
	if !config.Enabled || config.PeerURL == "" || config.PeerKey == "" {
		return fmt.Errorf("задайте устройство и ключ сопряжения библиотеки")
	}
	if !validLibraryPeer(config.PeerURL) {
		return fmt.Errorf("адрес устройства не является локальным")
	}
	storage, ok := a.store.(librarySyncStore)
	if !ok {
		return fmt.Errorf("синхронизация недоступна")
	}
	snapshot, err := storage.LibrarySyncSnapshot(ctx)
	if err != nil {
		return err
	}
	envelope, err := sealLibrary(config.PeerKey, "request", snapshot)
	if err != nil {
		return err
	}
	data, _ := json.Marshal(envelope)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, config.PeerURL+libraryExchangePath, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("устройство недоступно; изменения будут переданы после подключения")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return fmt.Errorf("устройство отклонило синхронизацию: HTTP %d; проверьте ключ и разрешение обмена", response.StatusCode)
	}
	var incoming libraryEnvelope
	if err = json.NewDecoder(io.LimitReader(response.Body, 32<<20)).Decode(&incoming); err != nil {
		return fmt.Errorf("неполный ответ устройства")
	}
	entries, err := openLibrary(config.PeerKey, "response", incoming)
	if err != nil {
		return err
	}
	changes, err := storage.LibrarySyncMerge(ctx, entries)
	if err != nil {
		return err
	}
	if changes > 0 && a.platform != nil {
		a.platform.Emit(favoritesChangedEvent)
	}
	return a.store.SettingSet(ctx, "library.sync.last", time.Now().UTC().Format(time.RFC3339))
}
func (a *App) startLibrarySync(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	a.librarySyncCancel = cancel
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				config, err := a.libraryConfig(ctx)
				if err != nil || !config.Enabled || config.PeerURL == "" {
					continue
				}
				attempt, done := context.WithTimeout(ctx, 20*time.Second)
				err = a.syncLibraryOnce(attempt)
				done()
				a.librarySyncStatusMu.Lock()
				a.librarySyncMessage = ""
				if err != nil {
					a.librarySyncMessage = err.Error()
				}
				a.librarySyncStatusMu.Unlock()
			}
		}
	}()
}
