package vkweb

import (
	"strconv"
	"strings"
)

// VK's alphabet swaps O and 0; standard base64 is not compatible.
const vkAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMN0PQRSTUVWXYZO123456789+/="

func vkBase64Decode(value string) (string, bool) {
	if value == "" || len(value)%4 == 1 {
		return "", false
	}
	var out []byte
	var accum int32
	count := 0
	for _, c := range value {
		index := strings.IndexRune(vkAlphabet, c)
		if index < 0 {
			continue
		}
		if count%4 == 0 {
			accum = int32(index)
		} else {
			accum = 64*accum + int32(index)
		}
		previous := count
		count++
		if previous%4 != 0 {
			out = append(out, byte(accum>>uint((-2*count)&6)))
		}
	}
	return string(out), len(out) > 0
}
func shuffle(value string, seed int64) string {
	n := len(value)
	if n == 0 {
		return value
	}
	if seed < 0 {
		seed = -seed
	}
	indexes := make([]int, n)
	for o := n - 1; o >= 0; o-- {
		seed = int64(int32(n*(o+1))^int32(seed+int64(o))) % int64(n)
		indexes[o] = int(seed)
	}
	arr := []byte(value)
	for o := 1; o < n; o++ {
		index := indexes[n-1-o]
		if index < 0 {
			index += n
		}
		arr[o], arr[index] = arr[index], arr[o]
	}
	return string(arr)
}
func decodeAudioURL(target string, uid int64) string {
	if !strings.Contains(target, "audio_api_unavailable") {
		if validURL(target) {
			return target
		}
		return ""
	}
	_, extra, ok := strings.Cut(target, "?extra=")
	if !ok {
		return ""
	}
	parts := strings.SplitN(extra, "#", 2)
	value, ok := vkBase64Decode(parts[0])
	if !ok {
		return ""
	}
	operations := ""
	if len(parts) == 2 && parts[1] != "" {
		operations, ok = vkBase64Decode(parts[1])
		if !ok {
			return ""
		}
	}
	var list []string
	if operations != "" {
		list = strings.Split(operations, "\t")
	}
	for i := len(list) - 1; i >= 0; i-- {
		args := strings.Split(list[i], "\v")
		arg := ""
		if len(args) > 1 {
			arg = args[1]
		}
		switch args[0] {
		case "v":
			arr := []byte(value)
			for l, r := 0, len(arr)-1; l < r; l, r = l+1, r-1 {
				arr[l], arr[r] = arr[r], arr[l]
			}
			value = string(arr)
		case "r":
			shift, err := strconv.Atoi(arg)
			if err != nil {
				return ""
			}
			alphabet := vkAlphabet + vkAlphabet
			var out strings.Builder
			for _, c := range value {
				index := strings.IndexRune(alphabet, c)
				if index < 0 {
					out.WriteRune(c)
					continue
				}
				index -= shift
				if index < 0 {
					index += len(alphabet)
				}
				if index >= 0 && index < len(alphabet) {
					out.WriteByte(alphabet[index])
				}
			}
			value = out.String()
		case "s", "i":
			seed, err := strconv.ParseInt(arg, 10, 64)
			if err != nil {
				return ""
			}
			if args[0] == "i" {
				seed = int64(int32(seed) ^ int32(uid))
			}
			value = shuffle(value, seed)
		case "x":
			if arg == "" {
				return ""
			}
			arr := []byte(value)
			for i := range arr {
				arr[i] ^= arg[0]
			}
			value = string(arr)
		default:
			return ""
		}
	}
	if validURL(value) {
		return value
	}
	return ""
}
