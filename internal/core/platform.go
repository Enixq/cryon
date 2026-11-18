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

// eventMirroringHost keeps the legacy platform-specific event channel and the
// public API v1 event stream in sync while clients migrate between them.
type eventMirroringHost struct {
	target platformHost
	events *apiEventHub
}

func newEventMirroringHost(target platformHost, events *apiEventHub) platformHost {
	return &eventMirroringHost{target: target, events: events}
}

func (h *eventMirroringHost) Emit(event string, data ...interface{}) {
	if h.events != nil {
		h.events.publish(event, data...)
	}
	if h.target != nil {
		h.target.Emit(event, data...)
	}
}

func (h *eventMirroringHost) OpenURL(url string) {
	if h.target != nil {
		h.target.OpenURL(url)
	}
}

func (h *eventMirroringHost) PickFile(title, filterName, filterPattern string) (string, error) {
	if h.target == nil {
		return "", nil
	}
	return h.target.PickFile(title, filterName, filterPattern)
}

func (h *eventMirroringHost) PickDirectory(title string) (string, error) {
	if h.target == nil {
		return "", nil
	}
	return h.target.PickDirectory(title)
}
