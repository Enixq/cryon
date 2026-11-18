package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

var Version = "v0.2.15"

const releasesURL = "https://api.github.com/repos/Enixq/cryon/releases/latest"
const maxDownloadSize = 300 << 20

var client = &http.Client{Timeout: 3 * time.Minute}

type Release struct {
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	Available bool   `json:"available"`
	Platform  string `json:"platform"`
	Size      int64  `json:"size"`
	NotesURL  string `json:"notesUrl"`
	assetURL  string
	digest    string
}

type githubRelease struct {
	TagName    string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name   string `json:"name"`
		URL    string `json:"browser_download_url"`
		Digest string `json:"digest"`
		Size   int64  `json:"size"`
	} `json:"assets"`
}

func platform() string {
	if runtime.GOOS == "android" && runtime.GOARCH == "arm64" {
		return "android-arm64"
	}
	if runtime.GOOS == "windows" && runtime.GOARCH == "amd64" {
		return "windows-amd64"
	}
	return ""
}

func Check(ctx context.Context) (Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releasesURL, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Cryon/"+Version)
	resp, err := client.Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("не удалось связаться с GitHub: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("GitHub вернул HTTP %d", resp.StatusCode)
	}
	var source githubRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&source); err != nil {
		return Release{}, fmt.Errorf("не удалось прочитать информацию о версии: %w", err)
	}
	return selectRelease(source, platform())
}

func selectRelease(source githubRelease, target string) (Release, error) {
	result := Release{Current: Version, Latest: source.TagName, Platform: target}
	if source.Draft || source.Prerelease || !semver.IsValid(source.TagName) || semver.Prerelease(source.TagName) != "" {
		return Release{}, errors.New("GitHub вернул неподходящую версию")
	}
	result.NotesURL = "https://github.com/Enixq/cryon/releases/tag/" + url.PathEscape(source.TagName)
	if semver.Compare(source.TagName, Version) <= 0 {
		return result, nil
	}
	if target == "" {
		return Release{}, errors.New("обновление для этой платформы пока недоступно")
	}
	extension := ".exe"
	if target == "android-arm64" {
		extension = ".apk"
	}
	name := "Cryon2-" + source.TagName + "-" + target + extension
	for _, asset := range source.Assets {
		if asset.Name != name || asset.Size <= 0 || asset.Size > maxDownloadSize {
			continue
		}
		parsed, err := url.Parse(asset.URL)
		expectedPath := "/Enixq/cryon/releases/download/" + source.TagName + "/" + name
		digest := strings.TrimPrefix(asset.Digest, "sha256:")
		if err != nil || parsed.Scheme != "https" || parsed.Host != "github.com" || parsed.Path != expectedPath || parsed.RawQuery != "" || !strings.HasPrefix(asset.Digest, "sha256:") || len(digest) != 64 {
			continue
		}
		if _, err := hex.DecodeString(digest); err != nil {
			continue
		}
		result.Available = true
		result.Size = asset.Size
		result.assetURL = asset.URL
		result.digest = strings.ToLower(digest)
		return result, nil
	}
	return Release{}, errors.New("в релизе не найден проверяемый файл для этого устройства")
}

func Download(ctx context.Context, directory string) (string, error) {
	release, err := Check(ctx)
	if err != nil {
		return "", err
	}
	if !release.Available {
		return "", errors.New("новой версии пока нет")
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return "", err
	}
	file, err := os.CreateTemp(directory, "cryon-update-*")
	if err != nil {
		return "", err
	}
	name := file.Name()
	defer func() { _ = os.Remove(name) }()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, release.assetURL, nil)
	if err != nil {
		_ = file.Close()
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		_ = file.Close()
		return "", fmt.Errorf("не удалось скачать обновление: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_ = file.Close()
		return "", fmt.Errorf("ошибка загрузки: HTTP %d", resp.StatusCode)
	}
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(resp.Body, maxDownloadSize+1))
	closeErr := file.Close()
	if err != nil || closeErr != nil || size != release.Size || hex.EncodeToString(hash.Sum(nil)) != release.digest {
		return "", errors.New("обновление загружено не полностью или не прошло проверку целостности")
	}
	extension := ".exe"
	if release.Platform == "android-arm64" {
		extension = ".apk"
	}
	final := name + extension
	if err := os.Rename(name, final); err != nil {
		return "", fmt.Errorf("не удалось сохранить обновление: %w", err)
	}
	return final, nil
}
