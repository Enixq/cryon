import { Play } from "lucide-react";
import type { Track } from "../types";
import { Cover } from "./Cover";
import { useTrackCover } from "../lib/useTrackCover";

interface TrackCardProps {
  track: Track;
  onPlay: (track: Track) => void;
}

/** Вертикальная карточка трека с обложкой (сетка "Недавно прослушано"). */
export function TrackCard({ track, onPlay }: TrackCardProps) {
  const coverUrl = useTrackCover(track);
  return (
    <button
      onClick={() => onPlay(track)}
      className="group flex flex-col text-left"
    >
      <div className="relative">
        <Cover accent={track.accent} src={coverUrl} alt={track.title} className="aspect-square w-full" iconSize={40} neon />
        <span className="pointer-events-none absolute bottom-2 right-2 grid h-11 w-11 translate-y-2 place-items-center rounded-full bg-[#a855f7] text-white opacity-0 shadow-lg shadow-[#a855f7]/40 transition-all duration-200 group-hover:translate-y-0 group-hover:opacity-100">
          <Play size={18} className="ml-0.5" fill="currentColor" />
        </span>
      </div>
      <strong className="mt-3 truncate text-[15px] font-semibold text-white">{track.title}</strong>
      <p className="truncate text-sm text-slate-400">{track.artist}</p>
    </button>
  );
}
