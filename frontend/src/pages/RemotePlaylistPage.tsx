import { ArrowLeft, Play, Music2 } from "lucide-react";
import { useLocation, useNavigate, useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { useTrackListPlayer } from "../store/playerStore";
import { remotePlaylistTracks } from "../shared/api/client";
import { SaveCollectionButton } from "../shared/ui/SaveCollectionButton";
import { TrackRow } from "../shared/ui/TrackRow";
import { sourceName } from "../shared/sources";
import type { SourceId } from "../shared/types";

export function RemotePlaylistPage() {
  const { source = "youtube", id = "" } = useParams();
  const navigate = useNavigate();
  const location = useLocation();
  const { playTrack, isPlaying, activeTrackId } = useTrackListPlayer();
  const sourceId = source as SourceId;
  const {
    data: tracks = [],
    isFetching,
    isError,
  } = useQuery({
    queryKey: ["remotePlaylistTracks", sourceId, id],
    queryFn: () => remotePlaylistTracks(sourceId, decodeURIComponent(id)),
    enabled: Boolean(id),
    retry: 1,
  });
  const title =
    (location.state as { playlist?: { title?: string } } | null)?.playlist?.title ||
    "Публичный плейлист";

  return (
    <div className="flex flex-col gap-6 py-2">
      <button
        type="button"
        onClick={() => navigate(-1)}
        className="flex w-fit items-center gap-2 text-sm text-slate-400 hover:text-white"
      >
        <ArrowLeft size={16} /> Назад
      </button>
      <div className="flex flex-col items-center gap-4 text-center sm:flex-row sm:items-end sm:text-left">
        <div className="grid h-40 w-40 shrink-0 place-items-center rounded-2xl bg-gradient-to-br from-violet-500 to-pink-500 shadow-2xl">
          <Music2 size={64} className="text-white/80" />
        </div>
        <div>
          <p className="text-sm font-medium uppercase tracking-wide text-slate-400">
            Публичный плейлист · {sourceName(sourceId)}
          </p>
          <h1 className="mt-1 text-3xl font-black text-white sm:text-5xl">{title}</h1>
          <p className="mt-2 text-sm text-slate-400">
            {isFetching ? "Загружаем треки…" : `${tracks.length} треков`}
          </p>
          <SaveCollectionButton
            collection={{
              id: `playlist:${sourceId}:${id}`,
              kind: "playlist",
              title,
              service: sourceId,
              catalogId: decodeURIComponent(id),
              artworkUrl: tracks[0]?.coverUrl,
              tracks,
            }}
          />
          {tracks.length > 0 && (
            <button
              type="button"
              onClick={() => playTrack(tracks[0], tracks)}
              className="mt-4 flex items-center gap-2 rounded-full bg-[#a855f7] px-5 py-2.5 font-semibold text-white"
            >
              <Play size={18} fill="currentColor" /> Слушать
            </button>
          )}
        </div>
      </div>
      {isError && (
        <p className="rounded-xl bg-white/5 p-6 text-center text-slate-400">
          Не удалось загрузить плейлист.
        </p>
      )}
      {!isFetching && !isError && tracks.length === 0 && (
        <p className="rounded-xl bg-white/5 p-6 text-center text-slate-400">
          В плейлисте нет доступных треков.
        </p>
      )}
      <div className="flex flex-col">
        {tracks.map((track, index) => (
          <TrackRow
            key={track.id}
            track={track}
            index={index}
            active={activeTrackId === track.id}
            playing={isPlaying}
            onPlay={(item) => playTrack(item, tracks)}
            showAlbum
          />
        ))}
      </div>
    </div>
  );
}

export default RemotePlaylistPage;
