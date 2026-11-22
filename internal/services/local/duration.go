package local

import (
	"bufio"
	"encoding/binary"
	"github.com/Eyevinn/mp4ff/mp4"
	"io"
	"os"
)

// AudioDurationMs reads container/sample timing without decoding or invoking an external program.
func AudioDurationMs(path string) (durationMs int) {
	defer func() {
		if recover() != nil {
			durationMs = 0
		}
	}()
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	var head [32]byte
	n, _ := f.ReadAt(head[:], 0)
	if n < 12 {
		return 0
	}
	b := head[:]
	switch {
	case string(b[:4]) == "fLaC":
		var info [34]byte
		if _, err := f.ReadAt(info[:], 8); err == nil {
			timing := binary.BigEndian.Uint64(info[10:18])
			rate := timing >> 44
			samples := timing & ((1 << 36) - 1)
			if rate > 0 {
				return int(samples * 1000 / rate)
			}
		}
	case string(b[4:8]) == "ftyp":
		movie, err := mp4.DecodeFile(f, mp4.WithDecodeMode(mp4.DecModeLazyMdat))
		if err != nil || movie.Moov == nil {
			return 0
		}
		for _, track := range movie.Moov.Traks {
			if track.Mdia == nil || track.Mdia.Hdlr == nil || track.Mdia.Hdlr.HandlerType != "soun" || track.Mdia.Mdhd.Timescale == 0 {
				continue
			}
			duration := track.Mdia.Mdhd.Duration
			for _, segment := range movie.Segments {
				for _, fragment := range segment.Fragments {
					if fragment.Moof == nil {
						continue
					}
					for _, traf := range fragment.Moof.Trafs {
						if traf.Tfhd == nil || traf.Tfhd.TrackID != track.Tkhd.TrackID {
							continue
						}
						defaultDuration := traf.Tfhd.DefaultSampleDuration
						if defaultDuration == 0 && movie.Moov.Mvex != nil {
							for _, trex := range movie.Moov.Mvex.Trexs {
								if trex.TrackID == track.Tkhd.TrackID {
									defaultDuration = trex.DefaultSampleDuration
								}
							}
						}
						var end uint64
						if traf.Tfdt != nil {
							end = traf.Tfdt.BaseMediaDecodeTime()
						}
						for _, trun := range traf.Truns {
							end += trun.Duration(defaultDuration)
						}
						if end > duration {
							duration = end
						}
					}
				}
			}
			return int(duration * 1000 / uint64(track.Mdia.Mdhd.Timescale))
		}
	case string(b[:4]) == "RIFF" && string(b[8:12]) == "WAVE":
		var rate, size uint64
		offset := int64(12)
		for i := 0; i < 4096; i++ {
			var chunk [8]byte
			if _, err := f.ReadAt(chunk[:], offset); err != nil {
				break
			}
			length := binary.LittleEndian.Uint32(chunk[4:])
			if string(chunk[:4]) == "fmt " {
				var format [16]byte
				if _, err := f.ReadAt(format[:], offset+8); err == nil {
					rate = uint64(binary.LittleEndian.Uint32(format[8:]))
				}
			}
			if string(chunk[:4]) == "data" {
				size = uint64(length)
				break
			}
			offset += 8 + int64(length) + int64(length%2)
		}
		if rate > 0 {
			return int(size * 1000 / rate)
		}
	case string(b[:4]) == "OggS":
		return oggDuration(f)
	default:
		return frameDuration(f)
	}
	return 0
}

func frameDuration(f *os.File) int {
	reader := bufio.NewReaderSize(f, 64<<10)
	if first, _ := reader.Peek(10); len(first) == 10 && string(first[:3]) == "ID3" {
		size := int64(first[6]&127)<<21 | int64(first[7]&127)<<14 | int64(first[8]&127)<<7 | int64(first[9]&127)
		if size > 16<<20 {
			return 0
		}
		_, _ = io.CopyN(io.Discard, reader, size+10)
	}
	var seconds float64
	scanned := 0
	for {
		h, err := reader.Peek(7)
		if err != nil {
			break
		}
		length, rate, samples := 0, 0, 0
		if h[0] == 255 && h[1]&0xf6 == 0xf0 {
			rates := []int{96000, 88200, 64000, 48000, 44100, 32000, 24000, 22050, 16000, 12000, 11025, 8000, 7350}
			index := int(h[2] >> 2 & 15)
			if index < len(rates) {
				rate = rates[index]
				length = int(h[3]&3)<<11 | int(h[4])<<3 | int(h[5]>>5)
				samples = 1024 * (int(h[6]&3) + 1)
			}
		} else if h[0] == 255 && h[1]&0xe0 == 0xe0 && h[1]&6 == 2 {
			version := h[1] >> 3 & 3
			index := int(h[2] >> 4)
			sr := int(h[2] >> 2 & 3)
			if version != 1 && sr < 3 && index > 0 && index < 15 {
				rate = []int{44100, 48000, 32000}[sr]
				bitrates := []int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320}
				samples = 1152
				scale := 144
				if version != 3 {
					rate /= 2
					if version == 0 {
						rate /= 2
					}
					bitrates = []int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160}
					samples = 576
					scale = 72
				}
				length = scale*bitrates[index]*1000/rate + int(h[2]>>1&1)
			}
		}
		if length < 7 || rate == 0 {
			if scanned > 1<<20 {
				break
			}
			_, _ = reader.ReadByte()
			scanned++
			continue
		}
		if _, err = io.CopyN(io.Discard, reader, int64(length)); err != nil {
			break
		}
		seconds += float64(samples) / float64(rate)
	}
	return int(seconds * 1000)
}

func oggDuration(f *os.File) int {
	var rate, skip uint64
	var last uint64
	var serial uint32
	offset := int64(0)
	for {
		var page [27]byte
		if _, err := f.ReadAt(page[:], offset); err != nil || string(page[:4]) != "OggS" {
			break
		}
		laces := make([]byte, int(page[26]))
		if _, err := f.ReadAt(laces, offset+27); err != nil {
			break
		}
		length := 0
		for _, n := range laces {
			length += int(n)
		}
		if offset == 0 {
			serial = binary.LittleEndian.Uint32(page[14:18])
			var packet [32]byte
			_, _ = f.ReadAt(packet[:], offset+27+int64(len(laces)))
			if string(packet[:8]) == "OpusHead" {
				rate = 48000
				skip = uint64(binary.LittleEndian.Uint16(packet[10:12]))
			} else if string(packet[1:7]) == "vorbis" {
				rate = uint64(binary.LittleEndian.Uint32(packet[12:16]))
			}
		}
		granule := binary.LittleEndian.Uint64(page[6:14])
		if binary.LittleEndian.Uint32(page[14:18]) == serial && granule != ^uint64(0) && granule > last {
			last = granule
		}
		offset += 27 + int64(len(laces)) + int64(length)
	}
	if rate > 0 && last > skip {
		return int((last - skip) * 1000 / rate)
	}
	return 0
}
