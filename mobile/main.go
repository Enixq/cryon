//go:build android

// Файл намеренно почти пуст. gomobile bind собирает пакет mobile как
// БИБЛИОТЕКУ (JNI .so + Java-обёртка) и сам генерирует package main с
// cgo-экспортами — отдельная main-функция биндимому пакету НЕ нужна (пустой
// main требует gomobile *build*, а не bind). Прежняя версия файла держала
// `import "C"` и `func main(){}` с неверным комментарием: импорт cgo в
// биндимом пакете ломает парсер gobind (go/packages не резолвит псевдопакет
// "C") — из-за этого падала CI-сборка `gomobile bind`.
//
// Вся публичная поверхность мобильного бэкенда — в mobile.go (Start/Stop).
// Этот файл можно безопасно удалить целиком (`git rm mobile/main.go`).
package mobile
