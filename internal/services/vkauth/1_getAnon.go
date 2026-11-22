package vkmauth

import (
	"context"

	"github.com/oklog/ulid/v2"
)

func getAnonymousToken(ctx context.Context) (*anonymousToken, error) {
	deviceId := ulid.Make().String()
	form := map[string]string{
		"client_id":     _clientId,
		"client_secret": _clientSecret,
		"device_id":     deviceId,
		"api_id":        _apiId,
	}

	result := &anonymousToken{}
	respErr := map[string]any{}

	cl := getClient()
	resp, err := cl.R().
		SetFormUrlMap(form).
		SetResult(result).
		SetError(&respErr).
		Post(ctx, _vkOAuthUrl+"/get_anonym_token")

	if err != nil {
		return nil, err
	}

	if resp.IsError() {
		return nil, newUnknownError(resp.StatusCode, "getAnonymousToken-resp.IsError()", respErr)
	}

	result.DeviceId = deviceId
	return result, err
}

type anonymousToken struct {
	Token string `json:"token"`

	// Unix ms, expires after 1 day.
	ExpiredAt int64 `json:"expired_at"`

	// Not response of API.
	DeviceId string `json:"-"`
}
