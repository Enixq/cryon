import { Play } from "lucide-react";
import type { Track } from "../types";
import { Cover } from "./Cover";
import { useTrackCover } from "../lib/useTrackCover";
import { ArtistLink } from "./ArtistLink";

interface TrackCardProps {
  track: Track;
  onPlay: (track: Track) => void;
}

export function TrackCard({ track, onPlay }: TrackCardProps) {
  const coverUrl = useTrackCover(track);

  return (
    <div className="group flex min-w-0 flex-col">
      <div
        role="button"
        tabIndex={0}
        onClick={() => onPlay(track)}
        onKeyDown={(event) => {
          if (event.key === "Enter" || event.key === " ") {
            event.preventDefault();
            onPlay(track);
          }
        }}
        className="flex min-w-0 cursor-pointer flex-col text-left outline-none"
        aria-label={`Play: ${track.title}`}
      >
        <div className="relative">
          <Cover accent={track.accent} src={coverUrl} alt={track.title} className="aspect-square w-full" iconSize={40} neon />
          <span className="pointer-events-none absolute bottom-2 right-2 grid h-11 w-11 translate-y-2 place-items-center rounded-full bg-[#a855f7] text-white opacity-0 shadow-lg shadow-[#a855f7]/40 transition-all duration-200 group-hover:translate-y-0 group-hover:opacity-100 group-focus-within:translate-y-0 group-focus-within:opacity-100">
            <Play size={18} className="ml-0.5" fill="currentColor" />
          </span>
        </div>
        <strong className="mt-2 line-clamp-2 whitespace-normal text-[15px] font-semibold text-white">{track.title}</strong>
      </div>
      <p className="line-clamp-2 whitespace-normal text-sm text-slate-400"><ArtistLink name={track.artist} /></p>
    </div>
  );
}
