import { IntelligencePanel } from "../shared/ui/IntelligencePanel";
import { useMemo } from "react";
import { useNavigate } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { listFavorites, listHistory, listPlaylists } from "../shared/api/client";
import { CollectionCard } from "../shared/ui/CollectionCard";
import { PageHeader } from "../shared/ui/PageHeader";
import { genres } from "../shared/data/genres";
import type { AccentColor } from "../shared/types";

export function HomePage() {
  const navigate = useNavigate();
  const { data: favorites = [] } = useQuery({ queryKey: ["favorites"], queryFn: listFavorites });
  const { data: history = [] } = useQuery({ queryKey: ["history"], queryFn: listHistory });
  const { data: playlists = [] } = useQuery({ queryKey: ["playlists"], queryFn: listPlaylists });
  const artists = useMemo(() => {
    const unique = new Map<string, { name: string; cover?: string }>();
    for (const track of [...favorites, ...history.map((row) => row.track)]) {
      const name = track.primaryArtist || track.artists?.[0] || track.artist;
      if (name && !unique.has(name.toLocaleLowerCase()))
        unique.set(name.toLocaleLowerCase(), { name, cover: track.coverUrl });
    }
    return [...unique.values()].slice(0, 8);
  }, [favorites, history]);
  const mixes: [string, string, string, AccentColor][] = [
    ["cryon-ai", "Подобрал AI Cryon", "Поиск новых направлений и выбор вашего AI DJ", "violet"],
    ["m1", "Мой микс", "Любимое и близкое к вашему вкусу", "violet"],
    ["m2", "Радар новинок", "Релизы ваших и похожих артистов за 7 дней", "emerald"],
    ["for-you", "Специально для вас", "Новые песни по вашим предпочтениям", "cyan"],
    ["m4", "Открытия недели", "Знакомство с новыми исполнителями", "pink"],
    ["recent", "Недавно прослушано", "Вернуться к музыке из истории", "amber"],
    ["m3", "Любимые треки", "Всё, что вы отметили сердцем", "rose"],
  ];
  return (
    <div className="flex min-w-0 flex-col gap-7 py-2 sm:gap-9">
      <PageHeader title="Ваша музыка" subtitle="Подборки, открытия и любимые исполнители" />
      <IntelligencePanel compact />
      <section>
        <h2 className="mb-3 text-lg font-semibold text-white">Для вас</h2>
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-[repeat(auto-fill,minmax(145px,1fr))]">
          {mixes.map(([id, title, subtitle, accent]) => (
            <CollectionCard
              key={id}
              title={title}
              subtitle={subtitle}
              accent={accent}
              catalogKind={`personal-${id}`}
              onOpen={() => navigate(`/collection/${id}`)}
            />
          ))}
        </div>
      </section>
      <section>
        <h2 className="mb-3 text-lg font-semibold text-white">Чарты</h2>
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <CollectionCard
            title="Чарт России"
            subtitle="Популярное сейчас · доступные источники"
            accent="amber"
            catalogKind="russia-chart"
            onOpen={() => navigate("/collection/russia-chart")}
          />
          <CollectionCard
            title="Мировой чарт"
            subtitle="Популярные треки · Deezer"
            accent="cyan"
            catalogKind="world-chart"
            onOpen={() => navigate("/collection/world-chart")}
          />
        </div>
      </section>
      <section>
        <h2 className="mb-3 text-lg font-semibold text-white">По жанрам</h2>
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-[repeat(auto-fill,minmax(145px,1fr))]">
          {genres.map((genre) => (
            <CollectionCard
              key={genre.slug}
              title={genre.title}
              subtitle="Жанровая подборка"
              accent={genre.accent}
              catalogKind={genre.slug}
              onOpen={() => navigate(`/genre/${genre.slug}`)}
            />
          ))}
        </div>
      </section>
      {artists.length > 0 && (
        <section>
          <h2 className="mb-3 text-lg font-semibold text-white">Ваши исполнители</h2>
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-[repeat(auto-fill,minmax(145px,1fr))]">
            {artists.map((artist) => (
              <CollectionCard
                key={artist.name}
                title={artist.name}
                subtitle="Популярное и новые релизы"
                cover={artist.cover}
                onOpen={() => navigate(`/artist/${encodeURIComponent(artist.name)}`)}
              />
            ))}
          </div>
        </section>
      )}
      {playlists.length > 0 && (
        <section>
          <h2 className="mb-3 text-lg font-semibold text-white">Ваши плейлисты</h2>
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-[repeat(auto-fill,minmax(145px,1fr))]">
            {playlists.slice(0, 6).map((playlist) => (
              <CollectionCard
                key={playlist.id}
                title={playlist.title}
                subtitle={`${playlist.trackCount} треков`}
                cover={playlist.coverUrls?.[0]}
                covers={playlist.coverUrls}
                onOpen={() => navigate(`/playlists/${playlist.id}`)}
              />
            ))}
          </div>
        </section>
      )}
    </div>
  );
}
