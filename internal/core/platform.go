package core

// platformHost абстрагирует зависящие от среды побочные эффекты — доставку
// событий во фронтенд, открытие системного браузера и нативные диалоги выбора
// файла/папки. Благодаря ей основная логика App НЕ зависит напрямую от
// Wails-рантайма.
//
// Зачем. Десктоп внедряет реализацию поверх Wails (см. platform_wails.go),
// а Android-сборка на устройстве (gomobile) внедряет SSE/no-op-реализацию:
// gomobile не умеет биндить `package main` и не линкует Wails-рантайм, поэтому
// весь код, вызывающий `runtime.*`, вынесен за этот интерфейс.
type platformHost interface {
	// Emit доставляет событие во фронтенд. Десктоп — Wails EventsEmit; мобайл —
	// поток Server-Sent Events. Без данных data пуст — как одноимённое событие.
	Emit(event string, data ...interface{})
	// OpenURL открывает ссылку во внешнем браузере устройства.
	OpenURL(url string)
	// PickFile показывает системный выбор файла и возвращает путь; "" — отмена.
	// filterName/filterPattern задают единственный фильтр (пусто — без фильтра).
	// На мобайле нативного диалога нет → "" без ошибки.
	PickFile(title, filterName, filterPattern string) (string, error)
	// PickDirectory показывает системный выбор папки; "" — отмена. Мобайл → "".
	PickDirectory(title string) (string, error)
}

// nullHost — безопасная заглушка platformHost. Используется по умолчанию (до
// внедрения реального хоста, в юнит-тестах и в средах без UI): события уходят
// «в никуда», диалоги возвращают пустой путь. Никогда не паникует.
type nullHost struct{}

func (nullHost) Emit(string, ...interface{})             {}
func (nullHost) OpenURL(string)                          {}
func (nullHost) PickFile(_, _, _ string) (string, error) { return "", nil }
func (nullHost) PickDirectory(string) (string, error)    { return "", nil }
