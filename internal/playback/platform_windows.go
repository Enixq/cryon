//go:build windows

package playback

import (
	"net"
	"os/exec"
	"syscall"
	"time"

	"github.com/Microsoft/go-winio"
)

// defaultIPCPath — именованный канал Windows для IPC с mpv.
func defaultIPCPath() string {
	return `\\.\pipe\cryon2-mpv`
}

// applyHideWindow прячет консольное окно mpv.
func applyHideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}

// dialNamedPipe подключается к именованному каналу Windows.
func dialNamedPipe(path string, timeout time.Duration) (net.Conn, error) {
	return winio.DialPipe(path, &timeout)
}
