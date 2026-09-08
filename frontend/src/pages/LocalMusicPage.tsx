import { FolderOpen, HardDrive, Play, RefreshCw, Trash2, X } from "lucide-react";
import { useState } from "react";
import { useQuery, useQueryClient, useMutation } from "@tanstack/react-query";
import { useTrackListPlayer } from "../store/playerStore";
import {
  addLocalFolder,
  listLocalFolders,
  listLocalTracks,
  pickMusicFolder,
  removeLocalFolder,
  rescanLocalLibrary,
} from "../shared/api/client";
import { TrackRow } from "../shared/ui/TrackRow";
import { formatDuration, pluralWithCount } from "../shared/lib/format";

export function LocalMusicPage() {
  const { playTrack, isPlaying, activeTrackId } = useTrackListPlayer();
  const queryClient = useQueryClient();
  const [folderPendingRemoval, setFolderPendingRemoval] = useState<string | null>(null);

  const { data: tracks = [] } = useQuery({
    queryKey: ["localTracks"],
    queryFn: listLocalTracks,
  });
  const { data: folders = [] } = useQuery({
    queryKey: ["localFolders"],
    queryFn: listLocalFolders,
  });

  const refresh = () => {
    queryClient.invalidateQueries({ queryKey: ["localTracks"] });
    queryClient.invalidateQueries({ queryKey: ["localFolders"] });
  };

  const addMutation = useMutation({
    mutationFn: async () => {
      const path = await pickMusicFolder();
      if (!path) return null;
      return addLocalFolder(path);
    },
    onSuccess: refresh,
  });

  const removeMutation = useMutation({
    mutationFn: (path: string) => removeLocalFolder(path),
    onSuccess: () => {
      setFolderPendingRemoval(null);
      refresh();
    },
  });

  const rescanMutation = useMutation({
    mutationFn: () => rescanLocalLibrary(),
    onSuccess: refresh,
  });

  const totalSeconds = tracks.reduce((sum, t) => sum + t.duration, 0);
  const scanning = addMutation.isPending || rescanMutation.isPending;

  return (
    <div className="flex flex-col gap-6 py-2">
      {/* Шапка */}
      <div className="flex flex-col items-center gap-4 text-center sm:flex-row sm:items-end sm:gap-6 sm:text-left">
        <div
          className="grid h-40 w-40 shrink-0 place-items-center rounded-2xl shadow-2xl sm:h-48 sm:w-48"
          style={{ background: "linear-gradient(135deg, #7c8cff, #22d3ee)" }}
        >
          <HardDrive size={72} className="text-white" />
        </div>
        <div className="flex flex-col items-center gap-3 sm:items-start">
          <span className="text-sm font-medium uppercase tracking-wider text-slate-400">Источник</span>
          <h1 className="text-3xl font-black text-white sm:text-5xl">Локальная музыка</h1>
          <div className="flex items-center justify-center gap-2 text-sm text-slate-400 sm:justify-start">
            <span>{pluralWithCount(tracks.length, "трек", "трека", "треков")}</span>
            <span>•</span>
            <span>{pluralWithCount(folders.length, "папка", "папки", "папок")}</span>
          </div>
        </div>
      </div>

      {/* Действия */}
      <div className="flex flex-wrap items-center justify-center gap-3 sm:justify-start">
        <button
          onClick={() => tracks.length > 0 && playTrack(tracks[0], tracks)}
          disabled={tracks.length === 0}
          className="flex items-center gap-2 rounded-full bg-[#a855f7] px-6 py-3 font-semibold text-white shadow-lg shadow-[#a855f7]/40 transition-transform hover:scale-105 disabled:opacity-40 disabled:hover:scale-100"
        >
          <Play size={20} fill="currentColor" />
          Слушать
        </button>
        <button
          onClick={() => addMutation.mutate()}
          disabled={scanning}
          className="flex items-center gap-2 rounded-xl bg-white/8 px-4 py-2.5 text-sm font-medium text-white transition-colors hover:bg-white/12 disabled:opacity-50"
        >
          <FolderOpen size={16} />
          Добавить папку
        </button>
        <button
          onClick={() => rescanMutation.mutate()}
          disabled={scanning || folders.length === 0}
          className="flex items-center gap-2 rounded-xl bg-white/8 px-4 py-2.5 text-sm font-medium text-white transition-colors hover:bg-white/12 disabled:opacity-50"
        >
          <RefreshCw size={16} className={scanning ? "animate-spin" : ""} />
          {scanning ? "Сканирование…" : "Пересканировать"}
        </button>
      </div>

      {/* Отслеживаемые папки */}
      <section className="flex flex-col gap-3">
        <h2 className="text-xl font-bold text-white">Папки с музыкой</h2>
        {folders.length > 0 ? (
          <div className="flex flex-col gap-2">
            {folders.map((path) => (
              <div
                key={path}
                className="group flex items-center gap-3 rounded-xl bg-white/5 px-4 py-3 transition-colors hover:bg-white/8"
              >
                <FolderOpen size={18} className="shrink-0 text-[#7c8cff]" />
                <span className="min-w-0 flex-1 truncate text-[15px] text-white">{path}</span>
                {folderPendingRemoval === path ? (
                  <div className="flex shrink-0 items-center gap-2 text-xs">
                    <span className="hidden text-slate-300 sm:inline">Убрать из библиотеки?</span>
                    <button
                      onClick={() => removeMutation.mutate(path)}
                      disabled={removeMutation.isPending}
                      className="rounded-lg bg-red-500/20 px-2.5 py-1.5 font-medium text-red-200 transition-colors hover:bg-red-500/35 disabled:opacity-50"
                    >
                      Убрать
                    </button>
                    <button
                      onClick={() => setFolderPendingRemoval(null)}
                      disabled={removeMutation.isPending}
                      className="rounded-lg bg-white/8 px-2.5 py-1.5 text-slate-300 transition-colors hover:bg-white/15 disabled:opacity-50"
                    >
                      Отмена
                    </button>
                  </div>
                ) : (
                  <button
                    onClick={() => setFolderPendingRemoval(path)}
                    className="shrink-0 text-slate-500 opacity-100 transition-colors hover:text-red-300 sm:opacity-0 sm:group-hover:opacity-100"
                    aria-label="Убрать папку из библиотеки"
                    title="Убрать папку из библиотеки"
                  >
                    <Trash2 size={16} />
                  </button>
                )}
              </div>
            ))}
          </div>
        ) : (
          <div className="grid place-items-center rounded-2xl border border-dashed border-white/10 bg-white/5 py-14 text-center text-slate-400">
            <div>
              <FolderOpen size={36} className="mx-auto mb-3 opacity-40" />
              <p className="mb-4">Папки не добавлены. Укажите, где хранится ваша музыка.</p>
              <button
                onClick={() => addMutation.mutate()}
                disabled={scanning}
                className="mx-auto flex items-center gap-2 rounded-xl bg-white/8 px-4 py-2.5 text-sm font-medium text-white transition-colors hover:bg-white/12 disabled:opacity-50"
              >
                <FolderOpen size={16} />
                Добавить папку
              </button>
            </div>
          </div>
        )}
      </section>

      {/* Список треков */}
      <section className="flex flex-col gap-3">
        <div className="flex items-center justify-between">
          <h2 className="text-xl font-bold text-white">Треки</h2>
          <span className="text-sm text-slate-400">
            {pluralWithCount(tracks.length, "трек", "трека", "треков")} · {formatDuration(totalSeconds)}
          </span>
        </div>
        {tracks.length > 0 ? (
          <div className="flex flex-col">
            {tracks.map((track, i) => (
              <TrackRow
                key={track.id}
                track={track}
                index={i}
                active={activeTrackId === track.id}
                playing={isPlaying}
                onPlay={(t) => playTrack(t, tracks)}
                showAlbum
              />
            ))}
          </div>
        ) : (
          <div className="grid place-items-center rounded-2xl border border-white/8 bg-white/5 py-16 text-center text-slate-400">
            <div>
              <X size={36} className="mx-auto mb-3 opacity-40" />
              <p>В отслеживаемых папках пока нет треков.</p>
            </div>
          </div>
        )}
      </section>
    </div>
  );
}
