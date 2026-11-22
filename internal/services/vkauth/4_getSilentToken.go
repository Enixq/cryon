package vkmauth

import (
	"context"
	"fmt"
)

func getSilentToken(
	ctx context.Context,
	sid,
	phone,
	password,
	anonymousToken,
	deviceId string,
	twoFactorSupported bool,
) (*getSilentTokenResponse, error) {

	tFa := "0"
	if twoFactorSupported {
		tFa = "1"
	}

	form := map[string]string{
		"sid":             sid,
		"grant_type":      "phone_confirmation_sid",
		"username":        phone,
		"password":        password,
		"2fa_supported":   tFa,
		"supported_ways":  _authSupportedWays.Join(","),
		"anonymous_token": anonymousToken,
		"device_id":       deviceId,
		"v":               _apiVersion,
		"api_id":          _apiId,
		"sak_version":     _sakVersion,
	}

	result := &getSilentTokenResponse{}
	cl := getClient()
	resp, err := cl.R().
		SetFormUrlMap(form).
		SetResult(result).
		Post(ctx, _vkOAuthUrl+"/token")

	if err != nil {
		return nil, err
	}

	if resp.IsError() || result.SilentToken == "" || result.SilentTokenUUID == "" {
		return nil, newError(fmt.Sprintf("%d", resp.StatusCode), "getSilentToken resp.IsError()")
	}

	return result, err
}

type getSilentTokenResponse struct {
	SilentToken     string `json:"silent_token"`
	SilentTokenUUID string `json:"silent_token_uuid"`
	SilentTokenTTL  int    `json:"silent_token_ttl"`
	TrustedHash     string `json:"trusted_hash"`
}
