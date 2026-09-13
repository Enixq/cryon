import { ListPlus, PlusCircle, Heart, ListMusic, ListStart, Share2 } from "lucide-react";
import { useState } from "react";
import { usePlayerStore } from "../../store/playerStore";
import { useUiStore } from "../../store/uiStore";
import type { Track } from "../types";
import { AddToPlaylistButton } from "./AddToPlaylistButton";
import { cn } from "../lib/cn";
import { useFavoriteIds } from "../lib/useFavorites";

interface Props {
  track: Track;
}

export function TrackQuickActions({ track }: Props) {
  const [open, setOpen] = useState(false);
  const favoriteIds = useFavoriteIds();
  const isFavorite = favoriteIds.has(track.id);
  const addToQueue = usePlayerStore((s) => s.addToQueue);
  const playNext = usePlayerStore((s) => s.playNext);
  const playTrack = usePlayerStore((s) => s.playTrack);
  const toggleLikeWithTrack = usePlayerStore((s) => s.toggleLikeWithTrack);
  const openShare = useUiStore((s) => s.openShare);

  return (
    <div className="relative">
      <button
        onClick={(e) => {
          e.stopPropagation();
          setOpen((v) => !v);
        }}
        className="grid h-9 w-9 place-items-center rounded-full text-slate-400 transition-colors hover:bg-white/10 hover:text-white active:scale-95"
        aria-label="Быстрые действия"
      >
        <PlusCircle size={16} />
      </button>

      {open && (
        <div className="absolute right-0 top-9 z-20 w-56 rounded-xl border border-white/10 bg-[#171722] p-2 shadow-2xl">
          <button
            onClick={(e) => {
              e.stopPropagation();
              addToQueue(track);
              setOpen(false);
            }}
            className="flex w-full items-center gap-2 rounded-lg px-2 py-2 text-left text-sm text-slate-200 transition-colors hover:bg-white/8"
          >
            <ListMusic size={15} />
            В очередь
          </button>
          <button
            onClick={(e) => {
              e.stopPropagation();
              playNext(track);
              setOpen(false);
            }}
            className="flex w-full items-center gap-2 rounded-lg px-2 py-2 text-left text-sm text-slate-200 transition-colors hover:bg-white/8"
          >
            <ListStart size={15} />
            Играть дальше
          </button>
          <button
            onClick={(e) => {
              e.stopPropagation();
              playTrack(track, [track]);
              setOpen(false);
            }}
            className="flex w-full items-center gap-2 rounded-lg px-2 py-2 text-left text-sm text-slate-200 transition-colors hover:bg-white/8"
          >
            <ListPlus size={15} />
            Играть сейчас
          </button>
          <button
            onClick={(e) => {
              e.stopPropagation();
              toggleLikeWithTrack({ ...track, liked: isFavorite });
              setOpen(false);
            }}
            className={cn("flex w-full items-center gap-2 rounded-lg px-2 py-2 text-left text-sm transition-colors hover:bg-white/8", isFavorite ? "text-[#a855f7]" : "text-slate-200")}
          >
            <Heart size={15} fill={isFavorite ? "currentColor" : "none"} />
            {isFavorite ? "Убрать из избранного" : "В избранное"}
          </button>
          <button
            onClick={(e) => {
              e.stopPropagation();
              openShare(track);
              setOpen(false);
            }}
            className="flex w-full items-center gap-2 rounded-lg px-2 py-2 text-left text-sm text-slate-200 transition-colors hover:bg-white/8"
          >
            <Share2 size={15} />
            Поделиться
          </button>
          <div className="mt-1 border-t border-white/8 pt-2">
            <AddToPlaylistButton track={track} />
          </div>
        </div>
      )}
    </div>
  );
}
