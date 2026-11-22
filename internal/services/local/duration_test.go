package local

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestWAVDurationAndFilenameMetadata(t *testing.T) {
	data := make([]byte, 44+16000)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 1)
	binary.LittleEndian.PutUint32(data[24:], 8000)
	binary.LittleEndian.PutUint32(data[28:], 16000)
	binary.LittleEndian.PutUint16(data[32:], 2)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], 16000)
	path := filepath.Join(t.TempDir(), "Артист — Песня.wav")
	os.WriteFile(path, data, 0600)
	track, err := readTrack(path)
	if err != nil || track.DurationMs != 1000 || track.Title != "Песня" || len(track.Artists) != 1 || track.Artists[0] != "Артист" {
		t.Fatalf("metadata %+v %v", track, err)
	}
}
func TestMalformedMP4DoesNotPanic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.m4a")
	os.WriteFile(path, []byte("\x00\x00\x00\x10ftypbad-data"), 0600)
	if AudioDurationMs(path) != 0 {
		t.Fatal("malformed file has duration")
	}
}
