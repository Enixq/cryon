package services

import (
	"testing"

	"Cryon2/internal/config"
	"Cryon2/internal/domain"
)

func TestBuildRegistryCoversAllSources(t *testing.T) {
	registry, local := BuildRegistry(config.Config{})
	if local == nil {
		t.Fatal("локальный адаптер не должен быть nil")
	}

	// VK убран из реестра (заглушка засоряла выдачу). Spotify остаётся ради
	// OAuth-импорта и «Радара новинок», хотя в поиске уже не участвует.
	want := []domain.ServiceID{
		domain.ServiceYouTube,
		domain.ServiceSoundCloud,
		domain.ServiceSpotify,
		domain.ServiceYandex,
		domain.ServiceLocal,
	}
	for _, id := range want {
		svc, ok := registry[id]
		if !ok {
			t.Errorf("в реестре нет источника %q", id)
			continue
		}
		if svc.ID() != id {
			t.Errorf("источник %q сообщает неверный ID %q", id, svc.ID())
		}
	}
	if len(registry) != len(want) {
		t.Errorf("в реестре %d источников, ожидалось %d", len(registry), len(want))
	}
}
