//go:build windows && !android

package core

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"

	"Cryon2/internal/logging"
	"Cryon2/internal/update"
)

func (a *App) InstallUpdate() error {
	dir, err := logging.DataDir("Cryon2")
	if err != nil {
		return err
	}
	file, err := update.Download(context.Background(), filepath.Join(dir, "updates"))
	if err != nil {
		return err
	}
	current, err := os.Executable()
	if err != nil {
		return err
	}
	return startUpdateHelper(file, current, os.Getpid())
}

func startUpdateHelper(file, current string, processID int) error {
	helper, err := os.CreateTemp(filepath.Dir(file), "install-*.ps1")
	if err != nil {
		return err
	}
	script := `param($ProcessId, $Source, $Target)
$ErrorActionPreference = 'Stop'
try {
    Wait-Process -Id $ProcessId -ErrorAction SilentlyContinue
    $staged = $Target + '.update-' + $ProcessId
    $backup = $Target + '.previous-' + $ProcessId
    Copy-Item -LiteralPath $Source -Destination $staged -Force
    Move-Item -LiteralPath $Target -Destination $backup
    try {
        Move-Item -LiteralPath $staged -Destination $Target
        Start-Process -FilePath $Target
        Remove-Item -LiteralPath $backup -Force
        Remove-Item -LiteralPath $Source -Force
    } catch {
        if (Test-Path -LiteralPath $backup) {
            Move-Item -LiteralPath $backup -Destination $Target -Force
        }
        throw
    }
} catch {
    Add-Type -AssemblyName PresentationFramework
    [System.Windows.MessageBox]::Show("Не удалось установить обновление Cryon: " + $_.Exception.Message)
}
Remove-Item -LiteralPath $MyInvocation.MyCommand.Path -Force -ErrorAction SilentlyContinue
`
	if _, err := helper.WriteString("\ufeff" + script); err != nil {
		_ = helper.Close()
		return err
	}
	if err := helper.Close(); err != nil {
		return err
	}
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", helper.Name(), strconv.Itoa(processID), file, current)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Start()
}
