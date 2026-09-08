//go:build !windows

package youtube

import "os/exec"

// hideConsole — на не-Windows платформах консольного окна нет, ничего не делаем.
func hideConsole(_ *exec.Cmd) {}
