//go:build !windows || android

package core

import "errors"

func (a *App) InstallUpdate() error {
	return errors.New("установка обновления доступна через оболочку устройства")
}
