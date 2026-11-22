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
			if r.Form.Get("supported_ways") != "push,email,sms_inbox" {
				t.Fatal("SMS not supported")
			}
			body = `{"response":{"sid":"phone-sid","validation_type":"sms_inbox","libverify_support":1}}`
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

func TestCaptchaKeepsPhoneSession(t *testing.T) {
	previous := http.DefaultTransport
	defer func() { http.DefaultTransport = previous }()
	phoneCalls := 0
	http.DefaultTransport = transportFunc(func(r *http.Request) (*http.Response, error) {
		_ = r.ParseForm()
		body := `{"token":"anonymous"}`
		if r.URL.Path == "/method/auth.validatePhone" {
			phoneCalls++
			if phoneCalls == 1 {
				body = `{"error":{"error_code":14,"error_msg":"Captcha needed","captcha_sid":"captcha-session","captcha_img":"https://api.vk.com/captcha.php?sid=fixture"}}`
			} else {
				if r.Form.Get("captcha_sid") != "captcha-session" || r.Form.Get("captcha_key") != "answer" || r.Form.Get("access_token") != "anonymous" {
					t.Fatal("captcha or anonymous session lost")
				}
				body = `{"response":{"sid":"sms-session","validation_type":"sms_inbox"}}`
			}
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	stop := errors.New("stop after code prompt")
	_, err := New(context.Background(), "+70000000000", "fixture", func(by CodeSended) (GotCode, error) {
		if by.Current != AuthSupportedWaySms {
			t.Fatal("SMS stage not reached")
		}
		return GotCode{}, stop
	}, func(c Captcha) (string, error) {
		if c.SID != "captcha-session" {
			t.Fatal("wrong captcha")
		}
		return "answer", nil
	})
	if !errors.Is(err, stop) || phoneCalls != 2 {
		t.Fatalf("flow=%v phone calls=%d", err, phoneCalls)
	}
}

func TestEmptyAnonymousTokenStopsBeforePhoneRequest(t *testing.T) {
	previous := http.DefaultTransport
	defer func() { http.DefaultTransport = previous }()
	http.DefaultTransport = transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/oauth/get_anonym_token" {
			t.Fatal("requested SMS without an anonymous token")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	})
	_, err := New(context.Background(), "+70000000000", "fixture", nil)
	var stage *StageError
	if !errors.As(err, &stage) || stage.Stage != "anonymous" {
		t.Fatal("empty token did not fail at anonymous stage")
	}
}

func TestVKErrorOnHTTPFailurePreservesCode(t *testing.T) {
	previous := http.DefaultTransport
	defer func() { http.DefaultTransport = previous }()
	http.DefaultTransport = transportFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 400, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"error_code":100,"error_msg":"Invalid parameter"}}`))}, nil
	})
	_, err := validatePhone(context.Background(), "", "", &anonymousToken{Token: "fixture"})
	var coded interface{ APIErrorCode() int }
	if !errors.As(err, &coded) || coded.APIErrorCode() != 100 {
		t.Fatalf("lost VK error code: %v", err)
	}
}
