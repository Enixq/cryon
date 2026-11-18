package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"testing"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (fn transportFunc) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }

func TestSelectReleaseRequiresNewerVersionAndValidAsset(t *testing.T) {
	var source githubRelease
	source.TagName = "v0.2.16"
	source.Assets = append(source.Assets, struct {
		Name   string `json:"name"`
		URL    string `json:"browser_download_url"`
		Digest string `json:"digest"`
		Size   int64  `json:"size"`
	}{Name: "Cryon2-v0.2.16-windows-amd64.exe", URL: "https://github.com/Enixq/cryon/releases/download/v0.2.16/Cryon2-v0.2.16-windows-amd64.exe", Digest: "sha256:" + strings.Repeat("a", 64), Size: 1024})
	result, err := selectRelease(source, "windows-amd64")
	if err != nil || !result.Available || result.Latest != "v0.2.16" {
		t.Fatalf("expected valid update: %+v, %v", result, err)
	}
	source.Assets[0].URL = "https://example.com/Cryon2-v0.2.16-windows-amd64.exe"
	if _, err := selectRelease(source, "windows-amd64"); err == nil {
		t.Fatal("accepted an asset outside the repository")
	}
	source.Assets[0].URL = "https://github.com/Enixq/cryon/releases/download/v0.2.16/Cryon2-v0.2.16-windows-amd64.exe"
	source.Assets[0].Digest = "sha256:invalid"
	if _, err := selectRelease(source, "windows-amd64"); err == nil {
		t.Fatal("accepted an invalid digest")
	}
	source.TagName = Version
	result, err = selectRelease(source, "windows-amd64")
	if err != nil || result.Available {
		t.Fatalf("current version must not be offered: %+v, %v", result, err)
	}
}

func TestDownloadVerifiesChecksumAndRemovesInvalidFiles(t *testing.T) {
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		t.Skip("test uses the Windows release asset")
	}
	payload := "valid update bytes"
	sum := sha256.Sum256([]byte(payload))
	digest := hex.EncodeToString(sum[:])
	original := client
	t.Cleanup(func() { client = original })
	client = &http.Client{Transport: transportFunc(func(request *http.Request) (*http.Response, error) {
		body := payload
		if request.URL.Host == "api.github.com" {
			body = fmt.Sprintf(`{"tag_name":"v0.2.16","assets":[{"name":"Cryon2-v0.2.16-windows-amd64.exe","browser_download_url":"https://github.com/Enixq/cryon/releases/download/v0.2.16/Cryon2-v0.2.16-windows-amd64.exe","digest":"sha256:%s","size":%d}]}`, digest, len(payload))
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: request}, nil
	})}
	directory := t.TempDir()
	path, err := Download(context.Background(), directory)
	if err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != payload {
		t.Fatalf("downloaded file: %q, %v", contents, err)
	}
	digest = strings.Repeat("0", 64)
	if _, err := Download(context.Background(), directory); err == nil {
		t.Fatal("accepted a file with an incorrect checksum")
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 {
		t.Fatalf("invalid download was not cleaned up: %v, %v", entries, err)
	}
}
