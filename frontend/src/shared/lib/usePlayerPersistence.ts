// Персист состояния плеера между запусками приложения: громкость, mute,
// последняя открытая вкладка и очередь воспроизведения. Благодаря этому
// приложение открывается там же, где его закрыли, а не «с чистого листа».
//
// Хранилище — то же, что у настроек (SQLite через getSetting/setSetting в
// Wails, localStorage в вебе). Значения читаются один раз при монтировании
// каркаса и затем персистятся с дебаунсом, чтобы частые изменения (перетягивание
// громкости, тикер позиции) не били по базе на каждый шаг.

import { useEffect, useRef } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { usePlayerStore } from "../../store/playerStore";
import { getSetting, setSetting } from "../api/client";
import type { Track } from "../types";

const KEY_VOLUME = "player.volume";
const KEY_MUTED = "player.muted";
const KEY_ROUTE = "ui.lastRoute";
const KEY_SESSION = "player.session";

// Очередь длиннее не персистим — храним «хвост» вокруг текущего трека, чтобы
// не раздувать запись в базе на огромных плейлистах.
const MAX_SESSION_TRACKS = 200;

interface PersistedSession {
  queue: Track[];
  currentIndex: number;
}

function clampVol(v: number): number {
  if (!Number.isFinite(v)) return 68;
  return Math.max(0, Math.min(100, Math.round(v)));
}

export function usePlayerPersistence() {
  const hydrated = useRef(false);
  const navigate = useNavigate();
  const location = useLocation();

  // Гидратация один раз при монтировании.
  useEffect(() => {
    let cancelled = false;
    (async () => {
      const [vol, muted, route, session] = await Promise.all([
        getSetting(KEY_VOLUME),
        getSetting(KEY_MUTED),
        getSetting(KEY_ROUTE),
        getSetting(KEY_SESSION),
      ]);
      if (cancelled) return;

      const patch: Partial<ReturnType<typeof usePlayerStore.getState>> = {};
      if (vol !== "") patch.volume = clampVol(Number(vol));
      if (muted !== "") patch.muted = muted === "1";

      // Очередь восстанавливаем только если сейчас она пуста (не перетираем
      // уже начатое воспроизведение) и в сохранённой сессии есть треки.
      // Всегда на паузе и с нулевым прогрессом: точную позицию внутри стрима
      // восстановить нельзя (у сетевых источников она известна лишь после
      // старта), поэтому честно начинаем трек сначала по нажатию Play.
      if (session !== "" && usePlayerStore.getState().queue.length === 0) {
        try {
          const parsed = JSON.parse(session) as Partial<PersistedSession>;
          if (Array.isArray(parsed.queue) && parsed.queue.length > 0) {
            patch.queue = parsed.queue as Track[];
            patch.currentIndex = Math.max(
              0,
              Math.min(parsed.currentIndex ?? 0, parsed.queue.length - 1),
            );
            patch.progress = 0;
            patch.isPlaying = false;
            patch.engineDuration = 0;
          }
        } catch {
          /* повреждённая запись — игнорируем */
        }
      }

      if (Object.keys(patch).length > 0) usePlayerStore.setState(patch);

      // Восстанавливаем последнюю вкладку только с начального маршрута ("/"),
      // чтобы не перебивать явную навигацию, случившуюся до завершения
      // гидратации. location здесь — значение на момент монтирования каркаса.
      if (route !== "" && route !== "/" && location.pathname === "/") {
        navigate(route, { replace: true });
      }

      hydrated.current = true;
    })();
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Персист громкости/mute и очереди с дебаунсом.
  useEffect(() => {
    let volTimer: number | undefined;
    let sessTimer: number | undefined;
    const unsub = usePlayerStore.subscribe((state, prev) => {
      if (!hydrated.current) return;
      if (state.volume !== prev.volume || state.muted !== prev.muted) {
        window.clearTimeout(volTimer);
        volTimer = window.setTimeout(() => {
          void setSetting(KEY_VOLUME, String(state.volume)).catch(() => {});
          void setSetting(KEY_MUTED, state.muted ? "1" : "0").catch(() => {});
        }, 400);
      }
      if (state.queue !== prev.queue || state.currentIndex !== prev.currentIndex) {
        window.clearTimeout(sessTimer);
        sessTimer = window.setTimeout(() => {
          const { queue, currentIndex } = usePlayerStore.getState();
          if (queue.length === 0) {
            void setSetting(KEY_SESSION, "").catch(() => {});
            return;
          }
          const session: PersistedSession = {
            queue: queue.slice(0, MAX_SESSION_TRACKS),
            currentIndex: Math.min(currentIndex, MAX_SESSION_TRACKS - 1),
          };
          void setSetting(KEY_SESSION, JSON.stringify(session)).catch(() => {});
        }, 800);
      }
    });
    return () => {
      window.clearTimeout(volTimer);
      window.clearTimeout(sessTimer);
      unsub();
    };
  }, []);

  // Персист текущей вкладки (маршрута) с дебаунсом.
  useEffect(() => {
    if (!hydrated.current) return;
    const t = window.setTimeout(() => {
      void setSetting(KEY_ROUTE, location.pathname).catch(() => {});
    }, 400);
    return () => window.clearTimeout(t);
  }, [location.pathname]);
}
