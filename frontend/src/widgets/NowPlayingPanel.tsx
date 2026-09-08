import { useEffect, useRef, useState } from "react";
import {
  Heart,
  ListMusic,
  Maximize2,
  MoreHorizontal,
  Pause,
  Play,
  Repeat,
  Repeat1,
  Shuffle,
  SkipBack,
  SkipForward,
  Volume1,
  Volume2,
  VolumeX,
  X,
} from "lucide-react";
import { usePlayerStore } from "../store/playerStore";
import type { Track } from "../shared/types";
import { Slider } from "../shared/ui/Slider";
import { Waveform } from "../shared/ui/Waveform";
import { Cover } from "../shared/ui/Cover";
import { TrackQuickActions } from "../shared/ui/TrackQuickActions";
import { accentStyle } from "../shared/ui/accents";
import { formatDuration, pluralWithCount } from "../shared/lib/format";
import { useFavoriteIds } from "../shared/lib/useFavorites";
import { useTrackCover } from "../shared/lib/useTrackCover";
import { useCoverPalette } from "../shared/lib/useCoverPalette";
import { sourceName } from "../shared/sources";
import { cn } from "../shared/lib/cn";
import { useLyrics } from "../shared/lib/useLyrics";
import { activeLineIndex, type LyricLine } from "../shared/lib/lyrics";

interface NowPlayingPanelProps {
  /** Если задан — панель показана как выдвижная (узкий экран): рисуем кнопку закрытия. */
  onClose?: () => void;
}

export function NowPlayingPanel({ onClose }: NowPlayingPanelProps = {}) {
  const {
    queue,
    currentIndex,
    isPlaying,
    progress,
    volume,
    muted,
    shuffle,
    repeat,
    togglePlay,
    next,
    previous,
    seek,
    setVolume,
    toggleMute,
    toggleShuffle,
    cycleRepeat,
    playAt,
    removeFromQueue,
    clearQueue,
    toggleLikeWithTrack,
    engineDuration,
  } = usePlayerStore();

  const track = queue[currentIndex];
  const favoriteIds = useFavoriteIds();
  const isFavorite = track ? favoriteIds.has(track.id) : false;
  const effectiveVolume = muted ? 0 : volume;
  const coverUrl = useTrackCover(track);
  // Акцент подстраивается под обложку текущего трека (кнопка play, волна).
  const accent = useCoverPalette(coverUrl);
  const [showQuickActions, setShowQuickActions] = useState(false);
  const [isExpanded, setIsExpanded] = useState(false);
  // Нижняя область панели переключается между очередью и текстом песни.
  const [bottomTab, setBottomTab] = useState<"queue" | "lyrics">("queue");

  const copyTrackInfo = async () => {
    if (!track) return;
    try {
      await navigator.clipboard.writeText(`${track.title} — ${track.artist}`);
      setShowQuickActions(false);
    } catch {
      setShowQuickActions(false);
    }
  };

  return (
    <aside
      className={cn(
        "flex shrink-0 flex-col gap-4 border-l border-white/5 bg-[#0c0e1a]/60 p-5",
        // В выдвижном режиме (узкий экран) панель занимает всю высоту контента
        // и фиксированную ширину; иначе — обычная встроенная ширина.
        onClose ? "h-full w-[360px]" : isExpanded ? "w-[410px]" : "w-[340px]",
      )}
    >
      {/* Заголовок */}
      <div className="flex items-center justify-between">
        <span className="text-sm font-semibold text-white">Сейчас играет</span>
        <div className="flex items-center gap-1 text-slate-400">
          {onClose ? (
            <button
              onClick={onClose}
              className="grid h-8 w-8 place-items-center rounded-lg hover:bg-white/5 hover:text-white"
              aria-label="Закрыть панель"
            >
              <X size={18} />
            </button>
          ) : null}
          <button
            onClick={() => setShowQuickActions((v) => !v)}
            className="grid h-8 w-8 place-items-center rounded-lg hover:bg-white/5 hover:text-white"
            aria-label="Ещё"
          >
            <MoreHorizontal size={18} />
          </button>
          <button
            onClick={() => setIsExpanded((v) => !v)}
            className="grid h-8 w-8 place-items-center rounded-lg hover:bg-white/5 hover:text-white"
            aria-label="Развернуть"
          >
            <Maximize2 size={16} />
          </button>
        </div>
      </div>
      {showQuickActions && track ? (
        <div className="rounded-xl border border-white/10 bg-white/5 p-2 text-sm text-slate-300">
          <button onClick={copyTrackInfo} className="flex w-full items-center rounded-lg px-2 py-2 text-left hover:bg-white/10">
            Копировать название трека
          </button>
          <button
            onClick={() => {
              setShowQuickActions(false);
              setIsExpanded(true);
            }}
            className="flex w-full items-center rounded-lg px-2 py-2 text-left hover:bg-white/10"
          >
            Развернуть очередь
          </button>
        </div>
      ) : null}

      {track ? (
        <>
          {/* Обложка. Высота ограничена, чтобы очередь ниже показывала сразу
              несколько треков, а не один: на всю ширину панели обложка
              занимала бы ~340px и «съедала» список. */}
          <div
            className={cn(
              "relative mx-auto aspect-square w-full overflow-hidden rounded-2xl",
              isExpanded ? "max-w-[280px]" : "max-w-[200px]",
            )}
            style={accentStyle(track.accent)}
          >
            {coverUrl ? (
              <img
                src={coverUrl}
                alt={track.title}
                onError={(e) => (e.currentTarget.style.display = "none")}
                className="absolute inset-0 h-full w-full object-cover"
              />
            ) : (
              <div className="flex h-full w-full items-center justify-center bg-black/10">
                <span className="text-6xl font-black text-white/20">{track.title.charAt(0)}</span>
              </div>
            )}
          </div>

          {/* Мета */}
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <h3 className="truncate text-xl font-bold text-white">{track.title}</h3>
              <p className="truncate text-slate-400">{track.artist}</p>
            </div>
            <button
              onClick={() => toggleLikeWithTrack({ ...track, liked: isFavorite })}
              className={cn("mt-1 transition-colors", isFavorite ? "text-[#a855f7]" : "text-slate-400 hover:text-white")}
              aria-label="В избранное"
            >
              <Heart size={22} fill={isFavorite ? "currentColor" : "none"} />
            </button>
          </div>

          {/* Форма волны + время */}
          <div className="flex flex-col gap-2">
            <Waveform
              seed={track.id}
              progress={progress}
              duration={engineDuration || track.duration}
              onSeek={seek}
              fillColor={accent}
            />
            <div className="flex justify-between text-xs tabular-nums text-slate-400">
              <span>{formatDuration(progress)}</span>
              <span>{formatDuration(engineDuration || track.duration)}</span>
            </div>
          </div>

          {/* Управление */}
          <div className="flex items-center justify-between px-1">
            <button
              onClick={toggleShuffle}
              className={cn("transition-colors", shuffle ? "text-[#a855f7]" : "text-slate-400 hover:text-white")}
              aria-label="Перемешать"
            >
              <Shuffle size={20} />
            </button>
            <button onClick={previous} className="text-white transition-transform hover:scale-110" aria-label="Предыдущий">
              <SkipBack size={24} fill="currentColor" />
            </button>
            <button
              onClick={togglePlay}
              className="grid h-14 w-14 place-items-center rounded-full text-white shadow-lg transition-transform hover:scale-105"
              style={{ backgroundColor: accent, boxShadow: `0 10px 25px -5px ${accent}66` }}
              aria-label={isPlaying ? "Пауза" : "Воспроизвести"}
            >
              {isPlaying ? <Pause size={26} fill="currentColor" /> : <Play size={26} fill="currentColor" className="ml-1" />}
            </button>
            <button onClick={next} className="text-white transition-transform hover:scale-110" aria-label="Следующий">
              <SkipForward size={24} fill="currentColor" />
            </button>
            <button
              onClick={cycleRepeat}
              className={cn("transition-colors", repeat !== "off" ? "text-[#a855f7]" : "text-slate-400 hover:text-white")}
              aria-label="Повтор"
            >
              {repeat === "one" ? <Repeat1 size={20} /> : <Repeat size={20} />}
            </button>
          </div>

          {/* Громкость */}
          <div className="flex items-center gap-3">
            <button onClick={toggleMute} className="text-slate-400 transition-colors hover:text-white" aria-label="Без звука">
              {muted || volume === 0 ? <VolumeX size={20} /> : volume < 50 ? <Volume1 size={20} /> : <Volume2 size={20} />}
            </button>
            <Slider
              value={effectiveVolume}
              max={100}
              onChange={setVolume}
              wheelStep={4}
              ariaLabel="Громкость"
              className="flex-1"
            />
            <span className="w-9 text-right text-xs tabular-nums text-slate-400">{effectiveVolume}%</span>
          </div>
        </>
      ) : (
        <div className="grid flex-1 place-items-center text-center text-slate-500">
          <div>
            <ListMusic size={40} className="mx-auto mb-3 opacity-40" />
            <p>Очередь пуста</p>
          </div>
        </div>
      )}

      {/* Очередь / Текст песни */}
      <div className="flex min-h-0 flex-1 flex-col">
        <div className="mb-2 flex items-center gap-3">
          <button
            onClick={() => setBottomTab("queue")}
            className={cn(
              "text-sm font-semibold transition-colors",
              bottomTab === "queue" ? "text-white" : "text-slate-500 hover:text-slate-300",
            )}
          >
            Очередь
          </button>
          <button
            onClick={() => setBottomTab("lyrics")}
            className={cn(
              "text-sm font-semibold transition-colors",
              bottomTab === "lyrics" ? "text-white" : "text-slate-500 hover:text-slate-300",
            )}
          >
            Текст
          </button>
          {bottomTab === "queue" ? (
            <button onClick={clearQueue} className="ml-auto text-xs text-slate-400 transition-colors hover:text-white">
              Очистить
            </button>
          ) : null}
        </div>

        {bottomTab === "queue" ? (
          <div id="now-playing-queue" className="flex min-h-0 flex-1 flex-col overflow-hidden">
            <div className="-mr-2 flex min-h-0 flex-1 flex-col gap-0.5 overflow-y-auto pr-2">
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
            <div className="mt-2 text-xs text-slate-500">
              {pluralWithCount(queue.length, "трек", "трека", "треков")} в очереди
            </div>
          </div>
        ) : (
          <LyricsView track={track} progress={progress} accent={accent} onSeek={seek} />
        )}
      </div>
    </aside>
  );
}

/**
 * Текст песни для текущего трека. Синхронизированный LRC подсвечивает активную
 * строку по позиции воспроизведения и позволяет перематывать кликом; при
 * отсутствии синхронизации показывается обычный текст. Пустые состояния
 * (загрузка / инструментал / не найдено) обрабатываются отдельно.
 */
function LyricsView({
  track,
  progress,
  accent,
  onSeek,
}: {
  track: Track | undefined;
  progress: number;
  accent: string;
  onSeek: (position: number) => void;
}) {
  const { loading, found, instrumental, lines, plain } = useLyrics(track);
  const active = activeLineIndex(lines, progress);
  const activeRef = useRef<HTMLButtonElement>(null);

  // Держим активную строку в поле зрения при её смене.
  useEffect(() => {
    activeRef.current?.scrollIntoView({ block: "center", behavior: "smooth" });
  }, [active]);

  if (!track) {
    return <LyricsHint text="Нет трека" />;
  }
  if (loading) {
    return <LyricsHint text="Загружаем текст…" />;
  }
  if (instrumental) {
    return <LyricsHint text="Инструментальная композиция" />;
  }
  if (!found || (lines.length === 0 && !plain.trim())) {
    return <LyricsHint text="Текст не найден" />;
  }

  // Синхронизированный текст: строки-кнопки с подсветкой активной.
  if (lines.length > 0) {
    return (
      <div className="-mr-2 flex min-h-0 flex-1 flex-col gap-1 overflow-y-auto pr-2">
        {lines.map((line: LyricLine, i: number) => (
          <button
            key={`${line.time}-${i}`}
            ref={i === active ? activeRef : undefined}
            onClick={() => onSeek(line.time)}
            className={cn(
              "rounded-md px-2 py-1 text-left text-sm leading-snug transition-colors",
              i === active
                ? "font-semibold text-white"
                : "text-slate-500 hover:text-slate-300",
            )}
            style={i === active ? { color: accent } : undefined}
          >
            {line.text || "♪"}
          </button>
        ))}
      </div>
    );
  }

  // Фолбэк: обычный текст без синхронизации.
  return (
    <div className="-mr-2 min-h-0 flex-1 overflow-y-auto whitespace-pre-wrap pr-2 text-sm leading-relaxed text-slate-300">
      {plain}
    </div>
  );
}

/** Центрированная подсказка для пустых состояний текста. */
function LyricsHint({ text }: { text: string }) {
  return (
    <div className="grid min-h-0 flex-1 place-items-center text-center text-sm text-slate-500">
      {text}
    </div>
  );
}

/** Строка трека в очереди воспроизведения. */
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
  return (
    <div
      onDoubleClick={onPlay}
      className={cn(
        "group flex items-center gap-3 rounded-lg px-2 py-1.5 transition-colors hover:bg-white/5",
        active && "bg-white/8",
      )}
    >
      <button onClick={onPlay} aria-label="Воспроизвести">
        <Cover accent={item.accent} src={coverUrl} alt={item.title} className="h-9 w-9" rounded="rounded-md" iconSize={14} />
      </button>
      <div className="min-w-0 flex-1">
        <div className={cn("truncate text-sm font-medium", active ? "text-[#c084fc]" : "text-white")}>
          {item.title}
        </div>
        <div className="truncate text-xs text-slate-400">{item.artist}</div>
      </div>
      {active && playing ? (
        <span className="text-[#a855f7]" aria-hidden>
          <EqIcon />
        </span>
      ) : (
        <span className="hidden text-xs text-slate-500 group-hover:inline">{sourceName(item.source)}</span>
      )}
      <div className="flex items-center gap-1">
        <TrackQuickActions track={item} />
        <button
          onClick={onRemove}
          className="text-slate-500 opacity-0 transition-opacity hover:text-white group-hover:opacity-100"
          aria-label="Убрать из очереди"
        >
          <X size={14} />
        </button>
      </div>
    </div>
  );
}

/** Анимированный эквалайзер для активного трека. */
function EqIcon() {
  return (
    <span className="flex h-4 items-end gap-0.5">
      <span className="w-0.5 animate-[eq_0.8s_ease-in-out_infinite] bg-current" style={{ height: "60%" }} />
      <span className="w-0.5 animate-[eq_0.8s_ease-in-out_infinite_0.2s] bg-current" style={{ height: "100%" }} />
      <span className="w-0.5 animate-[eq_0.8s_ease-in-out_infinite_0.4s] bg-current" style={{ height: "40%" }} />
    </span>
  );
}
