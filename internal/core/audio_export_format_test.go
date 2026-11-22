package core

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Eyevinn/mp4ff/mp4"
)

func TestMP4ExportCopiesOnlyAudioAndKeepsTiming(t *testing.T) {
	input, err := os.Open("testdata/muxed-audio-video.mp4")
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	original, err := mp4.DecodeFile(input)
	if err != nil {
		t.Fatal(err)
	}
	var source *mp4.TrakBox
	for _, track := range original.Moov.Traks {
		if track.Mdia.Hdlr.HandlerType == "soun" {
			source = track
		}
	}
	if source == nil || len(original.Moov.Traks) < 2 {
		t.Fatal("fixture must contain audio and video")
	}
	var expected bytes.Buffer
	if err = original.CopySampleData(&expected, nil, source, 1, source.GetNrSamples(), nil); err != nil {
		t.Fatal(err)
	}
	output, err := extractMP4Audio(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { output.Close(); os.Remove(output.Name()) }()
	audio, err := mp4.DecodeFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(audio.Moov.Traks) != 1 || audio.Moov.Trak.Mdia.Hdlr.HandlerType != "soun" {
		t.Fatal("export still contains video")
	}
	if audio.Moov.Trak.GetNrSamples() != source.GetNrSamples() || audio.Moov.Trak.Mdia.Mdhd.Duration != source.Mdia.Mdhd.Duration {
		t.Fatal("audio duration or samples changed")
	}
	var actual bytes.Buffer
	if err = audio.CopySampleData(&actual, nil, audio.Moov.Trak, 1, audio.Moov.Trak.GetNrSamples(), nil); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual.Bytes(), expected.Bytes()) {
		t.Fatal("audio samples were changed during remux")
	}
	if audio.Mdat.Size() >= original.Mdat.Size() {
		t.Fatal("video bytes were not removed")
	}
}

func TestHLSExportAssemblesRangesAndRejectsEncryptedPlaylist(t *testing.T) {
	payload := []byte("ID3\x04\x00\x00\x00\x00\x00\x00complete-audio")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/encrypted" {
			io.WriteString(w, "#EXTM3U\n#EXT-X-KEY:METHOD=AES-128,URI=\"secret\"\n#EXTINF:3,\n/audio\n#EXT-X-ENDLIST")
			return
		}
		if r.URL.Path == "/audio" {
			http.ServeContent(w, r, "audio", time.Time{}, bytes.NewReader(payload))
			return
		}
		io.WriteString(w, "#EXTM3U\n#EXT-X-BYTERANGE:10@0\n/audio\n#EXT-X-BYTERANGE:14@10\n/audio\n#EXT-X-ENDLIST\n")
	}))
	defer server.Close()
	output, err := exportHLS(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { output.Close(); os.Remove(output.Name()) }()
	data, err := io.ReadAll(output)
	if err != nil || !bytes.Equal(data, payload) {
		t.Fatalf("incomplete HLS export: %q %v", data, err)
	}
	if file, err := exportHLS(context.Background(), server.URL+"/encrypted"); err == nil || file != nil {
		t.Fatal("encrypted HLS must not be exported")
	}
}

func TestUnknownFileDoesNotBecomeMP3ByHeader(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "unknown")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	io.WriteString(f, "<html>login required</html>")
	kind, err := audioFileType(f)
	if err != nil || strings.HasPrefix(kind, "audio/") {
		t.Fatal("HTML misidentified as audio")
	}
}
