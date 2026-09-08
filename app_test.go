package main

import "testing"

// seedSignature: порядок затравок не влияет на подпись (их всё равно
// пересортировывает PersonalizeReleases), а разный набор даёт разные ключи.
// Пустой набор — стабильный «-».
func TestSeedSignature(t *testing.T) {
	if got := seedSignature(nil); got != "-" {
		t.Fatalf("пустой набор должен давать \"-\", got %q", got)
	}

	a := seedSignature([]string{"Aurora", "Boards"})
	b := seedSignature([]string{"boards", " aurora "}) // другой порядок/регистр/пробелы
	if a != b {
		t.Fatalf("подпись не должна зависеть от порядка/регистра: %q vs %q", a, b)
	}

	if a == seedSignature([]string{"Aurora"}) {
		t.Fatalf("разный набор затравок должен давать разные подписи")
	}
}
