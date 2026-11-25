package core

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"Cryon2/internal/domain"
	"Cryon2/internal/services/vkweb"
)

const vkStreamPath = "/v1/vk/stream"

var playlistURI = regexp.MustCompile(`URI="([^"]+)"`)

func vkMediaURL(id string) string { return vkStreamPath + "?" + url.Values{"id": {id}}.Encode() }
func mediaTargetValid(target string) bool {
	u, err := url.Parse(target)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Hostname() == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return false
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()) {
		return false
	}
	return true
}

func rewriteVKPlaylist(body, base, id string, provider *vkweb.Service) (string, error) {
	uri, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("неверная ссылка плейлиста ВК")
	}
	link := func(ref string) (string, error) {
		u, err := url.Parse(ref)
		if err != nil {
			return "", fmt.Errorf("неверная ссылка HLS")
		}
		target := uri.ResolveReference(u).String()
		if !mediaTargetValid(target) {
			return "", fmt.Errorf("неподдерживаемая ссылка HLS")
		}
		encoded := base64.RawURLEncoding.EncodeToString([]byte(target))
		return vkStreamPath + "?" + url.Values{"id": {id}, "u": {encoded}, "sig": {provider.SignMedia(id, target)}}.Encode(), nil
	}
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "#") {
			lines[i], err = link(trimmed)
			if err != nil {
				return "", err
			}
			continue
		}
		var replaceErr error
		lines[i] = playlistURI.ReplaceAllStringFunc(line, func(match string) string {
			ref := playlistURI.FindStringSubmatch(match)[1]
			target, e := link(ref)
			if e != nil {
				replaceErr = e
			}
			return `URI="` + target + `"`
		})
		if replaceErr != nil {
			return "", replaceErr
		}
	}
	return strings.Join(lines, "\n"), nil
}

func (a *App) handleVKWebStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeAPIError(w, 405, "требуется GET или HEAD")
		return
	}
	provider, ok := a.registry[domain.ServiceVK].(*vkweb.Service)
	if !ok || !provider.HasToken() {
		writeAPIError(w, 401, "ВК Музыка не подключена")
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		writeAPIError(w, 400, "нужен трек ВК")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	target := ""
	mime := ""
	if encoded := r.URL.Query().Get("u"); encoded != "" {
		raw, err := base64.RawURLEncoding.DecodeString(encoded)
		if err != nil || len(raw) > 8192 {
			writeAPIError(w, 400, "неверная ссылка потока ВК")
			return
		}
		target = string(raw)
		if !provider.VerifyMedia(id, target, r.URL.Query().Get("sig")) {
			writeAPIError(w, 403, "ссылка потока ВК истекла")
			return
		}
	} else {
		var err error
		target, mime, err = provider.ResolveStream(ctx, id)
		if err != nil {
			writeAPIAppError(w, err)
			return
		}
		if !strings.Contains(strings.ToLower(mime), "mpegurl") {
			file, ct, err := (&localAssetHandler{app: a}).ensureStreamFile(ctx, "vk", id)
			if err != nil {
				writeAPIAppError(w, err)
				return
			}
			defer file.Close()
			info, err := file.Stat()
			if err != nil {
				writeAPIError(w, 500, "не удалось прочитать аудио ВК")
				return
			}
			w.Header().Set("Content-Type", ct)
			http.ServeContent(w, r, "", info.ModTime(), file)
			return
		}
	}
	if !mediaTargetValid(target) {
		writeAPIError(w, 400, "неподдерживаемая ссылка потока ВК")
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		writeAPIError(w, 400, "неверная ссылка потока ВК")
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Referer", "https://vk.ru/audio")
	req.Header.Set("Origin", "https://vk.ru")
	if value := r.Header.Get("Range"); value != "" {
		req.Header.Set("Range", value)
	}
	response, err := streamDownloadClient.Do(req)
	if err != nil {
		writeAPIError(w, 502, "ВК: не удалось загрузить сегмент аудио")
		return
	}
	defer response.Body.Close()
	if response.StatusCode != 200 && response.StatusCode != 206 {
		writeAPIError(w, 502, fmt.Sprintf("ВК: сервер аудио ответил HTTP %d", response.StatusCode))
		return
	}
	reader := bufio.NewReader(response.Body)
	prefix, _ := reader.Peek(10)
	if strings.HasPrefix(strings.TrimSpace(string(prefix)), "#EXTM3U") || strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "mpegurl") {
		body, err := io.ReadAll(io.LimitReader(reader, (2<<20)+1))
		if err != nil || len(body) > 2<<20 {
			writeAPIError(w, 502, "ВК: не удалось прочитать плейлист аудио")
			return
		}
		playlist, err := rewriteVKPlaylist(string(body), target, id, provider)
		if err != nil {
			writeAPIAppError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/x-mpegURL")
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodHead {
			_, _ = io.WriteString(w, playlist)
		}
		return
	}
	for _, header := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges"} {
		if value := response.Header.Get(header); value != "" {
			w.Header().Set(header, value)
		}
	}
	w.Header().Set("Cache-Control", "private, max-age=120")
	w.WriteHeader(response.StatusCode)
	if r.Method != http.MethodHead {
		_, _ = io.Copy(w, io.LimitReader(reader, 64<<20))
	}
}
