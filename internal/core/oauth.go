package core
import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"Cryon2/internal/domain"
	"Cryon2/internal/store"
)

// oauthCoordinator owns the one active loopback OAuth callback server.
type oauthCoordinator struct {
	app    *App
	mu     sync.Mutex
	active bool
}

// spotifyRedirectURI must exactly match the Redirect URI configured in the
// Spotify Developer Dashboard. A stable local port is required by Spotify;
// dynamically choosing one makes every authorization request invalid.
const spotifyRedirectURI = "http://127.0.0.1:8888/callback"

func newOAuthCoordinator(app *App) *oauthCoordinator {
	return &oauthCoordinator{app: app}
}

func randomURLToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// StartSpotifyLogin initiates Authorization Code with PKCE in the system browser.
func (a *App) StartSpotifyLogin() error {
	if a.store == nil || a.ctx == nil {
		return fmt.Errorf("хранилище недоступно")
	}
	clientID, _, err := a.store.SettingGet(a.ctx, keySpotifyOAuthClientID)
	if err != nil || strings.TrimSpace(clientID) == "" {
		return fmt.Errorf("сначала укажите OAuth Client ID Spotify")
	}
	return a.oauth.startSpotify(strings.TrimSpace(clientID))
}

func (o *oauthCoordinator) startSpotify(clientID string) error {
	o.mu.Lock()
	if o.active {
		o.mu.Unlock()
		return fmt.Errorf("авторизация уже открыта")
	}
	o.active = true
	o.mu.Unlock()

	listener, err := net.Listen("tcp", "127.0.0.1:8888")
	if err != nil {
		o.finish()
		return fmt.Errorf("не удалось открыть OAuth callback на 127.0.0.1:8888: %w", err)
	}
	redirectURI := spotifyRedirectURI
	state, err := randomURLToken(24)
	if err != nil {
		_ = listener.Close()
		o.finish()
		return err
	}
	verifier, err := randomURLToken(48)
	if err != nil {
		_ = listener.Close()
		o.finish()
		return err
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	done := make(chan struct{})
	var once sync.Once
	finish := func() { once.Do(func() { close(done) }) }
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("state") != state {
			http.Error(w, "Invalid OAuth state", http.StatusBadRequest)
			finish()
			return
		}
		if query.Get("error") != "" {
			fmt.Fprint(w, "Вход отменён. Можно закрыть это окно.")
			finish()
			return
		}
		if err := o.exchangeSpotify(r.Context(), clientID, redirectURI, query.Get("code"), verifier); err != nil {
			http.Error(w, "Не удалось завершить вход: "+err.Error(), http.StatusInternalServerError)
		} else {
			fmt.Fprint(w, "Spotify подключён. Можно закрыть это окно и вернуться в Cryon.")
		}
		finish()
	})

	server := &http.Server{Handler: mux}
	go func() { _ = server.Serve(listener) }()
	go func() {
		select {
		case <-done:
		case <-time.After(5 * time.Minute):
		}
		_ = server.Shutdown(context.Background())
		o.finish()
	}()

	u, _ := url.Parse("https://accounts.spotify.com/authorize")
	query := u.Query()
	query.Set("client_id", clientID)
	query.Set("response_type", "code")
	query.Set("redirect_uri", redirectURI)
	query.Set("scope", "user-library-read")
	query.Set("state", state)
	query.Set("code_challenge_method", "S256")
	query.Set("code_challenge", challenge)
	u.RawQuery = query.Encode()
	o.app.platform.OpenURL(u.String())
	return nil
}

func (o *oauthCoordinator) finish() {
	o.mu.Lock()
	o.active = false
	o.mu.Unlock()
}

func (o *oauthCoordinator) exchangeSpotify(ctx context.Context, clientID, redirectURI, code, verifier string) error {
	if code == "" {
		return fmt.Errorf("Spotify не вернул код авторизации")
	}
	token, err := spotifyTokenRequest(ctx, url.Values{
		"client_id":     {clientID},
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {verifier},
	})
	if err != nil {
		return err
	}
	if err = o.app.store.TokenUpsert(o.app.ctx, domain.ServiceSpotify, token); err != nil {
		return err
	}
	o.app.emitSourceStatusChanged()
	o.app.pushNotification("success", "Spotify подключён", "Теперь можно импортировать любимые треки.")
	return nil
}

func refreshSpotifyToken(ctx context.Context, clientID, refreshToken string) (store.Token, error) {
	token, err := spotifyTokenRequest(ctx, url.Values{
		"client_id":     {clientID},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	})
	if err == nil && token.RefreshToken == "" {
		token.RefreshToken = refreshToken
	}
	return token, err
}

func spotifyTokenRequest(ctx context.Context, form url.Values) (store.Token, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://accounts.spotify.com/api/token", strings.NewReader(form.Encode()))
	if err != nil {
		return store.Token{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return store.Token{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return store.Token{}, fmt.Errorf("spotify token: %s", strings.TrimSpace(string(body)))
	}
	var raw struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		Scope        string `json:"scope"`
		TokenType    string `json:"token_type"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return store.Token{}, err
	}
	if raw.AccessToken == "" {
		return store.Token{}, fmt.Errorf("Spotify вернул пустой access token")
	}
	return store.Token{
		AccessToken:  raw.AccessToken,
		RefreshToken: raw.RefreshToken,
		ExpiresAtMs:  time.Now().Add(time.Duration(raw.ExpiresIn) * time.Second).UnixMilli(),
		Scope:        raw.Scope,
		TokenType:    raw.TokenType,
	}, nil
}
