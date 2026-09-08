import { Heart, Play } from "lucide-react";
import { useEffect } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useTrackListPlayer } from "../store/playerStore";
import { listFavorites, onFavoritesChanged } from "../shared/api/client";
import { TrackRow } from "../shared/ui/TrackRow";
import { formatDuration, pluralWithCount } from "../shared/lib/format";

export function FavoritesPage() {
  const { playTrack, isPlaying, activeTrackId } = useTrackListPlayer();
  const queryClient = useQueryClient();

  const { data: likedTracks = [] } = useQuery({
    queryKey: ["favorites"],
    queryFn: listFavorites,
  });

  useEffect(() => {
    const off = onFavoritesChanged(() => {
      queryClient.invalidateQueries({ queryKey: ["favorites"] });
    });
    return off;
  }, [queryClient]);

  const totalSeconds = likedTracks.reduce((sum, t) => sum + t.duration, 0);

  return (
    <div className="flex flex-col gap-6 py-2">
      {/* Шапка */}
      <div className="flex flex-col items-center gap-4 text-center sm:flex-row sm:items-end sm:gap-6 sm:text-left">
        <div
          className="grid h-40 w-40 shrink-0 place-items-center rounded-2xl shadow-2xl sm:h-48 sm:w-48"
          style={{ background: "linear-gradient(135deg, #a855f7, #ec4899)" }}
        >
          <Heart size={72} className="text-white" fill="currentColor" />
        </div>
        <div className="flex flex-col items-center gap-3 sm:items-start">
          <span className="text-sm font-medium uppercase tracking-wider text-slate-400">Коллекция</span>
          <h1 className="text-3xl font-black text-white sm:text-5xl">Избранное</h1>
          <div className="flex items-center justify-center gap-2 text-sm text-slate-400 sm:justify-start">
            <span>{pluralWithCount(likedTracks.length, "трек", "трека", "треков")}</span>
            <span>•</span>
            <span>{formatDuration(totalSeconds)}</span>
          </div>
        </div>
      </div>

      {likedTracks.length > 0 ? (
        <>
          <button
            onClick={() => playTrack(likedTracks[0], likedTracks)}
            className="flex w-fit items-center gap-2 rounded-full bg-[#a855f7] px-6 py-3 font-semibold text-white shadow-lg shadow-[#a855f7]/40 transition-transform hover:scale-105"
          >
            <Play size={20} fill="currentColor" />
            Слушать
          </button>

          <div className="flex flex-col">
            {likedTracks.map((track, i) => (
              <TrackRow
                key={track.id}
                track={{ ...track, liked: true }}
                index={i}
                active={activeTrackId === track.id}
                playing={isPlaying}
                onPlay={(t) => playTrack(t, likedTracks)}
                showAlbum
              />
            ))}
          </div>
        </>
      ) : (
        <div className="grid place-items-center rounded-2xl border border-white/8 bg-white/5 py-20 text-center text-slate-400">
          <div>
            <Heart size={40} className="mx-auto mb-3 opacity-40" />
            <p>Здесь появятся треки, которые вы отметите сердечком.</p>
          </div>
        </div>
      )}
    </div>
  );
}
