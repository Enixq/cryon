//go:build !windows

package playback

import (
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// defaultIPCPath — unix-сокет для IPC с mpv во временном каталоге.
func defaultIPCPath() string {
	return filepath.Join(os.TempDir(), "cryon2-mpv.sock")
}

// applyHideWindow — на unix ничего скрывать не нужно.
func applyHideWindow(_ *exec.Cmd) {}

// dialNamedPipe на unix не используется (IPC идёт через unix-сокет).
func dialNamedPipe(_ string, _ time.Duration) (net.Conn, error) {
	return nil, errors.New("named pipe доступен только в Windows")
}
