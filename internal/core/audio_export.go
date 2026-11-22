package core

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"Cryon2/internal/domain"
)

func exportExtension(contentType string) string {
	kind, _, _ := mime.ParseMediaType(contentType)
	switch kind {
	case "audio/mpeg", "audio/mp3":
		return ".mp3"
	case "audio/mp4", "audio/x-m4a":
		return ".m4a"
	case "audio/ogg", "application/ogg":
		return ".ogg"
	case "audio/webm":
		return ".webm"
	case "audio/flac", "audio/x-flac":
		return ".flac"
	case "audio/wav", "audio/x-wav", "audio/wave":
		return ".wav"
	case "audio/aac":
		return ".aac"
	}
	return ""
}

func exportName(name, ext string) string {
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '-'
		}
		return r
	}, name)
	name = strings.Trim(strings.TrimSpace(name), ". ")
	if name == "" {
		name = "Cryon track"
	}
	runes := []rune(name)
	if len(runes) > 160 {
		name = string(runes[:160])
	}
	return name + ext
}

func (a *App) exportFile(r *http.Request) (*os.File, string, string, error) {
	service, id := r.URL.Query().Get("service"), r.URL.Query().Get("id")
	if id == "" {
		return nil, "", "", fmt.Errorf("нужен трек")
	}
	if _, ok := a.registry[domain.ServiceID(service)]; !ok {
		return nil, "", "", fmt.Errorf("неизвестный источник")
	}
	var f *os.File
	var ct string
	var err error
	if service == "local" {
		path, ok := a.local.FilePath(id)
		if !ok {
			return nil, "", "", fmt.Errorf("файл не найден")
		}
		f, err = os.Open(path)
		if err == nil {
			bytes := make([]byte, 512)
			n, _ := f.Read(bytes)
			_, _ = f.Seek(0, 0)
			ct = http.DetectContentType(bytes[:n])
			// MP4 headers alone cannot distinguish audio from video; the local
			// library has already identified this as an audio file.
			switch strings.ToLower(filepath.Ext(path)) {
			case ".m4a":
				ct = "audio/mp4"
			case ".flac":
				ct = "audio/flac"
			}
		}
	} else {
		f, ct, err = (&localAssetHandler{app: a}).ensureStreamFile(r.Context(), service, id)
	}
	if err != nil {
		return nil, "", "", err
	}
	if detected, detectErr := audioFileType(f); detectErr != nil {
		f.Close()
		return nil, "", "", detectErr
	} else if detected == "video/webm" && strings.HasPrefix(ct, "audio/webm") {
		ct = "audio/webm"
	} else {
		ct = detected
	}
	if strings.Contains(strings.ToLower(ct), "mpegurl") {
		f.Close()
		target, resolveErr := (&localAssetHandler{app: a}).resolveStreamURL(service, id, false)
		if resolveErr != nil {
			return nil, "", "", resolveErr
		}
		f, err = exportHLS(r.Context(), target)
		if err != nil {
			return nil, "", "", err
		}
		ct, err = audioFileType(f)
		if err != nil {
			f.Close()
			os.Remove(f.Name())
			return nil, "", "", err
		}
	}
	if ct == "video/mp4" || ct == "audio/mp4" {
		audio, extractErr := extractMP4Audio(r.Context(), f)
		if audio != f {
			f.Close()
			if strings.HasPrefix(filepath.Base(f.Name()), "cryon-export-") {
				_ = os.Remove(f.Name())
			}
		}
		if extractErr != nil {
			return nil, "", "", extractErr
		}
		f, ct = audio, "audio/mp4"
	}
	ext := exportExtension(ct)
	if ext == "" {
		f.Close()
		if strings.HasPrefix(filepath.Base(f.Name()), "cryon-export-") {
			_ = os.Remove(f.Name())
		}
		return nil, "", "", fmt.Errorf("источник не отдал отдельный аудиофайл в поддерживаемом формате")
	}
	return f, ct, exportName(r.URL.Query().Get("name"), ext), nil
}

// Exports the complete audio file with its actual container/extension.
func (a *App) handleAPIAudioExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeAPIError(w, 405, "требуется GET или POST")
		return
	}
	f, ct, name, err := a.exportFile(r)
	if err != nil {
		writeAPIAppError(w, err)
		return
	}
	defer func() {
		f.Close()
		if strings.HasPrefix(filepath.Base(f.Name()), "cryon-export-") {
			_ = os.Remove(f.Name())
		}
	}()
	if r.URL.Query().Get("metadata") == "1" {
		writeAPIJSON(w, 200, map[string]string{"name": name, "mimeType": ct})
		return
	}
	if r.Method == http.MethodPost {
		host := a.platform
		if mirror, ok := host.(*eventMirroringHost); ok {
			host = mirror.target
		}
		saver, ok := host.(interface{ SaveFile(string) (string, error) })
		if !ok {
			writeAPIError(w, 501, "системное сохранение недоступно")
			return
		}
		path, err := saver.SaveFile(name)
		if err != nil {
			writeAPIAppError(w, err)
			return
		}
		if path == "" {
			writeAPIJSON(w, 200, map[string]any{"saved": false})
			return
		}
		destination, err := os.Create(path)
		if err == nil {
			_, err = io.Copy(destination, f)
			closeErr := destination.Close()
			if err == nil {
				err = closeErr
			}
		}
		if err != nil {
			writeAPIAppError(w, err)
			return
		}
		writeAPIJSON(w, 200, map[string]any{"saved": true, "name": name})
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, name, time.Time{}, f)
}
