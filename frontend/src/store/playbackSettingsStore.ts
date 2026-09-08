// Настройки воспроизведения, влияющие на движок в реальном времени:
// нормализация громкости и плавный переход между треками («кроссфейд»).
//
// В отличие от эквалайзера (личная настройка машины в localStorage), эти флаги
// уже хранились в SQLite через getSetting/setSetting (ключи playback.*), поэтому
// стор их оттуда и читает — чтобы не терять ранее сохранённые значения. Стор
// нужен, чтобы переключатель на экране настроек сразу влиял на живой движок
// (audioEngine подписан на стор), а не только записывал ключ в базу.

import { create } from "zustand";
import { getSetting, setSetting } from "../shared/api/client";

// Ключи совпадают с теми, что использовал SettingsPage до появления стора.
const KEY_CROSSFADE = "playback.crossfade";
const KEY_NORMALIZE = "playback.normalizeVolume";

// Длительность плавного перехода (мс) для встроенного плеера (HTML5).
export const CROSSFADE_MS = 1200;

interface PlaybackSettingsState {
  crossfade: boolean;
  normalizeVolume: boolean;
  /** Загружены ли значения из хранилища (до этого не персистим правки). */
  hydrated: boolean;
  hydrate: () => Promise<void>;
  setCrossfade: (v: boolean) => void;
  setNormalizeVolume: (v: boolean) => void;
}

export const usePlaybackSettingsStore = create<PlaybackSettingsState>((set, get) => ({
  // Дефолты до гидратации совпадают с прежним поведением SettingsPage:
  // нормализация включена, плавный переход выключен.
  crossfade: false,
  normalizeVolume: true,
  hydrated: false,

  hydrate: async () => {
    if (get().hydrated) return;
    try {
      const [cf, nv] = await Promise.all([getSetting(KEY_CROSSFADE), getSetting(KEY_NORMALIZE)]);
      set({
        // Пустая строка = ключа ещё нет → оставляем дефолт. Формат "1"/"0"
        // совпадает с тем, что писал экран настроек до появления стора.
        crossfade: cf === "" ? get().crossfade : cf === "1",
        normalizeVolume: nv === "" ? get().normalizeVolume : nv === "1",
        hydrated: true,
      });
    } catch {
      set({ hydrated: true });
    }
  },

  setCrossfade: (v) => {
    set({ crossfade: v });
    void setSetting(KEY_CROSSFADE, v ? "1" : "0").catch(() => {});
  },

  setNormalizeVolume: (v) => {
    set({ normalizeVolume: v });
    void setSetting(KEY_NORMALIZE, v ? "1" : "0").catch(() => {});
  },
}));
