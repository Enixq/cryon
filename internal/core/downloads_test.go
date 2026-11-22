package core

import (
	"Cryon2/internal/domain"
	"Cryon2/internal/logging"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestManagedDownloadDeletionRejectsTraversalAndPreservesOtherFiles(t *testing.T) {
	logging.SetDataDirOverride(t.TempDir())
	defer logging.SetDataDirOverride("")
	dir, err := downloadDir()
	if err != nil {
		t.Fatal(err)
	}
	track := domain.Track{ID: "one", Service: domain.ServiceSoundCloud}
	key := downloadKey(track.Service, track.ID)
	item := savedDownload{Key: key, Track: track, File: key + ".mp3", Mime: "audio/mpeg", Size: 3}
	raw, _ := json.Marshal(item)
	os.WriteFile(filepath.Join(dir, item.File), []byte("abc"), 0600)
	writeDownloadManifest(filepath.Join(dir, key+".json"), raw)
	sentinel := filepath.Join(dir, "my-other-song.mp3")
	os.WriteFile(sentinel, []byte("private"), 0600)
	app := &App{}
	bad := httptest.NewRecorder()
	app.handleAPIDownloads(bad, httptest.NewRequest("DELETE", "/v1/library/downloads?key=../my-other-song", nil))
	if bad.Code != 404 {
		t.Fatal("traversal accepted")
	}
	if _, _, ok := app.offlineDownload(track.Service, track.ID); !ok {
		t.Fatal("saved audio not available offline")
	}
	good := httptest.NewRecorder()
	app.handleAPIDownloads(good, httptest.NewRequest("DELETE", "/v1/library/downloads?key="+key, nil))
	if good.Code != 204 {
		t.Fatal(good.Body.String())
	}
	if _, err = os.Stat(sentinel); err != nil {
		t.Fatal("unrelated file deleted")
	}
}
func TestManagedDownloadRejectsTruncatedFile(t *testing.T) {
	logging.SetDataDirOverride(t.TempDir())
	defer logging.SetDataDirOverride("")
	dir, _ := downloadDir()
	track := domain.Track{ID: "two", Service: domain.ServiceYouTube}
	key := downloadKey(track.Service, track.ID)
	item := savedDownload{Key: key, Track: track, File: key + ".m4a", Mime: "audio/mp4", Size: 100}
	raw, _ := json.Marshal(item)
	writeDownloadManifest(filepath.Join(dir, key+".json"), raw)
	os.WriteFile(filepath.Join(dir, item.File), []byte("short"), 0600)
	if _, _, err := loadDownload(key); err == nil {
		t.Fatal("partial audio accepted")
	}
}
