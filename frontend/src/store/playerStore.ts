import { create } from "zustand";
import type { RepeatMode, Track } from "../shared/types";
import { clamp } from "../shared/lib/format";
import { initialQueue } from "../mocks/data";
import { isWailsRuntime, addFavorite, removeFavorite } from "../shared/api/client";
import { notify } from "./notificationStore";

// В настоящем приложении (Wails) очередь стартует пустой — иначе Play по
// демо-трекам без реального источника даёт тишину. Мок-очередь нужна только
// для веб-разработки (vite dev без backend), чтобы UI был наполнен.
const startingQueue: Track[] = isWailsRuntime() ? [] : initialQueue;

interface PlayerState {
  queue: Track[];
  /** Индекс текущего трека в очереди. */
  currentIndex: number;
  isPlaying: boolean;
  /** Прогресс воспроизведения текущего трека в секундах. */
  progress: number;
  /** Громкость 0..100. */
  volume: number;
  muted: boolean;
  shuffle: boolean;
  repeat: RepeatMode;
  /**
   * Режим радио (умная очередь): когда очередь заканчивается, плеер сам
   * дозаполняет её треками, похожими на текущий (через backend/Last.fm).
   */
  radio: boolean;

  /** Цель перемотки в секундах и счётчик запросов (для движка звука). */
  seekTarget: number;
  seekNonce: number;

  // Производные
  currentTrack: () => Track | undefined;

  // Управление воспроизведением
  play: () => void;
  pause: () => void;
  togglePlay: () => void;
  next: () => void;
  previous: () => void;
  seek: (seconds: number) => void;
  /** Позиция от движка воспроизведения (mpv или <audio>). */
  setEnginePosition: (seconds: number) => void;
  /** Реальная длительность трека, определённая движком (mpv/HTML5). */
  engineDuration: number;
  setEngineDuration: (id: string, seconds: number) => void;
  /** Эффективная длительность текущего трека: движок → метаданные → 0. */
  effectiveDuration: () => number;

  // Громкость
  setVolume: (value: number) => void;
  nudgeVolume: (delta: number) => void;
  toggleMute: () => void;

  // Режимы
  toggleShuffle: () => void;
  cycleRepeat: () => void;
  toggleRadio: () => void;

  // Очередь
  playTrack: (track: Track, queue?: Track[]) => void;
  playAt: (index: number) => void;
  /** Заменить текущую запись очереди на найденную воспроизводимую версию. */
  replaceCurrentTrack: (track: Track) => void;
  removeFromQueue: (id: string) => void;
  clearQueue: () => void;
  addToQueue: (track: Track) => void;
  /** Добавить несколько треков в конец очереди (дедуп по id). Для радио. */
  appendToQueue: (tracks: Track[]) => void;
  /** Поставить трек следующим после текущего. */
  playNext: (track: Track) => void;
  /** Переключить лайк с сохранением в избранное (backend). */
  toggleLikeWithTrack: (track: Track) => void;
}

export const usePlayerStore = create<PlayerState>((set, get) => ({
  queue: startingQueue,
  currentIndex: 0,
  isPlaying: false,
  progress: 0,
  volume: 68,
  muted: false,
  shuffle: false,
  repeat: "off",
  radio: false,
  seekTarget: 0,
  seekNonce: 0,
  engineDuration: 0,

  currentTrack: () => {
    const { queue, currentIndex } = get();
    return queue[currentIndex];
  },

  play: () => set({ isPlaying: true }),
  pause: () => set({ isPlaying: false }),
  togglePlay: () => set((s) => ({ isPlaying: !s.isPlaying })),

  next: () => {
    const { queue, currentIndex, repeat, shuffle } = get();
    if (queue.length === 0) return;
    if (repeat === "one") {
      // Перезапуск текущего трека. Одного set({progress:0}) недостаточно:
      // движок звука реагирует только на seekNonce, поэтому раньше полоска
      // прогресса прыгала в начало, а звук продолжал играть с того же места.
      set({ isPlaying: true });
      get().seek(0);
      return;
    }
    let nextIndex: number;
    if (shuffle) {
      nextIndex = Math.floor(Math.random() * queue.length);
    } else {
      nextIndex = currentIndex + 1;
      if (nextIndex >= queue.length) {
        nextIndex = repeat === "all" ? 0 : currentIndex;
      }
    }
    set({ currentIndex: nextIndex, progress: 0, isPlaying: true, engineDuration: 0 });
  },

  previous: () => {
    const { queue, currentIndex, progress } = get();
    if (queue.length === 0) return;
    // Если прошло больше 3 секунд — перемотка в начало трека. Через seek(),
    // иначе движок звука не получает сигнал и трек продолжает играть дальше.
    if (progress > 3) {
      get().seek(0);
      return;
    }
    const prevIndex = currentIndex - 1 < 0 ? 0 : currentIndex - 1;
    set({ currentIndex: prevIndex, progress: 0, isPlaying: true, engineDuration: 0 });
  },

  seek: (seconds) => {
    const max = get().effectiveDuration();
    const target = clamp(seconds, 0, max);
    // Обновляем прогресс сразу для отзывчивости и сигналим движку звука.
    set((s) => ({ progress: target, seekTarget: target, seekNonce: s.seekNonce + 1 }));
  },

  // Позиция, пришедшая от реального движка воспроизведения.
  setEnginePosition: (seconds) => {
    const max = get().effectiveDuration();
    set({ progress: clamp(seconds, 0, max || seconds) });
  },

  // Длительность, определённая движком (mpv `durationS` или <audio>.duration).
  // Это авторитетный источник: метаданные поиска у стримов (YouTube/SoundCloud/
  // Yandex) часто неточны или нулевые, а у локальных файлов теги нередко вовсе
  // не содержат длину. Пишем только для текущего трека и только при изменении,
  // чтобы тикер (каждые 500 мс) не порождал лишних ре-рендеров подписчиков.
  setEngineDuration: (id, seconds) => {
    if (!Number.isFinite(seconds) || seconds <= 0) return;
    const rounded = Math.round(seconds);
    set((s) => {
      if (s.currentTrack()?.id !== id || s.engineDuration === rounded) return {};
      return { engineDuration: rounded };
    });
  },

  // Эффективная длительность текущего трека: приоритет у движка, метаданные
  // поиска — фолбэк, пока движок не сообщил реальную длину.
  effectiveDuration: () => {
    const { engineDuration, currentTrack } = get();
    return engineDuration || currentTrack()?.duration || 0;
  },

  setVolume: (value) => set({ volume: clamp(Math.round(value), 0, 100), muted: false }),
  nudgeVolume: (delta) =>
    set((s) => ({ volume: clamp(s.volume + delta, 0, 100), muted: false })),
  toggleMute: () => set((s) => ({ muted: !s.muted })),

  toggleShuffle: () => set((s) => ({ shuffle: !s.shuffle })),
  cycleRepeat: () =>
    set((s) => ({
      repeat: s.repeat === "off" ? "all" : s.repeat === "all" ? "one" : "off",
    })),
  toggleRadio: () => set((s) => ({ radio: !s.radio })),

  playTrack: (track, queue) => {
    // История пишется в одном месте — в audioEngine, когда трек реально
    // стартует. Раньше запись дублировалась ещё и здесь, и каждое
    // прослушивание попадало в SQLite дважды: список истории показывал
    // дубли, а профиль вкусов удваивал вес артиста.
    if (queue && queue.length > 0) {
      const index = Math.max(0, queue.findIndex((t) => t.id === track.id));
      set({ queue, currentIndex: index, progress: 0, isPlaying: true, engineDuration: 0 });
      return;
    }
    // Добавляем в текущую очередь, если трека там ещё нет.
    const existing = get().queue.findIndex((t) => t.id === track.id);
    if (existing >= 0) {
      set({ currentIndex: existing, progress: 0, isPlaying: true, engineDuration: 0 });
    } else {
      set((s) => ({
        queue: [...s.queue, track],
        currentIndex: s.queue.length,
        progress: 0,
        isPlaying: true,
        engineDuration: 0,
      }));
    }
  },

  playAt: (index) => {
    const { queue } = get();
    if (index < 0 || index >= queue.length) return;
    set({ currentIndex: index, progress: 0, isPlaying: true, engineDuration: 0 });
  },

  replaceCurrentTrack: (track) =>
    set((s) => {
      if (!s.queue[s.currentIndex]) return {};
      const queue = [...s.queue];
      queue[s.currentIndex] = track;
      return { queue };
    }),

  removeFromQueue: (id) => {
    const { queue, currentIndex } = get();
    const idx = queue.findIndex((t) => t.id === id);
    if (idx < 0) return;
    const nextQueue = queue.filter((t) => t.id !== id);
    // При удалении последнего трека не оставляем плеер в состоянии
    // "воспроизводится" без текущего элемента очереди.
    if (nextQueue.length === 0) {
      set({ queue: [], currentIndex: 0, progress: 0, isPlaying: false, engineDuration: 0 });
      return;
    }
    let nextIndex = currentIndex;
    if (idx < currentIndex) nextIndex = currentIndex - 1;
    if (idx === currentIndex) nextIndex = clamp(currentIndex, 0, Math.max(0, nextQueue.length - 1));
    set({ queue: nextQueue, currentIndex: nextIndex, engineDuration: idx === currentIndex ? 0 : get().engineDuration });
  },

  clearQueue: () => {
    const track = get().currentTrack();
    set({ queue: track ? [track] : [], currentIndex: 0 });
  },

  addToQueue: (track) =>
    set((s) =>
      s.queue.some((t) => t.id === track.id) ? s : { queue: [...s.queue, track] },
    ),

  appendToQueue: (tracks) =>
    set((s) => {
      const existing = new Set(s.queue.map((t) => t.id));
      const fresh = tracks.filter((t) => !existing.has(t.id));
      if (fresh.length === 0) return {};
      return { queue: [...s.queue, ...fresh] };
    }),

  playNext: (track) =>
    set((s) => {
      const filtered = s.queue.filter((t) => t.id !== track.id);
      // Индекс текущего мог сдвинуться, если дубликат стоял до него.
      const removedBefore = s.queue
        .slice(0, s.currentIndex)
        .filter((t) => t.id === track.id).length;
      const insertAt = s.currentIndex - removedBefore + 1;
      const next = [...filtered.slice(0, insertAt), track, ...filtered.slice(insertAt)];
      return { queue: next, currentIndex: s.currentIndex - removedBefore };
    }),

  // Лайк с синхронизацией в backend: локальный флаг переключается сразу
  // (оптимистично), а при сбое записи откатывается — иначе сердечко оставалось
  // закрашенным, хотя избранное не сохранилось.
  toggleLikeWithTrack: (track) => {
    const willLike = !track.liked;
    const setLiked = (liked: boolean) =>
      set((s) => ({
        queue: s.queue.map((t) => (t.id === track.id ? { ...t, liked } : t)),
      }));
    setLiked(willLike);
    const request = willLike ? addFavorite(track) : removeFavorite(track.id);
    void request.catch(() => {
      setLiked(!willLike);
      notify({
        kind: "error",
        title: willLike ? "Не удалось добавить в избранное" : "Не удалось убрать из избранного",
        message: `${track.artist} — ${track.title}`,
      });
    });
  },
}));

/**
 * Состояние плеера для страниц со списками треков: запуск трека, флаг
 * воспроизведения и id активного трека (для подсветки строки).
 *
 * Здесь именно отдельные селекторы, а не `usePlayerStore()` целиком: полная
 * подписка перерисовывала страницу на каждой записи в стор, а движок звука
 * пишет позицию каждые 500 мс — то есть весь список треков перерисовывался
 * дважды в секунду всё время, пока играет музыка. Все три значения —
 * примитивы или стабильные ссылки на экшены, поэтому shallow-сравнение не нужно.
 */
export function useTrackListPlayer() {
  const playTrack = usePlayerStore((s) => s.playTrack);
  const isPlaying = usePlayerStore((s) => s.isPlaying);
  const activeTrackId = usePlayerStore((s) => s.queue[s.currentIndex]?.id);
  return { playTrack, isPlaying, activeTrackId };
}
