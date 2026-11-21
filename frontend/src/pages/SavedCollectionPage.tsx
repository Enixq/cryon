import { useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { Play } from "lucide-react";
import { listSavedCollections } from "../shared/api/collections";
import { getAlbumTracks, remotePlaylistTracks } from "../shared/api/client";
import { PageHeader } from "../shared/ui/PageHeader";
import { SaveCollectionButton } from "../shared/ui/SaveCollectionButton";
import { TrackRow } from "../shared/ui/TrackRow";
import { useTrackListPlayer } from "../store/playerStore";
import { pluralWithCount } from "../shared/lib/format";

export default function SavedCollectionPage() {
  const { id = "" } = useParams();
  const { data: saved = [], isLoading } = useQuery({
    queryKey: ["savedCollections"],
    queryFn: listSavedCollections,
  });
  const item = saved.find((value) => value.id === id);
  const { data: live } = useQuery({
    queryKey: ["savedCollectionTracks", id, item?.catalogId],
    queryFn: () =>
      item!.kind === "album"
        ? getAlbumTracks(item!.service, item!.catalogId!)
        : remotePlaylistTracks(item!.service, item!.catalogId!),
    enabled: Boolean(item?.catalogId) && item?.service !== "local",
    retry: false,
  });
  const tracks = live?.length ? live : (item?.tracks ?? []);
  const { playTrack, activeTrackId, isPlaying } = useTrackListPlayer();
  if (!item)
    return (
      <p className="py-8 text-slate-400">
        {isLoading ? "Загрузка…" : "Коллекция удалена из библиотеки."}
      </p>
    );
  return (
    <div className="flex flex-col gap-5 py-3">
      <PageHeader
        title={item.title}
        subtitle={`${item.artist || (item.kind === "album" ? "Альбом" : "Плейлист")} · ${pluralWithCount(tracks.length, "трек", "трека", "треков")}`}
      />
      <div className="flex flex-wrap gap-3">
        <button
          disabled={!tracks.length}
          onClick={() => playTrack(tracks[0], tracks)}
          className="flex items-center gap-2 rounded-full bg-[var(--app-accent)] px-5 py-2.5 font-semibold text-white disabled:opacity-40"
        >
          <Play size={18} />
          Слушать
        </button>
        <SaveCollectionButton collection={item} />
      </div>
      {tracks.map((track, index) => (
        <TrackRow
          key={track.id}
          track={track}
          index={index}
          active={track.id === activeTrackId}
          playing={isPlaying}
          onPlay={(value) => playTrack(value, tracks)}
          showAlbum
          showAddToPlaylist
        />
      ))}
    </div>
  );
}
