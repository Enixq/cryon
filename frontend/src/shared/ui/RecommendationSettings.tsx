import { useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { getSetting, setSetting } from "../api/client";
import { request } from "../api/v1";
import { mapTracks } from "../api/mappers";
import { usePlayerStore } from "../../store/playerStore";
import { TrackRow } from "./TrackRow";
import type { domain } from "../../../wailsjs/go/models";

type Metrics = { count: number; artistDiversity: number; artistNovelty: number };
type Comparison = {
  familiar: Metrics;
  adventurous: Metrics;
  familiarTracks: domain.Track[];
  adventurousTracks: domain.Track[];
};
export function RecommendationSettings() {
  const [value, setValue] = useState(50);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [comparison, setComparison] = useState<Comparison>();
  const [variant, setVariant] = useState<"familiar" | "adventurous">("familiar");
  const [rating, setRating] = useState("");
  const queryClient = useQueryClient();
  useEffect(() => {
    void getSetting("recommendations.adventure")
      .then((raw) => {
        if (raw) setValue(Math.max(0, Math.min(100, Number(raw) || 0)));
      })
      .catch(() => {});
  }, []);
  const save = async () => {
    setBusy(true);
    setError("");
    try {
      await setSetting("recommendations.adventure", String(value));
      await queryClient.invalidateQueries({
        predicate: (q) =>
          /recommendations|dailyMix|weekly|radioPreview|collection|smartPlaylist/i.test(
            String(q.queryKey),
          ),
      });
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  const compare = async () => {
    setBusy(true);
    setError("");
    try {
      setComparison(await request<Comparison>("/v1/recommendations/compare"));
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  const tracks = mapTracks(
    comparison?.[variant === "familiar" ? "familiarTracks" : "adventurousTracks"],
  );
  return (
    <section className="mb-5 break-inside-avoid rounded-2xl border border-white/10 bg-white/[.025] p-5">
      <h2 className="text-lg font-semibold text-white">Рекомендации</h2>
      <p className="mt-2 text-sm text-slate-400">
        Авантюрность повышает вес незнакомых исполнителей среди подходящих кандидатов. Жанровые
        ограничения и дизлайки сохраняются.
      </p>
      <label className="mt-4 block text-sm text-slate-200" htmlFor="adventure">
        Авантюрность · {value}%
      </label>
      <input
        id="adventure"
        type="range"
        min="0"
        max="100"
        value={value}
        onChange={(e) => setValue(Number(e.target.value))}
        className="my-3 w-full accent-[var(--app-accent)]"
      />
      <div className="flex justify-between text-xs text-slate-500">
        <span>Знакомое</span>
        <span>Новые исполнители</span>
      </div>
      <div className="mt-4 flex flex-wrap gap-2">
        <button
          disabled={busy}
          onClick={() => void save()}
          className="rounded-lg bg-[var(--app-accent)] px-3 py-2 text-sm text-white disabled:opacity-40"
        >
          Применить
        </button>
        <button
          disabled={busy}
          onClick={() => void compare()}
          className="rounded-lg bg-white/5 px-3 py-2 text-sm text-slate-200 disabled:opacity-40"
        >
          Сравнить подборки A/B
        </button>
      </div>
      {busy && (
        <p role="status" className="mt-2 text-sm text-slate-400">
          Подготавливаем…
        </p>
      )}
      {error && (
        <p role="alert" className="mt-2 text-sm text-rose-300">
          {error}
        </p>
      )}
      {comparison && (
        <div className="mt-5">
          <table className="w-full text-left text-xs text-slate-300">
            <thead>
              <tr>
                <th>Вариант</th>
                <th>Артисты</th>
                <th>Новые артисты</th>
              </tr>
            </thead>
            <tbody>
              {(["familiar", "adventurous"] as const).map((v) => (
                <tr key={v}>
                  <td className="py-2">
                    {v === "familiar" ? "A · знакомое" : "B · открытия"} ({comparison[v].count})
                  </td>
                  <td>{Math.round(comparison[v].artistDiversity * 100)}%</td>
                  <td>{Math.round(comparison[v].artistNovelty * 100)}%</td>
                </tr>
              ))}
            </tbody>
          </table>
          <p className="mt-2 text-xs text-slate-500">
            Доля разных артистов и треков незнакомых артистов. Оба варианта используют общий набор
            кандидатов. Это сравнение, а не доказательство качества; неожиданность оценивается при
            прослушивании.
          </p>
          <div className="my-3 flex gap-2">
            {(["familiar", "adventurous"] as const).map((v) => (
              <button
                key={v}
                aria-pressed={variant === v}
                onClick={() => setVariant(v)}
                className="rounded-lg bg-white/5 px-3 py-2 text-sm text-slate-200"
              >
                {v === "familiar" ? "Подборка A" : "Подборка B"}
              </button>
            ))}
          </div>
          <div className="max-h-72 overflow-y-auto">
            {tracks.map((t, i) => (
              <TrackRow
                key={t.id}
                track={t}
                index={i}
                onPlay={(t) => usePlayerStore.getState().playTrack(t, tracks)}
              />
            ))}
          </div>
          <p className="mt-3 text-xs text-slate-400">Какая подборка понравилась?</p>
          <div className="mt-2 flex gap-2">
            {["A", "B", "Обе", "Ни одна"].map((answer) => (
              <button
                key={answer}
                aria-pressed={rating === answer}
                onClick={() => {
                  setRating(answer);
                  void setSetting(
                    "recommendations.comparisonRating",
                    JSON.stringify({ answer, at: Date.now(), metrics: comparison }),
                  );
                }}
                className="rounded-lg bg-white/5 px-3 py-2 text-xs text-slate-300"
              >
                {answer}
              </button>
            ))}
          </div>
        </div>
      )}
    </section>
  );
}
