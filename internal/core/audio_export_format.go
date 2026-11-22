package core

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"

	"github.com/Eyevinn/mp4ff/mp4"
)

// Inspect bytes: several CDNs label MP3 as octet-stream or video/mp4.
func audioFileType(f *os.File) (string, error) {
	var head [4096]byte
	n, err := f.ReadAt(head[:], 0)
	if err != nil && err != io.EOF {
		return "", err
	}
	b := head[:n]
	switch {
	case bytes.HasPrefix(b, []byte("ID3")):
		return "audio/mpeg", nil
	case bytes.HasPrefix(b, []byte("fLaC")):
		return "audio/flac", nil
	case bytes.HasPrefix(b, []byte("OggS")):
		return "audio/ogg", nil
	case bytes.HasPrefix(b, []byte{0x1a, 0x45, 0xdf, 0xa3}):
		return "video/webm", nil // Container identified; upstream must mark it audio-only.
	case len(b) >= 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WAVE":
		return "audio/wav", nil
	case len(b) >= 12 && string(b[4:8]) == "ftyp":
		return "video/mp4", nil
	case bytes.HasPrefix(bytes.TrimSpace(b), []byte("#EXTM3U")):
		return "application/vnd.apple.mpegurl", nil
	}
	for i := 0; i+4 <= len(b) && i < 1024; i++ {
		if b[i] == 0xff && b[i+1]&0xe0 == 0xe0 {
			if b[i+1]&0xf6 == 0xf0 {
				return "audio/aac", nil
			}
			if b[i+1]&6 != 0 && b[i+2]&0xf0 != 0xf0 && b[i+2]&0xf0 != 0 {
				return "audio/mpeg", nil
			}
		}
	}
	return "", nil
}

type exportWriter struct {
	ctx    context.Context
	writer io.Writer
}

func (w exportWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	return w.writer.Write(p)
}

// Remux a progressive MP4 (including YouTube's muxed fallback) without a
// transcoder: copy only audio samples, preserve timing/codec, rebuild offsets.
// Playback's cached original is never modified.
func extractMP4Audio(ctx context.Context, input *os.File) (output *os.File, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("некорректный MP4: %v", recovered)
		}
	}()
	_, _ = input.Seek(0, io.SeekStart)
	movie, err := mp4.DecodeFile(input, mp4.WithDecodeMode(mp4.DecModeLazyMdat))
	if err != nil {
		return nil, fmt.Errorf("разбор MP4: %w", err)
	}
	if movie.Moov == nil {
		return nil, fmt.Errorf("MP4 не содержит описания аудиодорожки")
	}
	var audio *mp4.TrakBox
	for _, track := range movie.Moov.Traks {
		if track.Mdia != nil && track.Mdia.Hdlr != nil && track.Mdia.Hdlr.HandlerType == "soun" {
			audio = track
			break
		}
	}
	if audio == nil {
		return nil, fmt.Errorf("в файле нет аудиодорожки")
	}
	if movie.Moov.IsEncrypted(audio.Tkhd.TrackID) {
		return nil, fmt.Errorf("аудиодорожка защищена источником")
	}
	if len(movie.Moov.Traks) == 1 {
		_, err = input.Seek(0, io.SeekStart)
		return input, err
	}
	if movie.IsFragmented() {
		return nil, fmt.Errorf("сохранение смешанного фрагментированного MP4 пока недоступно")
	}
	table := audio.Mdia.Minf.Stbl
	if table == nil || table.Stsz == nil || table.Stsc == nil || table.Stsd == nil || len(table.Stsd.Children) != 1 {
		return nil, fmt.Errorf("неподдерживаемая таблица аудиодорожки")
	}
	count := audio.GetNrSamples()
	if count == 0 || count > 2000000 {
		return nil, fmt.Errorf("некорректное число аудиосэмплов")
	}
	dir, err := streamDownloadDir()
	if err != nil {
		return nil, err
	}
	samples, err := os.CreateTemp(dir, "cryon-export-samples-*")
	if err != nil {
		return nil, err
	}
	defer func() { samples.Close(); os.Remove(samples.Name()) }()
	if err = movie.CopySampleData(exportWriter{ctx, samples}, input, audio, 1, count, nil); err != nil {
		return nil, err
	}
	info, err := samples.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > 512<<20 {
		return nil, fmt.Errorf("слишком большой аудиофайл")
	}
	newTable := mp4.NewStblBox()
	for _, child := range table.Children {
		switch child.Type() {
		case "stsc", "stco", "co64":
			continue
		}
		newTable.AddChild(child)
	}
	chunks := &mp4.StscBox{}
	if err = chunks.AddEntry(1, count, 1); err != nil {
		return nil, err
	}
	offsets := &mp4.StcoBox{ChunkOffset: []uint32{0}}
	newTable.AddChild(chunks)
	newTable.AddChild(offsets)
	audio.Mdia.Minf.Stbl = newTable
	for i, child := range audio.Mdia.Minf.Children {
		if child.Type() == "stbl" {
			audio.Mdia.Minf.Children[i] = newTable
		}
	}
	moov := mp4.NewMoovBox()
	for _, child := range movie.Moov.Children {
		if child.Type() != "trak" && child.Type() != "mvex" {
			moov.AddChild(child)
		}
	}
	moov.AddChild(audio)
	ftyp := mp4.CreateFtyp()
	offsets.ChunkOffset[0] = uint32(ftyp.Size() + moov.Size() + 8)
	output, err = os.CreateTemp(dir, "cryon-export-audio-*.m4a")
	if err != nil {
		return nil, err
	}
	created := output
	success := false
	defer func() {
		if !success {
			created.Close()
			os.Remove(created.Name())
			output = nil
		}
	}()
	writer := exportWriter{ctx, output}
	if err = ftyp.Encode(writer); err != nil {
		return nil, err
	}
	if err = moov.Encode(writer); err != nil {
		return nil, err
	}
	header := make([]byte, 8)
	binary.BigEndian.PutUint32(header, uint32(info.Size()+8))
	copy(header[4:], "mdat")
	if _, err = writer.Write(header); err != nil {
		return nil, err
	}
	_, _ = samples.Seek(0, io.SeekStart)
	if _, err = io.Copy(writer, samples); err != nil {
		return nil, err
	}
	_, err = output.Seek(0, io.SeekStart)
	success = err == nil
	return output, err
}
