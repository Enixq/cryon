package vkmauth

import (
	"context"
	"fmt"
	"net/url"
)

func getToken(
	ctx context.Context,
	deviceId,
	uuid,
	silentToken string,
) (*getTokenResponse, error) {

	vals := url.Values{}
	vals.Set("device_id", deviceId)
	vals.Set("device_os", "android")
	vals.Set("uuid", uuid)
	vals.Set("silent_token", silentToken)

	result := &getTokenResponse{}
	cl := getClient()
	resp, err := cl.R().
		SetQueryParams(vals).
		SetResult(result).
		Get(ctx, _moosicVkConnectTokenUrl)

	if err != nil {
		return nil, err
	}

	if resp.IsError() {
		return nil, newError(fmt.Sprintf("%d", resp.StatusCode), "getToken resp.IsError()")
	}

	return result, err
}

type getTokenResponse struct {
	AccessToken string `json:"access_token"`
	// ~89 days.
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}
