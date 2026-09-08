package yandex

import (
	"crypto/md5"
	"fmt"
	"strings"
	"testing"
)

func TestResolveDownloadURLSign(t *testing.T) {
	// Проверяем формулу подписи: md5(salt + path[1:] + s).
	path := "/get-mp3/abc/0/track.mp3"
	s := "someSalt"
	want := fmt.Sprintf("%x", md5.Sum([]byte(downloadInfoSalt+strings.TrimPrefix(path, "/")+s)))

	got := fmt.Sprintf("%x", md5.Sum([]byte(downloadInfoSalt+"get-mp3/abc/0/track.mp3"+s)))
	if got != want {
		t.Fatalf("подпись не совпала: got %s want %s", got, want)
	}
}

func TestNormalizeCover(t *testing.T) {
	cases := map[string]string{
		"":                                "",
		"avatars.yandex.net/get-music/x/%%": "https://avatars.yandex.net/get-music/x/400x400",
	}
	for in, want := range cases {
		if got := normalizeCover(in); got != want {
			t.Errorf("normalizeCover(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHasToken(t *testing.T) {
	if New("").HasToken() {
		t.Error("пустой токен не должен считаться наличием токена")
	}
	if !New("tkn").HasToken() {
		t.Error("непустой токен должен считаться наличием токена")
	}
	if New("  ").HasToken() {
		t.Error("токен из пробелов должен обрезаться до пустого")
	}
}

func TestResolveStreamWithoutToken(t *testing.T) {
	_, err := New("").ResolveStream(nil, "123")
	if err == nil {
		t.Fatal("ожидалась ошибка при отсутствии токена")
	}
}

func TestServiceID(t *testing.T) {
	if New("").ID() != "yandex" {
		t.Errorf("ID() = %q, want yandex", New("").ID())
	}
}
