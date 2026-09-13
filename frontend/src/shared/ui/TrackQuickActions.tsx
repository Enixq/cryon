import { Heart, ListPlus, ListStart, MoreHorizontal, Share2 } from "lucide-react";
import { useState } from "react";
import { usePlayerStore } from "../../store/playerStore";
import { useUiStore } from "../../store/uiStore";
import type { Track } from "../types";
import { AddToPlaylistButton } from "./AddToPlaylistButton";
import { cn } from "../lib/cn";
import { useFavoriteIds } from "../lib/useFavorites";

interface Props { track: Track; }

export function TrackQuickActions({ track }: Props) {
  const [open, setOpen] = useState(false);
  const favoriteIds = useFavoriteIds();
  const isFavorite = favoriteIds.has(track.id);
  const addToQueue = usePlayerStore((state) => state.addToQueue);
  const playNext = usePlayerStore((state) => state.playNext);
  const playTrack = usePlayerStore((state) => state.playTrack);
  const toggleLikeWithTrack = usePlayerStore((state) => state.toggleLikeWithTrack);
  const openShare = useUiStore((state) => state.openShare);

  return (
    <div className="relative flex items-center gap-0.5">
      <button type="button" onClick={(event) => { event.stopPropagation(); addToQueue(track); }} className="grid h-9 w-9 place-items-center rounded-full text-lg text-slate-400 transition-colors hover:bg-[color-mix(in_srgb,var(--app-accent)_18%,transparent)] hover:text-white active:scale-95" aria-label="Добавить в очередь">+</button>
      <button type="button" onClick={(event) => { event.stopPropagation(); setOpen((value) => !value); }} className="grid h-9 w-9 place-items-center rounded-full text-slate-400 transition-colors hover:bg-white/10 hover:text-white active:scale-95" aria-label="Другие действия"><MoreHorizontal size={17} /></button>
      {open && (
        <div className="absolute right-0 top-9 z-20 w-56 rounded-xl border border-white/10 bg-[#171722] p-2 shadow-2xl">
          <button type="button" onClick={(event) => { event.stopPropagation(); playNext(track); setOpen(false); }} className="flex w-full items-center gap-2 rounded-lg px-2 py-2 text-left text-sm text-slate-200 hover:bg-white/8"><ListStart size={15} />Играть дальше</button>
          <button type="button" onClick={(event) => { event.stopPropagation(); playTrack(track, [track]); setOpen(false); }} className="flex w-full items-center gap-2 rounded-lg px-2 py-2 text-left text-sm text-slate-200 hover:bg-white/8"><ListPlus size={15} />Играть сейчас</button>
          <button type="button" onClick={(event) => { event.stopPropagation(); toggleLikeWithTrack({ ...track, liked: isFavorite }); setOpen(false); }} className={cn("flex w-full items-center gap-2 rounded-lg px-2 py-2 text-left text-sm hover:bg-white/8", isFavorite ? "text-[#a855f7]" : "text-slate-200")}><Heart size={15} fill={isFavorite ? "currentColor" : "none"} />{isFavorite ? "Убрать из избранного" : "В избранное"}</button>
          <button type="button" onClick={(event) => { event.stopPropagation(); openShare(track); setOpen(false); }} className="flex w-full items-center gap-2 rounded-lg px-2 py-2 text-left text-sm text-slate-200 hover:bg-white/8"><Share2 size={15} />Поделиться</button>
          <div className="mt-1 border-t border-white/8 pt-2"><AddToPlaylistButton track={track} /></div>
        </div>
      )}
    </div>
  );
}
