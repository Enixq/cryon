import { Music2, Plus, Trash2, X } from "lucide-react";
import { useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { SectionHeader } from "../shared/ui/SectionHeader";
import { PageHeader } from "../shared/ui/PageHeader";
import { SmartPlaylistCard } from "../shared/ui/SmartPlaylistCard";
import { usePlayerStore } from "../store/playerStore";
import {
  createPlaylist,
  deletePlaylist,
  listFavorites,
  listHistory,
  listLocalTracks,
  listPlaylists,
  type UserPlaylistDto,
} from "../shared/api/client";
import { pluralWithCount } from "../shared/lib/format";
import { deriveRecommendations, deriveSmartPlaylists } from "../shared/lib/recommendations";

export function PlaylistsPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const playTrack = usePlayerStore((state) => state.playTrack);
  const [creating, setCreating] = useState(false);
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");

  const { data: playlists = [] } = useQuery({
    queryKey: ["playlists"],
    queryFn: listPlaylists,
  });

  const { data: favorites = [] } = useQuery({
    queryKey: ["favorites"],
    queryFn: listFavorites,
  });

  const { data: history = [] } = useQuery({
    queryKey: ["history"],
    queryFn: listHistory,
  });

  const { data: localTracks = [] } = useQuery({
    queryKey: ["localTracks"],
    queryFn: listLocalTracks,
  });

  const createMutation = useMutation({
    mutationFn: () => createPlaylist(title.trim(), description.trim()),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["playlists"] });
      setTitle("");
      setDescription("");
      setCreating(false);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deletePlaylist(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["playlists"] }),
  });

  const canCreate = title.trim().length > 0 && !createMutation.isPending;
  // Обе подборки считаются перебором истории (до 300 записей) и избранного,
  // поэтому пересчитываем их только при смене данных, а не на каждый рендер
  // (страница перерисовывается на ввод в форме создания плейлиста).
  const smartPlaylists = useMemo(
    () => deriveSmartPlaylists(favorites, history),
    [favorites, history],
  );
  const recommended = useMemo(
    () => deriveRecommendations(favorites, history, localTracks),
    [favorites, history, localTracks],
  );

  return (
    <div className="flex flex-col gap-8 py-2">
      <PageHeader
        title="Плейлисты"
        icon={Music2}
        actions={
          <button
            onClick={() => setCreating((v) => !v)}
            className="flex items-center gap-2 rounded-xl bg-[#a855f7] px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-[#9333ea]"
          >
            <Plus size={16} />
            Создать плейлист
          </button>
        }
      />

      {/* Форма создания */}
      {creating && (
        <div className="flex flex-col gap-3 rounded-2xl border border-white/8 bg-white/5 p-4">
          <div className="flex items-center justify-between">
            <h2 className="font-semibold text-white">Новый плейлист</h2>
            <button
              onClick={() => setCreating(false)}
              className="text-slate-400 transition-colors hover:text-white"
              aria-label="Закрыть"
            >
              <X size={18} />
            </button>
          </div>
          <input
            autoFocus
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && canCreate && createMutation.mutate()}
            placeholder="Название"
            className="rounded-xl border border-white/8 bg-white/5 px-4 py-2.5 text-white placeholder:text-slate-500 focus:border-[#a855f7] focus:outline-none"
          />
          <input
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && canCreate && createMutation.mutate()}
            placeholder="Описание (необязательно)"
            className="rounded-xl border border-white/8 bg-white/5 px-4 py-2.5 text-white placeholder:text-slate-500 focus:border-[#a855f7] focus:outline-none"
          />
          <button
            onClick={() => createMutation.mutate()}
            disabled={!canCreate}
            className="w-fit rounded-xl bg-[#a855f7] px-5 py-2 text-sm font-medium text-white transition-colors hover:bg-[#9333ea] disabled:opacity-50"
          >
            {createMutation.isPending ? "Создание…" : "Создать"}
          </button>
        </div>
      )}

      {/* Умные плейлисты */}
      <section>
        <SectionHeader title="Умные плейлисты" />
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 xl:grid-cols-4">
          {smartPlaylists.map((playlist) => (
            <SmartPlaylistCard
              key={playlist.id}
              playlist={playlist}
              onOpen={() => navigate(`/smart/${playlist.id}`)}
              onPlay={() => playlist.tracks[0] && playTrack(playlist.tracks[0], playlist.tracks)}
            />
          ))}
        </div>
      </section>

      {/* Мои плейлисты */}
      <section>
        <SectionHeader title="Мои плейлисты" onAction={undefined} />
        {playlists.length === 0 ? (
          <div className="grid place-items-center rounded-2xl border border-white/8 bg-white/5 py-16 text-center text-slate-400">
            <div>
              <Music2 size={40} className="mx-auto mb-3 opacity-40" />
              <p>У вас пока нет плейлистов. Создайте первый.</p>
            </div>
          </div>
        ) : (
          <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 xl:grid-cols-4">
            {playlists.map((p) => (
              <UserPlaylistCard
                key={p.id}
                playlist={p}
                onOpen={() => navigate(`/playlists/${p.id}`)}
                onDelete={() => deleteMutation.mutate(p.id)}
              />
            ))}
          </div>
        )}
      </section>

      {/* Рекомендованные подборки */}
      <section>
        <SectionHeader title="Рекомендованные подборки" />
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
          {recommended.map((playlist) => (
            <SmartPlaylistCard
              key={playlist.id}
              playlist={playlist}
              onOpen={() => navigate(`/smart/${playlist.id}`)}
              onPlay={() => playlist.tracks[0] && playTrack(playlist.tracks[0], playlist.tracks)}
            />
          ))}
        </div>
      </section>
    </div>
  );
}

/**
 * Коллаж-обложка плейлиста из обложек первых треков. 4+ обложки — сетка 2×2,
 * 1–3 — одна картинка, пусто — градиентная заглушка с иконкой.
 */
function PlaylistCollage({ covers }: { covers: string[] }) {
  const valid = covers.filter(Boolean);
  if (valid.length === 0) {
    return <Music2 size={48} className="text-white/80" />;
  }
  if (valid.length < 4) {
    return (
      <img
        src={valid[0]}
        alt=""
        loading="lazy"
        className="absolute inset-0 h-full w-full object-cover"
      />
    );
  }
  return (
    <div className="absolute inset-0 grid grid-cols-2 grid-rows-2">
      {valid.slice(0, 4).map((url, i) => (
        <img key={i} src={url} alt="" loading="lazy" className="h-full w-full object-cover" />
      ))}
    </div>
  );
}

function UserPlaylistCard({
  playlist,
  onOpen,
  onDelete,
}: {
  playlist: UserPlaylistDto;
  onOpen: () => void;
  onDelete: () => void;
}) {
  // Удаление плейлиста необратимо, а иконка корзины появляется по наведению
  // прямо над обложкой — одного случайного клика было достаточно, чтобы
  // потерять плейлист без всякого предупреждения. Поэтому подтверждение в два
  // шага: первый клик показывает вопрос, второй удаляет.
  const [confirming, setConfirming] = useState(false);

  return (
    <div className="group relative flex flex-col text-left">
      <button
        onClick={onOpen}
        className="neon-frame relative flex aspect-square w-full items-center justify-center overflow-hidden rounded-2xl"
        style={{ background: "linear-gradient(135deg, #a855f7, #ec4899)" }}
      >
        <PlaylistCollage covers={playlist.coverUrls} />
      </button>
      {confirming ? (
        <div className="absolute inset-x-2 top-2 flex flex-col gap-2 rounded-xl bg-black/80 p-3 text-center backdrop-blur">
          <p className="text-xs text-slate-200">Удалить плейлист «{playlist.title}»?</p>
          <div className="flex justify-center gap-2">
            <button
              onClick={() => {
                setConfirming(false);
                onDelete();
              }}
              className="rounded-lg bg-rose-500 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-rose-600"
            >
              Удалить
            </button>
            <button
              onClick={() => setConfirming(false)}
              className="rounded-lg bg-white/10 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-white/20"
            >
              Отмена
            </button>
          </div>
        </div>
      ) : (
        <button
          onClick={(e) => {
            e.stopPropagation();
            setConfirming(true);
          }}
          className="absolute right-2 top-2 grid h-8 w-8 place-items-center rounded-full bg-black/50 text-white opacity-0 transition-opacity hover:bg-black/70 group-hover:opacity-100"
          aria-label="Удалить плейлист"
        >
          <Trash2 size={15} />
        </button>
      )}
      <strong className="mt-3 truncate text-[15px] font-semibold text-white">{playlist.title}</strong>
      {playlist.description && (
        <p className="truncate text-sm text-slate-400">{playlist.description}</p>
      )}
      <span className="mt-0.5 truncate text-xs text-slate-500">
        {pluralWithCount(playlist.trackCount, "трек", "трека", "треков")}
      </span>
    </div>
  );
}
