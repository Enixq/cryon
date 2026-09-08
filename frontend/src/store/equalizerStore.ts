// Состояние эквалайзера: вкл/выкл, усиления 5 полос и текущий пресет.
//
// Частоты полос фиксированы и совпадают с backend (internal/playback: eqFreqs),
// чтобы один и тот же ползунок означал одно и то же независимо от движка
// (mpv применяет ffmpeg-фильтр, HTML5-фолбэк — Web Audio BiquadFilter).
//
// Настройки хранятся локально (localStorage) — это персональная настройка
// звука на конкретной машине, серверу она не нужна.

import { create } from "zustand";

/** Центральные частоты полос (Гц). Порядок = порядок ползунков и усилений. */
export const EQ_FREQS = [60, 230, 910, 3600, 14000] as const;
/** Подписи полос под ползунками. */
export const EQ_LABELS = ["60", "230", "910", "3.6k", "14k"] as const;
export const EQ_MIN_DB = -12;
export const EQ_MAX_DB = 12;
export const EQ_BANDS = EQ_FREQS.length;

export type EqPresetId = "flat" | "bass" | "vocal" | "rock" | "electronic" | "night" | "custom";

export interface EqPreset {
  id: Exclude<EqPresetId, "custom">;
  name: string;
  gains: number[];
}

// Пресеты подобраны «на слух» для 5 полос 60/230/910/3.6k/14k.
export const EQ_PRESETS: EqPreset[] = [
  { id: "flat", name: "Ровный", gains: [0, 0, 0, 0, 0] },
  { id: "bass", name: "Бас", gains: [8, 5, 0, -1, -2] },
  { id: "vocal", name: "Вокал", gains: [-2, 0, 4, 3, 0] },
  { id: "rock", name: "Рок", gains: [5, 2, -1, 3, 5] },
  { id: "electronic", name: "Электро", gains: [6, 2, -2, 2, 6] },
  { id: "night", name: "Ночь", gains: [4, 1, 0, -2, -5] },
];

const STORAGE_KEY = "cryon.equalizer";

function clampDb(v: number): number {
  if (!Number.isFinite(v)) return 0;
  return Math.max(EQ_MIN_DB, Math.min(EQ_MAX_DB, v));
}

/** Совпадают ли усиления с одним из именованных пресетов (для подсветки). */
function matchPreset(gains: number[]): EqPresetId {
  for (const preset of EQ_PRESETS) {
    if (preset.gains.every((g, i) => g === gains[i])) return preset.id;
  }
  return "custom";
}

interface PersistedEq {
  enabled: boolean;
  gains: number[];
  preset: EqPresetId;
}

function loadInitial(): PersistedEq {
  const fallback: PersistedEq = { enabled: false, gains: [0, 0, 0, 0, 0], preset: "flat" };
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return fallback;
    const parsed = JSON.parse(raw) as Partial<PersistedEq>;
    const gains =
      Array.isArray(parsed.gains) && parsed.gains.length === EQ_BANDS
        ? parsed.gains.map((n) => clampDb(Number(n)))
        : fallback.gains;
    return {
      enabled: Boolean(parsed.enabled),
      gains,
      preset: matchPreset(gains),
    };
  } catch {
    return fallback;
  }
}

function persist(state: PersistedEq): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
  } catch {
    /* приватный режим / отключённое хранилище — не критично */
  }
}

interface EqualizerState extends PersistedEq {
  setEnabled: (v: boolean) => void;
  setBand: (index: number, gain: number) => void;
  applyPreset: (id: Exclude<EqPresetId, "custom">) => void;
  reset: () => void;
}

const initial = loadInitial();

export const useEqualizerStore = create<EqualizerState>((set, get) => ({
  enabled: initial.enabled,
  gains: initial.gains,
  preset: initial.preset,

  setEnabled: (v) => {
    set({ enabled: v });
    const s = get();
    persist({ enabled: s.enabled, gains: s.gains, preset: s.preset });
  },

  setBand: (index, gain) => {
    const gains = get().gains.slice();
    gains[index] = clampDb(gain);
    const preset = matchPreset(gains);
    // Ручная правка ползунка при выключенном EQ автоматически его включает —
    // иначе «кручу, а ничего не меняется» (частая претензия к эквалайзерам).
    set({ gains, preset, enabled: true });
    persist({ enabled: true, gains, preset });
  },

  applyPreset: (id) => {
    const preset = EQ_PRESETS.find((p) => p.id === id);
    if (!preset) return;
    const gains = preset.gains.slice();
    // Пресет «Ровный» ничего не усиливает — его выбор осмысленно оставить EQ
    // включённым (пользователь явно нажал), фильтр всё равно снимется (нули).
    set({ gains, preset: id, enabled: true });
    persist({ enabled: true, gains, preset: id });
  },

  reset: () => {
    const gains = [0, 0, 0, 0, 0];
    set({ gains, preset: "flat", enabled: false });
    persist({ enabled: false, gains, preset: "flat" });
  },
}));
