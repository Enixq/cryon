// Интеграция с системными медиа-контролами и медиа-клавишами.
//
// navigator.mediaSession связывает плеер с ОС: на Windows (WebView2) это
// System Media Transport Controls — всплывающая панель управления с обложкой,
// названием и кнопками, реагирующая на аппаратные медиа-клавиши клавиатуры и
// гарнитур. Мы отдаём ОС метаданные трека, состояние (играет/пауза) и позицию,
// а действия ОС (play/pause/next/prev/seek) прокидываем в playerStore.
//
// Ограничение: SMTC привязывается к реально играющему в странице медиа —
// то есть к HTML5-фолбэку (<audio>). На пути mpv звук идёт мимо WebView, и
// системная панель может не появиться; действия при этом обрабатывает только
// keydown-фолбэк по медиа-клавишам (см. ниже).

import { useEffect } from "react";
import { usePlayerStore } from "../../store/playerStore";
import { useTrackCover } from "./useTrackCover";

// Смещение перемотки по умолчанию для seekbackward/seekforward (сек).
const SEEK_STEP = 10;

export function useMediaSession(): void {
  const track = usePlayerStore((s) => s.queue[s.currentIndex]);
  const isPlaying = usePlayerStore((s) => s.isPlaying);
  const radio = usePlayerStore((s) => s.radio);
  const progress = usePlayerStore((s) => s.progress);
  const duration = usePlayerStore((s) => s.effectiveDuration());
  const coverUrl = useTrackCover(track);

  // Обработчики действий ОС — регистрируем один раз. Внутри читаем актуальный
  // стор через getState(), поэтому пересоздавать их при смене трека не нужно.
  useEffect(() => {
    if (!("mediaSession" in navigator)) return;
    const ms = navigator.mediaSession;
    const store = () => usePlayerStore.getState();
    const handlers: [MediaSessionAction, MediaSessionActionHandler][] = [
      ["play", () => store().play()],
      ["pause", () => store().pause()],
      ["previoustrack", () => store().previous()],
      ["nexttrack", () => store().next()],
      ["stop", () => store().pause()],
      ["seekbackward", (d) => store().seek(store().progress - (d.seekOffset || SEEK_STEP))],
      ["seekforward", (d) => store().seek(store().progress + (d.seekOffset || SEEK_STEP))],
      [
        "seekto",
        (d) => {
          if (typeof d.seekTime === "number") store().seek(d.seekTime);
        },
      ],
    ];
    for (const [action, handler] of handlers) {
      // Не все действия поддерживаются любым движком/ОС — молча пропускаем.
      try {
        ms.setActionHandler(action, handler);
      } catch {
        /* действие не поддерживается */
      }
    }
    return () => {
      for (const [action] of handlers) {
        try {
          ms.setActionHandler(action, null);
        } catch {
          /* игнорируем */
        }
      }
    };
  }, []);

  useEffect(() => {
    const target = window as Window & { __cryonNativeMediaCommand?: (command: string) => void };
    target.__cryonNativeMediaCommand = (command) => {
      const store = usePlayerStore.getState();
      if (command === "play") store.play();
      else if (command === "pause") store.pause();
      else if (command === "next") store.next();
      else if (command === "previous") store.previous();
      else if (command === "favorite") { const current = store.currentTrack(); if (current) store.toggleLikeWithTrack(current); }
      else if (command === "radio") store.toggleRadio();
      else if (command.startsWith("seek:")) store.seek(Number(command.slice(5)) / 1000);
    };
    return () => { delete target.__cryonNativeMediaCommand; };
  }, []);

  // Метаданные текущего трека (название, артист, альбом, обложка).
  useEffect(() => {
    const nativeBridge = (window as Window & { CryonAndroid?: { updateNowPlaying?: (title: string, artist: string, playing: boolean, artworkUrl: string, duration: number, position: number, quality: string, liked: boolean, radio: boolean) => void; clearNowPlaying?: () => void } }).CryonAndroid;
    if (!track) {
      nativeBridge?.clearNowPlaying?.();
      if ("mediaSession" in navigator) navigator.mediaSession.metadata = null;
      return;
    }
    nativeBridge?.updateNowPlaying?.(track.title, track.artist, isPlaying, coverUrl ?? "", duration, progress, track.quality ?? "", Boolean(track.liked), radio);
    if (!("mediaSession" in navigator) || typeof MediaMetadata === "undefined") return;
    navigator.mediaSession.metadata = new MediaMetadata({
      title: track.title,
      artist: track.artist,
      album: track.album ?? "",
      artwork: coverUrl ? [{ src: coverUrl }] : [],
    });
  }, [track?.id, track?.title, track?.artist, track?.album, track?.quality, track?.liked, coverUrl, isPlaying, radio, progress, duration]);

  useEffect(() => {
    const nativeBridge = (window as Window & { CryonAndroid?: { updateNowPlaying?: (title: string, artist: string, playing: boolean, artworkUrl: string, duration: number, position: number, quality: string, liked: boolean, radio: boolean) => void; clearNowPlaying?: () => void } }).CryonAndroid;
    if (track) nativeBridge?.updateNowPlaying?.(track.title, track.artist, isPlaying, coverUrl ?? "", duration, progress, track.quality ?? "", Boolean(track.liked), radio);
    else nativeBridge?.clearNowPlaying?.();
    if (!("mediaSession" in navigator)) return;
    navigator.mediaSession.playbackState = track ? (isPlaying ? "playing" : "paused") : "none";
  }, [isPlaying, track, coverUrl, radio, progress, duration]);
  useEffect(() => {
    if (!("mediaSession" in navigator) || !("setPositionState" in navigator.mediaSession)) return;
    const update = () => {
      const s = usePlayerStore.getState();
      const duration = s.effectiveDuration();
      try {
        if (duration > 0) {
          navigator.mediaSession.setPositionState({
            duration,
            position: Math.min(Math.max(0, s.progress), duration),
            playbackRate: 1,
          });
        } else {
          // Нет достоверной длительности — сбрасываем, иначе ОС покажет мусор.
          navigator.mediaSession.setPositionState();
        }
      } catch {
        /* setPositionState отвергла значения — не критично */
      }
    };
    update();
    return usePlayerStore.subscribe(update);
  }, []);

  // Фолбэк для аппаратных медиа-клавиш: часть окружений доставляет их как
  // обычные keydown (Media*), а не только через mediaSession. Дублирование
  // безопасно — если ОС уже вызвала обработчик действия, второй раз нажатие
  // сюда не придёт.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const s = usePlayerStore.getState();
      switch (e.key) {
        case "MediaPlayPause":
          s.togglePlay();
          break;
        case "MediaTrackNext":
          s.next();
          break;
        case "MediaTrackPrevious":
          s.previous();
          break;
        case "MediaStop":
          s.pause();
          break;
        default:
          return;
      }
      e.preventDefault();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);
}
