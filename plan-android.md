# План: порт Cryon на Android

> Отдельный файл под порт (по просьбе пользователя). Общий журнал доработок —
> в [plan.md](plan.md). Реплики и комментарии — по-русски.

## 1. Цель

Перенести Cryon (десктопный агрегатор музыки на Wails 2) на Android так, чтобы:
- на выходе был устанавливаемый **.apk** для теста на телефоне;
- дизайн соответствовал присланному макету (тёмный «фиолетовый глассморфизм»,
  10 экранов, нижняя навигация из 5 пунктов, постоянный мини-плеер);
- по максимуму переиспользовать уже написанное (Go-бэкенд + React-фронтенд), а не
  переписывать приложение с нуля.

## 2. Ключевое ограничение (почему это не «просто пересобрать»)

**Wails 2 не поддерживает Android.** Его рантайм — нативный webview рабочего стола
(на Windows это WebView2) плюс биндинги Go↔JS через `window.go.main.App`. На
Android этого моста нет. Значит:
- нельзя просто «собрать под Android» текущий `main` пакет;
- нужен другой способ (а) исполнять Go-бэкенд на устройстве и (б) связать его с
  фронтендом.

Хорошая новость — фронтенд к этому уже готов архитектурно:
- [frontend/src/shared/api/client.ts](frontend/src/shared/api/client.ts) прячет
  весь бэкенд за функциями с проверкой `isWailsRuntime()` и мок-фолбэком. Достаточно
  добавить **третий транспорт** (HTTP к локальному серверу), не трогая экраны.
- [frontend/src/widgets/AppLayout.tsx](frontend/src/widgets/AppLayout.tsx) уже
  адаптивен (`useMediaQuery`, узкий режим: сайдбар-рельс + выдвижная панель
  «Сейчас играет»). Все 10 экранов из макета уже существуют как роуты (см. §7).
- Движок звука [audioEngine.ts](frontend/src/store/audioEngine.ts) умеет работать
  через HTML5 `<audio>` + внутренний стрим-прокси
  ([localserver.go](localserver.go)) — ровно то, что нужно на Android (mpv там нет).

## 3. Обзор вариантов архитектуры

| Вариант | Переиспользование | Реальный бэкенд | Риск/объём | Итог |
|---|---|---|---|---|
| **A. Встроенный Go-сервер + WebView (gomobile)** | Максимум (весь Go + весь React) | Да | Средний | **Выбран** |
| B. Capacitor + существующий фронтенд на моках | Только фронтенд | Нет (моки) | Низкий | Фаза 0 (демо дизайна) |
| C. Нативный Kotlin/Compose с нуля | Ничего | Да (заново) | Очень высокий | Отклонён |
| D. Wails 3 (экспериментальный Android) | Средне | Да | Высокий (сырой) | Отклонён (нестабилен) |

**Почему A.** Go-бэкенд Cryon (поиск по источникам, резолв потоков, стрим-прокси,
SQLite-библиотека) — зрелый и уже HTTP-ориентированный
([localserver.go](localserver.go) отдаёт стрим-прокси и локальные файлы по HTTP).
Если поднять его как **локальный HTTP-сервер на loopback внутри приложения** и
скормить фронтенду по `http://127.0.0.1:<порт>/`, то:
- стрим-прокси остаётся same-origin (как на десктопе) → перемотка/эквалайзер
  работают без переписывания;
- фронтенд грузится тем же сервером → один origin, никаких CORS;
- почти весь Go переиспользуется как есть.

**Почему не B как основной.** Capacitor быстро даёт APK, но без Go-бэкенда это лишь
UI на моках — «пощупать дизайн» да, слушать музыку нет. Оставляем как **Фаза 0**:
самый быстрый способ увидеть дизайн на реальном телефоне, пока делается A.

## 4. Выбранная архитектура (детально)

```
┌─────────────────────────── Android APK ───────────────────────────┐
│  MainActivity (Kotlin)                                             │
│   ├─ WebView (полноэкранный, JS вкл., media autoplay)             │
│   │    └─ грузит http://127.0.0.1:<port>/  (React-фронтенд)       │
│   ├─ ForegroundService + MediaSession (фон + медиа-кнопки)        │
│   └─ Mobile.start(filesDir) → возвращает базовый URL              │
│                                                                    │
│  cryonmobile.aar  (gomobile bind, только start/stop наружу)       │
│   └─ internal/core.App  (весь бэкенд Cryon)                       │
│        ├─ HTTP: //go:embed frontend/dist  → отдаёт SPA           │
│        ├─ HTTP: /api/*  → REST/RPC-мост поверх методов App        │
│        └─ HTTP: /stream/*, /local/*  → существующий localserver   │
└────────────────────────────────────────────────────────────────────┘
```

- **Мост Go↔JS.** Вместо десктопных Wails-биндингов — тонкий REST/RPC-слой. Чтобы
  не писать по эндпоинту на каждый из ~30 методов `App`, делаем один
  `POST /api/call {method, args[]}` с диспетчеризацией через рефлексию
  (белый список методов) → JSON-ответ. Фронтенд получает `rpcCall(method, ...args)`.
- **События.** Десктопные `runtime.EventsEmit` (`yandex:connected`,
  `source:status`, `notifications:changed`) на Android заменяются на **SSE**
  (`GET /api/events`, поток `text/event-stream`); фронтенд-подписки
  (`onYandexConnected` и др. в client.ts) переводятся на `EventSource`.

## 5. Что нужно установить (toolchain)

Это НЕ входит в текущую среду и НЕ может быть собрано здесь (см. §10). Для сборки
APK на машине пользователя:

- **JDK 17** (Temurin/OpenJDK).
- **Android SDK + NDK** (Android Studio → SDK Manager; NDK нужен для gomobile).
  `ANDROID_HOME`/`ANDROID_NDK_HOME` в окружении.
- **Go 1.26** (уже есть) + `gomobile`:
  `go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init`.
- **Node 20+** (для сборки фронтенда, уже есть).
- Для Фазы 0 дополнительно: **Capacitor CLI** (`@capacitor/cli`).

## 6. Фазы работ

> **Стратегия (по решению пользователя): как с десктопом — сначала ПОЛНОСТЬЮ
> фронтенд, потом прикручиваем бэкенд.** Мобильный UI строится и полируется на
> едином с десктопом React-коде (адаптив по брейкпоинтам), работает на
> мок-данных/существующей абстракции `client.ts`, пакуется в APK через Capacitor
> для теста на телефоне. Только когда фронт готов — фазы встраивания Go-бэкенда.

### Фаза F — Полный мобильный фронтенд (СНАЧАЛА) 🟢 текущий фокус
Единый кодовый фронт: на ширине телефона рендерится первоклассный мобильный UI,
на десктопе — прежний. Всё на существующем `client.ts` (моки/бэкенд-абстракция),
без изменений контракта бэкенда.
- [x] **Дизайн-токены** по брифу (§8): фоны `#08090D/#0D0F15/#11131B`, акцент,
      радиусы 16/20/28, safe-area — в CSS-переменных [styles.css](frontend/src/styles.css)
      (`--bg-0/1/2`, `--accent`, `--safe-*`) + токены Tailwind
      [tailwind.config.js](frontend/tailwind.config.js) (`surface`, `neon`, радиусы
      `card/cryon/pill` — без переопределения дефолтных `rounded-*`).
- [x] **Мобильный каркас** в [AppLayout](frontend/src/widgets/AppLayout.tsx):
      ветка `isPhone` (брейкпоинт `phone` в
      [useMediaQuery](frontend/src/shared/lib/useMediaQuery.ts)) заменяет
      сайдбар+топбар на нижнюю навигацию из 5 пунктов
      ([BottomNav](frontend/src/widgets/BottomNav.tsx):
      Главная/Поиск/Библиотека/Сервисы/Профиль) + safe-area insets. Отдельный
      мобильный хедер не вводился намеренно — каждая страница уже рисует свой
      [PageHeader](frontend/src/shared/ui/PageHeader.tsx) (иначе был бы двойной
      заголовок).
- [x] **Постоянный мини-плеер** над нижней навигацией
      ([MobileMiniPlayer](frontend/src/widgets/MobileMiniPlayer.tsx)): обложка +
      мета, play/pause, «следующий», тонкая полоса прогресса, тап → полный плеер.
      Скрыт, когда ничего не играет.
- [x] **Now Playing на весь экран** ([MobileNowPlaying](frontend/src/widgets/MobileNowPlaying.tsx)):
      крупная обложка, перемотка слайдером, полный транспорт, лайк, эквалайзер;
      очередь — переключаемая вкладка (кнопка-список). Анимация «снизу вверх»,
      сворачивание шевроном. Переиспользует Slider/Cover/formatDuration/
      usePlayerStore/useCoverPalette.
- [x] **Экран «Сервисы»** ([ServicesPage](frontend/src/pages/ServicesPage.tsx)) как
      отдельное назначение нижней навигации: карточки источников со статусом
      (listSourceStatus + градации ok/limited/off), тап → настройки. **Профиль**
      пока ведёт в существующую [SettingsPage](frontend/src/pages/SettingsPage.tsx)
      (отдельный экран профиля — позже, если понадобится).
- [x] Тач-эргономика: тап-таргеты ≥44px и `active:`-состояния сделаны, подсветка
      тапов убрана (`.no-tap-highlight`); **свайп вниз закрывает полноэкранный
      плеер** (2026-09-08, [MobileNowPlaying](frontend/src/widgets/MobileNowPlaying.tsx)):
      обработчики `onTouchStart/Move/End`, направление определяется по первому
      смещению (вниз и `dy>|dx|`), лист следует за пальцем через `translateY`,
      закрытие при протяжке > 100px, иначе плавный откат; активно только в режиме
      обложки (в очереди вертикаль — прокрутка), слайдер и транспорт помечены
      `data-noswipe`; вверху добавлена «ручка»-подсказка. **Свайп-влево удаляет
      трек из очереди** (2026-09-08): красная подложка «Убрать», порог 80px,
      активную строку не свайпаем, кнопка ✕ — запасной путь.
- [x] Косметика всех 10 экранов (§7) под макет (адаптив контента внутри страниц
      на ширине < 640px — сетки/типографика). **Готово (2026-09-08):** два общих
      приёма — (1) витринные сетки `grid-cols-3` → `grid-cols-2 gap-4
      sm:grid-cols-4 sm:gap-5 xl:…` (2 карточки в ряд на 360–390px вместо тесных
      3); (2) шапки детальных страниц `flex items-end gap-6` → `flex flex-col
      items-center gap-4 text-center sm:flex-row sm:items-end sm:gap-6
      sm:text-left`, обложка `h-40 w-40 sm:h-48 sm:w-48`, заголовок `text-3xl
      sm:text-5xl` + `max-w-full break-words`, мета/действия `justify-center
      sm:justify-start`. Применено на Home/Library/Album/Search/Artist/Genre/
      Favorites/LocalMusic/PlaylistDetail/SmartPlaylist; общий `PageHeader`
      получил `text-2xl sm:text-3xl`. Экраны Services/MobileNowPlaying/Settings/
      EqualizerModal сверстаны сразу под телефон (тап-таргеты ≥44px).
      **Проверено — зелёное (2026-09-08, локальный прогон):** tsc (чисто),
      vitest 46/46, vite build OK; backend go build/vet/test — все пакеты ok
      (понадобился `go mod tidy` для go.sum).
- [~] Упаковка в APK через Capacitor (§9, Фаза 0-команды) — тест дизайна на телефоне.

### Фаза 0 — Дизайн-демо APK (Capacitor) 🟢 упаковка Фазы F
Цель: устанавливаемый APK для оценки дизайна/UX на телефоне. Реального
воспроизведения нет — фронтенд идёт по мок-фолбэку
(`isWailsRuntime()===false`).
- [x] Добавить Capacitor во фронтенд ([capacitor.config.ts](frontend/capacitor.config.ts):
      `appId: com.cryon.app`, `appName: Cryon`, `webDir: dist`, `androidScheme: https`).
- [x] `vite build` → `npx cap add android` → `npx cap sync` (2026-09-08): проект
      `frontend/android/` создан, `dist/` собран и скопирован в assets.
- [~] Иконка/сплеш/название «Cryon», тема статус-бара под фон `#08090D`.
      **Сделано (2026-09-08, нативно, без npm/cap sync):** название уже `Cryon`
      (`strings.xml`); добавлен [colors.xml](frontend/android/app/src/main/res/values/colors.xml)
      (`cryonBg = #FF08090D`); в [styles.xml](frontend/android/app/src/main/res/values/styles.xml)
      обеим темам (`NoActionBar`, `NoActionBarLaunch`) прописан тёмный
      `windowBackground` + `statusBarColor` + `navigationBarColor` — **белой
      вспышки** между сплэшем и WebView больше нет. Веб-сторона тоже затемнена:
      `html { background: var(--bg-0) }` в [styles.css](frontend/src/styles.css)
      (overscroll не мелькает белым). Эти правки в `res/values/*` **переживают
      `cap sync`** (он копирует только `dist/` в assets). **Осталось:** сам
      `splash.png` — дефолтный (белый с лого); перекрасить бинарь вслепую нельзя.
      Идиоматичный путь для кастомного сплэша/иконки — плагин
      `@capacitor/splash-screen` + `@capacitor/assets` (когда вернётся npm-гейт).
      Опционально: `@capacitor/status-bar` для управления из JS (нативная тема
      уже покрывает базовый цвет).
- [ ] `./gradlew assembleDebug` → `app-debug.apk` — **нужен Android SDK + JDK 17**
      (в этой среде их нет; собираешь у себя). Каркас `android/` уже пытались
      собрать (есть `build/intermediates/`), но APK в `outputs/` пока нет.
- **Итог:** кликабельный APK с дизайном и мок-данными.

### Фаза 1 — Рефактор ядра (вынести из `package main`)
gomobile не биндит `package main`. Выносим бэкенд в импортируемый пакет.
- [ ] Создать `internal/core`: перенести туда `App`, реестр источников
      ([registry.go](internal/services/registry.go) уже в `internal/services`),
      health, wiring из [main.go](main.go)/[app.go](app.go). `main` и `mobile`
      оба импортируют `internal/core`.
- [ ] `localAssetHandler` ([localserver.go](localserver.go)) переносится вместе с
      ядром (он уже почти автономен, зависит только от `*App` и `local`).
- [ ] Десктопный `main` продолжает работать через `core` — проверить, что Wails-
      сборка не сломалась.

### Фаза 2 — Встроенный бэкенд-сервер + gomobile-обёртка
- [ ] Пакет `mobile` с bind-совместимой поверхностью:
      `func Start(dataDir string) (baseURL string, err error)` и `func Stop()`.
- [ ] Внутри `Start`: инициализировать `core.App` с `dataDir`
      (Android `context.getFilesDir()`), собрать `http.ServeMux`:
      - `//go:embed frontend/dist/*` → отдача SPA (index.html + ассеты);
      - существующий `localAssetHandler` на `/stream/` и `/local/`;
      - `/api/call` (RPC-мост), `/api/events` (SSE).
      Слушать `127.0.0.1:0` (случайный порт), вернуть `http://addr`.
- [ ] `gomobile bind -target=android -o cryonmobile.aar ./mobile`.

### Фаза 3 — HTTP-транспорт во фронтенде
- [ ] В [client.ts](frontend/src/shared/api/client.ts) добавить третий режим:
      если задан `window.__CRYON_BASE__` (инъектируется WebView'ом) — использовать
      `rpcCall` через `fetch(baseURL + "/api/call")` вместо Wails-биндингов.
- [ ] Порядок выбора транспорта: Wails → HTTP(loopback) → моки.
- [ ] События: `onYandexConnected`/`onSourceStatusChanged`/`onNotificationsChanged`
      перевести на `EventSource(baseURL + "/api/events")` в HTTP-режиме.
- [ ] Аудио: убедиться, что `audioEngine` в этом режиме идёт по HTML5-`<audio>`-
      ветке через `/stream/…` (mpv-ветка только десктоп).

### Фаза 4 — Android-оболочка (Kotlin) и сборка APK
- [ ] `MainActivity`: полноэкранный `WebView` (`javaScriptEnabled`,
      `mediaPlaybackRequiresUserGesture=false`, `domStorageEnabled`), вызов
      `Mobile.start(filesDir.path)`, `loadUrl(baseURL)`, проброс `__CRYON_BASE__`.
- [ ] `ForegroundService` + `MediaSessionCompat` для фонового воспроизведения и
      медиа-кнопок (аналог десктопного [useMediaSession](frontend/src/shared/lib/useMediaSession.ts)).
- [ ] `AndroidManifest`: `INTERNET`, `FOREGROUND_SERVICE`(+`_MEDIA_PLAYBACK`),
      `POST_NOTIFICATIONS`, `READ_MEDIA_AUDIO` (для локальной музыки), `WAKE_LOCK`.
- [ ] Gradle подключает `cryonmobile.aar`; `assembleDebug` → APK.

### Фаза 5 — Мобильный UI по макету
- [ ] **Нижняя навигация из 5 пунктов** (Главная/Поиск/Библиотека/Сервисы/Профиль)
      как отдельный мобильный каркас (сейчас — сайдбар/рельс из
      [Sidebar.tsx](frontend/src/widgets/Sidebar.tsx)); включать по
      `useMediaQuery`, реюзая существующие пороги.
- [ ] Постоянный мини-плеер над нижней навигацией
      ([PlayerBar.tsx](frontend/src/widgets/PlayerBar.tsx) → мобильная версия).
- [ ] Экран Now Playing на весь экран (существующая
      [NowPlayingPanel.tsx](frontend/src/widgets/NowPlayingPanel.tsx) → full-screen).
- [ ] Safe-area insets (`env(safe-area-inset-*)`), тач-таргеты ≥44px, свайпы.
- [ ] Применить дизайн-систему §8 (токены цветов/радиусов/типографики).

## 7. Экраны макета → существующие роуты

| Экран макета | Роут/компонент | Статус |
|---|---|---|
| Home / Главная | `/` [HomePage](frontend/src/pages/HomePage.tsx) | есть |
| Search / Поиск | `/search` [SearchPage](frontend/src/pages/SearchPage.tsx) | есть |
| Now Playing | [NowPlayingPanel](frontend/src/widgets/NowPlayingPanel.tsx) | есть (панель → full-screen) |
| Queue / Очередь | часть NowPlaying/PlayerBar | есть (вынести таб) |
| Library / Библиотека | `/library` [LibraryPage](frontend/src/pages/LibraryPage.tsx) | есть |
| Services / Сервисы | `/settings` [SettingsPage](frontend/src/pages/SettingsPage.tsx) (раздел источников) | есть (выделить) |
| For You / Discover | `/` секции Home (recommendations/autoMix/newReleases) | есть |
| Album | `/albums/:id` [AlbumDetailPage](frontend/src/pages/AlbumDetailPage.tsx) | есть |
| Profile / Settings | `/settings` [SettingsPage](frontend/src/pages/SettingsPage.tsx) | есть |
| Equalizer | [EqualizerModal](frontend/src/widgets/EqualizerModal.tsx) | есть |

Вывод: **новые экраны рисовать почти не нужно** — порт про доставку бэкенда на
устройство + мобильный каркас (нижняя навигация) + косметику под макет.

## 8. Дизайн-система (из брифа «CRYON DESIGN SYSTEM»)

- **Фоны:** `#08090D` (основной), `#0D0F15`, `#11131B` (поверхности). Текущий
  фронтенд уже использует близкий `#07080f`/`#090b14` — свести к токенам макета.
- **Акцент:** электрический фиолет/пурпур (текущие `#a78bfa`/`#a855f7`). Фиолет —
  для интеракции/фокуса/активных состояний, НЕ доминирует в фоне.
- **Радиусы:** 16px (стандарт), 20px (крупные карточки), 28px (круглые контролы).
- **Типографика:** Inter / SF Pro / Google Sans; сетка 8pt.
- **Поверхности:** тёмное стекло (glass), 1px тонкие бордеры, мягкие тени.
- Амбиентный фон уже «дышит» палитрой обложки (`--app-accent` в AppLayout) — это
  совпадает с духом макета, сохраняем.

Реализация — через CSS-переменные/токены Tailwind (theme extend), чтобы разом
привести десктоп и мобайл к одной палитре.

## 9. Сборка APK (команды — запускать у себя, см. §10)

```bash
# Фаза 0 (демо дизайна):
cd frontend
npm i -D @capacitor/cli @capacitor/core @capacitor/android
npm run build
npx cap add android && npx cap sync android
cd android && ./gradlew assembleDebug
# → frontend/android/app/build/outputs/apk/debug/app-debug.apk

# Полный порт (после фаз 1–4):
gomobile bind -target=android -androidapi 24 -o android/app/libs/cryonmobile.aar ./mobile
cd android && ./gradlew assembleDebug
```

## 10. Ограничения этой среды (честно)

- **APK здесь собрать нельзя.** Нет Android SDK/NDK/JDK/Gradle; gomobile не
  установлен. Готовый `.apk` появляется только после установки toolchain (§5) и
  запуска команд §9 на машине пользователя (или в этой среде, когда добавят
  инструменты).
- **Гейт исполнения команд сейчас недоступен** (сбой классификатора auto-режима):
  `go build`/`vet`/`test`, npm/npx/gradle заблокированы. Read-only операции
  (чтение/поиск/правки файлов) работают. Поэтому здесь готовится код и скрипты, а
  сборка/прогон — как только гейт вернётся и появится toolchain.
- Живого Android-устройства/эмулятора в среде нет — визуальная проверка на стороне
  пользователя.

## 11. Риски

- **gomobile-биндинг `package main`** — обязателен рефактор в `internal/core`
  (Фаза 1). Риск задеть десктопную сборку → проверять Wails-сборку после.
- **Рефлексивный RPC-мост** — следить за сериализацией доменных типов
  (`[]domain.Track` и т.п.) в JSON; белый список методов ради безопасности.
- **Фоновое воспроизведение** — Android жёстко требует ForegroundService +
  корректный MediaSession, иначе система убивает звук в фоне.
- **Локальная музыка** — Scoped Storage/`READ_MEDIA_AUDIO` (Android 13+); путь
  `local.FilePath` завязан на десктопную ФС — на Android читать через SAF/MediaStore.
- **Размер APK** — `.aar` тянет Go-рантайм (~несколько МБ на ABI); собирать под
  нужные ABI (`arm64-v8a`, при желании `armeabi-v7a`).

## 12. Безопасность

Ключ `sk-…P19T52a8` и токены `ANTHROPIC_AUTH_TOKEN` из прошлых сессий — считать
скомпрометированными: **отозвать/перевыпустить, в код и коммиты не вносить.** В
Android-сборке никакие ключи в APK не зашивать (легко извлекаются) — только
пользовательский ввод в настройках, хранить в зашифрованном хранилище.

## 13. Статус

- [x] Создан этот план (2026-09-08).
- [x] Стратегия зафиксирована: **сначала полностью фронтенд, потом бэкенд** (как
      делали десктоп) — решение пользователя (2026-09-08).
- [~] **Фаза F — полный мобильный фронтенд.** ← текущий фокус. Каркас готов
      (2026-09-08): токены дизайна, мобильная ветка `isPhone` в AppLayout, нижняя
      навигация из 5 пунктов (BottomNav), постоянный мини-плеер (MobileMiniPlayer),
      полноэкранный «Сейчас играет» с вкладкой очереди (MobileNowPlaying), экран
      «Сервисы» (ServicesPage) + роут `/services`, мобильные meta во `index.html`.
      **Готово (2026-09-08):** косметика 10 экранов < 640px (см. §6), свайп вниз
      закрывает полноэкранный плеер + **свайп-влево удаляет трек из очереди**
      (оба жеста — см. §6). Фронтенд Фазы F **завершён** — следующий крупный шаг
      Фаза 0 (упаковка APK).
      Проверка **зелёная (2026-09-08, локальный прогон)**: tsc / vitest 46/46 /
      vite build + backend go build/vet/test — всё ok (нужен был `go mod tidy`).
- [ ] Фаза 0 — упаковка фронта в дизайн-демо APK (Capacitor).
- [ ] Фазы 1–5 — встраивание Go-бэкенда, оболочка, полный порт.
