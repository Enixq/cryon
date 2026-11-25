package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"runtime"
	"sync"
	"time"

	"Cryon2/internal/domain"
	"Cryon2/internal/services/vkweb"
)

const keyVKWebSession = "vk.webSession"

type vkWebLogin struct {
	sync.Mutex
	current *vkWebAttempt
}
type vkWebAttempt struct {
	ID     string `json:"id"`
	State  string `json:"state"`
	Error  string `json:"error,omitempty"`
	ctx    context.Context
	cancel context.CancelFunc
}

func (a *App) handleVKWebLogin(w http.ResponseWriter, r *http.Request) {
	provider, ok := a.registry[domain.ServiceVK].(*vkweb.Service)
	if !ok || a.store == nil {
		writeAPIError(w, 503, "ВК Музыка недоступна")
		return
	}
	if r.Method == http.MethodGet {
		a.vkWebLogin.Lock()
		defer a.vkWebLogin.Unlock()
		var attempt *vkWebAttempt
		if current := a.vkWebLogin.current; current != nil && current.ID == r.URL.Query().Get("id") {
			attempt = current
		}
		remaining, limitMessage := 0, ""
		if limited := provider.RateLimit(); limited != nil {
			remaining, limitMessage = int(limited.RetryAfter.Seconds())+1, limited.Error()
		}
		writeAPIJSON(w, 200, map[string]any{"connected": provider.HasToken(), "session": attempt, "browserSupported": vkweb.BrowserSupported() || runtime.GOOS == "android", "retryAfterSeconds": remaining, "limitMessage": limitMessage})
		return
	}
	if r.Method == http.MethodDelete {
		a.vkWebLogin.Lock()
		defer a.vkWebLogin.Unlock()
		if attempt := a.vkWebLogin.current; attempt != nil {
			if id := r.URL.Query().Get("id"); id != "" && id != attempt.ID {
				writeAPIError(w, 409, "сеанс входа ВК изменился")
				return
			}
			attempt.cancel()
			if attempt.State == "browser" || attempt.State == "checking" {
				attempt.State = "cancelled"
			}
		}
		if r.URL.Query().Get("disconnect") == "1" {
			if err := a.store.SettingSet(r.Context(), keyVKWebSession, ""); err != nil {
				writeAPIAppError(w, err)
				return
			}
			provider.SetSession(vkweb.Session{})
			a.clearSourceValidation(domain.ServiceVK)
			a.emitSourceStatusChanged()
		}
		w.WriteHeader(204)
		return
	}
	if r.Method != http.MethodPost {
		writeAPIError(w, 405, "требуется GET, POST или DELETE")
		return
	}
	if !vkweb.BrowserSupported() && runtime.GOOS != "android" {
		writeAPIError(w, 400, "Веб-вход ВК доступен на Windows и Android")
		return
	}
	if limited := provider.RateLimit(); limited != nil {
		writeAPIError(w, http.StatusTooManyRequests, limited.Error())
		return
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		writeAPIAppError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	attempt := &vkWebAttempt{ID: hex.EncodeToString(random[:]), State: "browser", ctx: ctx, cancel: cancel}
	a.vkWebLogin.Lock()
	if previous := a.vkWebLogin.current; previous != nil {
		previous.cancel()
	}
	a.vkWebLogin.current = attempt
	writeAPIJSON(w, 202, attempt)
	a.vkWebLogin.Unlock()
	go func() {
		if runtime.GOOS == "android" {
			<-ctx.Done()
			a.vkWebLogin.Lock()
			defer a.vkWebLogin.Unlock()
			if a.vkWebLogin.current == attempt && attempt.State == "browser" {
				attempt.State = "error"
				attempt.Error = "Время входа ВК истекло. Откройте вход заново."
			}
			return
		}
		defer cancel()
		session, err := vkweb.ConnectBrowser(ctx)
		a.completeVKWebLogin(attempt, session, err)
	}()
}

// Android delivers website cookies from a separate native WebView with no JS bridge.
func (a *App) handleVKWebSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || runtime.GOOS != "android" {
		writeAPIError(w, 405, "недоступно в этом runtime")
		return
	}
	var input struct {
		ID        string `json:"id"`
		Cookies   string `json:"cookies"`
		UserAgent string `json:"userAgent"`
	}
	if decodeAPIJSON(w, r, &input) != nil {
		return
	}
	a.vkWebLogin.Lock()
	attempt := a.vkWebLogin.current
	if attempt == nil || attempt.ID != input.ID || attempt.State != "browser" || attempt.ctx.Err() != nil {
		a.vkWebLogin.Unlock()
		writeAPIError(w, 409, "сеанс входа ВК завершён")
		return
	}
	attempt.State = "checking"
	a.vkWebLogin.Unlock()
	session, err := vkweb.ParseCookies(input.Cookies, input.UserAgent)
	a.completeVKWebLogin(attempt, session, err)
	a.vkWebLogin.Lock()
	defer a.vkWebLogin.Unlock()
	if attempt.State == "connected" {
		writeAPIJSON(w, 200, map[string]bool{"connected": true})
	} else {
		writeAPIError(w, 400, attempt.Error)
	}
}

func (a *App) completeVKWebLogin(attempt *vkWebAttempt, session vkweb.Session, err error) {
	defer attempt.cancel()
	if err == nil {
		a.vkWebLogin.Lock()
		if a.vkWebLogin.current != attempt || attempt.ctx.Err() != nil {
			a.vkWebLogin.Unlock()
			return
		}
		attempt.State = "checking"
		a.vkWebLogin.Unlock()
		candidate := vkweb.New()
		candidate.SetSession(session)
		err = candidate.ValidateToken(attempt.ctx)
		if err == nil {
			session = candidate.Session()
		}
	}
	a.vkWebLogin.Lock()
	defer a.vkWebLogin.Unlock()
	if a.vkWebLogin.current != attempt || attempt.State == "cancelled" {
		return
	}
	if err == nil && attempt.ctx.Err() != nil {
		err = attempt.ctx.Err()
	}
	if err == nil {
		var encoded string
		encoded, err = vkweb.EncodeSession(session)
		if err == nil {
			err = a.store.SettingSet(attempt.ctx, keyVKWebSession, encoded)
		}
		if err == nil {
			if provider, ok := a.registry[domain.ServiceVK].(*vkweb.Service); ok {
				provider.SetSession(session)
				a.clearSourceValidation(domain.ServiceVK)
				a.emitSourceStatusChanged()
				attempt.State = "connected"
				return
			}
		}
	}
	attempt.State = "error"
	attempt.Error = vkWebError(err)
}
func vkWebError(err error) string {
	var known *vkweb.Error
	if errors.As(err, &known) {
		return known.Error()
	}
	if errors.Is(err, context.Canceled) {
		return "Вход ВК отменён."
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "Время входа ВК истекло. Откройте вход заново."
	}
	return "Не удалось сохранить подключение ВК на этом устройстве."
}
