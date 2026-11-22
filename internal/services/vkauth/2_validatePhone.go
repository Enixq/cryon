package vkmauth

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

var ErrMissingCodeSession = errors.New("VK did not create a code session")

type Captcha struct{ SID, Image string }
type captchaAnswer struct{ SID, Key string }

// Only display VK's HTTPS challenge images in the account form.
func validCaptchaImage(image string) bool {
	u, err := url.Parse(image)
	if err != nil || u.Scheme != "https" || u.User != nil {
		return false
	}
	host := u.Hostname()
	return host == "vk.com" || strings.HasSuffix(host, ".vk.com") || host == "vk.ru" || strings.HasSuffix(host, ".vk.ru")
}

func validatePhone(ctx context.Context, phone, sid string, by *anonymousToken, captcha ...*captchaAnswer) (*validatePhoneResponse, error) {

	form := map[string]string{
		"access_token":   by.Token,
		"device_id":      by.DeviceId,
		"phone":          phone,
		"supported_ways": _authSupportedWays.Join(","),
		"sak_version":    _sakVersion,
		"v":              _apiVersion,
		"api_id":         _apiId,
	}
	if len(sid) > 0 {
		form["sid"] = sid
	}
	if len(captcha) > 0 && captcha[0] != nil {
		form["captcha_sid"], form["captcha_key"] = captcha[0].SID, captcha[0].Key
	}

	result := &validatePhoneResponse{}

	cl := getClient()
	resp, err := cl.R().
		SetFormUrlMap(form).
		SetResult(result).
		SetError(result).
		Post(ctx, _vkApiUrl+"/method/auth.validatePhone")

	if err != nil {
		return nil, err
	}

	if result.IsError() {
		return nil, result
	}

	if resp.IsError() {
		return nil, newError(fmt.Sprintf("%d", resp.StatusCode), "validatePhone resp.IsError()")
	}

	if result.Response.Sid == "" {
		return nil, ErrMissingCodeSession
	}
	return result, err
}

type validatePhoneResponse struct {
	ResponseWithError

	Response struct {
		// "general"?
		Type string `json:"type"`
		// Session ID?
		Sid              string  `json:"sid"`
		Delay            int     `json:"delay"`
		LibverifySupport apiBool `json:"libverify_support"`
		// Type code by this way.
		ValidationType AuthSupportedWay `json:"validation_type"`
		// If resend, code will be resended by this way.
		ValidationResend AuthSupportedWay `json:"validation_resend"`
		CodeLength       int              `json:"code_length"`
	} `json:"response"`
}
