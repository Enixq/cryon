//go:build android

package mobile

import "C"

// Пустая main-функция обязательна для gomobile bind: он компилирует пакет как
// библиотеку, но Go-линковщик требует наличие main в сборочной единице.
func main() {}
