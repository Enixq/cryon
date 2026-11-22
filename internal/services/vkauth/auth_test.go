package vkmauth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestSMSFlowAndNumericTwoFactorFlags(t *testing.T) {
	previous := http.DefaultTransport
	defer func() { http.DefaultTransport = previous }()
	calls := 0
	http.DefaultTransport = transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		r.ParseForm()
		body := ""
		switch r.URL.Path {
		case "/oauth/get_anonym_token":
			body = `{"token":"anonymous"}`
		case "/method/auth.validatePhone":
			if !strings.Contains(r.Form.Get("supported_ways"), "sms") {
				t.Fatal("SMS not supported")
			}
			body = `{"response":{"sid":"phone-sid","validation_type":"sms","libverify_support":1}}`
		case "/method/auth.validatePhoneConfirm":
			if r.Form.Get("sid") != "phone-sid" || r.Form.Get("code") != "123456" {
				t.Fatal("code/session mismatch")
			}
			body = `{"response":{"sid":"confirmed-sid","profile_exist":1,"profile":{"has_2fa":0,"can_unbind_phone":1}}}`
		case "/oauth/token":
			if r.Form.Get("sid") != "confirmed-sid" {
				t.Fatal("confirmation SID lost")
			}
			body = `{"silent_token":"silent","silent_token_uuid":"uuid"}`
		case "/oauth/check_silent_token":
			body = `{"silent_token":"checked","silent_token_uuid":"checked-uuid"}`
		case "/oauth/vkconnect/vk/token":
			if r.URL.Query().Get("uuid") != "checked-uuid" {
				t.Fatal("exchange lost")
			}
			body = `{"access_token":"music-test-token","expires_in":7000000}`
		default:
			t.Fatalf("unexpected auth stage %s", r.URL.Path)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	token, err := New(context.Background(), "+70000000000", "fixture", func(by CodeSended) (GotCode, error) {
		if by.Current != AuthSupportedWaySms {
			t.Fatal("not SMS")
		}
		return GotCode{Code: "123456"}, nil
	})
	if err != nil || token == nil || token.AccessToken != "music-test-token" || calls != 6 {
		t.Fatalf("flow failed at stage: %v calls=%d", err, calls)
	}
}
func TestStageErrorDoesNotExposeCredentials(t *testing.T) {
	err := &StageError{Stage: "password", Cause: errors.New("sensitive payload")}
	if strings.Contains(err.Error(), "sensitive") {
		t.Fatal("credentials exposed")
	}
}
