//go:build !windows

package audiofetcher

import "os/exec"

func hideConsole(_ *exec.Cmd) {}
