import { Play, Pause, ThumbsDown, ThumbsUp } from "lucide-react";
import type { Track } from "../types";
import { Cover } from "./Cover";
import { formatDuration } from "../lib/format";
import { useTrackCover } from "../lib/useTrackCover";
import { sourceName } from "../sources";
import { cn } from "../lib/cn";
import { TrackQuickActions } from "./TrackQuickActions";
import { ServiceIcon } from "./ServiceIcon";
import { ArtistLink } from "./ArtistLink";

interface TrackRowProps {
  track: Track;
  index?: number;
  active?: boolean;
  playing?: boolean;
  onPlay: (track: Track) => void;
  showSource?: boolean;
  showAlbum?: boolean;
  /** Показать кнопку добавления в плейлист (по умолчанию включено). */
  showAddToPlaylist?: boolean;
  /**
   * Оценка рекомендации: 1 — нравится, -1 — не нравится, 0 — не оценён. Если
   * передан onRate — в строке появляются кнопки 👍/👎 (списочный вид подборок).
   */
  score?: number;
  onRate?: (track: Track, score: 1 | -1) => void;
  /** Идёт сохранение оценки — кнопки блокируются, чтобы не двоить запросы. */
  rateDisabled?: boolean;
}

/** Строка трека для списков библиотеки, плейлистов и истории. */
export function TrackRow({
  track,
  index,
  active,
  playing,
  onPlay,
  showSource = true,
  showAlbum = false,
  showAddToPlaylist = true,
  score,
  onRate,
  rateDisabled,
}: TrackRowProps) {
  const coverUrl = useTrackCover(track);
  const liked = (score ?? 0) > 0;
  const disliked = (score ?? 0) < 0;
  return (
    <div
      onDoubleClick={() => onPlay(track)}
      className={cn(
        "group grid grid-cols-[40px_1fr_auto] items-center gap-4 rounded-xl px-3 py-2 transition-colors hover:bg-white/5",
        active && "bg-white/8",
      )}
    >
      {/* Номер / кнопка воспроизведения */}
      <div className="grid h-10 w-10 place-items-center">
        {index !== undefined && (
          <span className={cn("text-sm text-slate-500 group-hover:hidden", active && "hidden text-[#a855f7]")}>
            {active && playing ? "" : index + 1}
          </span>
        )}
        <button
          onClick={() => onPlay(track)}
          className={cn(
            "hidden place-items-center text-white group-hover:grid",
            active && "grid",
          )}
          aria-label={active && playing ? "Пауза" : "Воспроизвести"}
        >
          {active && playing ? <Pause size={18} fill="currentColor" /> : <Play size={18} fill="currentColor" />}
        </button>
      </div>

      {/* Обложка + название */}
      <div className="flex min-w-0 items-center gap-3">
        <Cover accent={track.accent} src={coverUrl} alt={track.title} className="h-10 w-10 shrink-0" rounded="rounded-lg" iconSize={16} />
        <div className="min-w-0">
          <div className={cn("truncate text-[15px] font-medium text-white", active && "text-[#c084fc]")}>
            {track.title}
          </div>
          <div className="truncate text-sm text-slate-400">
            <ArtistLink name={track.artist} />
            {showAlbum && track.album ? ` · ${track.album}` : ""}
          </div>
        </div>
      </div>

      {/* Правая часть */}
      <div className="flex items-center gap-4">
        {/* Оценка рекомендации. Видна по наведению, но остаётся видимой, если
            оценка уже стоит — иначе непонятно, что было нажато. «Нравится» ещё и
            кладёт трек в избранное (см. useRecoFeedback). */}
        {onRate && (
          <div
            className={cn(
              "flex items-center gap-1 transition-opacity",
              liked || disliked ? "opacity-100" : "opacity-0 group-hover:opacity-100",
            )}
          >
            <button
              onClick={() => onRate(track, 1)}
              disabled={rateDisabled}
              aria-pressed={liked}
              aria-label={liked ? "Убрать «нравится»" : "Нравится — больше такого"}
              title={liked ? "Убрать «нравится»" : "Нравится — больше такого"}
              className={cn(
                "grid h-8 w-8 place-items-center rounded-full transition-colors disabled:opacity-50",
                liked ? "bg-[#a855f7] text-white shadow-lg shadow-[#a855f7]/40" : "text-slate-400 hover:bg-white/10 hover:text-white",
              )}
            >
              <ThumbsUp size={15} fill={liked ? "currentColor" : "none"} />
            </button>
            <button
              onClick={() => onRate(track, -1)}
              disabled={rateDisabled}
              aria-pressed={disliked}
              aria-label={disliked ? "Убрать «не нравится»" : "Не нравится — меньше такого"}
              title={disliked ? "Убрать «не нравится»" : "Не нравится — меньше такого"}
              className={cn(
                "grid h-8 w-8 place-items-center rounded-full transition-colors disabled:opacity-50",
                disliked ? "bg-rose-500 text-white shadow-lg shadow-rose-500/40" : "text-slate-400 hover:bg-white/10 hover:text-white",
              )}
            >
              <ThumbsDown size={15} fill={disliked ? "currentColor" : "none"} />
            </button>
          </div>
        )}
        {showSource && (
          <span
            className="hidden items-center gap-1.5 text-xs text-slate-500 sm:inline-flex"
            title={sourceName(track.source)}
          >
            <ServiceIcon id={track.source} size={16} />
            <span className="hidden md:inline">{sourceName(track.source)}</span>
          </span>
        )}
        {showAddToPlaylist && <TrackQuickActions track={track} />}
        <span className="w-10 text-right text-sm tabular-nums text-slate-400">
          {formatDuration(track.duration)}
        </span>
      </div>
    </div>
  );
}
