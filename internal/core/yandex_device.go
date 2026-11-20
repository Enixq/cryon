package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Public OAuth client used by the existing Yandex Music token helper.
const yandexDeviceClientSecret = "53bc75238f0c4d08a118e51fe9203300"

var yandexDeviceHTTP = &http.Client{Timeout: 15 * time.Second}

func deviceRequest(ctx context.Context, endpoint string, form url.Values) (map[string]any, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := yandexDeviceHTTP.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	var data map[string]any
	err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&data)
	return data, resp.StatusCode, err
}

func (a *App) handleYandexDeviceStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		writeAPIError(w, http.StatusMethodNotAllowed, "требуется POST с JSON")
		return
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		writeAPIAppError(w, err)
		return
	}
	data, status, err := deviceRequest(r.Context(), "https://oauth.yandex.ru/device/code", url.Values{"client_id": {yandexOAuthClientID}, "device_id": {hex.EncodeToString(id[:])}, "device_name": {"Cryon"}})
	if err != nil {
		writeAPIError(w, 502, "Не удалось связаться с Яндексом: "+err.Error())
		return
	}
	code, _ := data["device_code"].(string)
	userCode, _ := data["user_code"].(string)
	if status != 200 || code == "" || userCode == "" {
		writeAPIError(w, 502, "Яндекс не выдал код подтверждения")
		return
	}
	expires, _ := data["expires_in"].(float64)
	if expires <= 0 {
		expires = 300
	}
	interval, _ := data["interval"].(float64)
	if interval < 5 {
		interval = 5
	}
	a.yaLogin.mu.Lock()
	a.yaLogin.deviceCode = code
	a.yaLogin.expires = time.Now().Add(time.Duration(expires) * time.Second)
	a.yaLogin.mu.Unlock()
	writeAPIJSON(w, 200, map[string]any{"code": userCode, "interval": interval, "expiresIn": expires})
}

func (a *App) handleYandexDevicePoll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		writeAPIError(w, 405, "требуется POST с JSON")
		return
	}
	a.yaLogin.mu.Lock()
	code, expiry := a.yaLogin.deviceCode, a.yaLogin.expires
	a.yaLogin.mu.Unlock()
	if code == "" || time.Now().After(expiry) {
		writeAPIError(w, 410, "Срок действия кода истёк. Получите новый код.")
		return
	}
	data, status, err := deviceRequest(r.Context(), "https://oauth.yandex.ru/token", url.Values{"grant_type": {"device_code"}, "code": {code}, "client_id": {yandexOAuthClientID}, "client_secret": {yandexDeviceClientSecret}})
	if err != nil {
		writeAPIError(w, 502, "Не удалось проверить подтверждение: "+err.Error())
		return
	}
	if status != 200 {
		reason, _ := data["error"].(string)
		if reason == "authorization_pending" || reason == "slow_down" {
			writeAPIJSON(w, 200, map[string]any{"connected": false, "slowDown": reason == "slow_down"})
			return
		}
		writeAPIError(w, 400, "Яндекс отклонил подтверждение: "+reason)
		return
	}
	token, _ := data["access_token"].(string)
	if token == "" {
		writeAPIError(w, 502, "Яндекс не вернул токен")
		return
	}
	a.yaLogin.mu.Lock()
	defer a.yaLogin.mu.Unlock()
	if a.yaLogin.deviceCode != code {
		writeAPIError(w, 409, "Вход отменён или начат заново")
		return
	}
	if err := a.SetYandexToken(token); err != nil {
		writeAPIAppError(w, err)
		return
	}
	a.yaLogin.deviceCode = ""
	writeAPIJSON(w, 200, map[string]any{"connected": true})
}
