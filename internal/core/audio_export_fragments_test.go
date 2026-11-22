package core

import (
	"Cryon2/internal/services/local"
	"bytes"
	"context"
	"github.com/Eyevinn/mp4ff/mp4"
	"os"
	"testing"
)

func TestFragmentExportPreservesAllSamplesAndTiming(t *testing.T) {
	init := mp4.CreateEmptyInit()
	init.AddEmptyTrack(48000, "audio", "und")
	init.Moov.Trak.Mdia.Minf.Stbl.Stsd.AddChild(mp4.CreateAudioSampleEntryBox("mp4a", 2, 16, 48000, nil))
	input, err := os.CreateTemp(t.TempDir(), "fragmented-*.m4a")
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	if err = init.Encode(input); err != nil {
		t.Fatal(err)
	}
	var expected bytes.Buffer
	var time uint64
	for i := uint32(1); i <= 3; i++ {
		fragment, _ := mp4.CreateFragment(i, 1)
		for j := 0; j < 8; j++ {
			data := []byte{byte(i), byte(j), 1, 2, 3}
			expected.Write(data)
			fragment.AddFullSample(mp4.FullSample{Sample: mp4.Sample{Dur: 48000, Size: uint32(len(data))}, DecodeTime: time, Data: data})
			time += 48000
		}
		if err = fragment.Encode(input); err != nil {
			t.Fatal(err)
		}
	}
	output, err := extractMP4Audio(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { output.Close(); os.Remove(output.Name()) }()
	movie, err := mp4.DecodeFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if movie.IsFragmented() || movie.Moov.Trak.GetNrSamples() != 24 || local.AudioDurationMs(output.Name()) != 24000 {
		t.Fatal("fragment timing/sample table incomplete")
	}
	var actual bytes.Buffer
	if err = movie.CopySampleData(&actual, nil, movie.Moov.Trak, 1, 24, nil); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual.Bytes(), expected.Bytes()) {
		t.Fatal("remux changed audio samples")
	}
}
