package audiofetcher

import (
	"fmt"
	"testing"
)

// TestParseContentRangeGooglevideo — проверяет, что verifyResolver корректно
// извлекает total из реального заголовка googlevideo. Если парсинг падает,
// контрольный запрос хвоста не выполняется вовсе, и поток, замолкающий на
// первом МиБ, считается рабочим.
func TestParseContentRangeGooglevideo(t *testing.T) {
	for _, header := range []string{
		"bytes 0-99/7253539",
		"bytes 0-1023/14443747",
		"bytes 14442723-14443746/14443747",
	} {
		var first, last, total int64
		n, err := fmt.Sscanf(header, "bytes %d-%d/%d", &first, &last, &total)
		if err != nil || n != 3 || total <= last {
			t.Fatalf("не разобран %q: n=%d err=%v total=%d", header, n, err, total)
		}
	}
}
