//go:build windows

package playback

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"
)

func TestStatusReportsMPVFailureAfterItBecomesIdle(t *testing.T) {
	name := fmt.Sprintf(`\\.\pipe\cryon-test-%d-%d`, os.Getpid(), time.Now().UnixNano())
	listener, err := winio.ListenPipe(name, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				var req struct {
					Command []any `json:"command"`
				}
				if json.NewDecoder(conn).Decode(&req) != nil || len(req.Command) < 2 {
					return
				}
				response := map[string]any{"error": "property unavailable"}
				switch req.Command[1] {
				case "idle-active":
					response = map[string]any{"error": "success", "data": true}
				case "eof-reached":
					response = map[string]any{"error": "success", "data": false}
				}
				_ = json.NewEncoder(conn).Encode(response)
			}(conn)
		}
	}()
	controller := &Controller{backend: BackendMPV, ipcPath: name, cmd: &exec.Cmd{}, state: "playing", trackID: "soundcloud:one", startedAt: time.Now().Add(-4 * time.Second)}
	status := controller.Status()
	if status.State != "error" || status.Message == "" || status.TrackID != "soundcloud:one" {
		t.Fatalf("unexpected playback status: %+v", status)
	}
}
