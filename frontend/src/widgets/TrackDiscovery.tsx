import { djRadio, resetDJSession } from "../shared/api/intelligence";
import { useRecoFeedback } from "../shared/lib/useRecoFeedback";
import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Play, X } from "lucide-react";
import { useUiStore } from "../store/uiStore";
import { usePlayerStore } from "../store/playerStore";
import { listRelatedTracks } from "../shared/api/client";
import { searchV1 } from "../shared/api/v1";
import { TrackRow } from "../shared/ui/TrackRow";
import { ServiceIcon } from "../shared/ui/ServiceIcon";
import { SOURCES } from "../shared/sources";
import type { Track, SourceId } from "../shared/types";

export function TrackDiscovery() {
  const feedback = useRecoFeedback();
  const radio = useUiStore((s) => s.radioPreviewTrack);
  const sourceTrack = useUiStore((s) => s.sourcePickerTrack);
  const track = sourceTrack || radio;
  const [selectedSource, setSelectedSource] = useState<SourceId>("soundcloud");
  const close = () =>
    sourceTrack
      ? useUiStore.getState().openSourcePicker(null)
      : useUiStore.getState().openRadioPreview(null);
  const closeRef = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (!track) return;
    const previous = document.activeElement as HTMLElement | null;
    closeRef.current?.focus();
    const key = (e: KeyboardEvent) => {
      if (e.key === "Escape") close();
    };
    document.addEventListener("keydown", key);
    return () => {
      document.removeEventListener("keydown", key);
      previous?.focus();
    };
  }, [track, sourceTrack]);
  const {
    data: tracks = [],
    isFetching,
    error,
  } = useQuery({
    queryKey: [
      sourceTrack ? "trackSources" : "radioPreview",
      track?.id,
      sourceTrack ? selectedSource : "",
    ],
    enabled: Boolean(track),
    queryFn: async () =>
      sourceTrack
        ? searchV1(`${track!.primaryArtist || track!.artist} ${track!.title}`, selectedSource).then(
            (found) => found.filter((t) => t.playableKind !== "external_only"),
          )
        : (async () => {
            resetDJSession();
            return (
              (await djRadio(track!, track!, [track!], 24).catch(() => null)) ??
              listRelatedTracks(track!, 24, [track!])
            );
          })(),
    staleTime: 10 * 60 * 1000,
    retry: false,
  });
  if (!track) return null;
  const playableTracks = tracks.filter((t) => feedback.scoreFor(t) >= 0);
  const play = (candidate: Track) => {
    if (feedback.scoreFor(candidate) < 0) return;
    const player = usePlayerStore.getState();
    if (sourceTrack) {
      candidate = { ...candidate, forcedSource: true };
      const current = player.currentTrack();
      if (current?.id === track.id) {
        const nextQueue = [...player.queue];
        nextQueue[player.currentIndex] = candidate;
        player.playTrack(candidate, nextQueue);
      } else player.playTrack(candidate, [candidate]);
    } else
      player.startTrackRadio(
        track,
        tracks.filter((t) => feedback.scoreFor(t) >= 0),
        candidate,
      );
    close();
  };
  return (
    <div
      className="fixed inset-0 z-[130] flex items-center justify-center bg-black/70 p-3 backdrop-blur-md"
      onClick={close}
    >
      <section
        role="dialog"
        aria-modal="true"
        aria-label={sourceTrack ? "Источник трека" : "Радио по треку"}
        className="flex max-h-[92dvh] w-full max-w-3xl flex-col rounded-2xl border border-white/10 bg-[var(--bg-0)] p-4 sm:p-6"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="mb-4 flex items-start justify-between gap-4">
          <div>
            <h2 className="text-xl font-bold text-white">
              {sourceTrack ? "Сменить источник" : "Радио по треку"}
            </h2>
            <p className="mt-1 text-sm text-slate-400">
              {track.artist} — {track.title}
            </p>
            <p className="mt-2 text-xs text-slate-500">
              {sourceTrack
                ? "Выберите версию и источник. Название, длительность и наличие цензуры могут отличаться."
                : "Похожие песни других исполнителей. Музыка начнёт играть после вашего выбора."}
            </p>
          </div>
          <button
            ref={closeRef}
            onClick={close}
            aria-label="Закрыть подборку"
            className="p-2 text-slate-300"
          >
            <X />
          </button>
        </div>
        {sourceTrack ? (
          <div className="mb-4 flex flex-wrap gap-2">
            {Object.values(SOURCES)
              .filter((s) => s.id !== "spotify" && s.id !== "local")
              .map((s) => (
                <button
                  key={s.id}
                  aria-pressed={selectedSource === s.id}
                  onClick={() => setSelectedSource(s.id)}
                  className={`flex items-center gap-2 rounded-xl px-3 py-2 text-sm ${selectedSource === s.id ? "bg-[var(--app-accent)] text-white" : "bg-white/5 text-slate-300"}`}
                >
                  <ServiceIcon id={s.id} size={20} />
                  {s.name}
                </button>
              ))}
          </div>
        ) : (
          <button
            disabled={!playableTracks.length}
            onClick={() => play(playableTracks[0])}
            className="mb-4 flex w-fit items-center gap-2 rounded-full bg-[var(--app-accent)] px-5 py-2 text-white disabled:opacity-40"
          >
            <Play size={18} />
            Слушать радио
          </button>
        )}
        <div className="min-h-0 overflow-y-auto">
          {isFetching && (
            <p role="status" className="py-8 text-center text-slate-400">
              Подбираем треки…
            </p>
          )}
          {!isFetching && !tracks.length && (
            <p className="py-8 text-center text-slate-400">
              {error ? error.message : "Треки не найдены. Попробуйте другой источник или трек."}
            </p>
          )}
          {tracks.map((t, i) => (
            <TrackRow key={t.id} track={t} index={i} onPlay={play} />
          ))}
        </div>
      </section>
    </div>
  );
}
