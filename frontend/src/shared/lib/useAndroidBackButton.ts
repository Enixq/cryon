import { useEffect } from "react";
import { useNavigate } from "react-router-dom";
import { useUiStore } from "../../store/uiStore";

/**
 * Минимальный «поверхностный» доступ к плагину @capacitor/app через глобальный
 * мост window.Capacitor. Пакет НАМЕРЕННО не импортируется статически: тогда
 * tsc/vite-сборка не зависят от того, установлен ли @capacitor/app прямо сейчас.
 * На устройстве плагин доступен как window.Capacitor.Plugins.App после
 * `npm i @capacitor/app` + `npx cap sync android`. В десктопном WebView2 и в
 * браузере моста нет — хук просто ничего не делает.
 */
interface BackButtonListener {
  remove: () => void;
}
interface CapacitorAppPlugin {
  addListener: (
    event: "backButton",
    cb: (data: { canGoBack?: boolean }) => void,
  ) => Promise<BackButtonListener> | BackButtonListener;
  exitApp?: () => void | Promise<void>;
}
interface CapacitorBridge {
  isNativePlatform?: () => boolean;
  getPlatform?: () => string;
  Plugins?: { App?: CapacitorAppPlugin };
}

function getCapacitor(): CapacitorBridge | undefined {
  return (window as unknown as { Capacitor?: CapacitorBridge }).Capacitor;
}

/**
 * Аппаратная кнопка / жест «Назад» на Android.
 *
 * Без обработчика Capacitor по умолчанию сразу закрывает приложение — на что и
 * жаловался пользователь («свайп назад сразу выходит»). Здесь порядок такой:
 *  1. открыт оверлей (полноэкранный плеер / эквалайзер / «Поделиться») —
 *     закрываем его, а не уходим с экрана;
 *  2. иначе, если в приложении есть история — возвращаемся на предыдущий экран;
 *  3. иначе, если мы не на «Главной» — уходим на «Главную»;
 *  4. и только на «Главной» без истории — выходим из приложения.
 *
 * Ничего не делает вне нативной Android-оболочки (десктоп/браузер), поэтому
 * безопасно вызывается один раз в общем каркасе.
 */
export function useAndroidBackButton(): void {
  const navigate = useNavigate();

  useEffect(() => {
    const cap = getCapacitor();
    const appPlugin = cap?.Plugins?.App;
    // Только нативная Android-оболочка с доступным плагином @capacitor/app.
    const isNative = cap?.isNativePlatform?.() ?? false;
    if (!isNative || !appPlugin) return;

    // Закрыть верхний открытый оверлей. true — если что-то закрыли.
    const closeTopOverlay = (): boolean => {
      const ui = useUiStore.getState();
      if (ui.shareOpen) {
        ui.closeShare();
        return true;
      }
      if (ui.equalizerOpen) {
        ui.setEqualizerOpen(false);
        return true;
      }
      if (ui.nowPlayingOpen) {
        ui.setNowPlayingOpen(false);
        return true;
      }
      return false;
    };

    const handler = (data: { canGoBack?: boolean }) => {
      if (closeTopOverlay()) return;

      // HashRouter: «Главная» — это пустой хэш или #/.
      const hash = window.location.hash;
      const atHome = hash === "" || hash === "#/" || hash === "#";
      // canGoBack от Capacitor учитывает и хэш-навигации WebView; в качестве
      // подстраховки — длина истории браузера.
      const canGoBack = data?.canGoBack ?? (window.history.length > 1);

      if (!atHome && canGoBack) {
        navigate(-1);
      } else if (!atHome) {
        navigate("/");
      } else {
        void appPlugin.exitApp?.();
      }
    };

    let removed = false;
    let remove: () => void = () => {};
    Promise.resolve(appPlugin.addListener("backButton", handler)).then((listener) => {
      if (removed) listener.remove();
      else remove = listener.remove;
    });

    return () => {
      removed = true;
      remove();
    };
  }, [navigate]);
}
