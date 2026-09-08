import { useEffect, useMemo } from "react";
import { Play, Sparkles } from "lucide-react";
import { useNavigate, useParams } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { TrackRow } from "../shared/ui/TrackRow";
import { useTrackListPlayer } from "../store/playerStore";
import { accentStyle } from "../shared/ui/accents";
import { formatDuration, pluralWithCount } from "../shared/lib/format";
import {
  listFavorites,
  listHistory,
  listLocalTracks,
  onFavoritesChanged,
} from "../shared/api/client";
import { deriveRecommendations, deriveSmartPlaylists } from "../shared/lib/recommendations";

// Умный плейлист открывается как отдельный экран (а не «играет сразу по клику»):
// пользователь видит все треки списком и сам выбирает, что запустить. Подборки
// клиентские — собираются из избранного и истории теми же функциями, что и на
// главной, поэтому экран восстанавливается по :id при прямом заходе/обновлении.
export function SmartPlaylistPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { playTrack, isPlaying, activeTrackId } = useTrackListPlayer();
  const queryClient = useQueryClient();

  const { data: favorites = [] } = useQuery({ queryKey: ["favorites"], queryFn: listFavorites });
  const { data: history = [] } = useQuery({ queryKey: ["history"], queryFn: listHistory });
  const { data: localTracks = [] } = useQuery({ queryKey: ["localTracks"], queryFn: listLocalTracks });

  useEffect(() => {
    const off = onFavoritesChanged(() => {
      queryClient.invalidateQueries({ queryKey: ["favorites"] });
    });
    return off;
  }, [queryClient]);

  const playlist = useMemo(() => {
    const all = [
      ...deriveSmartPlaylists(favorites, history),
      ...deriveRecommendations(favorites, history, localTracks),
    ];
    return all.find((p) => p.id === id);
  }, [favorites, history, localTracks, id]);

  const tracks = playlist?.tracks ?? [];
  const totalSeconds = tracks.reduce((sum, t) => sum + t.duration, 0);

  if (!playlist || tracks.length === 0) {
    return (
      <div className="flex flex-col gap-6 py-4">
        <h1 className="text-3xl font-bold text-white">{playlist?.title ?? "Умный плейлист"}</h1>
        <div className="grid place-items-center rounded-2xl border border-white/8 bg-white/5 px-6 py-16 text-center text-slate-400">
          <div className="max-w-md">
            <Sparkles size={40} className="mx-auto mb-3 opacity-40" />
            <p>
              Этот умный плейлист собирается автоматически из вашего избранного и истории.
              Послушайте музыку и отметьте треки сердцем — и он появится здесь.
            </p>
            <button
              onClick={() => navigate("/")}
              className="mt-4 rounded-xl border border-white/10 bg-white/5 px-4 py-2 text-sm text-white transition-colors hover:bg-white/10"
            >
              На главную
            </button>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-6 py-4">
      {/* Шапка */}
      <div className="flex flex-col items-center gap-4 text-center sm:flex-row sm:flex-wrap sm:items-end sm:gap-6 sm:text-left">
        <div
          className="grid h-40 w-40 shrink-0 place-items-center rounded-2xl text-5xl font-black text-white/90 shadow-2xl"
          style={accentStyle(playlist.accent)}
        >
          {playlist.title.charAt(0)}
        </div>
        <div className="flex min-w-0 flex-col items-center gap-3 sm:items-start">
          <span className="text-sm font-medium uppercase tracking-wider text-slate-400">Умный плейлист</span>
          <h1 className="max-w-full break-words text-3xl font-black text-white sm:text-5xl">{playlist.title}</h1>
          <p className="text-slate-400">{playlist.description}</p>
          <div className="text-sm text-slate-500">
            {pluralWithCount(tracks.length, "трек", "трека", "треков")} · {formatDuration(totalSeconds)}
          </div>
        </div>
      </div>

      <button
        onClick={() => playTrack(tracks[0], tracks)}
        className="flex w-fit items-center gap-2 rounded-full bg-[#a855f7] px-6 py-3 font-semibold text-white shadow-lg shadow-[#a855f7]/40 transition-transform hover:scale-105"
      >
        <Play size={20} fill="currentColor" />
        Слушать
      </button>

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
    </div>
  );
}

export default SmartPlaylistPage;
