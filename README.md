# Cryon

**Кроссплатформенный музыкальный плеер-агрегатор**: один интерфейс для нескольких источников музыки — YouTube Music, SoundCloud, Spotify, Яндекс Музыка и локальных файлов.

- **Desktop (Windows)** — нативное приложение на [Wails 2](https://wails.io) (Go + WebView2), собирается в `.exe` с NSIS-установщиком.
- **Android** — порт через Capacitor (дизайн-демо) с планами переноса полного Go-бэкенда через gomobile (см. [plan-android.md](plan-android.md)).

## Возможности

- 🎧 **Мультиисточники** — поиск и воспроизведение из YouTube Music, SoundCloud, Spotify, Яндекс Музыки и локальной библиотеки
- 🎨 **Дизайн-система Cryon** — тёмный «фиолетовый глассморфизм», адаптивная раскладка (десктоп-сайдбар / мобильная нижняя навигация)
- 📱 **10 экранов**: Главная, Поиск, Библиотека, Коллекция, Плейлисты, Умные плейлисты, Избранное, История, Жанры, Настройки
- ▶️ **Полноценный плеер**: очередь, перемотка, эквалайзер, waveform-визуализация, тексты песен, MediaSession API
- 🧠 **Рекомендации** — контентный движок рекомендаций с обратной связью (лайк/дизлайк/скип) на Go
- 💾 **SQLite** — локальное хранилище библиотеки, плейлистов и истории
- 🌐 **Стрим-прокси** — локальный HTTP-сервер на Go для same-origin стриминга аудио

## Архитектура

```
Cryon2/
├── main.go                  # Точка входа Wails (embed frontend/dist)
├── app.go                   # Основные методы, биндятся во фронтенд
├── localserver.go           # HTTP-сервер: стрим-прокси + локальные файлы
├── oauth.go, yandexlogin.go # Авторизация во внешних сервисах
├── internal/
│   ├── domain/              # Модели: трек, артист, плейлист
│   ├── services/            # Адаптеры источников (domain.MusicService)
│   │   ├── youtube/  soundcloud/  spotify/  yandex/  local/
│   │   └── registry.go      # Реестр сервисов
│   ├── playback/            # Контроллер воспроизведения (mpv / HTML5-фолбэк)
│   ├── recommendations/     # Движок рекомендаций
│   ├── store/sqlite/        # SQLite-хранилище
│   └── config/, logging/, trackmeta/, websearch/
└── frontend/
    ├── src/
    │   ├── pages/           # Экраны приложения
    │   ├── widgets/         # Layout, плеер, навигация
    │   ├── shared/          # API-клиент, UI-примитивы, утилиты
    │   └── store/           # Zustand-сторы (плеер, UI, эквалайзер)
    └── android/             # Capacitor-обёртка для Android
```

**Ключевая идея фронтенда**: `shared/api/client.ts` прячет весь бэкенд за функциями с проверкой `isWailsRuntime()` и мок-фолбэком — экраны не знают, откуда приходят данные. Это позволяет запускать UI на моках (Vite dev) и легко добавлять новые транспорты (HTTP для Android).

## Технологии

| Слой | Стек |
|---|---|
| Desktop shell | Wails 2 (Go + WebView2) |
| Backend | Go 1.26, SQLite (modernc.org/sqlite, без CGO) |
| Frontend | React 19, TypeScript, Vite 7 |
| UI | Tailwind CSS, Lucide Icons |
| State | Zustand, TanStack Query |
| Тесты | Vitest + RTL, Playwright (e2e), go test |
| Android | Capacitor 7 |

## Запуск

### Требования

- **Go** ≥ 1.26
- **Node.js** ≥ 20
- **Wails CLI**: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- Для Windows: [WebView2 Runtime](https://developer.microsoft.com/en-us/microsoft-edge/webview2/)

### Режим разработки (только фронтенд, на моках)

```bash
cd frontend
npm install
npm run dev          # Vite dev-сервер
```

### Desktop-приложение (Wails)

```bash
wails dev            # dev-режим с горячей перезагрузкой
wails build          # сборка Cryon2.exe
```

Готовый билд: `build/bin/Cryon2.exe` (+ NSIS-установщик).

### Android (дизайн-демо APK)

```bash
cd frontend
npm run build
npx cap sync
cd android && ./gradlew assembleDebug
```

APK: `frontend/android/app/build/outputs/apk/debug/app-debug.apk`

> Требуется JDK 21 и Android SDK. Полный план порта с Go-бэкендом — в [plan-android.md](plan-android.md).

### Тесты

```bash
go test ./...                        # Go
cd frontend && npm run test          # Vitest (unit)
cd frontend && npm run test:e2e      # Playwright (e2e)
```

## Статус проекта

- [x] Полный фронтенд (10 экранов, дизайн-система, плеер)
- [x] Go-бэкенд: адаптеры источников, SQLite, рекомендации, стрим-прокси
- [x] Desktop-сборка Windows (Wails + NSIS)
- [x] Фаза 0 Android: дизайн-демо APK через Capacitor
- [ ] Фазы 1–5 Android: gomobile-бэкенд, MediaSession, публикация

Подробные планы и журнал разработки — в [plan.md](plan.md) и [plan-android.md](plan-android.md).

## Лицензия

Проект для личного использования.
