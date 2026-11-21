import { useEffect, useState } from "react";
import { createPortal } from "react-dom";
import { Plus, Check, X, Search } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { addTrackToPlaylist, listFavorites, listLocalTracks, searchAll } from "../api/client";
import type { Track } from "../types";
import { notify } from "../../store/notificationStore";

export function PlaylistTrackPicker({
  playlistId,
  existing,
}: {
  playlistId: string;
  existing: Track[];
}) {
  const [open, setOpen] = useState(false),
    [query, setQuery] = useState(""),
    [search, setSearch] = useState("");
  const client = useQueryClient();
  useEffect(() => {
    const timer = setTimeout(() => setSearch(query.trim()), 350);
    return () => clearTimeout(timer);
  }, [query]);
  const { data: favorites = [] } = useQuery({
    queryKey: ["favorites"],
    queryFn: listFavorites,
    enabled: open,
  });
  const { data: local = [] } = useQuery({
    queryKey: ["localTracks"],
    queryFn: listLocalTracks,
    enabled: open,
  });
  const {
    data: results = [],
    isFetching,
    error,
  } = useQuery({
    queryKey: ["playlistSearch", search],
    queryFn: () => searchAll(search),
    enabled: open && search.length >= 2,
  });
  const add = useMutation({
    mutationFn: (track: Track) => addTrackToPlaylist(playlistId, track),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: ["playlistTracks", playlistId] });
      void client.invalidateQueries({ queryKey: ["playlists"] });
    },
    onError: (error) =>
      notify({ kind: "error", title: "Не удалось добавить трек", message: error.message }),
  });
  const items =
    search.length >= 2
      ? results
      : [...new Map([...favorites, ...local].map((track) => [track.id, track])).values()];
  return (
    <>
      <button
        type="button"
        onClick={() => setOpen(true)}
        className="flex items-center gap-2 self-start rounded-full border border-white/15 px-4 py-2.5 text-sm text-white hover:bg-white/10"
      >
        <Plus size={18} />
        Добавить треки
      </button>
      {open &&
        createPortal(
          <div
            className="fixed inset-0 z-[110] grid place-items-center bg-black/70 p-4"
            onClick={() => setOpen(false)}
          >
            <section
              role="dialog"
              aria-modal="true"
              aria-label="Добавить треки в плейлист"
              className="flex max-h-[80dvh] w-full max-w-xl flex-col gap-4 rounded-2xl border border-white/10 bg-[#141622] p-4 shadow-2xl"
              onClick={(event) => event.stopPropagation()}
            >
              <div className="flex items-center justify-between">
                <h2 className="font-semibold text-white">Добавить треки</h2>
                <button aria-label="Закрыть" onClick={() => setOpen(false)}>
                  <X size={22} />
                </button>
              </div>
              <label className="flex items-center gap-2 rounded-xl bg-white/5 px-3">
                <Search size={18} />
                <input
                  autoFocus
                  value={query}
                  onChange={(event) => setQuery(event.target.value)}
                  placeholder="Поиск по всем источникам"
                  className="min-w-0 flex-1 bg-transparent py-3 text-white outline-none"
                />
              </label>
              <div className="min-h-0 overflow-y-auto">
                {isFetching && <p className="p-3 text-sm text-slate-400">Поиск…</p>}
                {error && <p className="p-3 text-sm text-rose-300">{error.message}</p>}
                {items.map((track) => {
                  const added = existing.some((item) => item.id === track.id);
                  return (
                    <button
                      key={track.id}
                      disabled={added || add.isPending}
                      onClick={() => add.mutate(track)}
                      className="flex w-full items-center gap-3 rounded-xl p-3 text-left hover:bg-white/5 disabled:opacity-60"
                    >
                      <span className="min-w-0 flex-1">
                        <span className="block truncate text-sm text-white">{track.title}</span>
                        <span className="block truncate text-xs text-slate-400">
                          {track.artist}
                        </span>
                      </span>
                      {added ? <Check size={18} /> : <Plus size={18} />}
                    </button>
                  );
                })}
                {!items.length && !isFetching && (
                  <p className="p-5 text-center text-sm text-slate-400">
                    Введите название трека или исполнителя.
                  </p>
                )}
              </div>
            </section>
          </div>,
          document.body,
        )}
    </>
  );
}
