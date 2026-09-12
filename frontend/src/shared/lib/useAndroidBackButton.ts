import { useEffect } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { useUiStore } from "../../store/uiStore";

interface BackButtonListener { remove: () => void; }
interface CapacitorAppPlugin {
  addListener: (event: "backButton", cb: (data: { canGoBack?: boolean }) => void) => Promise<BackButtonListener> | BackButtonListener;
  exitApp?: () => void | Promise<void>;
}
interface CapacitorBridge {
  isNativePlatform?: () => boolean;
  getPlatform?: () => string;
  Plugins?: { App?: CapacitorAppPlugin };
}
interface AndroidWindow { CryonAndroid?: unknown; __cryonAndroidBack?: () => boolean; }

function getCapacitor(): CapacitorBridge | undefined {
  return (window as unknown as { Capacitor?: CapacitorBridge }).Capacitor;
}

function closeTopOverlay(): boolean {
  const ui = useUiStore.getState();
  if (ui.shareOpen) { ui.closeShare(); return true; }
  if (ui.equalizerOpen) { ui.setEqualizerOpen(false); return true; }
  if (ui.nowPlayingOpen) { ui.setNowPlayingOpen(false); return true; }
  return false;
}

export function useAndroidBackButton(): void {
  const navigate = useNavigate();
  const location = useLocation();

  useEffect(() => {
    const cap = getCapacitor();
    const appPlugin = cap?.Plugins?.App;
    const androidWindow = window as unknown as AndroidWindow;
    const isCustomAndroid = Boolean(androidWindow.CryonAndroid);
    const isAndroid = cap?.getPlatform?.() === "android" || isCustomAndroid || /Android/i.test(window.navigator.userAgent);
    if (!isAndroid) return;

    const handleBack = (data?: { canGoBack?: boolean }): boolean => {
      if (closeTopOverlay()) return true;
      const atHome = location.pathname === "/";
      const canGoBack = data?.canGoBack ?? window.history.length > 1;
      if (!atHome && canGoBack) { navigate(-1); return true; }
      if (!atHome) { navigate("/"); return true; }
      if (appPlugin) { void appPlugin.exitApp?.(); return true; }
      return false;
    };

    if (isCustomAndroid) androidWindow.__cryonAndroidBack = () => handleBack();

    let removed = false;
    let remove: () => void = () => {};
    if (appPlugin && (cap?.isNativePlatform?.() ?? false)) {
      Promise.resolve(appPlugin.addListener("backButton", (data) => handleBack(data))).then((listener) => {
        if (removed) listener.remove();
        else remove = listener.remove;
      });
    }

    return () => {
      removed = true;
      remove();
      if (isCustomAndroid) delete androidWindow.__cryonAndroidBack;
    };
  }, [location.pathname, navigate]);
}
