package core

import (
	"Cryon2/internal/domain"
	"Cryon2/internal/logging"
	"Cryon2/internal/services/local"
	"Cryon2/internal/store"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type savedDownload struct {
	Key     string       `json:"key"`
	Track   domain.Track `json:"track"`
	Name    string       `json:"name"`
	File    string       `json:"file"`
	Mime    string       `json:"mimeType"`
	Size    int64        `json:"size"`
	SavedAt int64        `json:"savedAt"`
}

func downloadKey(service domain.ServiceID, id string) string {
	sum := sha256.Sum256([]byte(string(service) + "\x00" + id))
	return hex.EncodeToString(sum[:])
}
func downloadDir() (string, error) {
	base, err := logging.DataDir("Cryon2")
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "downloads")
	err = os.MkdirAll(dir, 0700)
	return dir, err
}
func validDownloadKey(key string) bool {
	raw, err := hex.DecodeString(key)
	return err == nil && len(raw) == 32 && strings.ToLower(key) == key
}
func loadDownload(key string) (savedDownload, string, error) {
	var item savedDownload
	if !validDownloadKey(key) {
		return item, "", os.ErrNotExist
	}
	dir, err := downloadDir()
	if err != nil {
		return item, "", err
	}
	raw, err := os.ReadFile(filepath.Join(dir, key+".json"))
	if err != nil {
		return item, "", err
	}
	if json.Unmarshal(raw, &item) != nil || item.Size <= 0 || downloadKey(item.Track.Service, item.Track.ID) != key || item.Key != key || item.File != key+exportExtension(item.Mime) || exportExtension(item.Mime) == "" {
		return item, "", fmt.Errorf("некорректная запись скачивания")
	}
	path := filepath.Join(dir, item.File)
	info, err := os.Lstat(path)
	if err != nil {
		return item, "", err
	}
	if !info.Mode().IsRegular() || info.Size() != item.Size {
		return item, "", fmt.Errorf("скачанный файл изменён или неполный")
	}
	return item, path, nil
}
func (a *App) offlineDownload(service domain.ServiceID, id string) (savedDownload, string, bool) {
	item, path, err := loadDownload(downloadKey(service, id))
	return item, path, err == nil
}

func (a *App) handleAPIDownloads(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		dir, err := downloadDir()
		if err != nil {
			writeAPIAppError(w, err)
			return
		}
		files, _ := os.ReadDir(dir)
		out := []savedDownload{}
		for _, file := range files {
			if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
				continue
			}
			key := strings.TrimSuffix(file.Name(), ".json")
			if item, _, err := loadDownload(key); err == nil {
				if _, err := os.Stat(filepath.Join(dir, key+".cover")); err == nil {
					item.Track.ArtworkURL = "/v1/library/downloads/artwork?key=" + key
				}
				out = append(out, item)
			}
		}
		writeAPIJSON(w, 200, out)
	case http.MethodPost:
		var input struct {
			Track domain.Track `json:"track"`
		}
		if decodeAPIJSON(w, r, &input) != nil {
			return
		}
		if err := validateAPITrack(input.Track); err != nil {
			writeAPIError(w, 400, err.Error())
			return
		}
		if input.Track.Service == domain.ServiceLocal {
			writeAPIError(w, 400, "локальный файл уже доступен без сети")
			return
		}
		if a.store == nil {
			writeAPIError(w, 503, "хранилище недоступно")
			return
		}
		key := downloadKey(input.Track.Service, input.Track.ID)
		if _, _, err := loadDownload(key); err == nil {
			writeAPIJSON(w, 200, map[string]any{"key": key, "saved": true})
			return
		}
		payload, _ := json.Marshal(input.Track)
		jobs, _ := a.store.JobList(r.Context(), 100)
		for _, job := range jobs {
			if job.Kind == JobKindAudioDownload && job.PayloadJSON == string(payload) && (job.State == store.JobQueued || job.State == store.JobRunning || job.State == store.JobRetrying) {
				writeAPIJSON(w, 202, map[string]any{"key": key, "jobId": job.ID})
				return
			}
		}
		job, err := a.EnqueueJob(JobKindAudioDownload, 10, string(payload))
		if err != nil {
			writeAPIAppError(w, err)
			return
		}
		writeAPIJSON(w, 202, map[string]any{"key": key, "jobId": job.ID})
	case http.MethodDelete:
		item, path, err := loadDownload(r.URL.Query().Get("key"))
		if err != nil {
			writeAPIError(w, 404, "скачивание не найдено; посторонние файлы не удаляются")
			return
		}
		// Only a validated, app-created manifest can name a file for deletion. No arbitrary path input.
		if err = os.Remove(path); err != nil {
			writeAPIAppError(w, err)
			return
		}
		dir := filepath.Dir(path)
		_ = os.Remove(filepath.Join(dir, item.Key+".json"))
		_ = os.Remove(filepath.Join(dir, item.Key+".cover"))
		w.WriteHeader(204)
	default:
		writeAPIError(w, 405, "требуется GET, POST или DELETE")
	}
}
func (a *App) handleAPIDownloadFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, 405, "требуется GET")
		return
	}
	item, path, err := loadDownload(r.URL.Query().Get("key"))
	if err != nil {
		writeAPIError(w, 404, "скачанный файл недоступен")
		return
	}
	if strings.HasSuffix(r.URL.Path, "/artwork") {
		path = filepath.Join(filepath.Dir(path), item.Key+".cover")
		http.ServeFile(w, r, path)
		return
	}
	if r.URL.Query().Get("metadata") == "1" {
		writeAPIJSON(w, 200, item)
		return
	}
	w.Header().Set("Content-Type", item.Mime)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s", url.PathEscape(item.Name)))
	http.ServeFile(w, r, path)
}

func (a *App) downloadAudio(ctx context.Context, job store.Job) error {
	var track domain.Track
	if json.Unmarshal([]byte(job.PayloadJSON), &track) != nil {
		return fmt.Errorf("неверный трек скачивания")
	}
	key := downloadKey(track.Service, track.ID)
	if _, _, err := loadDownload(key); err == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	last := time.Time{}
	ctx = context.WithValue(ctx, transferProgressKey{}, func(done, total int64) {
		if time.Since(last) < 400*time.Millisecond {
			return
		}
		last = time.Now()
		value := .1
		if total > 0 {
			value += .65 * float64(done) / float64(total)
		}
		_ = a.store.JobProgress(ctx, job.ID, min(.8, value), fmt.Sprintf("Загружено %.1f МБ", float64(done)/(1<<20)))
	})
	params := url.Values{"service": {string(track.Service)}, "id": {track.ID}, "name": {strings.Join(track.Artists, ", ") + " — " + track.Title}}
	r, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1/v1/library/export?"+params.Encode(), nil)
	audio, ct, name, err := a.exportFile(r)
	if err != nil {
		return err
	}
	defer func() {
		audio.Close()
		if audio.temporary {
			_ = os.Remove(audio.Name())
		}
	}()
	_ = a.store.JobProgress(ctx, job.ID, .82, "Проверяем полный аудиофайл")
	actual := local.AudioDurationMs(audio.Name())
	if track.DurationMs > 30000 && actual > 0 && actual < track.DurationMs-15000 {
		return fmt.Errorf("источник отдал только %.0f секунд из %.0f; неполный трек не сохранён", float64(actual)/1000, float64(track.DurationMs)/1000)
	}
	if actual > 0 {
		track.DurationMs = actual
	}
	dir, err := downloadDir()
	if err != nil {
		return err
	}
	filename := key + exportExtension(ct)
	part, err := os.CreateTemp(dir, key+"-*.part")
	if err != nil {
		return err
	}
	partName := part.Name()
	defer os.Remove(partName)
	_, _ = audio.Seek(0, 0)
	size, err := io.Copy(exportWriter{ctx, part}, audio)
	closeErr := part.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	final := filepath.Join(dir, filename)
	if err = os.Rename(partName, final); err != nil {
		return err
	}
	item := savedDownload{key, track, name, filename, ct, size, time.Now().UnixMilli()}
	raw, _ := json.Marshal(item)
	manifest := filepath.Join(dir, key+".json")
	if err = writeDownloadManifest(manifest, raw); err != nil {
		_ = os.Remove(final)
		return err
	}
	if track.ArtworkURL != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, track.ArtworkURL, nil)
		if err == nil {
			client := &http.Client{Timeout: 8 * time.Second}
			if resp, err := client.Do(req); err == nil {
				if resp.StatusCode == 200 && strings.HasPrefix(resp.Header.Get("Content-Type"), "image/") {
					image, _ := io.ReadAll(io.LimitReader(resp.Body, 6<<20))
					if len(image) > 0 {
						_ = os.WriteFile(filepath.Join(dir, key+".cover"), image, 0600)
					}
				}
				resp.Body.Close()
			}
		}
	}
	return a.store.JobProgress(ctx, job.ID, .99, "Трек доступен без сети")
}

type transferProgressKey struct{}
type transferWriter struct {
	ctx         context.Context
	dst         io.Writer
	done, total int64
}

func (w *transferWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := w.dst.Write(p)
	w.done += int64(n)
	if report, ok := w.ctx.Value(transferProgressKey{}).(func(int64, int64)); ok {
		report(w.done, w.total)
	}
	return n, err
}
