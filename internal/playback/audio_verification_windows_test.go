//go:build windows

package playback

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"Cryon2/internal/services/audiofetcher"
)

// TestLiveAudioDecodedIsNotSilence проверяет, что mpv реально декодирует
// аудиопоток YouTube и содержит звук, а не тишину.
// Включается переменной окружения CRYON_AUDIO_IT=1.
func TestLiveAudioDecodedIsNotSilence(t *testing.T) {
	if os.Getenv("CRYON_AUDIO_IT") != "1" {
		t.Skip("live audio verification test disabled (set CRYON_AUDIO_IT=1)")
	}
	mpvPath := os.Getenv("CRYON_MPV_PATH")
	if mpvPath == "" {
		mpvPath = findMPV("")
	}
	if mpvPath == "" {
		t.Fatal("mpv not found: set CRYON_MPV_PATH or install mpv")
	}

	// 1. Резолвим YouTube-поток
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	videoID := "GCdwKhTtNNw" // Sweater Weather — реальный музыкальный трек
	if vid := os.Getenv("CRYON_AUDIO_VIDEO_ID"); vid != "" {
		videoID = vid
	}
	stream, err := audiofetcher.GetYouTubeAudioStream(ctx, videoID)
	if err != nil {
		t.Fatalf("resolve YouTube stream: %v", err)
	}
	if stream.URL == "" {
		t.Fatal("empty stream URL")
	}

	t.Logf("resolved stream: %d bytes, format=%s", stream.Duration, stream.Format)

	// 2. Скачиваем поток локально через Go-клиент (mpv не умеет открывать
	// googlevideo URL напрямую: ip-bound URL отбрасывает TLS-соединение).
	localPath := filepath.Join(t.TempDir(), "source")
	if err := downloadStream(ctx, stream.URL, localPath); err != nil {
		t.Fatalf("download stream: %v", err)
	}
	info, _ := os.Stat(localPath)
	t.Logf("downloaded: %d bytes", info.Size())

	// 3. Запускаем mpv с ao=pcm — записывает сырой 16-bit PCM в WAV
	wavPath := filepath.Join(t.TempDir(), "output.wav")

	cmd := exec.CommandContext(ctx, mpvPath,
		"--force-window=no",
		"--keep-open=no",
		"--no-video", // пропускаем видеодорожку если формат прогрессивный
		"--ao=pcm",
		"--ao-pcm-file="+wavPath,
		// --start=5 пропускаем возможное тихое вступление;
		// --end=15 ограничивает 10 секундами.
		"--start=5",
		"--end=15",
		localPath,
	)
	mpvOut, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mpv ao=pcm failed: %v\nmpv output:\n%s", err, mpvOut)
	}

	// 4. Проверяем WAV-файл
	pcmData, err := readPCM16(wavPath)
	if err != nil {
		t.Fatalf("read PCM: %v", err)
	}
	if len(pcmData) == 0 {
		t.Fatal("PCM data is empty — mpv wrote no audio")
	}

	peak, rms := analyzePCM(pcmData)
	t.Logf("PCM: %d samples, peak=%.4f, rms=%.6f", len(pcmData), peak, rms)

	// 5. Проверяем что это не тишина
	// Тишина: peak < 0.01 (менее 1% от полной шкалы) и rms < 0.001
	const silencePeakThreshold = 0.01
	const silenceRMSThreshold = 0.0005
	if peak < silencePeakThreshold {
		t.Errorf("peak=%.4f — поток состоит из тишины (порог: %.2f)", peak, silencePeakThreshold)
	}
	if rms < silenceRMSThreshold {
		t.Errorf("rms=%.6f — поток состоит из тишины (порог: %.6f)", rms, silenceRMSThreshold)
	}
}

// TestLiveAudioSeekedIsNotSilence проверяет что звук идёт и после перемотки
// за пределы 1 МиБ (проблема с googlevideo range-ограничением).
func TestLiveAudioSeekedIsNotSilence(t *testing.T) {
	if os.Getenv("CRYON_AUDIO_IT") != "1" {
		t.Skip("live audio verification test disabled (set CRYON_AUDIO_IT=1)")
	}
	mpvPath := os.Getenv("CRYON_MPV_PATH")
	if mpvPath == "" {
		mpvPath = findMPV("")
	}
	if mpvPath == "" {
		t.Fatal("mpv not found: set CRYON_MPV_PATH or install mpv")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	videoID := "GCdwKhTtNNw"
	if vid := os.Getenv("CRYON_AUDIO_VIDEO_ID"); vid != "" {
		videoID = vid
	}
	stream, err := audiofetcher.GetYouTubeAudioStream(ctx, videoID)
	if err != nil {
		t.Fatalf("resolve YouTube stream: %v", err)
	}

	t.Logf("seeking to %ds in %ds track", 120, stream.Duration)

	// Скачиваем поток локально
	localPath := filepath.Join(t.TempDir(), "source_seek")
	if err := downloadStream(ctx, stream.URL, localPath); err != nil {
		t.Fatalf("download stream: %v", err)
	}

	// Перемотка на 120 сек — далеко за пределами 1 МиБ (примерно 35-секундный чанк)
	wavPath := filepath.Join(t.TempDir(), "seeked.wav")

	cmd := exec.CommandContext(ctx, mpvPath,
		"--force-window=no",
		"--keep-open=no",
		"--no-video",
		"--ao=pcm",
		"--ao-pcm-file="+wavPath,
		"--start=120",
		"--end=128",
		localPath,
	)
	mpvOut, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mpv ao=pcm seek failed: %v\nmpv output:\n%s", err, mpvOut)
	}

	pcmData, err := readPCM16(wavPath)
	if err != nil {
		t.Fatalf("read PCM: %v", err)
	}
	if len(pcmData) == 0 {
		t.Fatal("PCM data is empty — mpv wrote no audio after seek")
	}

	peak, rms := analyzePCM(pcmData)
	t.Logf("seeked PCM: %d samples, peak=%.4f, rms=%.6f", len(pcmData), peak, rms)

	const silencePeakThreshold = 0.01
	const silenceRMSThreshold = 0.0005
	if peak < silencePeakThreshold {
		t.Errorf("seeked peak=%.4f — поток тишины после перемотки (порог: %.2f)", peak, silencePeakThreshold)
	}
	if rms < silenceRMSThreshold {
		t.Errorf("seeked rms=%.6f — поток тишины после перемотки (порог: %.6f)", rms, silenceRMSThreshold)
	}
}

// downloadStream скачивает поток по URL в локальный файл.
func downloadStream(ctx context.Context, url, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://www.youtube.com/")
	req.Header.Set("Origin", "https://www.youtube.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return nil
}

// readPCM16 читает WAV-файл и возвращает сэмплы как float64 в диапазоне [-1, 1].
// Поддерживает 16-bit LE и 32-bit IEEE float, включая WAVE_FORMAT_EXTENSIBLE.
func readPCM16(path string) ([]float64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// RIFF header (12 bytes)
	var riffHeader [12]byte
	if _, err := io.ReadFull(f, riffHeader[:]); err != nil {
		return nil, fmt.Errorf("read RIFF header: %w", err)
	}
	if string(riffHeader[0:4]) != "RIFF" || string(riffHeader[8:12]) != "WAVE" {
		return nil, fmt.Errorf("not a WAV file")
	}

	// Сканируем чанки в поисках fmt и data
	bitsPerSample := 0
	var pcm []byte
	for {
		var chunkHeader [8]byte
		if _, err := io.ReadFull(f, chunkHeader[:]); err != nil {
			break
		}
		chunkID := string(chunkHeader[0:4])
		chunkSize := int(binary.LittleEndian.Uint32(chunkHeader[4:8]))

		switch chunkID {
		case "fmt ":
			if chunkSize < 16 {
				return nil, fmt.Errorf("fmt chunk too small: %d", chunkSize)
			}
			fmtData := make([]byte, chunkSize)
			if _, err := io.ReadFull(f, fmtData); err != nil {
				return nil, fmt.Errorf("read fmt: %w", err)
			}
			_ = int(binary.LittleEndian.Uint16(fmtData[2:4])) // channels
			bitsPerSample = int(binary.LittleEndian.Uint16(fmtData[14:16]))

		case "data":
			if chunkSize > 1<<24 {
				return nil, fmt.Errorf("data chunk too large: %d", chunkSize)
			}
			pcm = make([]byte, chunkSize)
			if _, err := io.ReadFull(f, pcm); err != nil {
				return nil, fmt.Errorf("read data: %w", err)
			}

		default:
			// Пропускаем другие чанки (LIST, fact и т.п.)
			remaining := chunkSize
			if chunkSize%2 != 0 {
				remaining++
			}
			if _, err := io.CopyN(io.Discard, f, int64(remaining)); err != nil {
				return nil, fmt.Errorf("skip chunk %s: %w", chunkID, err)
			}
		}
	}

	if pcm == nil {
		return nil, fmt.Errorf("no data chunk found in WAV")
	}

	// Определяем битность: если fmt не найден — предполагаем 16-bit
	if bitsPerSample == 0 {
		bitsPerSample = 16
	}

	switch bitsPerSample {
	case 16:
		return samplesInt16(pcm), nil
	case 32:
		return samplesFloat32(pcm), nil
	default:
		return nil, fmt.Errorf("unsupported bits-per-sample: %d", bitsPerSample)
	}
}

// samplesInt16 парсит 16-bit signed LE сэмплы (все каналы).
func samplesInt16(pcm []byte) []float64 {
	samples := make([]float64, len(pcm)/2)
	for i := range samples {
		val := int16(pcm[i*2]) | int16(pcm[i*2+1])<<8
		samples[i] = float64(val) / 32768.0
	}
	return samples
}

// samplesFloat32 парсит 32-bit IEEE float сэмплы (все каналы).
func samplesFloat32(pcm []byte) []float64 {
	samples := make([]float64, len(pcm)/4)
	for i := range samples {
		val := binary.LittleEndian.Uint32(pcm[i*4:])
		samples[i] = float64(math.Float32frombits(val))
	}
	return samples
}

// analyzePCM вычисляет пиковый уровень и среднеквадратичное значение сэмплов
// в диапазоне [-1, 1].
func analyzePCM(samples []float64) (peak, rms float64) {
	for _, s := range samples {
		if a := math.Abs(s); a > peak {
			peak = a
		}
		rms += s * s
	}
	if len(samples) > 0 {
		rms = math.Sqrt(rms / float64(len(samples)))
	}
	return peak, rms
}
