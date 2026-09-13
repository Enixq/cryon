import { useRef, useState } from "react";
import type { TouchEvent } from "react";
import {
  ChevronDown,
  Heart,
  ListMusic,
  Radio,
  Pause,
  Play,
  Repeat,
  Repeat1,
  Shuffle,
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
import { useLyrics } from "../shared/lib/useLyrics";
import { activeLineIndex } from "../shared/lib/lyrics";
import { useCoverPalette } from "../shared/lib/useCoverPalette";
import { useFavoriteIds } from "../shared/lib/useFavorites";
import { sourceName } from "../shared/sources";
import { cn } from "../shared/lib/cn";
import { ArtistLink } from "../shared/ui/ArtistLink";
import type { Track } from "../shared/types";

/**
 * Полноэкранный плеер «Сейчас играет» на телефоне (мобильный аналог
 * [NowPlayingPanel](NowPlayingPanel.tsx)). Открывается тапом по мини-плееру,
 * сворачивается шевроном вниз. Крупная обложка, перемотка слайдером, полный
 * транспорт и лайк; кнопка-список переключает середину на очередь. Появляется
 * анимацией «снизу вверх» (styles.css → .animate-sheet-up).
 */
export function MobileNowPlaying() {
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
  const isFavorite = track ? favoriteIds.has(track.id) : false;
  const [showQueue, setShowQueue] = useState(false);
  const [showLyrics, setShowLyrics] = useState(false);
  const lyrics = useLyrics(track);
  const activeLyric = activeLineIndex(lyrics.lines, progress);

  // Свайп вниз закрывает плеер (жест «смахнуть лист»). Активен только в режиме
  // обложки: когда открыта очередь, вертикальный свайп нужен для её прокрутки.
  // Слайдер перемотки и транспорт помечены data-noswipe — горизонтальная
  // перемотка и тапы по кнопкам не должны превращаться в закрытие.
  const drag = useRef<{ startX: number; startY: number; active: boolean; y: number } | null>(null);
  const [dragY, setDragY] = useState(0);
  const [dragging, setDragging] = useState(false);

  function onTouchStart(e: TouchEvent<HTMLDivElement>) {
    if (showQueue) return;
    if ((e.target as HTMLElement).closest?.("[data-noswipe]")) return;
    const t = e.touches[0];
    drag.current = { startX: t.clientX, startY: t.clientY, active: false, y: 0 };
  }

  function onTouchMove(e: TouchEvent<HTMLDivElement>) {
    const st = drag.current;
    if (!st) return;
    const t = e.touches[0];
    const dx = t.clientX - st.startX;
    const dy = t.clientY - st.startY;
    if (!st.active) {
      // Пока направление не определено — ждём заметного смещения и решаем: это
      // свайп вниз (тянем лист) или что-то другое (тогда отпускаем жест).
      if (Math.abs(dx) < 8 && Math.abs(dy) < 8) return;
      if (dy > 0 && dy > Math.abs(dx)) {
        st.active = true;
        setDragging(true);
      } else {
        drag.current = null;
        return;
      }
    }
    st.y = Math.max(0, dy);
    setDragY(st.y);
  }

  function endDrag() {
    const st = drag.current;
    drag.current = null;
    setDragging(false);
    // Протянули больше порога — закрываем; иначе плавно возвращаем на место.
    if (st?.active && st.y > 100) {
      setNowPlayingOpen(false);
      return;
    }
    setDragY(0);
  }

  const duration = track ? engineDuration || track.duration : 0;

  return (
    <div
      className="animate-sheet-up no-tap-highlight fixed inset-0 z-40 flex flex-col"
      style={{
        background: `radial-gradient(130% 60% at 50% -10%, color-mix(in srgb, ${accent} 32%, transparent) 0%, transparent 60%), var(--bg-0)`,
        transform: dragY ? `translateY(${dragY}px)` : undefined,
        transition: dragging ? "none" : "transform 0.28s ease",
      }}
      onTouchStart={onTouchStart}
      onTouchMove={onTouchMove}
      onTouchEnd={endDrag}
      onTouchCancel={endDrag}
    >
      {/* Верхняя панель: «ручка» для свайпа вниз + свернуть / источник / очередь */}
      <div className="pt-[calc(8px+var(--safe-top))]">
        <div className="mx-auto mb-2 h-1.5 w-10 rounded-full bg-white/20" aria-hidden />
        <div className="flex items-center justify-between px-3">
          <button
            type="button"
            onClick={() => setNowPlayingOpen(false)}
            className="grid h-11 w-11 place-items-center rounded-full text-slate-200 transition-colors active:bg-white/10"
            aria-label="Свернуть плеер"
          >
            <ChevronDown size={26} />
          </button>
          <span className="text-xs font-semibold uppercase tracking-wider text-slate-400">
            {track ? sourceName(track.source) : "Сейчас играет"}
          </span>
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
        <div className="flex min-h-0 flex-1 flex-col px-6 pb-[calc(18px+var(--safe-bottom))] pt-2">
          {/* Середина: обложка ИЛИ очередь */}
          {showLyrics ? (
            <div className="min-h-0 flex-1 overflow-y-auto rounded-2xl bg-white/[0.03] px-4 py-5">
              {lyrics.loading ? <div className="grid min-h-[220px] place-items-center text-sm text-slate-500">Загрузка текста…</div> : lyrics.instrumental ? <div className="grid min-h-[220px] place-items-center text-sm text-slate-500">Инструментальная композиция</div> : lyrics.lines.length > 0 ? <div className="space-y-3">{lyrics.lines.map((line, index) => <button key={`${line.time}-${index}`} type="button" onClick={() => seek(line.time)} className={cn("block w-full text-left text-lg font-semibold transition-colors", index === activeLyric ? "text-white" : "text-slate-500")}>{line.text || "…"}</button>)}</div> : <div className="grid min-h-[220px] place-items-center text-center text-sm text-slate-500">{lyrics.found && lyrics.plain ? lyrics.plain : "Текст не найден"}</div>}
            </div>
          ) : showQueue ? (
            <div className="flex min-h-0 flex-1 flex-col">
              <div className="-mr-2 flex min-h-0 flex-1 flex-col gap-1 overflow-y-auto overscroll-contain pr-2">
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
          ) : (
            <div className="flex min-h-0 flex-1 items-center justify-center">
              <Cover
                accent={track.accent}
                src={coverUrl}
                alt={track.title}
                className="aspect-square w-full max-w-[340px] shadow-2xl"
                rounded="rounded-[28px]"
                iconSize={80}
              />
            </div>
          )}

          {/* Название + исполнитель + лайк */}
          <div className={cn("mt-5 flex items-center justify-between gap-3", (showQueue || showLyrics) && "hidden") }>
            <div className="min-w-0">
              <h2 className="truncate text-2xl font-bold text-white">{track.title}</h2>
              <p className="flex flex-wrap items-center gap-2 text-slate-400">
                <ArtistLink name={track.artist} onNavigate={() => useUiStore.getState().setNowPlayingOpen(false)} />
                {track.quality && <span className="rounded-full bg-white/10 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-slate-300">{track.quality}</span>}
              </p>
            </div>
            <button
              type="button"
              onClick={() => toggleLikeWithTrack({ ...track, liked: isFavorite })}
              className={cn("shrink-0 transition-colors", isFavorite ? "text-[var(--app-accent)]" : "text-slate-400 active:text-white")}
              aria-label="В избранное"
            >
              <Heart size={26} fill={isFavorite ? "currentColor" : "none"} />
            </button>
          </div>

          {/* Перемотка */}
          <div className={cn("mt-4 flex flex-col gap-1.5", (showQueue || showLyrics) && "hidden")} data-noswipe>
            <Slider value={progress} max={duration || 100} onChange={seek} ariaLabel="Перемотка воспроизведения" />
            <div className="flex justify-between text-xs tabular-nums text-slate-400">
              <span>{formatDuration(progress)}</span>
              <span>{formatDuration(duration)}</span>
            </div>
          </div>

          {/* Транспорт */}
          <div className={cn("mt-3 flex items-center justify-between", (showQueue || showLyrics) && "hidden")} data-noswipe>
            <button
              type="button"
              onClick={toggleShuffle}
              className="grid h-11 w-11 place-items-center transition-colors"
              style={{ color: shuffle ? "var(--app-accent)" : undefined }}
              aria-label="Перемешать"
            >
              <Shuffle size={22} className={shuffle ? undefined : "text-slate-400"} />
            </button>
            <button type="button" onClick={previous} className="grid h-12 w-12 place-items-center text-white transition-transform active:scale-90" aria-label="Предыдущий">
              <SkipBack size={30} fill="currentColor" />
            </button>
            <button
              type="button"
              onClick={togglePlay}
              className="grid h-16 w-16 place-items-center rounded-full text-white shadow-xl transition-transform active:scale-95"
              style={{ backgroundColor: accent, boxShadow: `0 16px 36px -12px ${accent}` }}
              aria-label={isPlaying ? "Пауза" : "Воспроизвести"}
            >
              {isPlaying ? <Pause size={30} fill="currentColor" /> : <Play size={30} fill="currentColor" className="ml-1" />}
            </button>
            <button type="button" onClick={next} className="grid h-12 w-12 place-items-center text-white transition-transform active:scale-90" aria-label="Следующий">
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
          <div className={cn("mt-4 flex items-center justify-center gap-6 text-slate-400", (showQueue || showLyrics) && "hidden")}>
            <button
              type="button"
              onClick={() => setEqualizerOpen(true)}
              className="flex flex-col items-center gap-1 text-[11px] transition-colors active:text-white"
              aria-label="Эквалайзер"
            >
              <SlidersHorizontal size={20} />
              Эквалайзер
            </button>
            <button
              type="button"
              onClick={toggleRadio}
              className={cn("flex flex-col items-center gap-1 text-[11px] transition-colors active:text-white", radio && "text-[var(--app-accent)]")}
              aria-label="Радио по треку"
            >
              <Radio size={20} />
              Радио
            </button>
            <button
              type="button"
              onClick={() => setShowQueue((v) => !v)}
              className={cn("flex flex-col items-center gap-1 text-[11px] transition-colors active:text-white", showQueue && "text-[var(--app-accent)]")}
              aria-label="Очередь"
            >
              <ListMusic size={20} />
              Очередь
            </button>
            <button type="button" onClick={() => setShowLyrics((value) => !value)} className={cn("flex flex-col items-center gap-1 text-[11px] transition-colors active:text-white", showLyrics && "text-[var(--app-accent)]")} aria-label="Текст песни">
              <span className="text-base leading-none">Aa</span>
              Текст
            </button>
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
        <div className="absolute inset-y-0 right-0 flex items-center gap-1.5 pr-4 text-red-300" aria-hidden>
          <Trash2 size={16} />
          <span className="text-xs font-medium">Убрать</span>
        </div>
      )}
      <div
        className={cn("flex h-[60px] min-h-[60px] items-center gap-3 rounded-xl px-2 py-2", active ? "bg-[color-mix(in_srgb,var(--app-accent)_14%,transparent)] ring-1 ring-inset ring-[color-mix(in_srgb,var(--app-accent)_28%,transparent)]" : "bg-[var(--bg-0)]")}
        style={{
          transform: offset ? `translateX(${offset}px)` : undefined,
          transition: sliding ? "none" : "transform 0.24s ease",
        }}
        onTouchStart={onRowTouchStart}
        onTouchMove={onRowTouchMove}
        onTouchEnd={onRowTouchEnd}
        onTouchCancel={onRowTouchEnd}
      >
        <button type="button" onClick={onPlay} className="flex min-w-0 flex-1 items-center gap-3 text-left" aria-label="Воспроизвести">
          <Cover accent={item.accent} src={coverUrl} alt={item.title} className="h-11 w-11 shrink-0" rounded="rounded-lg" iconSize={16} />
          <div className="min-w-0">
            <div className={cn("line-clamp-2 whitespace-normal text-sm font-medium", active ? "text-[var(--app-accent)]" : "text-white")}>{item.title}</div>
            <div className="line-clamp-2 whitespace-normal text-xs text-slate-400">
              <ArtistLink name={item.artist} onNavigate={() => useUiStore.getState().setNowPlayingOpen(false)} /> · {sourceName(item.source)}
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
      <span className="w-0.5 animate-[eq_0.8s_ease-in-out_infinite] bg-current" style={{ height: "60%" }} />
      <span className="w-0.5 animate-[eq_0.8s_ease-in-out_infinite_0.2s] bg-current" style={{ height: "100%" }} />
      <span className="w-0.5 animate-[eq_0.8s_ease-in-out_infinite_0.4s] bg-current" style={{ height: "40%" }} />
    </span>
  );
}
