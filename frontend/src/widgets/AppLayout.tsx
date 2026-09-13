import { Outlet } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";
import { Sidebar } from "./Sidebar";
import { Topbar } from "./Topbar";
import { NowPlayingPanel } from "./NowPlayingPanel";
import { PlayerBar } from "./PlayerBar";
import { BottomNav } from "./BottomNav";
import { MobileMiniPlayer } from "./MobileMiniPlayer";
import { MobileNowPlaying } from "./MobileNowPlaying";
import { useAudioEngine } from "../store/audioEngine";
import { usePlayerStore } from "../store/playerStore";
import { useTrackCover } from "../shared/lib/useTrackCover";
import { useCoverPalette } from "../shared/lib/useCoverPalette";
import {
  onYandexConnected,
  onSourceStatusChanged,
  onNotificationsChanged,
} from "../shared/api/client";
import { useNotificationStore } from "../store/notificationStore";
import { usePlaybackSettingsStore } from "../store/playbackSettingsStore";
import { usePlayerPersistence } from "../shared/lib/usePlayerPersistence";
import { useMediaSession } from "../shared/lib/useMediaSession";
import { useAndroidBackButton } from "../shared/lib/useAndroidBackButton";
import { useUiStore } from "../store/uiStore";
import { useMediaQuery, LAYOUT_BREAKPOINTS } from "../shared/lib/useMediaQuery";
import { OnboardingModal } from "./OnboardingModal";
import { EqualizerModal } from "./EqualizerModal";
import { ShareTrackModal } from "./ShareTrackModal";
import { RouteErrorBoundary } from "./ErrorBoundary";
import type { CSSProperties } from "react";

// Ключи запросов, зависящих от подключённых источников: их выдача меняется
// при появлении/смене токена или ключа (например, треки Yandex до токена
// приходят как external_only и не играют прямым потоком, а после токена —
// как stream). Инвалидация заставляет их перезапроситься с активным
// источником без перезапуска приложения.
const SOURCE_DEPENDENT_QUERY_KEYS = [
  ["search"],
  ["recommendations"],
  ["autoMix"],
  ["newReleases"],
  ["collection"],
];

/**
 * Общий каркас приложения: боковая навигация, верхняя строка, контент,
 * правая панель "Сейчас играет" и нижняя панель воспроизведения.
 */
export function AppLayout() {
  // Реальный движок воспроизведения (mpv или HTML5 <audio>).
  useAudioEngine();
  const queryClient = useQueryClient();

  // Персист состояния плеера (громкость, вкладка, очередь) между запусками.
  usePlayerPersistence();

  // Системные медиа-контролы (SMTC на Windows) + аппаратные медиа-клавиши.
  useMediaSession();

  // Аппаратная кнопка / жест «Назад» на Android: закрыть оверлей → шаг назад по
  // истории → на «Главную» → и только затем выход. Без этого WebView по
  // умолчанию сразу закрывал приложение. Вне нативной оболочки — no-op.
  useAndroidBackButton();

  // Гидратация настроек воспроизведения (нормализация, плавный переход) из
  // хранилища — один раз при старте. audioEngine подписан на этот стор и
  // применяет значения к движку.
  useEffect(() => {
    void usePlaybackSettingsStore.getState().hydrate();
  }, []);

  // Централизованно сбрасываем кэш источникозависимых запросов, когда
  // подключается Yandex или меняется статус любого источника. Слушатель живёт
  // в постоянно смонтированном каркасе (а не на экране настроек, который
  // размонтируется при переходе), поэтому подключение токена сразу делает
  // треки играбельными — без перезапуска (задача 26 в plan.md).
  useEffect(() => {
    const invalidate = () => {
      for (const key of SOURCE_DEPENDENT_QUERY_KEYS) {
        void queryClient.invalidateQueries({ queryKey: key });
      }
    };
    const offYandex = onYandexConnected(invalidate);
    const offStatus = onSourceStatusChanged(invalidate);
    return () => {
      offYandex();
      offStatus();
    };
  }, [queryClient]);

  // Оповещения (задача 24): загружаем список из SQLite при старте и
  // перечитываем по событию notifications:changed, которое backend эмитит
  // после записи нового оповещения (сканирование, подключение источника,
  // ошибка воспроизведения). Слушатель в постоянно смонтированном каркасе.
  useEffect(() => {
    const hydrate = useNotificationStore.getState().hydrate;
    void hydrate();
    const off = onNotificationsChanged(() => {
      void hydrate();
    });
    return off;
  }, []);

  const currentTrack = usePlayerStore((s) => s.queue[s.currentIndex]);
  const coverUrl = useTrackCover(currentTrack);
  const accent = useCoverPalette(coverUrl);

  const appAccentStyle = {
    "--app-accent": accent,
  } as CSSProperties;

  // Адаптив каркаса (задача 29). Ниже порогов встроенная панель «Сейчас
  // играет» скрывается и открывается поверх контента как выдвижная, а боковая
  // навигация сворачивается в узкий значковый рельс. Пороги в CSS-пикселях,
  // поэтому корректно срабатывают и на «узком» ноутбуке, и на 1080p с
  // DPI-масштабом 125/150% (WebView2 отдаёт уже масштабированные пиксели).
  const isNarrow = useMediaQuery(LAYOUT_BREAKPOINTS.hideNowPlaying);
  // Телефонная ширина: десктопный каркас целиком заменяется мобильным (нижняя
  // навигация + мини-плеер + полноэкранный «Сейчас играет»). isPhone ⊂ isNarrow
  // (639 < 1179), поэтому авто-закрытие ниже (`!isNarrow && …`) на телефоне не
  // срабатывает — nowPlayingOpen безопасно переиспользуется для мобильного
  // полноэкранного плеера.
  const isPhone = useMediaQuery(LAYOUT_BREAKPOINTS.phone);
  const nowPlayingOpen = useUiStore((s) => s.nowPlayingOpen);
  const setNowPlayingOpen = useUiStore((s) => s.setNowPlayingOpen);
  const nowPlayingCollapsed = useUiStore((s) => s.nowPlayingCollapsed);

  // Когда экран снова расширяется, выдвижная панель больше не нужна: закрываем
  // её, чтобы при возврате на узкий экран она не всплывала сама собой.
  useEffect(() => {
    if (!isNarrow && nowPlayingOpen) setNowPlayingOpen(false);
  }, [isNarrow, nowPlayingOpen, setNowPlayingOpen]);

  // Встроенная панель — только на широком экране И если пользователь не свернул
  // её кнопкой-очередью в плеере. На узком показываем её как выдвижную поверх
  // контента (по кнопке в топбаре / плеере).
  const showInlineNowPlaying = !isNarrow && !nowPlayingCollapsed;
  const showOverlayNowPlaying = isNarrow && nowPlayingOpen;

  return (
    <div
      className="relative flex h-screen w-screen overflow-hidden bg-[#07080f] text-slate-200"
      style={appAccentStyle}
    >
      {/* Амбиентный фон, окрашенный текущей палитрой обложки (var(--app-accent)).
          Два мягких радиальных пятна сверху — как в макете Unify Music: цвет
          интерфейса «дышит» вместе с играющим треком. Слой неинтерактивный и
          лежит под всем контентом. */}
      <div
        aria-hidden
        className="pointer-events-none absolute inset-0 z-0 transition-[background] duration-1000"
        style={{
          background:
            "radial-gradient(120% 80% at 15% -10%, color-mix(in srgb, var(--app-accent) 22%, transparent) 0%, transparent 55%), " +
            "radial-gradient(120% 70% at 100% 0%, color-mix(in srgb, var(--app-accent) 14%, transparent) 0%, transparent 50%)",
        }}
      />

      <div className="relative z-10 flex min-w-0 flex-1">
        {isPhone ? (
          /* ── Мобильный каркас (телефон) ─────────────────────────────────
             Скролл-контент на всю высоту, под ним мини-плеер и нижняя
             навигация; полноэкранный «Сейчас играет» — оверлеем поверх. */
          <div className="flex min-w-0 flex-1 flex-col">
            <main className="min-w-0 flex-1 overflow-y-auto overflow-x-hidden px-3 pb-4 pt-[calc(8px+var(--safe-top))] sm:px-4">
              <RouteErrorBoundary>
                <Outlet />
              </RouteErrorBoundary>
            </main>
            <MobileMiniPlayer />
            <BottomNav />
          </div>
        ) : (
          /* ── Десктопный каркас: сайдбар + топбар + контент + панели ───── */
          <>
            <Sidebar />

            <div className="flex min-w-0 flex-1 flex-col">
              <Topbar />
              <div className="relative flex min-h-0 flex-1">
                <main className="min-w-0 flex-1 overflow-y-auto px-4 pb-6 sm:px-6">
                  <RouteErrorBoundary>
                    <Outlet />
                  </RouteErrorBoundary>
                </main>

                {showInlineNowPlaying ? <NowPlayingPanel /> : null}

                {/* Выдвижная панель «Сейчас играет» на узких экранах */}
                {showOverlayNowPlaying ? (
                  <>
                    <button
                      type="button"
                      aria-label="Закрыть панель «Сейчас играет»"
                      className="absolute inset-0 z-20 bg-black/50 backdrop-blur-sm"
                      onClick={() => setNowPlayingOpen(false)}
                    />
                    <div className="absolute inset-y-0 right-0 z-30 max-w-[90vw] overflow-y-auto shadow-2xl">
                      <NowPlayingPanel onClose={() => setNowPlayingOpen(false)} />
                    </div>
                  </>
                ) : null}
              </div>
              <PlayerBar />
            </div>
          </>
        )}
      </div>

      {/* Полноэкранный мобильный плеер поверх каркаса (z-40 < модалок z-50). */}
      {isPhone && nowPlayingOpen ? <MobileNowPlaying /> : null}

      <OnboardingModal />
      <EqualizerModal />
      <ShareTrackModal />
    </div>
  );
}
