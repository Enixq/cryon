package vkmauth

import (
	"context"
	"fmt"
)

func validatePhoneConfirm(
	ctx context.Context,
	deviceId,
	code,
	phone,
	anonToken,
	sid string,
) (*validatePhoneConfirmResponse, error) {

	form := map[string]string{
		"access_token":      anonToken,
		"code":              code,
		"phone":             phone,
		"device_id":         deviceId,
		"sak_version":       _sakVersion,
		"can_skip_password": "0", // what if 1?
		"v":                 _apiVersion,
		"api_id":            _apiId,
		"sid":               sid,
	}

	result := &validatePhoneConfirmResponse{}
	cl := getClient()
	resp, err := cl.R().
		SetFormUrlMap(form).
		SetResult(result).
		Post(ctx, _vkApiUrl+"/method/auth.validatePhoneConfirm")

	if err != nil {
		return nil, err
	}

	if result.IsError() {
		return nil, result
	}

	if resp.IsError() {
		return nil, newError(fmt.Sprintf("%d", resp.StatusCode), "validatePhoneConfirm resp.IsError()")
	}

	return result, err
}

type validatePhoneConfirmResponse struct {
	ResponseWithError

	Response struct {
		Sid          string  `json:"sid"`
		ProfileExist apiBool `json:"profile_exist"`
		Profile      struct {
			FirstName      string  `json:"first_name"`
			Has2Fa         apiBool `json:"has_2fa"`
			LastName       string  `json:"last_name"`
			Photo200       string  `json:"photo_200"`
			Phone          string  `json:"phone"`
			CanUnbindPhone apiBool `json:"can_unbind_phone"`
		} `json:"profile"`
		SignupFields            []string `json:"signup_fields"`
		SignupRestrictedSubject string   `json:"signup_restricted_subject"`
		SignupParams            struct {
			PasswordMinLength int    `json:"password_min_length"`
			BirthDateMax      string `json:"birth_date_max"`
		} `json:"signup_params"`
	} `json:"response"`
}
