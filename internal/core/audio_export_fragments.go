package core

import (
	"context"
	"encoding/binary"
	"fmt"
	"github.com/Eyevinn/mp4ff/mp4"
	"io"
	"os"
)

// Convert finite fragmented MP4 into a regular M4A sample table. Some Android
// players stop at the first moof even though the exported file contains all media.
func flattenAudioFragments(ctx context.Context, input *os.File, movie *mp4.File, audio *mp4.TrakBox) (output *os.File, err error) {
	dir, err := streamDownloadDir()
	if err != nil {
		return nil, err
	}
	samples, err := os.CreateTemp(dir, "cryon-export-samples-*")
	if err != nil {
		return nil, err
	}
	defer func() { samples.Close(); os.Remove(samples.Name()) }()
	table := mp4.NewStblBox()
	if audio.Mdia.Minf.Stbl == nil || audio.Mdia.Minf.Stbl.Stsd == nil {
		return nil, fmt.Errorf("нет описания аудиосэмплов")
	}
	table.AddChild(audio.Mdia.Minf.Stbl.Stsd)
	sizes := &mp4.StszBox{}
	timing := &mp4.SttsBox{}
	var totalDuration uint64
	var byteCount uint64
	trex := &mp4.TrexBox{TrackID: audio.Tkhd.TrackID}
	if movie.Moov.Mvex != nil {
		for _, candidate := range movie.Moov.Mvex.Trexs {
			if candidate.TrackID == audio.Tkhd.TrackID {
				trex = candidate
			}
		}
	}
	for _, segment := range movie.Segments {
		for _, fragment := range segment.Fragments {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if fragment.Mdat == nil || fragment.Moof == nil {
				return nil, fmt.Errorf("неполный фрагмент MP4")
			}
			length := fragment.Mdat.DataLength()
			if fragment.Mdat.IsLazy() {
				length = fragment.Mdat.GetLazyDataSize()
			}
			if length > 64<<20 {
				return nil, fmt.Errorf("слишком большой фрагмент MP4")
			}
			data := make([]byte, length)
			if _, err = input.ReadAt(data, int64(fragment.Mdat.PayloadAbsoluteOffset())); err != nil {
				return nil, err
			}
			fragment.Mdat.SetData(data)
			full, err := fragment.GetFullSamples(trex)
			if err != nil {
				return nil, err
			}
			for _, sample := range full {
				if sample.Dur == 0 || sample.Size == 0 || len(sample.Data) != int(sample.Size) || sample.CompositionTimeOffset != 0 {
					return nil, fmt.Errorf("некорректный аудиосэмпл MP4")
				}
				if len(sizes.SampleSize) >= 2000000 {
					return nil, fmt.Errorf("слишком много аудиосэмплов")
				}
				if _, err = samples.Write(sample.Data); err != nil {
					return nil, err
				}
				sizes.SampleSize = append(sizes.SampleSize, sample.Size)
				totalDuration += uint64(sample.Dur)
				byteCount += uint64(sample.Size)
				if byteCount > streamFileMaxBytes {
					return nil, fmt.Errorf("слишком большой аудиофайл")
				}
				n := len(timing.SampleTimeDelta)
				if n > 0 && timing.SampleTimeDelta[n-1] == sample.Dur {
					timing.SampleCount[n-1]++
				} else {
					timing.SampleTimeDelta = append(timing.SampleTimeDelta, sample.Dur)
					timing.SampleCount = append(timing.SampleCount, 1)
				}
			}
			fragment.Mdat.Data = nil
		}
	}
	if len(sizes.SampleSize) == 0 || audio.Mdia.Mdhd.Timescale == 0 {
		return nil, fmt.Errorf("пустое аудио MP4")
	}
	sizes.SampleNumber = uint32(len(sizes.SampleSize))
	table.AddChild(timing)
	table.AddChild(sizes)
	chunks := &mp4.StscBox{}
	if err = chunks.AddEntry(1, sizes.SampleNumber, 1); err != nil {
		return nil, err
	}
	offsets := &mp4.StcoBox{ChunkOffset: []uint32{0}}
	table.AddChild(chunks)
	table.AddChild(offsets)
	audio.Mdia.Minf.Stbl = table
	for i, child := range audio.Mdia.Minf.Children {
		if child.Type() == "stbl" {
			audio.Mdia.Minf.Children[i] = table
		}
	}
	audio.Mdia.Mdhd.Duration = totalDuration
	audio.Tkhd.Duration = totalDuration * uint64(movie.Moov.Mvhd.Timescale) / uint64(audio.Mdia.Mdhd.Timescale)
	movie.Moov.Mvhd.Duration = audio.Tkhd.Duration
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
	binary.BigEndian.PutUint32(header, uint32(byteCount+8))
	copy(header[4:], "mdat")
	if _, err = writer.Write(header); err != nil {
		return nil, err
	}
	_, _ = samples.Seek(0, 0)
	if _, err = io.Copy(writer, samples); err != nil {
		return nil, err
	}
	_, err = output.Seek(0, 0)
	success = err == nil
	return output, err
}
