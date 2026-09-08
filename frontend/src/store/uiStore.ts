import { create } from "zustand";
import type { Track } from "../shared/types";

interface UiState {
  searchQuery: string;
  setSearchQuery: (q: string) => void;
  /**
   * Ручное сворачивание боковой панели пользователем. На узких экранах панель
   * сворачивается автоматически (см. useMediaQuery), а этот флаг позволяет
   * свернуть её и на широком экране — переключается кнопкой в топбаре.
   */
  sidebarCollapsed: boolean;
  toggleSidebar: () => void;
  setSidebarCollapsed: (v: boolean) => void;
  /**
   * Открыта ли панель «Сейчас играет» как выезжающий overlay. Актуально
   * только на узких экранах, где панель скрыта из основного потока и
   * вызывается кнопкой в топбаре.
   */
  nowPlayingOpen: boolean;
  toggleNowPlaying: () => void;
  setNowPlayingOpen: (v: boolean) => void;
  /** Открыто ли модальное окно эквалайзера. */
  equalizerOpen: boolean;
  toggleEqualizer: () => void;
  setEqualizerOpen: (v: boolean) => void;
  /**
   * Окно «Поделиться треком». Открыто, когда shareOpen=true. shareTrack — трек
   * для отправки (null = режим «получить по ссылке» без исходного трека).
   */
  shareOpen: boolean;
  shareTrack: Track | null;
  openShare: (track?: Track | null) => void;
  closeShare: () => void;
}

export const useUiStore = create<UiState>((set) => ({
  searchQuery: "",
  setSearchQuery: (q) => set({ searchQuery: q }),
  sidebarCollapsed: false,
  toggleSidebar: () => set((s) => ({ sidebarCollapsed: !s.sidebarCollapsed })),
  setSidebarCollapsed: (v) => set({ sidebarCollapsed: v }),
  nowPlayingOpen: false,
  toggleNowPlaying: () => set((s) => ({ nowPlayingOpen: !s.nowPlayingOpen })),
  setNowPlayingOpen: (v) => set({ nowPlayingOpen: v }),
  equalizerOpen: false,
  toggleEqualizer: () => set((s) => ({ equalizerOpen: !s.equalizerOpen })),
  setEqualizerOpen: (v) => set({ equalizerOpen: v }),
  shareOpen: false,
  shareTrack: null,
  openShare: (track) => set({ shareOpen: true, shareTrack: track ?? null }),
  closeShare: () => set({ shareOpen: false, shareTrack: null }),
}));
