//go:build !cryonmobile

package core

import (
	"errors"
	"io/fs"
)

// startMobileServer — заглушка для сборок без тега cryonmobile (десктоп без
// мобильного сервера). Реальная реализация — в mobileserver.go.
func (a *App) startMobileServer(spa fs.FS) (string, func(), error) {
	return "", nil, errors.New("мобильный сервер доступен только в сборке с тегом cryonmobile")
}
