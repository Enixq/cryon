import { create } from "zustand";
import type { SourceId, Track } from "../shared/types";

const STORAGE_KEY = "cryon.playback-diagnostics.v1";
const MAX_ITEMS = 30;

export type PlaybackDiagnosticKind = "fallback" | "failed" | "started";

export interface PlaybackDiagnostic {
  id: string;
  kind: PlaybackDiagnosticKind;
  trackId: string;
  title: string;
  artist: string;
  fromSource: SourceId;
  toSource?: SourceId;
  positionSeconds: number;
  message: string;
  createdAt: string;
  fallbackSearchMs?: number;
  startupMs?: number;
}

interface PlaybackDiagnosticsState {
  items: PlaybackDiagnostic[];
  record: (event: Omit<PlaybackDiagnostic, "id" | "createdAt">) => void;
  clear: () => void;
}

function loadItems(): PlaybackDiagnostic[] {
  if (typeof window === "undefined") return [];
  try {
    const value = JSON.parse(window.localStorage.getItem(STORAGE_KEY) || "[]");
    return Array.isArray(value) ? value.slice(0, MAX_ITEMS) : [];
  } catch {
    return [];
  }
}

function persist(items: PlaybackDiagnostic[]) {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(items));
  } catch {
    // Диагностика не должна мешать воспроизведению, если хранилище недоступно.
  }
}

export const usePlaybackDiagnosticsStore = create<PlaybackDiagnosticsState>((set) => ({
  items: loadItems(),

  record: (event) => {
    const item: PlaybackDiagnostic = {
      ...event,
      id: `playback-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
      createdAt: new Date().toISOString(),
    };
    set((state) => {
      const items = [item, ...state.items].slice(0, MAX_ITEMS);
      persist(items);
      return { items };
    });
  },

  clear: () => {
    persist([]);
    set({ items: [] });
  },
}));

export function recordPlaybackDiagnostic(
  kind: PlaybackDiagnosticKind,
  track: Track,
  positionSeconds: number,
  message: string,
  toSource?: SourceId,
  fallbackSearchMs?: number,
) {
  usePlaybackDiagnosticsStore.getState().record({
    kind,
    trackId: track.id,
    title: track.title,
    artist: track.artist,
    fromSource: track.source,
    toSource,
    positionSeconds: Number.isFinite(positionSeconds) ? Math.max(0, positionSeconds) : 0,
    message,
    fallbackSearchMs: fallbackSearchMs !== undefined && Number.isFinite(fallbackSearchMs) ? Math.max(0, Math.round(fallbackSearchMs)) : undefined,
  });
}
