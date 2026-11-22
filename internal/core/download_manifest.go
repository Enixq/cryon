package core

import (
	"os"
	"path/filepath"
)

func writeDownloadManifest(path string, raw []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), "manifest-*.part")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, err = file.Write(raw)
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(file.Name(), path)
	}
	return err
}
