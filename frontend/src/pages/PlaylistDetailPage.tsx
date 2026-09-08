import { ArrowLeft, Clock3, GripVertical, Music2, Play, Trash2 } from "lucide-react";
import { useEffect, useState } from "react";
import type { DragEvent } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTrackListPlayer } from "../store/playerStore";
import { recommendedPlaylists, tracks, userPlaylists } from "../mocks/data";
import { accentStyle } from "../shared/ui/accents";
import { TrackRow } from "../shared/ui/TrackRow";
import { sourceName } from "../shared/sources";
import { ServiceIcon } from "../shared/ui/ServiceIcon";
import { formatDuration, pluralWithCount } from "../shared/lib/format";
import type { SourceId, Track } from "../shared/types";
import {
  isWailsRuntime,
  listPlaylistTracks,
  listPlaylists,
  removeTrackFromPlaylist,
  reorderPlaylistTracks,
} from "../shared/api/client";

export function PlaylistDetailPage() {
  const { id = "" } = useParams();
  const navigate = useNavigate();
  const { playTrack, isPlaying, activeTrackId } = useTrackListPlayer();
  const queryClient = useQueryClient();

  // Пользовательские плейлисты имеют id вида "pl_<timestamp>". Витринные
  // (рекомендованные/моки) — короткие id вроде "p1"/"up1".
  const isUserPlaylist = id.startsWith("pl_");

  const { data: playlists = [] } = useQuery({
    queryKey: ["playlists"],
    queryFn: listPlaylists,
    enabled: isUserPlaylist,
  });

  const { data: userTracks = [] } = useQuery({
    queryKey: ["playlistTracks", id],
    queryFn: () => listPlaylistTracks(id),
    enabled: isUserPlaylist,
  });

  const removeMutation = useMutation({
    mutationFn: (compositeId: string) => removeTrackFromPlaylist(id, compositeId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["playlistTracks", id] });
      queryClient.invalidateQueries({ queryKey: ["playlists"] });
    },
  });

  const reorderMutation = useMutation({
    mutationFn: (orderedIds: string[]) => reorderPlaylistTracks(id, orderedIds),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["playlistTracks", id] });
      queryClient.invalidateQueries({ queryKey: ["playlists"] });
    },
  });

  if (isUserPlaylist) {
    const meta = playlists.find((p) => p.id === id);
    const totalSeconds = userTracks.reduce((sum, t) => sum + t.duration, 0);
    // Уникальные источники в порядке первого появления — для бейджей в шапке.
    // Плейлист может смешивать треки из разных сервисов (YouTube + Yandex + …).
    const sources = userTracks.reduce<SourceId[]>((acc, t) => {
      if (!acc.includes(t.source)) acc.push(t.source);
      return acc;
    }, []);

    return (
      <div className="flex flex-col gap-6 py-2">
        <BackButton onClick={() => navigate(-1)} />

        <div className="flex flex-col items-center gap-4 text-center sm:flex-row sm:items-end sm:gap-6 sm:text-left">
          <div
            className="grid h-40 w-40 shrink-0 place-items-center rounded-2xl shadow-2xl sm:h-48 sm:w-48"
            style={{ background: "linear-gradient(135deg, #a855f7, #ec4899)" }}
          >
            <Music2 size={64} className="text-white/80" />
          </div>
          <div className="flex min-w-0 flex-col items-center gap-3 sm:items-start">
            <span className="text-sm font-medium uppercase tracking-wider text-slate-400">Плейлист</span>
            <h1 className="max-w-full break-words text-3xl font-black text-white sm:text-5xl">{meta?.title ?? "Плейлист"}</h1>
            {meta?.description && <p className="text-slate-400">{meta.description}</p>}
            <div className="flex items-center justify-center gap-2 text-sm text-slate-400 sm:justify-start">
              <span>{pluralWithCount(userTracks.length, "трек", "трека", "треков")}</span>
              <span>•</span>
              <span>{formatDuration(totalSeconds)}</span>
            </div>
            {sources.length > 0 && (
              <div className="flex flex-wrap items-center justify-center gap-2 sm:justify-start">
                {sources.map((src) => (
                  <span
                    key={src}
                    className="flex items-center gap-1.5 rounded-full border border-white/10 bg-white/5 px-2.5 py-1 text-xs text-slate-300"
                    title={sourceName(src)}
                  >
                    <ServiceIcon id={src} size={16} />
                    {sourceName(src)}
                  </span>
                ))}
                {sources.length > 1 && (
                  <span className="text-xs text-slate-500">микс из нескольких сервисов</span>
                )}
              </div>
            )}
          </div>
        </div>

        {userTracks.length > 0 ? (
          <>
            <div className="flex items-center justify-center gap-4 sm:justify-start">
              <button
                onClick={() => playTrack(userTracks[0], userTracks)}
                className="flex items-center gap-2 rounded-full bg-[#a855f7] px-6 py-3 font-semibold text-white shadow-lg shadow-[#a855f7]/40 transition-transform hover:scale-105"
              >
                <Play size={20} fill="currentColor" />
                Слушать
              </button>
            </div>

            <ReorderableTrackList
              tracks={userTracks}
              activeTrackId={activeTrackId}
              playing={isPlaying}
              onPlay={(t, list) => playTrack(t, list)}
              onRemove={(compositeId) => removeMutation.mutate(compositeId)}
              onReorder={(orderedIds) => reorderMutation.mutate(orderedIds)}
            />
          </>
        ) : (
          <div className="grid place-items-center rounded-2xl border border-white/8 bg-white/5 py-20 text-center text-slate-400">
            <div>
              <Music2 size={40} className="mx-auto mb-3 opacity-40" />
              <p>Плейлист пуст. Добавьте треки из поиска или библиотеки.</p>
            </div>
          </div>
        )}
      </div>
    );
  }

  // Витринный плейлист (моки) — как и раньше, срез общей коллекции. Но только
  // в веб-режиме: в приложении сюда попадают лишь id вида "pl_<timestamp>" из
  // listPlaylists(), поэтому короткий id означает битую ссылку. Показывать
  // вместо неё демо-треки нельзя — у них нет реального источника, и «Слушать»
  // молча ничего не делает.
  if (isWailsRuntime()) {
    return (
      <div className="flex flex-col gap-6 py-2">
        <BackButton onClick={() => navigate(-1)} />
        <div className="grid place-items-center rounded-2xl border border-white/8 bg-white/5 py-20 text-center text-slate-400">
          <div>
            <Music2 size={40} className="mx-auto mb-3 opacity-40" />
            <p>Плейлист не найден.</p>
          </div>
        </div>
      </div>
    );
  }

  const playlist =
    [...userPlaylists, ...recommendedPlaylists].find((p) => p.id === id) ?? userPlaylists[0];
  const playlistTracks = tracks.slice(0, 8);
  const totalSeconds = playlistTracks.reduce((sum, t) => sum + t.duration, 0);

  return (
    <div className="flex flex-col gap-6 py-2">
      <BackButton onClick={() => navigate(-1)} />

      <div className="flex flex-col items-center gap-4 text-center sm:flex-row sm:items-end sm:gap-6 sm:text-left">
        <div
          className="relative h-40 w-40 shrink-0 overflow-hidden rounded-2xl shadow-2xl sm:h-48 sm:w-48"
          style={accentStyle(playlist.accent)}
        >
          {playlist.coverUrl && (
            <img
              src={playlist.coverUrl}
              alt={playlist.title}
              onError={(e) => (e.currentTarget.style.display = "none")}
              className="absolute inset-0 h-full w-full object-cover"
            />
          )}
        </div>
        <div className="flex min-w-0 flex-col items-center gap-3 sm:items-start">
          <span className="text-sm font-medium uppercase tracking-wider text-slate-400">Плейлист</span>
          <h1 className="max-w-full break-words text-3xl font-black text-white sm:text-5xl">{playlist.title}</h1>
          <p className="text-slate-400">{playlist.description}</p>
          <div className="flex flex-wrap items-center justify-center gap-2 text-sm text-slate-400 sm:justify-start">
            <span>{sourceName(playlist.source)}</span>
            <span>•</span>
            <span>{pluralWithCount(playlistTracks.length, "трек", "трека", "треков")}</span>
            <span>•</span>
            <span>{formatDuration(totalSeconds)}</span>
          </div>
        </div>
      </div>

      <div className="flex items-center justify-center gap-4 sm:justify-start">
        <button
          onClick={() => playTrack(playlistTracks[0], playlistTracks)}
          className="flex items-center gap-2 rounded-full bg-[#a855f7] px-6 py-3 font-semibold text-white shadow-lg shadow-[#a855f7]/40 transition-transform hover:scale-105"
        >
          <Play size={20} fill="currentColor" />
          Слушать
        </button>
      </div>

      <div className="flex flex-col">
        <div className="grid grid-cols-[40px_1fr_auto] items-center gap-4 border-b border-white/8 px-3 pb-2 text-xs uppercase tracking-wider text-slate-500">
          <span>#</span>
          <span>Название</span>
          <Clock3 size={14} className="justify-self-end" />
        </div>
        {playlistTracks.map((track, i) => (
          <TrackRow
            key={track.id}
            track={track}
            index={i}
            active={activeTrackId === track.id}
            playing={isPlaying}
            onPlay={(t) => playTrack(t, playlistTracks)}
            showAlbum
          />
        ))}
      </div>
    </div>
  );
}

interface ReorderableTrackListProps {
  tracks: Track[];
  activeTrackId: string | undefined;
  playing: boolean;
  onPlay: (track: Track, list: Track[]) => void;
  onRemove: (compositeId: string) => void;
  onReorder: (orderedIds: string[]) => void;
}

/**
 * Список треков плейлиста с перетаскиванием строк (HTML5 drag-and-drop).
 * Порядок обновляется локально во время перетаскивания (оптимистично), а по
 * отпусканию сохраняется в backend через onReorder. Внешние обновления списка
 * подхватываются, пока пользователь ничего не тащит.
 */
function ReorderableTrackList({
  tracks,
  activeTrackId,
  playing,
  onPlay,
  onRemove,
  onReorder,
}: ReorderableTrackListProps) {
  const [order, setOrder] = useState<Track[]>(tracks);
  const [dragIndex, setDragIndex] = useState<number | null>(null);
  const [overIndex, setOverIndex] = useState<number | null>(null);

  // Синхронизируем локальный порядок с сервером, только когда не тащим — иначе
  // инвалидация во время drag сбросила бы промежуточное состояние.
  useEffect(() => {
    if (dragIndex === null) setOrder(tracks);
  }, [tracks, dragIndex]);

  function handleDragStart(index: number) {
    setDragIndex(index);
    setOverIndex(index);
  }

  function handleDragOver(index: number, e: DragEvent) {
    e.preventDefault();
    if (dragIndex === null || index === dragIndex) {
      setOverIndex(index);
      return;
    }
    setOrder((prev) => {
      const next = [...prev];
      const [moved] = next.splice(dragIndex, 1);
      next.splice(index, 0, moved);
      return next;
    });
    setDragIndex(index);
    setOverIndex(index);
  }

  function handleDrop() {
    const finalOrder = order;
    setDragIndex(null);
    setOverIndex(null);
    // Сохраняем, только если порядок реально изменился относительно сервера.
    const changed = finalOrder.some((t, i) => t.id !== tracks[i]?.id);
    if (changed) onReorder(finalOrder.map((t) => t.id));
  }

  return (
    <div className="flex flex-col">
      <div className="grid grid-cols-[40px_1fr_auto] items-center gap-4 border-b border-white/8 px-3 pb-2 text-xs uppercase tracking-wider text-slate-500">
        <span>#</span>
        <span>Название</span>
        <Clock3 size={14} className="justify-self-end" />
      </div>
      {order.map((track, i) => (
        <div
          key={track.id}
          className={`group/row relative rounded-xl transition-colors ${
            overIndex === i && dragIndex !== null ? "bg-white/5" : ""
          } ${dragIndex === i ? "opacity-50" : ""}`}
          onDragOver={(e) => handleDragOver(i, e)}
          onDrop={handleDrop}
        >
          <div
            draggable
            onDragStart={() => handleDragStart(i)}
            onDragEnd={handleDrop}
            className="absolute left-0 top-1/2 z-10 flex h-8 w-8 -translate-x-full -translate-y-1/2 cursor-grab items-center justify-center text-slate-600 opacity-0 transition-opacity hover:text-slate-300 group-hover/row:opacity-100 active:cursor-grabbing"
            aria-label="Перетащить для сортировки"
          >
            <GripVertical size={16} />
          </div>
          <TrackRow
            track={track}
            index={i}
            active={activeTrackId === track.id}
            playing={playing}
            onPlay={(t) => onPlay(t, order)}
            showAlbum
          />
          <button
            onClick={() => onRemove(track.id)}
            className="absolute right-16 top-1/2 -translate-y-1/2 text-slate-500 opacity-0 transition-opacity hover:text-white group-hover/row:opacity-100"
            aria-label="Убрать из плейлиста"
          >
            <Trash2 size={15} />
          </button>
        </div>
      ))}
    </div>
  );
}

function BackButton({ onClick }: { onClick: () => void }) {
  return (
    <button
      onClick={onClick}
      className="flex w-fit items-center gap-2 text-sm text-slate-400 transition-colors hover:text-white"
    >
      <ArrowLeft size={16} />
      Назад
    </button>
  );
}
