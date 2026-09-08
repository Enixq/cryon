package main

// Live-контроль доступности источников (задача 38).
//
// Раньше статус источника в боковой панели выводился только из конфигурации
// (есть ключ/токен — «зелёный»), и реальный отказ во время работы никак не был
// виден: поиск молча пропускал упавший источник. Теперь фактические отказы
// (пользовательский поиск и периодический health-check) фиксируются в
// runtimeErrors, отражаются в ListSourceStatus и один раз оповещают
// пользователя — с последующим уведомлением о восстановлении.

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"Cryon2/internal/domain"
	"Cryon2/internal/logging"
	"Cryon2/internal/services/spotify"
	"Cryon2/internal/services/yandex"
)

// Периодичность live-проверки и запрос-зонд. Запрос намеренно нейтральный и
// популярный — он должен что-то находить у любого рабочего источника.
const (
	healthCheckInterval = 5 * time.Minute
	healthCheckDelay    = 20 * time.Second
	healthCheckTimeout  = 12 * time.Second
	healthCheckQuery    = "music"
)

// sourceDisplayName — человекочитаемое имя источника для оповещений.
func sourceDisplayName(id domain.ServiceID) string {
	switch id {
	case domain.ServiceYouTube:
		return "YouTube Music"
	case domain.ServiceSoundCloud:
		return "SoundCloud"
	case domain.ServiceSpotify:
		return "Spotify"
	case domain.ServiceYandex:
		return "Yandex Music"
	case domain.ServiceVK:
		return "VK Музыка"
	case domain.ServiceLocal:
		return "Локальная музыка"
	default:
		return string(id)
	}
}

// configLevels возвращает конфигурационную готовность каждого источника
// (без учёта live-сбоев). Вынесено из ListSourceStatus, чтобы health-check
// мог по нему выбрать, какие источники вообще имеет смысл зондировать.
func (a *App) configLevels() map[domain.ServiceID]string {
	yaConnected := false
	if ya, ok := a.registry[domain.ServiceYandex].(*yandex.Service); ok {
		yaConnected = ya.HasToken()
	}
	localConnected := a.local != nil && len(a.local.All()) > 0

	// Spotify: с валидными ключами доступен официальный API и «Радар новинок»
	// (зелёный). Без ключей — только веб-поиск с внешними ссылками (жёлтый).
	spotifyLevel := sourceLimited
	if sp, ok := a.registry[domain.ServiceSpotify].(*spotify.Service); ok && sp.HasCredentials() {
		spotifyLevel = sourceOK
	}

	yandexLevel := sourceLimited
	if yaConnected {
		yandexLevel = sourceOK
	}
	localLevel := sourceOff
	if localConnected {
		localLevel = sourceOK
	}

	return map[domain.ServiceID]string{
		domain.ServiceYouTube:    sourceOK,
		domain.ServiceSoundCloud: sourceOK,
		domain.ServiceSpotify:    spotifyLevel,
		domain.ServiceYandex:     yandexLevel,
		domain.ServiceLocal:      localLevel,
	}
}

// recordSourceResult фиксирует итог обращения к источнику. Оповещение шлётся
// только на переходе состояния: здоровый → упал (error) и упал → снова работает
// (success). Повторные однотипные результаты статус не трогают, поэтому поиск
// «как есть» (в т.ч. при вводе по буквам) не порождает поток уведомлений.
func (a *App) recordSourceResult(id domain.ServiceID, err error) {
	// Отмена по завершению приложения — не сбой источника, игнорируем.
	if errors.Is(err, context.Canceled) {
		return
	}

	a.statusMu.Lock()
	_, had := a.runtimeErrors[id]
	var transitioned bool
	if err != nil {
		a.runtimeErrors[id] = err.Error()
		transitioned = !had
	} else {
		if had {
			delete(a.runtimeErrors, id)
		}
		transitioned = had
	}
	a.statusMu.Unlock()

	if !transitioned {
		return
	}
	a.emitSourceStatusChanged()
	if err != nil {
		logging.L().Warn("источник стал недоступен", "service", id, "err", err)
		a.pushNotification("error", "Источник недоступен",
			fmt.Sprintf("%s: %v", sourceDisplayName(id), err))
	} else {
		logging.L().Info("источник снова доступен", "service", id)
		a.pushNotification("success", "Источник снова доступен", sourceDisplayName(id))
	}
}

// startHealthChecks запускает фоновый цикл live-проверки источников. Первый
// прогон — с задержкой, чтобы не конкурировать с инициализацией при старте.
// Цикл завершается вместе с контекстом приложения.
func (a *App) startHealthChecks(ctx context.Context) {
	if ctx == nil {
		return
	}
	go func() {
		timer := time.NewTimer(healthCheckDelay)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				a.runHealthChecks(ctx)
				timer.Reset(healthCheckInterval)
			}
		}
	}()
}

// runHealthChecks параллельно зондирует сетевые источники лёгким поиском и
// обновляет их live-статус. Локальную библиотеку (не сеть) и заведомо
// отключённые источники (VK-заглушка) не трогаем.
func (a *App) runHealthChecks(ctx context.Context) {
	levels := a.configLevels()
	var wg sync.WaitGroup
	for id, svc := range a.registry {
		if id == domain.ServiceLocal || levels[id] == sourceOff {
			continue
		}
		wg.Add(1)
		go func(id domain.ServiceID, svc domain.MusicService) {
			defer wg.Done()
			probeCtx, cancel := context.WithTimeout(ctx, healthCheckTimeout)
			defer cancel()
			_, err := svc.Search(probeCtx, healthCheckQuery)
			a.recordSourceResult(id, err)
		}(id, svc)
	}
	wg.Wait()
}
