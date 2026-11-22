import { useEffect, useLayoutEffect, useRef, useState } from "react";
import type { TouchEvent } from "react";
import {
  ChevronDown,
  Heart,
  ThumbsDown,
  ListMusic,
  Radio,
  Pause,
  Play,
  Repeat,
  Repeat1,
  Shuffle,
  Share2,
  SkipBack,
  SkipForward,
  SlidersHorizontal,
  Trash2,
  X,
} from "lucide-react";
import { usePlayerStore } from "../store/playerStore";
import { useUiStore } from "../store/uiStore";
import { Slider } from "../shared/ui/Slider";
import { Cover } from "../shared/ui/Cover";
import { formatDuration, pluralWithCount } from "../shared/lib/format";
import { useTrackCover } from "../shared/lib/useTrackCover";
import { LyricsSources } from "../shared/ui/LyricsSources";
import { useLyrics } from "../shared/lib/useLyrics";
import { activeLineIndex } from "../shared/lib/lyrics";
import { useCoverPalette } from "../shared/lib/useCoverPalette";
import { useFavoriteIds } from "../shared/lib/useFavorites";
import { useRecoFeedback } from "../shared/lib/useRecoFeedback";
import { sourceName } from "../shared/sources";
import { cn } from "../shared/lib/cn";
import { ArtistLink } from "../shared/ui/ArtistLink";
import { TrackQuickActions } from "../shared/ui/TrackQuickActions";
import type { Track } from "../shared/types";

/** Keep the sheet present until its closing transition finishes. */
export function MobileNowPlayingSheet({ open }: { open: boolean }) {
  const [present, setPresent] = useState(open);
  useEffect(() => {
    if (open) {
      setPresent(true);
      return;
    }
    const timer = window.setTimeout(() => setPresent(false), 380);
    return () => window.clearTimeout(timer);
  }, [open]);
  return open || present ? <MobileNowPlaying open={open} /> : null;
}

/**
 * Полноэкранный плеер «Сейчас играет» на телефоне (мобильный аналог
 * [NowPlayingPanel](NowPlayingPanel.tsx)). Открывается тапом по мини-плееру,
 * сворачивается шевроном вниз. Крупная обложка, перемотка слайдером, полный
 * транспорт и лайк; кнопка-список переключает середину на очередь. Появляется
 * плавным листом; обложка, очередь и текст следуют за вертикальным жестом.
 */
export function MobileNowPlaying({ open = true }: { open?: boolean }) {
  const {
    queue,
    currentIndex,
    isPlaying,
    progress,
    shuffle,
    repeat,
    radio,
    togglePlay,
    next,
    previous,
    seek,
    playAt,
    removeFromQueue,
    toggleShuffle,
    cycleRepeat,
    toggleRadio,
    toggleLikeWithTrack,
    engineDuration,
  } = usePlayerStore();
  const setNowPlayingOpen = useUiStore((s) => s.setNowPlayingOpen);
  const setEqualizerOpen = useUiStore((s) => s.setEqualizerOpen);

  const track = queue[currentIndex];
  const coverUrl = useTrackCover(track);
  // Акцент подстраивается под обложку текущего трека (кнопка play, фон-свечение).
  const accent = useCoverPalette(coverUrl);
  const favoriteIds = useFavoriteIds();
  const feedback = useRecoFeedback();
  const isFavorite = track ? favoriteIds.has(track.id) : false;
  const [seekPreview, setSeekPreview] = useState<{ id: string; value: number } | null>(null);
  const displayProgress = seekPreview?.id === track?.id ? seekPreview!.value : progress;
  const view = useUiStore((s) => s.nowPlayingView);
  const showQueue = view === "queue";
  const showLyrics = view === "lyrics";
  const setShowQueue = (value: boolean | ((current: boolean) => boolean)) =>
    useUiStore
      .getState()
      .setNowPlayingView(
        (typeof value === "function" ? value(showQueue) : value) ? "queue" : "cover",
      );
  const setShowLyrics = (value: boolean | ((current: boolean) => boolean)) =>
    useUiStore
      .getState()
      .setNowPlayingView(
        (typeof value === "function" ? value(showLyrics) : value) ? "lyrics" : "cover",
      );
  const lyrics = useLyrics(track);
  const activeLyric = activeLineIndex(lyrics.lines, progress);

  const rootRef = useRef<HTMLDivElement>(null);
  const viewportRef = useRef<HTMLDivElement>(null);
  const coverPageRef = useRef<HTMLDivElement>(null);
  const [extraView, setExtraView] = useState<"queue" | "lyrics">(showLyrics ? "lyrics" : "queue");
  const [entered, setEntered] = useState(false);
  const [pageOffset, setPageOffset] = useState<number | null>(null);
  const drag = useRef<{
    startX: number;
    startY: number;
    started: number;
    active: boolean;
    y: number;
    base: number;
    extra: boolean;
    scroll: HTMLElement | null;
  } | null>(null);
  const suppressClickUntil = useRef(0);
  const coverDrag = useRef<{ x: number; y: number; delta: number; horizontal: boolean } | null>(
    null,
  );
  const [dragY, setDragY] = useState(0);
  const [dragging, setDragging] = useState(false);
  const [coverOffset, setCoverOffset] = useState(0);
  const [coverAnimating, setCoverAnimating] = useState(false);
  const coverTimer = useRef<number | undefined>(undefined);
  const coverFrame = useRef<number | undefined>(undefined);
  useEffect(
    () => () => {
      window.clearTimeout(coverTimer.current);
      if (coverFrame.current !== undefined) cancelAnimationFrame(coverFrame.current);
    },
    [],
  );

  useEffect(() => {
    const frame = requestAnimationFrame(() => setEntered(true));
    return () => cancelAnimationFrame(frame);
  }, []);

  useLayoutEffect(() => {
    if (view !== "cover") setExtraView(view);
  }, [view]);

  const pagePosition = pageOffset ?? (view === "cover" ? 0 : 1);
  const pageTransition = dragging ? "none" : "transform 360ms cubic-bezier(.22, 1, .36, 1)";

  function onTouchStart(e: TouchEvent<HTMLDivElement>) {
    drag.current = null;
    if (!open || e.touches.length !== 1) return;
    const target = e.target as HTMLElement;
    if (target.closest?.("[data-noswipe], input, a")) return;
    const scroll = target.closest<HTMLElement>("[data-player-scroll]");
    if (!scroll && target.closest?.("button")) return;
    const t = e.touches[0];
    const height = viewportRef.current?.clientHeight || window.innerHeight;
    const matrix = coverPageRef.current ? getComputedStyle(coverPageRef.current).transform : "none";
    // A new gesture can catch a page while its previous transition is settling.
    const values = matrix
      .match(/^matrix(?:3d)?\((.+)\)$/)?.[1]
      .split(",")
      .map(Number);
    const visualPosition = values
      ? -(values.length === 16 ? values[13] : values[5]) / height
      : pagePosition;
    drag.current = {
      startX: t.clientX,
      startY: t.clientY,
      started: performance.now(),
      active: false,
      y: 0,
      base: Math.max(0, Math.min(1, visualPosition)),
      extra: view !== "cover",
      scroll,
    };
  }

  function onTouchMove(e: globalThis.TouchEvent) {
    const st = drag.current;
    if (!st) return;
    if (e.touches.length !== 1) {
      cancelDrag();
      return;
    }
    const t = e.touches[0];
    const dx = t.clientX - st.startX;
    const dy = t.clientY - st.startY;
    if (!st.active) {
      if (Math.abs(dx) < 8 && Math.abs(dy) < 8) return;
      if (
        Math.abs(dy) > Math.abs(dx) * 1.2 &&
        (!st.extra || (dy > 0 && (!st.scroll || st.scroll.scrollTop <= 0)))
      ) {
        st.active = true;
        setDragging(true);
      } else {
        drag.current = null;
        return;
      }
    }
    if (e.cancelable) e.preventDefault();
    st.y = dy;
    const height = viewportRef.current?.clientHeight || window.innerHeight;
    if (!st.extra && dy > 0 && st.base < 0.01) {
      setDragY(dy);
      setPageOffset(0);
    } else {
      setDragY(0);
      setPageOffset(Math.max(0, Math.min(1, st.base - dy / height)));
    }
  }

  // React's delegated touchmove is passive in Chromium. A native non-passive
  // listener lets the sheet own vertical dragging, including at list scrollTop=0.
  const touchMoveRef = useRef(onTouchMove);
  touchMoveRef.current = onTouchMove;
  useEffect(() => {
    const root = rootRef.current;
    if (!root) return;
    const move = (event: globalThis.TouchEvent) => touchMoveRef.current(event);
    root.addEventListener("touchmove", move, { passive: false });
    return () => root.removeEventListener("touchmove", move);
  }, []);

  function cancelDrag() {
    drag.current = null;
    setDragging(false);
    setDragY(0);
    setPageOffset(null);
  }

  function endDrag() {
    const st = drag.current;
    drag.current = null;
    setDragging(false);
    if (st?.active) {
      suppressClickUntil.current = performance.now() + 350;
      const height = viewportRef.current?.clientHeight || window.innerHeight;
      const speed = Math.abs(st.y) / Math.max(1, performance.now() - st.started);
      const commit =
        Math.abs(st.y) > Math.min(120, height * 0.2) || (Math.abs(st.y) > 45 && speed > 0.55);
      if (commit) {
        if (st.extra) useUiStore.getState().setNowPlayingView("cover");
        else if (st.y < 0) setShowQueue(true);
        else setNowPlayingOpen(false);
      }
    }
    setDragY(0);
    setPageOffset(null);
  }

  function onCoverTouchStart(event: TouchEvent<HTMLDivElement>) {
    coverDrag.current = null;
    if (coverAnimating || event.touches.length !== 1) return;
    const touch = event.touches[0];
    if (touch)
      coverDrag.current = { x: touch.clientX, y: touch.clientY, delta: 0, horizontal: false };
  }

  function onCoverTouchMove(event: TouchEvent<HTMLDivElement>) {
    if (event.touches.length !== 1) {
      coverDrag.current = null;
      setCoverOffset(0);
      return;
    }
    const gesture = coverDrag.current;
    const touch = event.touches[0];
    if (!gesture || !touch || drag.current?.active) return;
    const dx = touch.clientX - gesture.x;
    const dy = touch.clientY - gesture.y;
    if (!gesture.horizontal && Math.abs(dx) > 12 && Math.abs(dx) > Math.abs(dy) * 1.2)
      gesture.horizontal = true;
    if (gesture.horizontal) {
      drag.current = null;
      gesture.delta = dx;
      setCoverOffset(Math.max(-180, Math.min(180, dx)));
      event.stopPropagation();
    }
  }

  function onCoverTouchEnd(event: TouchEvent<HTMLDivElement>) {
    const gesture = coverDrag.current;
    coverDrag.current = null;
    if (!gesture) return;
    if (!gesture.horizontal || Math.abs(gesture.delta) < 55) {
      setCoverAnimating(true);
      setCoverOffset(0);
      window.clearTimeout(coverTimer.current);
      coverTimer.current = window.setTimeout(() => setCoverAnimating(false), 220);
      return;
    }
    event.stopPropagation();
    endDrag();
    const direction = gesture.delta < 0 ? -1 : 1;
    if (direction > 0 && currentIndex === 0 && repeat !== "all") {
      setCoverAnimating(true);
      setCoverOffset(0);
      coverTimer.current = window.setTimeout(() => setCoverAnimating(false), 220);
      return;
    }
    setCoverAnimating(true);
    suppressClickUntil.current = performance.now() + 400;
    setCoverOffset(direction * Math.max(window.innerWidth, 320));
    coverTimer.current = window.setTimeout(() => {
      if (direction < 0) next();
      else if (currentIndex > 0) playAt(currentIndex - 1);
      else if (repeat === "all" && queue.length > 1) playAt(queue.length - 1);
      setCoverAnimating(false);
      setCoverOffset(-direction * Math.max(window.innerWidth, 320));
      coverFrame.current = requestAnimationFrame(() => {
        coverFrame.current = requestAnimationFrame(() => {
          setCoverAnimating(true);
          setCoverOffset(0);
          coverTimer.current = window.setTimeout(() => setCoverAnimating(false), 220);
        });
      });
    }, 180);
  }

  if (!track) return null;

  const duration = track ? engineDuration || track.duration : 0;

  return (
    <div
      ref={rootRef}
      data-player-sheet
      aria-hidden={!open}
      inert={!open}
      className="no-tap-highlight fixed inset-0 z-40 flex flex-col overflow-hidden"
      style={{
        background: `radial-gradient(130% 60% at 50% -10%, color-mix(in srgb, ${accent} 32%, transparent) 0%, transparent 60%), var(--bg-0)`,
        transform: !open || !entered ? "translate3d(0, 100%, 0)" : `translate3d(0, ${dragY}px, 0)`,
        transition: pageTransition,
      }}
      onTouchStart={onTouchStart}
      onTouchEnd={endDrag}
      onTouchCancel={cancelDrag}
      onClickCapture={(event) => {
        if (performance.now() < suppressClickUntil.current) {
          event.preventDefault();
          event.stopPropagation();
        }
      }}
    >
      {/* Верхняя панель: «ручка» для свайпа вниз + свернуть / источник / очередь */}
      <div className="shrink-0 pt-[calc(8px+var(--safe-top))]" style={{ touchAction: "pan-x" }}>
        <div className="mx-auto mb-2 h-1.5 w-10 rounded-full bg-white/20" aria-hidden />
        <div className="flex items-center justify-between px-3">
          <button
            type="button"
            onClick={() =>
              view === "cover"
                ? setNowPlayingOpen(false)
                : useUiStore.getState().setNowPlayingView("cover")
            }
            className="grid h-11 w-11 place-items-center rounded-full text-slate-200 transition-colors active:bg-white/10"
            aria-label="Свернуть плеер"
          >
            <ChevronDown size={26} />
          </button>
          <button
            type="button"
            onClick={() => track && useUiStore.getState().openSourcePicker(track)}
            aria-label="Сменить источник трека"
            className="text-xs font-semibold uppercase tracking-wider text-slate-400"
          >
            {track ? sourceName(track.source) : "Сейчас играет"}
          </button>
          <button
            type="button"
            onClick={() => setShowQueue((v) => !v)}
            className={cn(
              "grid h-11 w-11 place-items-center rounded-full transition-colors active:bg-white/10",
              showQueue ? "text-[var(--app-accent)]" : "text-slate-200",
            )}
            aria-label="Очередь воспроизведения"
          >
            <ListMusic size={22} />
          </button>
        </div>
      </div>

      {track ? (
        <div ref={viewportRef} className="relative min-h-0 flex-1 overflow-hidden">
          <div
            ref={coverPageRef}
            data-player-page="cover"
            aria-hidden={view !== "cover"}
            inert={view !== "cover"}
            className="absolute inset-0 flex min-h-0 flex-col px-6 pb-[calc(12px+var(--safe-bottom))] pt-2"
            style={{
              transform: `translate3d(0, ${-pagePosition * 100}%, 0)`,
              transition: pageTransition,
              touchAction: "pan-x",
            }}
          >
            <div
              role="group"
              aria-label="Обложка: свайп для смены трека"
              className="flex min-h-0 flex-1 items-center justify-center overflow-hidden"
              style={{ touchAction: "none" }}
              onTouchStart={onCoverTouchStart}
              onTouchMove={onCoverTouchMove}
              onTouchEnd={onCoverTouchEnd}
              onTouchCancel={() => {
                coverDrag.current = null;
                setCoverOffset(0);
              }}
            >
              <Cover
                accent={track.accent}
                src={coverUrl}
                alt={track.title}
                className="aspect-square h-full max-h-[300px] w-auto max-w-full shadow-2xl sm:max-h-[340px]"
                rounded="rounded-[28px]"
                iconSize={80}
                style={{
                  transform: `translate3d(${coverOffset}px, 0, 0) rotate(${coverOffset / 45}deg)`,
                  opacity: Math.max(0.35, 1 - Math.abs(coverOffset) / 260),
                  transition: coverAnimating ? "transform 180ms ease, opacity 180ms ease" : "none",
                }}
              />
            </div>

            {/* Название + исполнитель + лайк */}
            <div className={cn("mt-3 flex items-center justify-between gap-3 sm:mt-5")}>
              <div className="min-w-0">
                <h2
                  className="whitespace-normal break-words text-xl font-bold leading-tight text-white sm:text-2xl"
                  title={track.title}
                >
                  {track.title}
                </h2>
                <p className="flex flex-wrap items-center gap-1.5 text-sm text-slate-400">
                  <ArtistLink
                    name={track.artist}
                    onNavigate={() => useUiStore.getState().setNowPlayingOpen(false)}
                  />
                  {track.quality && (
                    <span className="rounded-full bg-white/10 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-slate-300">
                      {track.quality}
                    </span>
                  )}
                </p>
              </div>
              <button
                type="button"
                disabled={feedback.saving}
                onClick={() => feedback.rate(track, -1)}
                aria-label={
                  feedback.scoreFor(track) < 0
                    ? "Вернуть трек в рекомендации"
                    : "Не рекомендовать трек"
                }
                aria-pressed={feedback.scoreFor(track) < 0}
                className={cn(
                  "shrink-0",
                  feedback.scoreFor(track) < 0 ? "text-rose-400" : "text-slate-400",
                )}
              >
                <ThumbsDown size={23} />
              </button>
              <button
                type="button"
                onClick={() => toggleLikeWithTrack({ ...track, liked: isFavorite })}
                className={cn(
                  "shrink-0 transition-colors",
                  isFavorite ? "text-[var(--app-accent)]" : "text-slate-400 active:text-white",
                )}
                aria-label="В избранное"
              >
                <Heart size={26} fill={isFavorite ? "currentColor" : "none"} />
              </button>
            </div>

            {/* Перемотка */}
            <div className={cn("mt-3 flex flex-col gap-1.5 sm:mt-4")} data-noswipe>
              <Slider
                key={track.id}
                value={displayProgress}
                max={duration || 100}
                visibleThumb
                onChange={(value) => setSeekPreview({ id: track.id, value })}
                onCommit={(value) => {
                  seek(value);
                  setSeekPreview(null);
                }}
                onCancel={() => setSeekPreview(null)}
                ariaLabel="Перемотка воспроизведения"
              />
              <div className="flex justify-between text-xs tabular-nums text-slate-400">
                <span>{formatDuration(displayProgress)}</span>
                <span>{formatDuration(duration)}</span>
              </div>
            </div>

            {/* Транспорт */}
            <div className={cn("mt-3 flex items-center justify-between")} data-noswipe>
              <button
                type="button"
                onClick={toggleShuffle}
                className="grid h-11 w-11 place-items-center transition-colors"
                style={{ color: shuffle ? "var(--app-accent)" : undefined }}
                aria-label="Перемешать"
              >
                <Shuffle size={22} className={shuffle ? undefined : "text-slate-400"} />
              </button>
              <button
                type="button"
                onClick={previous}
                className="grid h-12 w-12 place-items-center text-white transition-transform active:scale-90"
                aria-label="Предыдущий"
              >
                <SkipBack size={30} fill="currentColor" />
              </button>
              <button
                type="button"
                onClick={togglePlay}
                className="grid h-16 w-16 place-items-center rounded-full text-white shadow-xl transition-transform active:scale-95"
                style={{ backgroundColor: accent, boxShadow: `0 16px 36px -12px ${accent}` }}
                aria-label={isPlaying ? "Пауза" : "Воспроизвести"}
              >
                {isPlaying ? (
                  <Pause size={30} fill="currentColor" />
                ) : (
                  <Play size={30} fill="currentColor" className="ml-1" />
                )}
              </button>
              <button
                type="button"
                onClick={next}
                className="grid h-12 w-12 place-items-center text-white transition-transform active:scale-90"
                aria-label="Следующий"
              >
                <SkipForward size={30} fill="currentColor" />
              </button>
              <button
                type="button"
                onClick={cycleRepeat}
                className="grid h-11 w-11 place-items-center transition-colors"
                style={{ color: repeat !== "off" ? "var(--app-accent)" : undefined }}
                aria-label="Повтор"
              >
                {repeat === "one" ? (
                  <Repeat1 size={22} className="text-slate-200" />
                ) : (
                  <Repeat size={22} className={repeat !== "off" ? undefined : "text-slate-400"} />
                )}
              </button>
            </div>

            {/* Второстепенные действия */}
            <div
              data-noswipe
              className={cn("mt-3 grid shrink-0 grid-cols-6 items-center text-slate-400")}
            >
              <button
                type="button"
                onClick={() => setEqualizerOpen(true)}
                className="grid min-h-11 place-items-center rounded-full transition-colors active:bg-white/10 active:text-white"
                aria-label="Эквалайзер"
              >
                <SlidersHorizontal size={20} />
              </button>
              <button
                type="button"
                onClick={toggleRadio}
                className={cn(
                  "grid min-h-11 place-items-center rounded-full transition-colors active:bg-white/10 active:text-white",
                  radio && "text-[var(--app-accent)]",
                )}
                aria-label="Радио по треку"
              >
                <Radio size={20} />
              </button>
              <button
                type="button"
                onClick={() => setShowQueue((v) => !v)}
                className={cn(
                  "grid min-h-11 place-items-center rounded-full transition-colors active:bg-white/10 active:text-white",
                  showQueue && "text-[var(--app-accent)]",
                )}
                aria-label="Очередь"
              >
                <ListMusic size={20} />
              </button>
              <button
                type="button"
                onClick={() => setShowLyrics((value) => !value)}
                className={cn(
                  "grid min-h-11 place-items-center rounded-full transition-colors active:bg-white/10 active:text-white",
                  showLyrics && "text-[var(--app-accent)]",
                )}
                aria-label="Текст песни"
              >
                <span className="text-base leading-none">Aa</span>
              </button>
              <button
                type="button"
                onClick={() => useUiStore.getState().openShare(track)}
                aria-label="Поделиться треком"
                title="Поделиться треком"
                className="grid min-h-11 place-items-center rounded-full transition-colors active:bg-white/10 active:text-white"
              >
                <Share2 size={20} />
              </button>
              <TrackQuickActions track={track} playerControl />
            </div>
          </div>
          <div
            data-player-page="extra"
            aria-hidden={view === "cover"}
            inert={view === "cover"}
            className="absolute inset-0 flex min-h-0 flex-col px-6 pb-[calc(18px+var(--safe-bottom))] pt-2"
            style={{
              transform: `translate3d(0, ${(1 - pagePosition) * 100}%, 0)`,
              transition: pageTransition,
            }}
          >
            <div
              className="mb-3 flex shrink-0 items-center justify-between"
              style={{ touchAction: "pan-x" }}
            >
              <h2 className="text-xl font-bold text-white">
                {extraView === "queue" ? "Очередь" : "Текст песни"}
              </h2>
              <button
                type="button"
                onClick={() => useUiStore.getState().setNowPlayingView("cover")}
                aria-label="Вернуться к обложке"
                className="flex min-h-11 items-center gap-1 rounded-full px-3 text-xs text-slate-400 active:bg-white/10"
              >
                <ChevronDown size={18} /> К обложке
              </button>
            </div>
            {extraView === "lyrics" ? (
              <div
                data-player-scroll
                className="min-h-0 flex-1 overflow-y-auto overscroll-contain rounded-2xl bg-white/[0.03] px-4 py-5"
              >
                <LyricsSources lyrics={lyrics} hasSource={track.source === "yandex"} />
                {lyrics.loading ? (
                  <div className="grid min-h-[220px] place-items-center text-sm text-slate-500">
                    Загрузка текста…
                  </div>
                ) : lyrics.instrumental ? (
                  <div className="grid min-h-[220px] place-items-center text-sm text-slate-500">
                    Инструментальная композиция
                  </div>
                ) : lyrics.lines.length > 0 ? (
                  <div className="space-y-3">
                    {lyrics.lines.map((line, index) => (
                      <button
                        key={`${line.time}-${index}`}
                        type="button"
                        onClick={() => seek(line.time)}
                        className={cn(
                          "block w-full text-left text-lg font-semibold transition-colors",
                          index === activeLyric ? "text-white" : "text-slate-500",
                        )}
                      >
                        {line.text || "…"}
                      </button>
                    ))}
                  </div>
                ) : (
                  <div className="grid min-h-[220px] place-items-center whitespace-pre-wrap text-center text-sm text-slate-500">
                    {lyrics.found && lyrics.plain ? lyrics.plain : "Текст не найден"}
                  </div>
                )}
              </div>
            ) : (
              <div className="flex min-h-0 flex-1 flex-col" key="queue">
                <div
                  data-player-scroll
                  className="-mr-2 flex min-h-0 flex-1 flex-col gap-1 overflow-y-auto overscroll-contain pr-2"
                >
                  {queue.map((item, index) => (
                    <QueueRow
                      key={item.id}
                      item={item}
                      active={index === currentIndex}
                      playing={isPlaying}
                      onPlay={() => playAt(index)}
                      onRemove={() => removeFromQueue(item.id)}
                    />
                  ))}
                </div>
                <div className="pt-2 text-center text-xs text-slate-500">
                  {pluralWithCount(queue.length, "трек", "трека", "треков")} в очереди
                </div>
              </div>
            )}
          </div>
        </div>
      ) : (
        <div className="grid flex-1 place-items-center px-6 text-center text-slate-500">
          <div>
            <ListMusic size={44} className="mx-auto mb-3 opacity-40" />
            <p>Очередь пуста</p>
          </div>
        </div>
      )}
    </div>
  );
}

/** Строка трека в очереди полноэкранного плеера. */
function QueueRow({
  item,
  active,
  playing,
  onPlay,
  onRemove,
}: {
  item: Track;
  active: boolean;
  playing: boolean;
  onPlay: () => void;
  onRemove: () => void;
}) {
  const coverUrl = useTrackCover(item);

  // Свайп влево удаляет трек из очереди (альтернатива кнопке ✕). Активную
  // (сейчас играющую) строку не свайпаем — у неё вместо ✕ индикатор эквалайзера,
  // и «смахнуть текущий трек» неочевидно. Жест горизонтальный: строка следует за
  // пальцем, при протяжке > порога — удаление, иначе плавный откат.
  const swipe = useRef<{ startX: number; startY: number; active: boolean } | null>(null);
  const [offset, setOffset] = useState(0);
  const [sliding, setSliding] = useState(false);
  const swipeable = !(active && playing);

  function onRowTouchStart(e: TouchEvent<HTMLDivElement>) {
    if (!swipeable) return;
    const t = e.touches[0];
    swipe.current = { startX: t.clientX, startY: t.clientY, active: false };
  }

  function onRowTouchMove(e: TouchEvent<HTMLDivElement>) {
    const st = swipe.current;
    if (!st) return;
    const t = e.touches[0];
    const dx = t.clientX - st.startX;
    const dy = t.clientY - st.startY;
    if (!st.active) {
      // Определяем направление по первому смещению: горизонталь влево — наш жест,
      // иначе (вертикаль — прокрутка списка) отпускаем.
      if (Math.abs(dx) < 8 && Math.abs(dy) < 8) return;
      if (dx < 0 && Math.abs(dx) > Math.abs(dy)) {
        st.active = true;
        setSliding(true);
      } else {
        swipe.current = null;
        return;
      }
    }
    setOffset(Math.min(0, dx));
  }

  function onRowTouchEnd() {
    const st = swipe.current;
    swipe.current = null;
    setSliding(false);
    if (st?.active && offset < -80) {
      onRemove();
      return;
    }
    setOffset(0);
  }

  return (
    <div className="relative h-[60px] min-h-[60px] shrink-0 overflow-hidden rounded-xl">
      {/* Красная подложка-подсказка проступает по мере свайпа. */}
      {offset < 0 && (
        <div
          className="absolute inset-y-0 right-0 flex items-center gap-1.5 pr-4 text-red-300"
          aria-hidden
        >
          <Trash2 size={16} />
          <span className="text-xs font-medium">Убрать</span>
        </div>
      )}
      <div
        className={cn(
          "flex h-[60px] min-h-[60px] items-center gap-3 rounded-xl px-2 py-2",
          active
            ? "bg-[color-mix(in_srgb,var(--app-accent)_14%,transparent)] ring-1 ring-inset ring-[color-mix(in_srgb,var(--app-accent)_28%,transparent)]"
            : "bg-[var(--bg-0)]",
        )}
        style={{
          transform: offset ? `translateX(${offset}px)` : undefined,
          transition: sliding ? "none" : "transform 0.24s ease",
        }}
        onTouchStart={onRowTouchStart}
        onTouchMove={onRowTouchMove}
        onTouchEnd={onRowTouchEnd}
        onTouchCancel={onRowTouchEnd}
      >
        <button
          type="button"
          onClick={onPlay}
          className="flex min-w-0 flex-1 items-center gap-3 text-left"
          aria-label="Воспроизвести"
        >
          <Cover
            accent={item.accent}
            src={coverUrl}
            alt={item.title}
            className="h-11 w-11 shrink-0"
            rounded="rounded-lg"
            iconSize={16}
          />
          <div className="min-w-0">
            <div
              className={cn(
                "line-clamp-2 whitespace-normal text-sm font-medium",
                active ? "text-[var(--app-accent)]" : "text-white",
              )}
            >
              {item.title}
            </div>
            <div className="line-clamp-2 whitespace-normal text-xs text-slate-400">
              <ArtistLink
                name={item.artist}
                onNavigate={() => useUiStore.getState().setNowPlayingOpen(false)}
              />{" "}
              · {sourceName(item.source)}
            </div>
          </div>
        </button>
        {active && playing ? (
          <span className="text-[var(--app-accent)]" aria-hidden>
            <EqIcon />
          </span>
        ) : (
          <button
            type="button"
            onClick={onRemove}
            className="grid h-9 w-9 shrink-0 place-items-center text-slate-500 transition-colors active:text-white"
            aria-label="Убрать из очереди"
          >
            <X size={16} />
          </button>
        )}
      </div>
    </div>
  );
}

/** Анимированный эквалайзер для активного трека (как в NowPlayingPanel). */
function EqIcon() {
  return (
    <span className="flex h-4 items-end gap-0.5">
      <span
        className="w-0.5 animate-[eq_0.8s_ease-in-out_infinite] bg-current"
        style={{ height: "60%" }}
      />
      <span
        className="w-0.5 animate-[eq_0.8s_ease-in-out_infinite_0.2s] bg-current"
        style={{ height: "100%" }}
      />
      <span
        className="w-0.5 animate-[eq_0.8s_ease-in-out_infinite_0.4s] bg-current"
        style={{ height: "40%" }}
      />
    </span>
  );
}
