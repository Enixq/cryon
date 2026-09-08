import { Check, Plus } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { Track } from "../types";
import {
  addTrackToPlaylist,
  createPlaylist,
  listPlaylists,
} from "../api/client";
import { cn } from "../lib/cn";

interface Props {
  track: Track;
}

/**
 * Кнопка «+» рядом с треком: открывает меню пользовательских плейлистов,
 * позволяет добавить трек в существующий или создать новый плейлист.
 */
export function AddToPlaylistButton({ track }: Props) {
  const [open, setOpen] = useState(false);
  const [newTitle, setNewTitle] = useState("");
  const [added, setAdded] = useState<string | null>(null);
  const containerRef = useRef<HTMLDivElement | null>(null);
  const queryClient = useQueryClient();

  const { data: playlists = [] } = useQuery({
    queryKey: ["playlists"],
    queryFn: listPlaylists,
    enabled: open,
  });

  const addMutation = useMutation({
    mutationFn: (playlistId: string) => addTrackToPlaylist(playlistId, track),
    onSuccess: (_data, playlistId) => {
      queryClient.invalidateQueries({ queryKey: ["playlists"] });
      queryClient.invalidateQueries({ queryKey: ["playlistTracks", playlistId] });
      setAdded(playlistId);
      window.setTimeout(() => setAdded(null), 1200);
    },
  });

  const createMutation = useMutation({
    mutationFn: async () => {
      const pl = await createPlaylist(newTitle.trim());
      if (pl) await addTrackToPlaylist(pl.id, track);
      return pl;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["playlists"] });
      setNewTitle("");
      setOpen(false);
    },
  });

  // Закрытие меню по клику вне его.
  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener("mousedown", onDown);
    return () => document.removeEventListener("mousedown", onDown);
  }, [open]);

  const canCreate = newTitle.trim().length > 0 && !createMutation.isPending;

  return (
    <div ref={containerRef} className="relative">
      <button
        onClick={(e) => {
          e.stopPropagation();
          setOpen((v) => !v);
        }}
        className={cn(
          "grid h-7 w-7 place-items-center rounded-full text-slate-400 transition-colors hover:bg-white/10 hover:text-white",
          !open && "opacity-0 group-hover:opacity-100",
        )}
        aria-label="Добавить в плейлист"
      >
        <Plus size={16} />
      </button>

      {open && (
        <div
          className="absolute right-0 top-9 z-20 w-60 rounded-xl border border-white/10 bg-[#1a1a24] p-2 shadow-2xl"
          onClick={(e) => e.stopPropagation()}
        >
          <div className="px-2 py-1 text-xs uppercase tracking-wider text-slate-500">
            Добавить в плейлист
          </div>

          <div className="max-h-52 overflow-y-auto">
            {playlists.length === 0 ? (
              <div className="px-2 py-2 text-sm text-slate-500">Плейлистов пока нет</div>
            ) : (
              playlists.map((p) => (
                <button
                  key={p.id}
                  onClick={() => addMutation.mutate(p.id)}
                  className="flex w-full items-center justify-between rounded-lg px-2 py-2 text-left text-sm text-slate-200 transition-colors hover:bg-white/8"
                >
                  <span className="truncate">{p.title}</span>
                  {added === p.id && <Check size={15} className="text-[#a855f7]" />}
                </button>
              ))
            )}
          </div>

          <div className="mt-1 flex items-center gap-2 border-t border-white/8 pt-2">
            <input
              value={newTitle}
              onChange={(e) => setNewTitle(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && canCreate && createMutation.mutate()}
              placeholder="Новый плейлист"
              className="min-w-0 flex-1 rounded-lg border border-white/8 bg-white/5 px-2 py-1.5 text-sm text-white placeholder:text-slate-500 focus:border-[#a855f7] focus:outline-none"
            />
            <button
              onClick={() => createMutation.mutate()}
              disabled={!canCreate}
              className="grid h-8 w-8 shrink-0 place-items-center rounded-lg bg-[#a855f7] text-white transition-colors hover:bg-[#9333ea] disabled:opacity-40"
              aria-label="Создать и добавить"
            >
              <Plus size={16} />
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
