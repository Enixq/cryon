package vkmauth

import (
	"context"
	"fmt"
)

func checkSilentToken(
	ctx context.Context,
	token,
	uuid,
	anonToken,
	deviceId string,
) (*checkSilentTokenResponse, error) {

	form := map[string]string{
		"token":           token,
		"uuid":            uuid,
		"anonymous_token": anonToken,
		"v":               _apiVersion,
		"device_id":       deviceId,
		"api_id":          _apiId,
		"sak_version":     _sakVersion,
	}

	result := &checkSilentTokenResponse{}
	cl := getClient()
	resp, err := cl.R().
		SetFormUrlMap(form).
		SetResult(result).
		Post(ctx, _vkOAuthUrl+"/check_silent_token")

	if err != nil {
		return nil, err
	}

	if resp.IsError() {
		return nil, newError(fmt.Sprintf("%d", resp.StatusCode), "checkSilentToken resp.IsError()")
	}

	return result, err
}

type checkSilentTokenResponse struct {
	SilentToken     string `json:"silent_token"`
	SilentTokenUUID string `json:"silent_token_uuid"`
	SilentTokenTTL  int    `json:"silent_token_ttl"`
}
