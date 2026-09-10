import { Play, ThumbsDown, ThumbsUp } from "lucide-react";
import type { Track } from "../types";
import { Cover } from "./Cover";
import { useTrackCover } from "../lib/useTrackCover";
import { cn } from "../lib/cn";
import { ServiceIcon } from "./ServiceIcon";
import { sourceName } from "../sources";
import { ArtistLink } from "./ArtistLink";

interface RecoTrackCardProps {
  track: Track;
  onPlay: (track: Track) => void;
  /** Текущая оценка: 1 — нравится, -1 — не нравится, 0 — не оценён. */
  score: number;
  onRate: (track: Track, score: 1 | -1) => void;
  disabled?: boolean;
}

/**
 * Карточка рекомендованного трека — та же вёрстка, что у TrackCard, плюс оценка
 * «нравится»/«не нравится». Корень намеренно не <button>: кнопки оценки живут
 * внутри карточки, а вложенные <button> — невалидная разметка (React ругается
 * предупреждением validateDOMNesting).
 */
export function RecoTrackCard({ track, onPlay, score, onRate, disabled }: RecoTrackCardProps) {
  const coverUrl = useTrackCover(track);
  const liked = score > 0;
  const disliked = score < 0;

  return (
    <div className="group flex flex-col text-left">
      <div className="relative">
        <button
          onClick={() => onPlay(track)}
          className="block w-full"
          aria-label={`Воспроизвести ${track.artist} — ${track.title}`}
        >
          <Cover
            accent={track.accent}
            src={coverUrl}
            alt={track.title}
            className="aspect-square w-full"
            iconSize={40}
            neon
          />
          <span className="pointer-events-none absolute bottom-2 right-2 grid h-11 w-11 translate-y-2 place-items-center rounded-full bg-[#a855f7] text-white opacity-0 shadow-lg shadow-[#a855f7]/40 transition-all duration-200 group-hover:translate-y-0 group-hover:opacity-100">
            <Play size={18} className="ml-0.5" fill="currentColor" />
          </span>
          {/* Значок источника: из какого сервиса поедет трек. Важно, потому что
              играют не все сервисы — пользователь сразу видит, откуда воспроизведётся. */}
          <span
            className="pointer-events-none absolute bottom-2 left-2 drop-shadow"
            title={`Источник: ${sourceName(track.source)}`}
          >
            <ServiceIcon id={track.source} size={20} badge />
          </span>
        </button>

        {/* Оценка. Появляется по наведению, но остаётся видимой, если оценка
            уже поставлена — иначе пользователь не понимает, что он нажимал. */}
        <div
          className={cn(
            "absolute left-2 top-2 flex gap-1 transition-opacity",
            liked || disliked ? "opacity-100" : "opacity-0 group-hover:opacity-100",
          )}
        >
          <button
            onClick={() => onRate(track, 1)}
            disabled={disabled}
            aria-pressed={liked}
            aria-label={liked ? "Убрать «нравится»" : "Нравится — больше такого"}
            title={liked ? "Убрать «нравится»" : "Нравится — больше такого"}
            className={cn(
              "grid h-8 w-8 place-items-center rounded-full backdrop-blur transition-colors disabled:opacity-50",
              liked
                ? "bg-[#a855f7] text-white shadow-lg shadow-[#a855f7]/40"
                : "bg-black/55 text-slate-200 hover:bg-black/75 hover:text-white",
            )}
          >
            <ThumbsUp size={14} fill={liked ? "currentColor" : "none"} />
          </button>
          <button
            onClick={() => onRate(track, -1)}
            disabled={disabled}
            aria-pressed={disliked}
            aria-label={disliked ? "Убрать «не нравится»" : "Не нравится — меньше такого"}
            title={disliked ? "Убрать «не нравится»" : "Не нравится — меньше такого"}
            className={cn(
              "grid h-8 w-8 place-items-center rounded-full backdrop-blur transition-colors disabled:opacity-50",
              disliked
                ? "bg-rose-500 text-white shadow-lg shadow-rose-500/40"
                : "bg-black/55 text-slate-200 hover:bg-black/75 hover:text-white",
            )}
          >
            <ThumbsDown size={14} fill={disliked ? "currentColor" : "none"} />
          </button>
        </div>
      </div>
      <strong className="mt-3 truncate text-[15px] font-semibold text-white">{track.title}</strong>
      <p className="truncate text-sm text-slate-400"><ArtistLink name={track.artist} /></p>
    </div>
  );
}
