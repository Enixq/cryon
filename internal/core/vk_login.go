package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"Cryon2/internal/domain"
	"Cryon2/internal/services/vk"
	vkmauth "Cryon2/internal/services/vkauth"
)

type vkLoginSession struct {
	ID        string `json:"id"`
	State     string `json:"state"`
	Method    string `json:"method,omitempty"`
	CanResend bool   `json:"canResend"`
	Error     string `json:"error,omitempty"`
	cancel    context.CancelFunc
	code      chan vkmauth.GotCode
}
type vkLoginState struct {
	sync.Mutex
	session *vkLoginSession
}

func (a *App) saveVKToken(ctx context.Context, token string) error {
	service, ok := a.registry[domain.ServiceVK].(*vk.Service)
	if !ok || a.store == nil {
		return fmt.Errorf("ВК Музыка недоступна")
	}
	token = strings.TrimSpace(token)
	if token != "" {
		probe := vk.New(token)
		if err := probe.ValidateToken(ctx); err != nil {
			return err
		}
	}
	if err := a.store.SettingSet(ctx, "vk.token", token); err != nil {
		return err
	}
	service.SetToken(token)
	a.clearSourceValidation(domain.ServiceVK)
	a.emitSourceStatusChanged()
	return nil
}

// This route is only on the loopback API, never the LAN sync listener.
func (a *App) handleAPIVKLogin(w http.ResponseWriter, r *http.Request) {
	service, ok := a.registry[domain.ServiceVK].(*vk.Service)
	if !ok {
		writeAPIError(w, 503, "ВК Музыка недоступна")
		return
	}
	if r.Method == http.MethodGet {
		a.vkLogin.Lock()
		defer a.vkLogin.Unlock()
		var session *vkLoginSession
		if current := a.vkLogin.session; current != nil && current.ID == r.URL.Query().Get("id") {
			session = current
		}
		writeAPIJSON(w, 200, map[string]any{"connected": service.HasToken(), "session": session})
		return
	}
	if r.Method == http.MethodDelete {
		a.vkLogin.Lock()
		defer a.vkLogin.Unlock()
		if current := a.vkLogin.session; current != nil && current.ID == r.URL.Query().Get("id") {
			current.cancel()
			current.State = "cancelled"
		}
		writeAPIJSON(w, 200, map[string]bool{"cancelled": true})
		return
	}
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		writeAPIError(w, 405, "требуется GET, POST, PUT или DELETE")
		return
	}
	var input struct {
		Phone    string `json:"phone"`
		Password string `json:"password"`
		Token    string `json:"token"`
		ID       string `json:"id"`
		Code     string `json:"code"`
		Resend   bool   `json:"resend"`
	}
	if decodeAPIJSON(w, r, &input) != nil {
		return
	}
	if r.Method == http.MethodPut {
		if len(input.Token) > 8192 {
			writeAPIError(w, 400, "слишком длинный токен")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		if err := a.saveVKToken(ctx, input.Token); err != nil {
			writeAPIAppError(w, err)
			return
		}
		writeAPIJSON(w, 200, map[string]bool{"connected": input.Token != ""})
		return
	}
	if input.ID != "" {
		a.vkLogin.Lock()
		defer a.vkLogin.Unlock()
		session := a.vkLogin.session
		if session == nil || session.ID != input.ID || session.State != "code" {
			writeAPIError(w, 409, "сеанс входа не ожидает код")
			return
		}
		if input.Resend && !session.CanResend {
			writeAPIError(w, 400, "повторная отправка недоступна")
			return
		}
		if !input.Resend && (strings.TrimSpace(input.Code) == "" || len(input.Code) > 20) {
			writeAPIError(w, 400, "введите код подтверждения")
			return
		}
		select {
		case session.code <- vkmauth.GotCode{Code: strings.TrimSpace(input.Code), Resend: input.Resend}:
			session.State = "loading"
		default:
			writeAPIError(w, 409, "код уже обрабатывается")
			return
		}
		writeAPIJSON(w, 200, session)
		return
	}
	if strings.TrimSpace(input.Phone) == "" || len(input.Phone) > 40 || len(input.Password) > 1024 {
		writeAPIError(w, 400, "введите номер телефона ВК")
		return
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		writeAPIAppError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	session := &vkLoginSession{ID: hex.EncodeToString(random[:]), State: "loading", cancel: cancel, code: make(chan vkmauth.GotCode, 1)}
	a.vkLogin.Lock()
	if previous := a.vkLogin.session; previous != nil {
		previous.cancel()
	}
	a.vkLogin.session = session
	writeAPIJSON(w, 200, session)
	a.vkLogin.Unlock()
	go func() {
		defer cancel()
		// Phone/password live only in this call and are never persisted or logged.
		token, err := vkmauth.New(ctx, strings.TrimSpace(input.Phone), input.Password, func(method vkmauth.CodeSended) (vkmauth.GotCode, error) {
			a.vkLogin.Lock()
			session.State, session.Method, session.CanResend = "code", method.Current.String(), method.Resend != ""
			a.vkLogin.Unlock()
			select {
			case code := <-session.code:
				return code, nil
			case <-ctx.Done():
				return vkmauth.GotCode{}, ctx.Err()
			}
		})
		input.Password, input.Phone = "", ""
		a.vkLogin.Lock()
		defer a.vkLogin.Unlock()
		if a.vkLogin.session != session || session.State == "cancelled" {
			return
		}
		if err == nil && token != nil && token.AccessToken != "" && ctx.Err() == nil {
			err = a.saveVKToken(ctx, token.AccessToken)
		}
		if err != nil || token == nil || token.AccessToken == "" || ctx.Err() != nil {
			session.State, session.Error = "error", vkLoginMessage(err)
			return
		}
		session.State = "connected"
	}()
}

func vkLoginMessage(err error) string {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return "Время входа истекло. Начните вход заново и введите свежий код ВК."
	}
	var step *vkmauth.StageError
	if errors.As(err, &step) {
		switch step.Stage {
		case "phone":
			return "ВК не создал сеанс подтверждения номера. Проверьте номер с кодом страны. Если SMS уже пришла, начните вход заново; код от прежнего сеанса не подойдёт."
		case "code":
			return "ВК отклонил код подтверждения. Начните вход заново и введите самый последний код."
		case "password":
			return "ВК подтвердил номер, но не выдал разрешение после проверки пароля. Проверьте пароль и уведомления безопасности в ВК."
		case "exchange", "music-token":
			return "ВК подтвердил вход, но не выдал токен Музыки. Этот способ входа сейчас недоступен у ВК."
		default:
			return "ВК не ответил при начале входа. Проверьте сеть и повторите попытку."
		}
	}
	return "Не удалось проверить подключение к ВК Музыке. Проверьте сеть и повторите вход."
}
