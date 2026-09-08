package services

import (
	"Cryon2/internal/config"
	"Cryon2/internal/domain"
	"Cryon2/internal/services/local"
	"Cryon2/internal/services/soundcloud"
	"Cryon2/internal/services/spotify"
	"Cryon2/internal/services/yandex"
	"Cryon2/internal/services/youtube"
)

// BuildRegistry собирает набор адаптеров источников. Рабочие для поиска —
// YouTube, SoundCloud, Yandex Music и локальная музыка (Yandex даёт реальный
// поиск при наличии токена, иначе веб-поиск). Spotify зарегистрирован ради
// OAuth-импорта Liked Songs и «Радара новинок», но в поиске не участвует
// (прямого потока нет, только внешние ссылки). VK убран полностью — заглушка
// лишь засоряла выдачу.
//
// Возвращает также ссылку на локальный адаптер: он нужен App напрямую
// для сканирования папок, отдачи обложек и путей к файлам.
func BuildRegistry(cfg config.Config) (map[domain.ServiceID]domain.MusicService, *local.Service) {
	localSvc := local.New()
	list := []domain.MusicService{
		youtube.New(cfg.YouTubeAPIKey),
		soundcloud.New(cfg.SoundCloudClient),
		spotify.New(cfg.SpotifyClientID, cfg.SpotifyClientSecret),
		yandex.New(cfg.YandexToken),
		localSvc,
	}
	registry := make(map[domain.ServiceID]domain.MusicService, len(list))
	for _, svc := range list {
		registry[svc.ID()] = svc
	}
	return registry, localSvc
}
