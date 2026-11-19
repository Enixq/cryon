//go:build windows

package playback

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLiveMPVStartsAtRequestedPosition(t *testing.T) {
	if os.Getenv("CRYON_MPV_IT") != "1" {
		t.Skip("live mpv test disabled")
	}
	mpvPath := os.Getenv("CRYON_MPV_PATH")
	if mpvPath == "" {
		t.Fatal("CRYON_MPV_PATH must point to mpv")
	}
	const sampleRate = 8000
	const dataSize = sampleRate * 2 * 90
	data := make([]byte, 44+dataSize)
	copy(data[0:], "RIFF")
	binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 1)
	binary.LittleEndian.PutUint32(data[24:], sampleRate)
	binary.LittleEndian.PutUint32(data[28:], sampleRate*2)
	binary.LittleEndian.PutUint16(data[32:], 2)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], dataSize)
	path := filepath.Join(t.TempDir(), "silence.wav")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	controller := New(mpvPath)
	controller.ipcPath = fmt.Sprintf(`\\.\pipe\cryon-position-test-%d-%d`, os.Getpid(), time.Now().UnixNano())
	defer controller.Shutdown()
	if err := controller.PlayAt("test", path, 71); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		status := controller.Status()
		if status.PositionS >= 70 && status.PositionS < 80 {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("mpv did not reach the requested position: %+v", controller.Status())
}
