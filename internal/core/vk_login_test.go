package core

import (
	vkmauth "Cryon2/internal/services/vkauth"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"testing"
)

func TestVKLoginMessageDistinguishesResponseAndNetworkErrors(t *testing.T) {
	var response vkmauth.ResponseWithError
	_ = json.Unmarshal([]byte(`{"error":{"error_code":100,"error_msg":"secret phone and token"}}`), &response)
	api := vkLoginMessage(&vkmauth.StageError{Stage: "phone", Cause: response})
	if !strings.Contains(api, "100") || strings.Contains(api, "secret") || strings.Contains(api, "не создал") {
		t.Fatalf("wrong API message: %s", api)
	}
	network := vkLoginMessage(&vkmauth.StageError{Stage: "phone", Cause: &url.Error{Op: "Post", URL: "https://api.vk.com", Err: errors.New("offline")}})
	if !strings.Contains(network, "связаться") {
		t.Fatalf("wrong network message: %s", network)
	}
}
