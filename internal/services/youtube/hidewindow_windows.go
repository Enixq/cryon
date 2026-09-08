//go:build windows

package youtube

import (
	"os/exec"
	"syscall"
)

// hideConsole не даёт дочернему процессу (yt-dlp) открывать чёрное консольное
// окно при каждом поиске. CREATE_NO_WINDOW (0x08000000) запускает процесс
// без консоли — иначе на Windows окно мигает поверх приложения.
func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
