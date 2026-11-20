package core

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestDeviceStateRoundTrip(t *testing.T) {
	app := &App{}
	state := []byte(`{"deviceId":"phone","queue":[{"id":"one"}],"isPlaying":true}`)
	w := httptest.NewRecorder()
	app.handleAPIDeviceState(w, httptest.NewRequest(http.MethodPut, "/v1/devices/state", bytes.NewReader(state)))
	if w.Code != http.StatusNoContent {
		t.Fatalf("PUT: %d", w.Code)
	}
	w = httptest.NewRecorder()
	app.handleAPIDeviceState(w, httptest.NewRequest(http.MethodGet, "/v1/devices/state", nil))
	var got struct {
		DeviceID string `json:"deviceId"`
		Queue    []struct {
			ID string `json:"id"`
		} `json:"queue"`
		IsPlaying bool `json:"isPlaying"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.DeviceID != "phone" || len(got.Queue) != 1 || got.Queue[0].ID != "one" || !got.IsPlaying {
		t.Fatalf("unexpected state: %+v", got)
	}
}

func TestDeviceDiscoveryFromNetworkLive(t *testing.T) {
	if os.Getenv("CRYON_DISCOVERY_IT") != "1" {
		t.Skip("network discovery test is disabled")
	}
	startDeviceDiscoveryResponder()
	t.Log("discovery responder active for 20 seconds")
	time.Sleep(20 * time.Second)
}

func TestLANPreflightAllowsDeviceStateWrites(t *testing.T) {
	w := httptest.NewRecorder()
	setCORS(w)
	if got := w.Header().Get("Access-Control-Allow-Methods"); !bytes.Contains([]byte(got), []byte("PUT")) {
		t.Fatalf("PUT not allowed: %s", got)
	}
}
