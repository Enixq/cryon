package core

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// SoundCloud frequently supplies a finite MP3 HLS playlist with byte ranges.
// Assemble its media bytes, never export the playlist as if it were audio.
func exportHLS(ctx context.Context, target string) (*os.File, error) {
	dir, err := streamDownloadDir()
	if err != nil {
		return nil, err
	}
	output, err := os.CreateTemp(dir, "cryon-export-hls-*")
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			output.Close()
			os.Remove(output.Name())
		}
	}()
	var total int64
	var playlist func(string, int) error
	fetch := func(target, byteRange string, writer io.Writer, limit int64) error {
		parsed, err := url.Parse(target)
		if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") {
			return fmt.Errorf("неподдерживаемая ссылка HLS")
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0")
		if byteRange != "" {
			req.Header.Set("Range", byteRange)
		}
		response, err := streamDownloadClient.Do(req)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		if response.StatusCode != 200 && response.StatusCode != 206 {
			return fmt.Errorf("сегмент HLS недоступен: HTTP %d", response.StatusCode)
		}
		if byteRange != "" && response.StatusCode != 206 {
			return fmt.Errorf("источник HLS не поддерживает диапазон байтов")
		}
		n, err := io.Copy(writer, io.LimitReader(response.Body, limit+1))
		if n > limit {
			return fmt.Errorf("слишком большой поток HLS")
		}
		if byteRange != "" {
			var start, end int64
			if _, scanErr := fmt.Sscanf(byteRange, "bytes=%d-%d", &start, &end); scanErr != nil || n != end-start+1 {
				return fmt.Errorf("неполный сегмент HLS")
			}
		}
		return err
	}
	playlist = func(target string, depth int) error {
		if depth > 3 {
			return fmt.Errorf("слишком много уровней HLS")
		}
		var text strings.Builder
		if err := fetch(target, "", &text, 2<<20); err != nil {
			return err
		}
		body := text.String()
		if !strings.HasPrefix(strings.TrimSpace(body), "#EXTM3U") {
			return fmt.Errorf("неверный плейлист HLS")
		}
		base, _ := url.Parse(target)
		resolve := func(ref string) string {
			uri, _ := url.Parse(ref)
			if uri == nil {
				return ""
			}
			return base.ResolveReference(uri).String()
		}
		lines := strings.Split(body, "\n")
		variant := false
		byteRange := ""
		var rangeEnd int64
		segments := 0
		totalSegments := 0
		for _, entry := range lines {
			if strings.HasPrefix(strings.TrimSpace(entry), "#EXTINF:") {
				totalSegments++
			}
		}
		for _, line := range lines {
			line = strings.TrimSpace(line)
			switch {
			case strings.HasPrefix(line, "#EXT-X-KEY:") && !strings.Contains(line, "METHOD=NONE"):
				return fmt.Errorf("сохранение зашифрованного HLS недоступно")
			case strings.HasPrefix(line, "#EXT-X-STREAM-INF:"):
				variant = true
			case strings.HasPrefix(line, "#EXT-X-MAP:"):
				_, rest, ok := strings.Cut(line, "URI=\"")
				if !ok {
					return fmt.Errorf("неверная карта HLS")
				}
				ref, _, _ := strings.Cut(rest, "\"")
				if err := fetch(resolve(ref), "", output, 2<<20); err != nil {
					return err
				}
			case strings.HasPrefix(line, "#EXT-X-BYTERANGE:"):
				lengthText, offsetText, explicit := strings.Cut(strings.TrimPrefix(line, "#EXT-X-BYTERANGE:"), "@")
				length, err := strconv.ParseInt(lengthText, 10, 64)
				if err != nil || length <= 0 || length > streamFileMaxBytes {
					return fmt.Errorf("неверный диапазон HLS")
				}
				start := rangeEnd
				if explicit {
					start, err = strconv.ParseInt(offsetText, 10, 64)
					if err != nil || start < 0 {
						return fmt.Errorf("неверное смещение HLS")
					}
				}
				rangeEnd = start + length
				byteRange = fmt.Sprintf("bytes=%d-%d", start, rangeEnd-1)
			case line != "" && !strings.HasPrefix(line, "#"):
				if variant {
					return playlist(resolve(line), depth+1)
				}
				if !strings.Contains(body, "#EXT-X-ENDLIST") {
					return fmt.Errorf("скачивание живой трансляции недоступно")
				}
				if segments >= 2000 {
					return fmt.Errorf("слишком много сегментов HLS")
				}
				if err := fetch(resolve(line), byteRange, output, streamFileMaxBytes-total); err != nil {
					return err
				}
				info, err := output.Stat()
				if err != nil {
					return err
				}
				total = info.Size()
				byteRange = ""
				segments++
				if report, ok := ctx.Value(transferProgressKey{}).(func(int64, int64)); ok {
					report(total, 0)
				}
			}
		}
		if segments == 0 {
			return fmt.Errorf("пустой плейлист HLS")
		}
		return nil
	}
	if err = playlist(target, 0); err != nil {
		return nil, err
	}
	_, err = output.Seek(0, io.SeekStart)
	success = err == nil
	return output, err
}
