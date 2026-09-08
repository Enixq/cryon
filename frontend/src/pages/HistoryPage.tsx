import { Clock3, Trash2 } from "lucide-react";
import { useQuery, useQueryClient, useMutation } from "@tanstack/react-query";
import { useEffect } from "react";
import { useTrackListPlayer } from "../store/playerStore";
import { clearHistory, listHistory, onHistoryChanged, type HistoryEntryDto } from "../shared/api/client";
import { TrackRow } from "../shared/ui/TrackRow";
import { PageHeader } from "../shared/ui/PageHeader";

/** Относительное время: "1 час назад", "вчера" и т.п. */
function relativeTime(iso: string): string {
  const diffMs = Date.now() - new Date(iso).getTime();
  const hours = Math.floor(diffMs / 3600_000);
  if (hours < 1) return "Только что";
  if (hours < 24) return `${hours} ч назад`;
  const days = Math.floor(hours / 24);
  if (days === 1) return "Вчера";
  return `${days} дн назад`;
}

/** Группировка истории по секциям: Сегодня / Вчера / Ранее. */
function groupHistory(entries: HistoryEntryDto[]) {
  const groups: Record<string, HistoryEntryDto[]> = { Сегодня: [], Вчера: [], Ранее: [] };
  for (const entry of entries) {
    const hours = (Date.now() - new Date(entry.playedAt).getTime()) / 3600_000;
    if (hours < 24) groups["Сегодня"].push(entry);
    else if (hours < 48) groups["Вчера"].push(entry);
    else groups["Ранее"].push(entry);
  }
  return groups;
}

export function HistoryPage() {
  const { playTrack, isPlaying, activeTrackId } = useTrackListPlayer();
  const queryClient = useQueryClient();

  const { data: entries = [] } = useQuery({
    queryKey: ["history"],
    queryFn: listHistory,
    // История может измениться, пока экран не смонтирован (например, после
    // запуска трека в поиске). Не показываем минутный свежий, но устаревший
    // кэш глобального QueryClient при первом переходе на этот маршрут.
    refetchOnMount: "always",
  });

  useEffect(() => onHistoryChanged(() => {
    queryClient.invalidateQueries({ queryKey: ["history"] });
  }), [queryClient]);

  const clearMutation = useMutation({
    mutationFn: clearHistory,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["history"] }),
  });

  const groups = groupHistory(entries);
  const allTracks = entries.map((e) => e.track);

  return (
    <div className="flex flex-col gap-6 py-2">
      <PageHeader
        title="История прослушивания"
        icon={Clock3}
        actions={
          entries.length > 0 && (
            <button
              onClick={() => clearMutation.mutate()}
              disabled={clearMutation.isPending}
              aria-label="Очистить историю"
              className="flex items-center gap-2 rounded-xl border border-white/8 bg-white/5 px-4 py-2 text-sm text-slate-400 transition-colors hover:text-white disabled:opacity-50"
            >
              <Trash2 size={16} />
              Очистить историю
            </button>
          )
        }
      />

      {entries.length === 0 ? (
        <div className="grid place-items-center rounded-2xl border border-white/8 bg-white/5 py-20 text-center text-slate-400">
          <div>
            <Clock3 size={40} className="mx-auto mb-3 opacity-40" />
            <p>Здесь появятся треки, которые вы прослушаете.</p>
          </div>
        </div>
      ) : (
        Object.entries(groups).map(([label, groupEntries]) =>
          groupEntries.length === 0 ? null : (
            <section key={label}>
              <h2 className="mb-3 text-lg font-semibold text-slate-300">{label}</h2>
              <div className="flex flex-col">
                {groupEntries.map((entry, i) => (
                  <div key={`${entry.track.id}-${i}`} className="flex items-center gap-3">
                    <div className="min-w-0 flex-1">
                      <TrackRow
                        track={entry.track}
                        active={activeTrackId === entry.track.id}
                        playing={isPlaying}
                        onPlay={(t) => playTrack(t, allTracks)}
                      />
                    </div>
                    <span className="shrink-0 text-xs text-slate-500">
                      {relativeTime(entry.playedAt)}
                    </span>
                  </div>
                ))}
              </div>
            </section>
          ),
        )
      )}
    </div>
  );
}
