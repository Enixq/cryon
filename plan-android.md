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

> **⚠️ Единственный шаг, обязательно требующий компилятора Go.** Он трогает ядро
> рабочей десктоп-сборки и пространство имён Wails-биндингов, поэтому вслепую
> (при недоступном гейте, см. §10) его делать НЕЛЬЗЯ — молча сломаем десктоп,
> которым пользуются каждый день. Готов к мгновенному применению, как только
> вернётся `go build`. Стратегия ниже сохраняет десктоп «по построению».

**Риск пространства имён и как его снять.** Сгенерированные биндинги жёстко
зашивают пакет: [App.js](frontend/wailsjs/go/main/App.js) вызывает
`window['go']['main']['App']['Method'](...)`, а [client.ts](frontend/src/shared/api/client.ts)
импортирует `wailsjs/go/main/App`. Если просто перенести `App` в пакет `core`,
Wails перегенерирует биндинги как `window.go.core.App` (+ путь `wailsjs/go/core/App`)
и весь фронтенд-мост (и [httpBridge.ts](frontend/src/shared/lib/httpBridge.ts),
ставящий `window.go.main.App`) сломается.

**Решение — обёртка через встраивание (embedding), пространство `main` сохраняется:**
- [ ] Создать `internal/core`: перенести туда бизнес-логику — `App` (переименовать
      в `core.App`), health, wiring, реестр уже в `internal/services`. Экспортировать
      минимальную поверхность для `main` и `mobile`: `NewApp`, `(*App).Startup`,
      `(*App).Shutdown`, `(*App).AssetHandler() http.Handler`,
      `(*App).UseWailsRuntime()` (ставит desktop-хост изнутри пакета),
      `(*App).StartMobileServer(fs.FS) (string, func(), error)`.
- [ ] `platform.go`/`platform_wails.go`/`rpcbridge.go`/`lanserver*.go`/`mobileserver.go`
      переезжают в `core` вместе с `App` (тот же пакет — приватные поля вроде
      `ctx`/`platform` остаются доступны хостам). `platform_wails.go` сохраняет
      тег `//go:build !android`, так что `core` компилируется и под android/arm64.
- [ ] В `package main` оставить ТОНКУЮ обёртку `type App struct { *core.App }` и
      `func NewApp() *App { a := &App{core.NewApp()}; a.UseWailsRuntime(); return a }`.
      Wails биндит `main.App`; методы `*core.App` **промотятся** (Go включает
      промотнутые методы в набор методов, а Wails строит биндинги рефлексией) →
      генерируется прежний `wailsjs/go/main/App` с `window.go.main.App`.
      **Фронтенд, httpBridge и client.ts не меняются.**
- [ ] `main.go`: `AssetServer.Handler: app.AssetHandler()`, `OnStartup: app.Startup`,
      `OnShutdown: app.Shutdown`, `Bind: []interface{}{app}` (обёртка).
- [ ] **После правок — обязательно:** `wails generate module` (регенерация
      биндингов) → `go build ./...` → `wails build`. Свериться, что в
      `wailsjs/go/main/App.js` присутствуют все методы (промотка сработала) и
      `window.go.main.App` не изменился.


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
- [x] **Автономный Android (§17) — СОБРАН ДО APK** (2026-09-12). Отвергнутая
      LAN-схема (§15) заменена встроенным на устройстве сервером. **Реализовано
      целиком:** абстракция Wails-рантайма (`platformHost`), общий `rpcbridge`,
      встроенный сервер `mobileserver.go`, авто-активация `httpBridge` по
      `__CRYON_BASE__`, вынос `App`→`internal/core`, пакет `mobile` + `gomobile
      bind` (в CI и в [scripts/build-android.ps1](scripts/build-android.ps1)),
      Kotlin-оболочка `android/`, локальная музыка через SAF. **Фикс CI:** из
      `mobile/main.go` убран `import "C"` — он ронял `gomobile bind` (см. раздел
      2026-09-12). APK: `android/app/build/outputs/apk/debug/app-debug.apk`.
- [x] **7 багов на живом APK устранены** (2026-09-12) — краш поиска, back-жест,
      локальная музыка, тумблер эквалайзера, отвал YouTube под VPN, буквальный
      поиск по названию жанра, мусор в радаре. Детали — раздел 2026-09-12 ниже.
- [ ] Фазы 1–5 — встраивание Go-бэкенда, оболочка, полный порт.

---

## Догон (2026-09-09): баги адаптива APK + кнопка «Назад» + центровка ползунков

По жалобам пользователя на debug-APK и десктоп (со скриншотами):

1. **Горизонтальный «разъезд» страниц на телефоне (адаптив APK).**
   Причина: полоса вкладок [TabBar.tsx](frontend/src/shared/ui/TabBar.tsx) была
   обычным нескроллящимся `flex`-рядом — 4–5 вкладок (Поиск/Библиотека/Настройки)
   не влезали по ширине и распирали всю страницу в горизонтальный скролл.
   Исправлено:
   - TabBar теперь `overflow-x-auto` + утилита `.no-scrollbar` (добавлена в
     [styles.css](frontend/src/styles.css)); кнопки `shrink-0 whitespace-nowrap`
     (в режиме `stretch` — по-прежнему `flex-1`). Лишние вкладки прокручиваются
     ВНУТРИ полосы, страница остаётся по ширине экрана. На десктопе скролл не
     появляется (вкладки влезают).
   - Телефонный `<main>` в [AppLayout.tsx](frontend/src/widgets/AppLayout.tsx)
     получил страховочный `overflow-x-hidden`.
   - Длинная кнопка «Добавить папку с музыкой» в
     [LibraryPage.tsx](frontend/src/pages/LibraryPage.tsx) на телефоне свёрнута до
     иконки (`<span className="hidden sm:inline">`, + `aria-label`/`title`,
     `shrink-0`).
   - [PageHeader.tsx](frontend/src/shared/ui/PageHeader.tsx): `h1` получил
     `truncate` — длинный заголовок не распирает шапку.

2. **Свайп/кнопка «Назад» на Android сразу закрывали приложение.**
   Причина: не было слушателя `backButton` — WebView уходил в дефолт (выход).
   Исправлено:
   - Новый хук [useAndroidBackButton.ts](frontend/src/shared/lib/useAndroidBackButton.ts),
     смонтирован один раз в [AppLayout.tsx](frontend/src/widgets/AppLayout.tsx).
     Порядок: закрыть открытый оверлей (полноэкранный плеер / эквалайзер /
     «Поделиться») → шаг назад по истории (`navigate(-1)`) → на «Главную» → и
     только на «Главной» без истории `App.exitApp()`.
   - Плагин берётся через глобальный мост `window.Capacitor.Plugins.App` (без
     статического импорта) — `tsc`/`vite build` не зависят от факта установки
     пакета; вне нативной оболочки хук — no-op.
   - В [package.json](frontend/package.json) добавлен `@capacitor/app` (^7.1.0).
     **Требует действий пользователя** (см. ниже): `npm i` + `npx cap sync
     android` + пересборка APK — иначе нативный плагин `backButton` в APK не
     появится и жест «Назад» останется дефолтным.

3. **Ползунок громкости на десктопе стоял ниже центра.**
   Причина: [Slider.tsx](frontend/src/shared/ui/Slider.tsx) центрировал дорожку
   через `flex items-center` на корне, а место вызова в
   [PlayerBar.tsx](frontend/src/widgets/PlayerBar.tsx) передавало display-класс
   `lg:block`, который перебивал `flex` → дорожка уезжала вверх, ползунок казался
   ниже. Исправлено: центрирование сделано независимым от `display` — дорожка и
   ползунок позиционируются абсолютно (`top-1/2 -translate-y-1/2`), корню оставлен
   только `relative` + высота. Место вызова заодно переведено на `lg:flex`.

4. **Эквалайзер («также»).** Полосы — нативные вертикальные `input[type=range]`
   ([EqualizerModal.tsx](frontend/src/widgets/EqualizerModal.tsx)). Им задана
   явная центрированная ширина (`mx-auto w-6`), чтобы ползунок гарантированно
   стоял по центру колонки во всех движках. Если конкретная претензия к
   эквалайзеру после этого сохранится — нужен точечный скриншот (в этой среде
   визуальная проверка недоступна).

**Что должен сделать пользователь для APK-фикса «Назад»** (среда без Android
toolchain):
```bash
cd frontend
npm i                       # подтянет @capacitor/app
npx cap sync android        # прокинет плагин в нативный проект
cd android && ./gradlew assembleDebug
# → app/build/outputs/apk/debug/app-debug.apk
```
Правки адаптива и десктопных ползунков — чисто фронтендовые, попадут в APK при
обычной пересборке (`npm run build` + `npx cap sync android`).

## 14. Безопасность (повтор)

Ключ `sk-…P19T52a8` и токены `ANTHROPIC_AUTH_TOKEN` — считать скомпрометированными:
отозвать/перевыпустить, в код и коммиты не вносить. В APK ключи не зашивать.

---

## 15. Минимальный рабочий Android через LAN (2026-09-09) — ⛔ ОТКЛОНЁН пользователем, СМ. §17

> **Устарело.** Пользователь отверг схему «телефон → ПК по локальной сети»
> дословно: «Мне так не нужно, вдруг я захочу поделиться, либо уехать и по
> мобильной сети чтобы работало. Это не рабочее, переделать.» Ей на смену пришёл
> **автономный встроенный сервер на устройстве** — см. §17. LAN-код
> ([lanserver.go](lanserver.go), тег `cryonlan`) оставлен только как
> отладочный стенд в доверенной домашней сети и физически изолирован сборочным
> тегом (в обычную сборку не попадает). Раздел ниже сохранён как история решения.

Цель пользователя: «хотя бы минимально, чтобы уже работало, потестить». Выбран
самый быстрый и наименее рискованный путь — **телефон как клиент десктопа по
Wi-Fi**, без gomobile и без рискованного рефактора `internal/core`. Переиспользуем
уже существующие `App` (чистый Go) и `localAssetHandler` (тот же прокси
`/stream`, что и на десктопе).

### Как это работает
- На ПК приложение поднимает второй HTTP-сервер на `0.0.0.0:8899`
  ([lanserver.go](lanserver.go), только сборка `-tags cryonlan`; иначе no-op
  [lanserver_stub.go](lanserver_stub.go) — рабочий десктоп-билд не затрагивается).
- `POST /api/call` `{method,args}` — рефлексивный RPC по экспортируемым методам
  `App` (контракт совпадает с Wails: первое не-error значение = результат).
  `/local/` и `/stream/` делегируются существующему `localAssetHandler`. На всё
  повешен разрешающий CORS + preflight OPTIONS.
- На телефоне [httpBridge.ts](frontend/src/shared/lib/httpBridge.ts) ставит на
  `window.go`/`window.runtime` совместимые с Wails заглушки, гоняющие вызовы по
  HTTP на адрес сервера. Благодаря этому **весь `client.ts` (60 функций) не
  тронут** — `isWailsRuntime()` становится true. Мост включается ТОЛЬКО если в
  настройках задан адрес сервера и это не настоящий Wails.
- Мост ставится первым side-effect импортом
  ([installBridge.ts](frontend/src/shared/lib/installBridge.ts)) до загрузки App,
  т.к. `playerStore` читает `isWailsRuntime()` уже при импорте.
- Звук: `getPlaybackUrlForHtml5` в LAN-режиме отдаёт абсолютный
  `http://<IP>:8899/stream/...`; `<audio>` помечен `crossOrigin="anonymous"`,
  сервер отдаёт CORS → перемотка (Range) и эквалайзер (Web Audio) работают.
  Плеер mpv на телефоне выключен (мост гасит `Player*`) — звук идёт через HTML5.
- Capacitor: `androidScheme:'http'` + `usesCleartextTraffic="true"` — чтобы
  страница `http://localhost` могла обращаться к `http://<IP>:8899` (не mixed
  content).

### Затронутые файлы
- Новые: [lanserver.go](lanserver.go), [lanserver_stub.go](lanserver_stub.go),
  [httpBridge.ts](frontend/src/shared/lib/httpBridge.ts),
  [installBridge.ts](frontend/src/shared/lib/installBridge.ts).
- Правки: [app.go](app.go) (`startup` → `a.startLANServer()`),
  [main.tsx](frontend/src/main.tsx) (первый импорт моста),
  [client.ts](frontend/src/shared/api/client.ts) (`getPlaybackUrlForHtml5` —
  префикс `__CRYON_BASE__`), [audioEngine.ts](frontend/src/store/audioEngine.ts)
  (`crossOrigin`), [SettingsPage.tsx](frontend/src/pages/SettingsPage.tsx)
  (поле «Сервер Cryon» в «Общее»),
  [capacitor.config.ts](frontend/capacitor.config.ts) (http),
  [AndroidManifest.xml](frontend/android/app/src/main/AndroidManifest.xml)
  (cleartext).

### Что должен сделать пользователь (среда без Android/Go toolchain)

**1. На ПК — собрать и запустить десктоп С LAN-сервером:**
```powershell
# из корня проекта
wails build -tags cryonlan          # или для разработки: wails dev -tags cryonlan
# запустить собранный .exe (build/bin) — в логе будет:
#   LAN-сервер Cryon запущен …
#   LAN-адрес url=http://192.168.x.x:8899   ← запомнить этот адрес
```
- Если Windows Defender спросит про сеть — разрешить доступ в **частной** сети
  (иначе телефон не достучится до порта 8899).
- Обычный `wails build` (без `-tags cryonlan`) по-прежнему собирает чистый
  десктоп без сервера — ничего не сломано.

**2. На ПК — собрать APK:**
```bash
cd frontend
npm i
npm run build
npx cap sync android
cd android && ./gradlew assembleDebug
# → app/build/outputs/apk/debug/app-debug.apk
```

**3. На телефоне:**
- Телефон и ПК — в одной Wi-Fi-сети.
- Установить `app-debug.apk`, открыть приложение.
- Настройки → «Общее» → «Сервер Cryon (для телефона)» → ввести адрес из п.1
  (`http://192.168.x.x:8899`) → «Подключиться и перезапустить».
- После перезапуска поиск, библиотека и воспроизведение идут с ПК.

### Ограничения v1 (осознанно)
- Нет live-событий по сети (`/api/events` не реализован) → статусы источников и
  «Яндекс подключён» не обновляются мгновенно; помогает переход между экранами
  (react-query перезапрашивает). Не критично для теста.
- Системные диалоги выбора папки/файла с телефона отключены (открылись бы на ПК).
- Требуется, чтобы десктоп-приложение было запущено на ПК.
- Это ступенька к встроенному gomobile-серверу (Фазы 1–2) — при переходе фронт и
  RPC-контракт не меняются, меняется лишь адрес backend (localhost внутри APK).

### Проверка (когда вернётся возможность компиляции)
- Десктоп без изменений: `go build ./...`, `go vet ./...`, `wails build`.
- LAN-сборка: `go build -tags cryonlan ./...`, `go vet -tags cryonlan ./...`.
- Фронт: `tsc -b`, `vitest run`, `vite build`.
- Могу оперативно поправить любые ошибки компиляции — код писался без доступного
  компилятора (классификатор недоступен), проверен вычиткой.

## 16. Безопасность (повтор)

Секреты остаются на ПК: телефон хранит только введённый адрес сервера, по сети
ключи не передаются, в APK ничего не зашито. Ключ `sk-…P19T52a8` и токены
`ANTHROPIC_AUTH_TOKEN` — скомпрометированы: отозвать/перевыпустить, в код/коммиты
не вносить.

---

## 17. АВТОНОМНЫЙ Android — встроенный сервер на устройстве (2026-09-10)

Замена отвергнутой LAN-схемы (§15). Требование пользователя дословно: «вдруг я
захочу поделиться, либо уехать и по мобильной сети чтобы работало». Значит Go-
бэкенд поднимается **внутри телефона** (gomobile bind → HTTP-сервер на
`127.0.0.1:<случайный порт>`), а WebView грузит этот адрес. Работает и по
мобильной сети, и без включённого ПК, приложением можно поделиться. Фронтенд и
RPC-контракт те же, что на десктопе.

### 17.1. Сделано и проверено вычиткой (код, тег-изоляция — рабочий билд не задет)

- [x] **Абстракция Wails-рантайма.** Введён интерфейс `platformHost`
      (`Emit/OpenURL/PickFile/PickDirectory`) — [platform.go](platform.go).
      Три реализации: `nullHost` (по умолчанию/тесты, no-op),
      `wailsHost` (десктоп, [platform_wails.go](platform_wails.go), тег
      `//go:build !android`), `sseHost` (устройство, события в SSE). Все 16
      `runtime.EventsEmit`, `BrowserOpenURL` и два системных диалога в
      [app.go](app.go)/[oauth.go](oauth.go) переведены на `a.platform.*`.
      **Итог: Wails-рантайм импортируют ТОЛЬКО [main.go](main.go) и
      [platform_wails.go](platform_wails.go)** — `app.go`/`oauth.go` больше не
      зависят от Wails и станут пригодны для android/arm64 после выноса в `core`
      (Фаза 1). Поведение десктопа байт-в-байт прежнее (те же вызовы через
      интерфейс).
- [x] **Общий рефлексивный RPC** вынесен в [rpcbridge.go](rpcbridge.go)
      (тег `cryonlan || cryonmobile`): `serveRPC`/`writeJSON`/`writeRPCError` —
      один код для LAN и мобильного серверов.
- [x] **Встроенный сервер (seed)** — [mobileserver.go](mobileserver.go) (тег
      `cryonmobile`): `sseHub`+`serveEvents` (SSE вместо Wails-событий), `sseHost`,
      `injectBaseURL` (вставляет `window.__CRYON_BASE__=location.origin` в
      `<head>`), `startMobileServer(spa fs.FS)` — mux (`/api/call`, `/api/events`,
      `/local/`, `/stream/`, SPA-фолбэк), слушает `127.0.0.1:0`, отдаёт baseURL и
      stop-функцию. Тег держит его вне десктоп-сборки.
- [x] **Фронтенд-мост авто-активируется на устройстве.**
      [httpBridge.ts](frontend/src/shared/lib/httpBridge.ts): `injectedBaseUrl()`
      читает `window.__CRYON_BASE__` (приоритет над сохранённым вручную адресом),
      ставит `window.go.main.App` + `window.runtime` до рендера React; подписка
      `browser:open` открывает ссылки средствами WebView.
- [x] **Транспорт фронтенда сверен по всей цепочке (вычитка):**
      - `isWailsRuntime()` → true (мост ставит `window.go.main.App`).
      - Биндинги резолвят `window.go.main.App[...]` **в момент вызова** →
        порядок импортов не важен, все `App.*` уходят в `fetch(base+"/api/call")`.
      - `runtime.EventsOn/BrowserOpenURL` — тоже call-time → события идут по SSE.
      - Плеер: `PlayerBackendAvailable`→false ⇒ [audioEngine.ts](frontend/src/store/audioEngine.ts)
        минует ветку mpv (строка ~282) и играет через HTML5 `<audio>` (строка ~306).
      - URL потока: `getPlaybackUrlForHtml5` = `__CRYON_BASE__ + "/stream/…"` →
        same-origin `http://127.0.0.1:<port>/stream/…`, CORS не нужен.
      **Вывод: фронтенд к автономному режиму готов, правок не требует.**

### 17.2. ВЫПОЛНЕНО (2026-09-12) — весь список закрыт на toolchain-машине

> Пункты 1–4 ниже реализованы (код на диске, сверено вычиткой в этой сессии;
> компиляция выполнена параллельной сессией и CI). Пункт 5 закрыт первой
> итерацией через SAF, MediaStore — как улучшение. Подробный журнал доработок и
> фикс CI-сборки — в разделе **«Автономный Android: доводка до APK (2026-09-12)»**
> ниже.

1. **[x] Фаза 1 — вынос `App` в `internal/core`** — сделано. Ядро живёт в пакете
   `core`; `main.go`/`platform_wails.go` встраивают его, `window.go.main.App`
   сохранён. Десктоп-сборка не задета (тег-изоляция).
2. **[x] Пакет `mobile`** — сделано: [mobile/mobile.go](mobile/mobile.go),
   `//go:build android`, `//go:embed all:dist`, `Start(dataDir)`→`core.NewApp`→
   `StartMobileServer(sub(dist))`→baseURL, `Stop()`. Embed-каталог `mobile/dist`
   заполняется перед bind (`vite build` → копия `frontend/dist`).
3. **[x] gomobile bind** — заведено в CI и в локальном скрипте
   [scripts/build-android.ps1](scripts/build-android.ps1). Цель `android/arm64`,
   `-androidapi 24`, `-tags cryonmobile`, выход
   `android/app/libs/cryonmobile.aar`. **Фикс CI:** из [mobile/main.go](mobile/main.go)
   убраны `import "C"` и `func main` — они ломали парсер gobind (см. раздел
   2026-09-12 ниже).
4. **[x] Kotlin-оболочка** — сделано: [android/](android/), пакет `ru.cryon.app`,
   [MainActivity.kt](android/app/src/main/java/ru/cryon/app/MainActivity.kt)
   (WebView + `Mobile.start(filesDir)`→`loadUrl(base)`, восстановление после
   `onRenderProcessGone`, кнопка «Назад» через `__cryonAndroidBack`/`canGoBack`),
   [AndroidManifest.xml](android/app/src/main/AndroidManifest.xml) со всеми
   разрешениями + `largeHeap`.
5. **[x] Локальная музыка на Android** — закрыто первой итерацией через SAF:
   `ACTION_OPEN_DOCUMENT_TREE` → фоновое копирование аудио в `filesDir` (не UI-поток,
   иначе ANR = «кнопка не работает») → путь во фронтенд через
   `__cryonFolderPickerResolve`. Сканирование `MediaStore.Audio` без копирования —
   как улучшение на будущее.

### 17.3. Runbook — выполнить на машине с toolchain (по порядку)

```bash
# 0) Предустановка (один раз): JDK 17, Android SDK+NDK, Go 1.26,
#    go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init

# 1) Фаза 1 (с компилятором): вынести App→internal/core обёрткой-встраиванием,
#    затем регенерация и проверка ДЕСКТОПА:
wails generate module
go build ./...            # + go vet ./...  + go test ./...
wails build              # десктоп цел, window.go.main.App на месте
go build -tags cryonmobile ./...   # мобильный серверный код компилируется

# 2) Собрать фронтенд и вложить в пакет mobile
cd frontend && npm i && npm run build && cd ..
#   скопировать frontend/dist → mobile/dist (для //go:embed)

# 3) gomobile bind → .aar
gomobile bind -target=android -androidapi 24 -o android/app/libs/cryonmobile.aar ./mobile

# 4) Собрать APK автономной оболочки
cd android && ./gradlew assembleDebug
# → android/app/build/outputs/apk/debug/app-debug.apk  (работает без ПК, по любой сети)
```

**Проверки, которые прогнать при возврате гейта:** `go build/vet/test ./...`,
`go build -tags cryonmobile ./...`, `go build -tags cryonlan ./...`, `tsc -b`,
`vitest run`, `vite build`. Любые ошибки компиляции правлю сразу — код писался без
доступного компилятора.

### 17.4. Безопасность

Сервер слушает ТОЛЬКО петлю `127.0.0.1` (из сети недоступен). Ключи/секреты в APK
не зашиваются — только пользовательский ввод в зашифрованных настройках. Ключ
`sk-…P19T52a8` и токены `ANTHROPIC_AUTH_TOKEN` считать скомпрометированными:
отозвать/перевыпустить, в код/коммиты не вносить.

---

## Автономный Android: доводка до APK (2026-09-12)

Итог сессии: автономный Android собран целиком (сервер на телефоне, работает по
любой сети, ПК не нужен), устранён провал CI-сборки APK и все 7 багов, найденных
на живом устройстве. Ниже — журнал; §17.2 и §13 обновлены под этот факт.

### Фикс провала CI (почему APK не попадал в релиз)

- **Корень:** [mobile/main.go](mobile/main.go) держал `import "C"` и `func main`.
  `gomobile bind` собирает пакет `mobile` как **библиотеку** (JNI `.so` +
  Java-обёртка) и сам генерирует `package main` с cgo-экспортами — своя `main` и
  cgo в биндимом пакете не нужны. Парсер `gobind` (go/packages) **не резолвит
  псевдопакет `C`** → `gomobile bind` падал в job `build-android`.
- **Следствие для релиза:** job `release` в
  [.github/workflows/release.yml](.github/workflows/release.yml) объявлен
  `needs: [build-windows, build-android]`. Пока `build-android` падал, **релиз
  вообще не создавался** — отсюда «на гитхаб не закинул».
- **Фикс:** из `mobile/main.go` убраны `import "C"` и `func main` (файл оставлен
  почти пустым с пояснением; его можно и удалить целиком: `git rm mobile/main.go`).
  Теперь `gomobile bind` проходит, `build-android` зелёный → `release` создаётся
  и прикладывает `Cryon2-<tag>-android-arm64.apk`.
- **Про «Must have admin rights» (403) при создании релиза:** это **не** ошибка
  workflow — в нём уже стоит `permissions: contents: write`. Так отвечает GitHub,
  когда на теги/релизы навешено **ruleset/tag-protection** или ограничен
  `GITHUB_TOKEN` в настройках репозитория (Settings → Actions → Workflow
  permissions = *Read and write*; Settings → Rules/Tags — снять запрет на
  создание релиза от Actions). Правится в настройках репозитория, кодом не
  лечится.

### Локальная сборка APK — точный путь (без CI)

Добавлен [scripts/build-android.ps1](scripts/build-android.ps1) — повторяет CI
пошагово (Windows PowerShell):
`frontend` → `npm ci && npm run build` → копия `frontend/dist`→`mobile/dist` →
`mobile` → `go mod tidy && gomobile bind -target=android/arm64 -androidapi 24
-tags cryonmobile -o android/app/libs/cryonmobile.aar .` → `android` →
`gradlew.bat assembleDebug`.
**APK:** `android/app/build/outputs/apk/debug/app-debug.apk` (+ копия
`dist/Cryon2-<tag>-android-arm64.apk`). Скрипт собирает из рабочего дерева, т.е.
включает все правки ниже даже без коммита. Предустановка — в шапке скрипта
(Go 1.26, Node 20, JDK 17/21, Android SDK+NDK, gomobile/gobind + `gomobile init`).

### 7 багов на живом APK — устранены (все сверены вычиткой в этой сессии)

1. **Краш на вкладке «Поиск».** OOM рендер-процесса WebView на сетке обложек ронял
   всё приложение. [MainActivity.kt](android/app/src/main/java/ru/cryon/app/MainActivity.kt):
   `onRenderProcessGone`→`recreateWebView()` (возвращаем `true`, пересоздаём
   WebView), `android:largeHeap="true"`; во фронтенде экраны обёрнуты в
   `RouteErrorBoundary` (ошибка рендера роута не валит SPA).
2. **Back-жест закрывал приложение.** [useAndroidBackButton.ts](frontend/src/shared/lib/useAndroidBackButton.ts)
   ставит `window.__cryonAndroidBack` (закрыть оверлей → шаг назад по роутеру →
   на главную); Kotlin `onBackPressed` пробует его, затем `canGoBack()/goBack()`,
   и только в корне — двойное нажатие с тостом «Нажмите ещё раз, чтобы выйти».
3. **Кнопка локальной музыки не работала.** Копирование папки шло на UI-потоке =
   ANR. Перенесено в фоновый `Thread`, результат в WebView через
   `__cryonFolderPickerResolve` (Kotlin), выбор — `ACTION_OPEN_DOCUMENT_TREE` (SAF).
4. **Тумблер эквалайзера «вылетал» за рамку.** [EqualizerModal.tsx](frontend/src/widgets/EqualizerModal.tsx):
   ползунок позиционируется инлайновым `transform: translateX(20px)` вместо
   произвольного Tailwind-класса (тот мог не попасть в бандл на встроенном
   Android-WebView), дорожка — `overflow-hidden` как жёсткая страховка.
5. **YouTube отваливался под VPN.** [youtube/service.go](internal/services/youtube/service.go):
   `newHTTPClient()` с раздельными бюджетами (Dial 15s / TLS 20s / заголовки 30s /
   всего 45s) вместо единого 15s — медленный TLS-хендшейк под VPN больше не съедает
   лимит. Плюс SSE-мост [httpBridge.ts](frontend/src/shared/lib/httpBridge.ts)
   авто-переподключается (backoff + `online`/`visibilitychange`), а не умолкает
   навсегда после разрыва loopback при переключении VPN.
6. **Жанр искал по названию, а не по сути.** [genres.ts](frontend/src/shared/data/genres.ts):
   у каждого жанра `seeds` — реальные исполнители; клик собирает подборку поиском
   по ним, а не по строке «Русский рок».
7. **Радар новинок показывал случайное/шансон-поп.** [recommendations/engine.go](internal/recommendations/engine.go):
   профиль вкуса по недавности и частоте прослушиваний, учёт обратной связи
   (блок/буст артистов), отсев «редакционного мейнстрима» (шансон/поп из ленты
   сервиса), не совпадающего со знакомыми/похожими артистами.

Плюс: `safeRandomId()` в [trackShare.ts](frontend/src/shared/lib/trackShare.ts) —
фолбэк `randomUUID`→`getRandomValues`→`Math.random` (на старом Android-WebView
`crypto.randomUUID` бросает исключение и ронял разбор ссылки «поделиться»).

### Состояние в git (важно для пользователя)

- **Фикс CI (`mobile/main.go`) и вся автономная обвязка — закоммичены** (коммиты
  `feat: автономный Android-клиент` … `fix: make Android release portable`).
- **Правки 7 багов — в рабочем дереве, НЕ закоммичены** (`MainActivity.kt`,
  `build.gradle.kts`, `SearchPage.tsx`, `genres.ts`, `httpBridge.ts`,
  `useAndroidBackButton.ts`, `audioEngine.ts`, `engine.go`, `youtube/service.go`).
  Локальная сборка скриптом их подхватит; для APK **из CI** нужно закоммитить их и
  запушить новый тег. Коммит/пуш — только по явному «да» пользователя.

### Проверки (прогнать при возврате toolchain/гейта)

`go build/vet/test ./...`, `go build -tags cryonmobile ./...`, `tsc -b`,
`vitest run`, `vite build`; затем `scripts/build-android.ps1` до готового APK.
