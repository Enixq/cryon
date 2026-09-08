// Package trackmeta разбирает «сырые» названия треков из пользовательских
// загрузок (YouTube, SoundCloud) в исполнителя и чистое название.
//
// Зачем отдельный пакет: и на YouTube, и на SoundCloud подавляющая часть
// результатов по запросу артиста — это ПЕРЕЗАЛИВЫ. Автором загрузки числится
// случайный аккаунт («GAZ LIVE», «Mirarion», «N1P3NDO»), а настоящий исполнитель
// спрятан в заголовке вида «Баста - 8800». Если брать исполнителя из аккаунта,
// в выдаче появляются мусорные «артисты», ни один из которых не совпадает с
// запросом, — именно на это жаловался пользователь. Логика общая для обоих
// адаптеров, поэтому живёт в одном месте.
package trackmeta

import (
	"regexp"
	"strings"
)

// Separators — разделители «исполнитель/название» в порядке предпочтения.
// И на YouTube, и на SoundCloud заголовок почти всегда имеет вид
// «Исполнитель - Название».
var Separators = []string{" - ", " – ", " — ", " · "}

// SplitArtistTitle разбирает заголовок на исполнителя и название. Пустой artist
// означает «разобрать не удалось» — тогда исполнителя лучше не выдумывать:
// фронтенд покажет «Неизвестный исполнитель», а профиль вкусов не получит
// фиктивного артиста.
func SplitArtistTitle(raw string) (artist, title string) {
	title = strings.TrimSpace(raw)
	for _, sep := range Separators {
		idx := strings.Index(title, sep)
		if idx <= 0 {
			continue
		}
		left := strings.TrimSpace(title[:idx])
		right := strings.TrimSpace(title[idx+len(sep):])
		if left == "" || right == "" {
			continue
		}
		// Отсекаем случаи, когда слева не имя, а мусор («prod», «official»):
		// имя исполнителя коротко и не состоит из «украшений».
		if isDecorationOnly(left) {
			continue
		}
		return left, right
	}
	return "", title
}

// decorationWords — слова-«украшения» из заголовков загрузок. Скобочная группа
// удаляется из названия ТОЛЬКО если состоит целиком из таких слов: так
// «(Official Video)», «(Lyric Video)», «(HD)», «(Клип)» уходят, а значимые
// пометки — «(Slowed + Reverb)», «(prod. Metro)», «(Live)», «(Remix)» —
// сохраняются, ведь они меняют саму версию трека и важны для совпадения.
var decorationWords = map[string]bool{
	"official": true, "officiel": true, "video": true, "audio": true,
	"lyric": true, "lyrics": true, "visualizer": true, "visualiser": true,
	"visual": true, "hd": true, "hq": true, "uhd": true, "4k": true, "8k": true,
	"mv": true, "clip": true, "music": true, "mood": true, "premiere": true,
	"официальное": true, "официальный": true, "официальная": true, "премьера": true,
	"клип": true, "видео": true, "лирика": true, "лирик": true, "текст": true,
	"песни": true, "аудио": true, "со": true, "словами": true,
	// связки, встречающиеся рядом с «украшениями»
	"the": true, "a": true, "and": true, "with": true, "of": true,
}

var (
	bracketGroupRe = regexp.MustCompile(`\s*[\(\[][^()\[\]]*[\)\]]`)
	tokenRe        = regexp.MustCompile(`[^\p{L}\p{N}]+`)
)

// isDecorationOnly сообщает, что строка состоит только из слов-«украшений».
func isDecorationOnly(s string) bool {
	hasToken := false
	for _, tok := range tokenRe.Split(strings.ToLower(s), -1) {
		if tok == "" {
			continue
		}
		hasToken = true
		if !decorationWords[tok] {
			return false
		}
	}
	return hasToken
}

// CleanTitle убирает из названия трека чисто оформительские скобочные хвосты
// («(Official Video)», «[Official Audio]», «(Lyric Video)», «(HD)», «(Клип)»),
// не трогая значимые пометки версии. Чистит и отображение, и совпадение при
// поиске: в заголовке остаётся суть трека.
func CleanTitle(raw string) string {
	title := bracketGroupRe.ReplaceAllStringFunc(strings.TrimSpace(raw), func(group string) string {
		if isDecorationOnly(strings.Trim(group, " ()[]")) {
			return ""
		}
		return group // есть значимое слово — группу оставляем целиком
	})
	return strings.TrimSpace(title)
}
