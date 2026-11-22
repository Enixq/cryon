import { useEffect, useRef, useState } from "react";
import { Heart, ListMusic, Maximize2, Minimize2, Play, X } from "lucide-react";
import { usePlayerStore } from "../store/playerStore";
import { useUiStore } from "../store/uiStore";
import type { Track } from "../shared/types";
import { Cover } from "../shared/ui/Cover";
import { TrackQuickActions } from "../shared/ui/TrackQuickActions";
import { formatDuration, pluralWithCount } from "../shared/lib/format";
import { useFavoriteIds } from "../shared/lib/useFavorites";
import { useTrackCover } from "../shared/lib/useTrackCover";
import { useCoverPalette } from "../shared/lib/useCoverPalette";
import { sourceName } from "../shared/sources";
import { cn } from "../shared/lib/cn";
import { ArtistLink } from "../shared/ui/ArtistLink";
import { LyricsSources } from "../shared/ui/LyricsSources";
import { useLyrics } from "../shared/lib/useLyrics";
import { activeLineIndex, type LyricLine } from "../shared/lib/lyrics";

export function NowPlayingPanel({ onClose }: { onClose?: () => void } = {}) {
  const {
    queue,
    currentIndex,
    isPlaying,
    progress,
    playAt,
    removeFromQueue,
    clearQueue,
    toggleLikeWithTrack,
    seek,
  } = usePlayerStore();
  const track = queue[currentIndex];
  const favoriteIds = useFavoriteIds();
  const isFavorite = track ? favoriteIds.has(track.id) : false;
  const coverUrl = useTrackCover(track);
  const accent = useCoverPalette(coverUrl);
  const [isExpanded, setIsExpanded] = useState(false);
  const [bottomTab, setBottomTab] = useState<"queue" | "lyrics">("queue");
  return (
    <aside
      data-view={bottomTab}
      data-expanded={isExpanded}
      className={cn(
        "now-playing-panel flex shrink-0 flex-col gap-4 border-l border-white/5 bg-[#0c0e1a]/60 p-4",
        onClose ? "h-full w-[min(360px,90vw)]" : "w-[clamp(280px,24vw,340px)]",
      )}
    >
      <div className="flex shrink-0 items-center justify-between">
        <span className="text-sm font-semibold text-white">Сейчас играет</span>
        <div className="flex items-center gap-1 text-slate-400">
          {track && <TrackQuickActions track={track} />}
          <button
            disabled={!track}
            onClick={() => setIsExpanded((v) => !v)}
            aria-pressed={isExpanded}
            aria-label={isExpanded ? "Уменьшить обложку" : "Увеличить обложку"}
            className="p-2 hover:text-white"
          >
            {isExpanded ? <Minimize2 size={16} /> : <Maximize2 size={16} />}
          </button>
          <button
            onClick={() =>
              onClose ? onClose() : useUiStore.getState().setNowPlayingCollapsed(true)
            }
            aria-label="Свернуть панель"
            className="p-2 hover:text-white"
          >
            <X size={18} />
          </button>
        </div>
      </div>
      {track && (
        <>
          <button
            className="panel-cover mx-auto aspect-square overflow-hidden rounded-2xl"
            onClick={() => setIsExpanded((v) => !v)}
            aria-label={isExpanded ? "Уменьшить обложку" : "Рассмотреть обложку"}
          >
            <Cover
              src={coverUrl}
              accent={track.accent}
              alt={track.title}
              className="h-full w-full"
            />
          </button>
          <div className="panel-details flex items-start justify-between gap-3">
            <div className="min-w-0">
              <h3 className="break-words text-lg font-bold leading-tight text-white">
                {track.title}
              </h3>
              <ArtistLink name={track.artist} />
              <button
                className="mt-1 block text-xs text-slate-500 hover:text-white"
                onClick={() => useUiStore.getState().openSourcePicker(track)}
              >
                {sourceName(track.source)} · Сменить
              </button>
            </div>
            <button
              onClick={() => toggleLikeWithTrack({ ...track, liked: isFavorite })}
              aria-label="В избранное"
              className={isFavorite ? "text-[var(--app-accent)]" : "text-slate-400"}
            >
              <Heart size={22} fill={isFavorite ? "currentColor" : "none"} />
            </button>
          </div>
        </>
      )}
      <div className="flex min-h-0 flex-1 flex-col">
        <div className="mb-2 flex shrink-0 items-center gap-3">
          <button
            onClick={() => setBottomTab("queue")}
            className={
              bottomTab === "queue" ? "text-sm font-semibold text-white" : "text-sm text-slate-500"
            }
          >
            Очередь
          </button>
          <button
            onClick={() => setBottomTab("lyrics")}
            className={
              bottomTab === "lyrics" ? "text-sm font-semibold text-white" : "text-sm text-slate-500"
            }
          >
            Текст
          </button>
          {bottomTab === "queue" && (
            <button onClick={clearQueue} className="ml-auto text-xs text-slate-400">
              Очистить
            </button>
          )}
        </div>
        {bottomTab === "queue" ? (
          <div id="now-playing-queue" className="flex min-h-0 flex-1 flex-col">
            <div className="min-h-0 flex-1 overflow-y-auto">
              {queue.map((item, index) => (
                <QueueRow
                  key={`${item.id}:${index}`}
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
  const lyrics = useLyrics(track);
  const active = activeLineIndex(lyrics.lines, progress);
  const activeRef = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    activeRef.current?.scrollIntoView?.({ block: "nearest", behavior: "smooth" });
  }, [active]);
  return (
    <div className="flex min-h-0 flex-1 flex-col gap-3">
      <LyricsSources
        lyrics={lyrics}
        hasSource={track?.source === "yandex" || track?.source === "vk"}
      />
      {lyrics.loading ? (
        <LyricsHint text="Загружаем текст…" />
      ) : lyrics.instrumental ? (
        <LyricsHint text="Инструментальная композиция" />
      ) : lyrics.lines.length > 0 ? (
        <div className="min-h-0 flex-1 overflow-y-auto">
          {lyrics.lines.map((line: LyricLine, index: number) => (
            <button
              key={`${line.time}-${index}`}
              ref={index === active ? activeRef : undefined}
              onClick={() => onSeek(line.time)}
              className={cn(
                "block w-full rounded-md px-2 py-1 text-left text-sm leading-relaxed",
                index === active
                  ? "font-semibold text-white"
                  : "text-slate-500 hover:text-slate-300",
              )}
              style={index === active ? { color: accent } : undefined}
            >
              {line.text || "♪"}
            </button>
          ))}
        </div>
      ) : lyrics.found && lyrics.plain ? (
        <div className="min-h-0 flex-1 overflow-y-auto whitespace-pre-wrap text-sm leading-relaxed text-slate-300">
          {lyrics.plain}
        </div>
      ) : (
        <LyricsHint
          text={
            lyrics.error
              ? "Источник текста недоступен. Попробуйте другой."
              : "Текст не найден — попробуйте другой источник"
          }
        />
      )}
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
        <Cover
          accent={item.accent}
          src={coverUrl}
          alt={item.title}
          className="h-9 w-9"
          rounded="rounded-md"
          iconSize={14}
        />
      </button>
      <div className="min-w-0 flex-1">
        <div
          className={cn("truncate text-sm font-medium", active ? "text-[#c084fc]" : "text-white")}
        >
          {item.title}
        </div>
        <div className="truncate text-xs text-slate-400">
          <ArtistLink
            name={item.artist}
            onNavigate={() => useUiStore.getState().setNowPlayingOpen(false)}
          />
        </div>
      </div>
      {active && playing ? (
        <span className="text-[#a855f7]" aria-hidden>
          <EqIcon />
        </span>
      ) : (
        <span className="hidden text-xs text-slate-500 group-hover:inline">
          {sourceName(item.source)}
        </span>
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
