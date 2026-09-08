import { Play } from "lucide-react";
import type { SourceId, AccentColor } from "../types";
import { accentStyle } from "./accents";
import { sourceName } from "../sources";

interface PlaylistCardProps {
  playlist: {
    id: string;
    title: string;
    description: string;
    trackCount: number;
    coverUrl?: string;
    source?: SourceId;
    accent?: AccentColor;
  };
  onOpen?: (playlist: PlaylistCardProps["playlist"]) => void;
}

/** Карточка плейлиста с крупной обложкой и подписью сервиса. */
export function PlaylistCard({ playlist, onOpen }: PlaylistCardProps) {
  const accent = playlist.accent ?? "violet";
  const source = playlist.source ?? "local";
  return (
    <button onClick={() => onOpen?.(playlist)} className="group flex flex-col text-left">
      <div
        className="neon-frame relative flex aspect-square w-full items-end overflow-hidden rounded-2xl p-4"
        style={accentStyle(accent)}
      >
        {playlist.coverUrl && (
          <img
            src={playlist.coverUrl}
            alt={playlist.title}
            loading="lazy"
            onError={(e) => (e.currentTarget.style.display = "none")}
            className="absolute inset-0 h-full w-full object-cover"
          />
        )}
        <div className="absolute inset-0 bg-black/20 transition-colors group-hover:bg-black/10" />
        <span className="pointer-events-none absolute bottom-3 right-3 grid h-11 w-11 translate-y-2 place-items-center rounded-full bg-[#a855f7] text-white opacity-0 shadow-lg shadow-[#a855f7]/40 transition-all duration-200 group-hover:translate-y-0 group-hover:opacity-100">
          <Play size={18} className="ml-0.5" fill="currentColor" />
        </span>
      </div>
      <strong className="mt-3 truncate text-[15px] font-semibold text-white">{playlist.title}</strong>
      <p className="truncate text-sm text-slate-400">{playlist.description}</p>
      <span className="mt-0.5 truncate text-xs text-slate-500">
        Плейлист · {sourceName(source)}
      </span>
    </button>
  );
}
