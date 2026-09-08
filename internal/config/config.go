package config

import "os"

// Config — настройки интеграций. Ключи и токены необязательны:
// адаптеры умеют работать без них (через веб-поиск и парсинг).
type Config struct {
	SpotifyClientID     string
	SpotifyClientSecret string
	SpotifyRedirect     string
	YouTubeAPIKey       string
	SoundCloudClient    string
	// YandexToken — OAuth-токен Yandex Music. При наличии адаптер выполняет
	// реальный поиск и разрешает прямой аудиопоток; без него — веб-поиск.
	YandexToken string
	// MPVPath — путь к mpv.exe. Если пусто, ищется в стандартных местах и PATH.
	MPVPath string
	// LastFMAPIKey — бесплатный ключ Last.fm API для рекомендаций (похожие
	// артисты/треки). Без него движок рекомендаций работает в оффлайн-режиме
	// поверх локальной истории и избранного.
	LastFMAPIKey string
}

// Load читает конфигурацию из переменных окружения.
// Для личного использования этого достаточно; при отсутствии значений
// адаптеры переходят в режим веб-поиска без ключей.
func Load() Config {
	return Config{
		SpotifyClientID:     os.Getenv("CRYON_SPOTIFY_CLIENT_ID"),
		SpotifyClientSecret: os.Getenv("CRYON_SPOTIFY_CLIENT_SECRET"),
		SpotifyRedirect:     os.Getenv("CRYON_SPOTIFY_REDIRECT"),
		YouTubeAPIKey:       os.Getenv("CRYON_YOUTUBE_API_KEY"),
		SoundCloudClient:    os.Getenv("CRYON_SOUNDCLOUD_CLIENT_ID"),
		YandexToken:         os.Getenv("CRYON_YANDEX_TOKEN"),
		MPVPath:             os.Getenv("CRYON_MPV_PATH"),
		LastFMAPIKey:        os.Getenv("CRYON_LASTFM_API_KEY"),
	}
}
