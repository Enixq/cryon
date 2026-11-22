import { useEffect, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import type { domain } from "../../../wailsjs/go/models";
import { apiV1BaseUrl, hasApiV1Runtime, request } from "../api/v1";
import { mapTrack } from "../api/mappers";
import { usePlayerStore } from "../../store/playerStore";
import { stopLocalPlayback } from "../lib/localPlayback";
import { TrackRow } from "./TrackRow";
type Download = { key: string; track: domain.Track; name: string; size: number };
export function SavedDownloads() {
  const client = useQueryClient();
  const [deleting, setDeleting] = useState("");
  const [error, setError] = useState("");
  const { data: items = [] } = useQuery({
    queryKey: ["downloads"],
    queryFn: () => request<Download[]>("/v1/library/downloads"),
    enabled: hasApiV1Runtime(),
  });
  useEffect(() => {
    const refresh = () => {
      void client.invalidateQueries({ queryKey: ["downloads"] });
    };
    window.addEventListener("cryon:downloads-changed", refresh);
    return () => window.removeEventListener("cryon:downloads-changed", refresh);
  }, [client]);
  const tracks = items.map((item) => {
    const t = mapTrack(item.track);
    return {
      ...t,
      coverUrl: t.coverUrl?.startsWith("/") ? apiV1BaseUrl() + t.coverUrl : t.coverUrl,
    };
  });
  const remove = async (item: Download) => {
    setError("");
    try {
      if (
        usePlayerStore.getState().currentTrack()?.id === `${item.track.service}:${item.track.id}`
      ) {
        usePlayerStore.getState().pause();
        await stopLocalPlayback();
      }
      if (
        window.CryonAndroid?.nativeDeleteDownload &&
        !window.CryonAndroid.nativeDeleteDownload(item.key)
      )
        throw new Error("Не удалось удалить копию Music/Cryon");
      await request(`/v1/library/downloads?key=${item.key}`, { method: "DELETE" });
      if (usePlayerStore.getState().currentTrack()?.id === `${item.track.service}:${item.track.id}`)
        usePlayerStore.setState({ isPlaying: false });
      setDeleting("");
      await client.invalidateQueries({ queryKey: ["downloads"] });
    } catch (failure) {
      setError((failure as Error).message);
    }
  };
  return (
    <section>
      <h2 className="mb-2 text-xl font-semibold text-white">Скачано · без сети</h2>
      <p className="mb-3 text-xs text-slate-500">
        Полные файлы в хранилище Cryon. На Android также сохраняются в Music/Cryon. Удаление
        затрагивает только выбранную загрузку Cryon.
      </p>
      {!items.length && (
        <p className="py-4 text-sm text-slate-400">
          Скачивайте песни через меню трека — они появятся здесь.
        </p>
      )}
      {error && (
        <p role="alert" className="text-sm text-rose-300">
          {error}
        </p>
      )}
      {items.map((item, i) => (
        <div key={item.key}>
          <TrackRow
            track={tracks[i]}
            index={i}
            onPlay={(track) => usePlayerStore.getState().playTrack(track, tracks)}
          />
          <div className="mb-2 flex items-center justify-end gap-2 text-xs text-slate-500">
            <span>{(item.size / (1 << 20)).toFixed(1)} МБ</span>
            {deleting === item.key ? (
              <>
                <button
                  onClick={() => void remove(item)}
                  className="rounded bg-rose-500/20 px-3 py-2 text-rose-200"
                >
                  Удалить этот скачанный файл
                </button>
                <button onClick={() => setDeleting("")}>Отмена</button>
              </>
            ) : (
              <button onClick={() => setDeleting(item.key)} className="px-3 py-2 hover:text-white">
                Удалить скачивание
              </button>
            )}
          </div>
        </div>
      ))}
    </section>
  );
}
