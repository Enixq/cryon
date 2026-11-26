import { pluralWithCount } from "../lib/format";
import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import { getDJStatus, refreshDJCollection } from "../api/intelligence";
import { hasApiV1Runtime } from "../api/v1";

export function IntelligencePanel({ compact = false }: { compact?: boolean }) {
  const navigate = useNavigate();
  const client = useQueryClient();
  const [message, setMessage] = useState("");
  const [starting, setStarting] = useState(false);
  const { data: status } = useQuery({
    queryKey: ["intelligenceStatus"],
    queryFn: getDJStatus,
    enabled: hasApiV1Runtime(),
    refetchInterval: 4000,
  });
  const active = status?.enabled && status.configured;
  const preparing = status?.state === "preparing";
  const diagnostic = status?.diagnostic;
  const stages: Record<string, string> = {
    planning: "AI планирует поиск музыки…",
    searching: "Ищем треки по плану AI…",
    ranking: "AI оценивает найденные треки…",
    saving: "Сохраняем подборку…",
  };
  const label = !status
    ? "Проверяем подключение AI DJ…"
    : !active
      ? "AI DJ выключен — работают обычные рекомендации"
      : preparing
        ? stages[diagnostic?.stage ?? ""] || "AI подготавливает музыку…"
        : status.state === "fallback"
          ? diagnostic?.error ||
            "Предыдущая попытка не завершилась. Запустите поиск для подробной диагностики"
          : status.updatedAt
            ? "AI-подборка готова"
            : "AI подключён. Запустите подборку или радио";
  const start = async () => {
    setStarting(true);
    setMessage("");
    try {
      await refreshDJCollection();
      setMessage("Поиск запущен. Треки появятся автоматически.");
      await client.invalidateQueries({ queryKey: ["intelligenceStatus"] });
    } catch (e) {
      setMessage((e as Error).message);
    } finally {
      setStarting(false);
    }
  };
  return (
    <section className="rounded-2xl border border-purple-400/20 bg-purple-500/5 p-4 text-sm">
      <div className="font-semibold text-white">Cryon DJ · {label}</div>
      {status?.updatedAt && (
        <p className="mt-1 text-slate-400">
          Cryon DJ · {pluralWithCount(status.trackCount, "трек", "трека", "треков")} ·{" "}
          {new Date(status.updatedAt).toLocaleString()}
        </p>
      )}
      {active && (
        <p className="mt-1 text-slate-400">
          {status.model} · сегодня {status.usedToday}/{status.dailySegments} попыток ·{" "}
          {status.tokensToday} токенов
        </p>
      )}
      {diagnostic && diagnostic.stage && (
        <div className="mt-2 text-slate-400">
          Найдено: {diagnostic.found} · допущено к оценке: {diagnostic.candidates} · в результате:{" "}
          {diagnostic.selected}
          {(diagnostic.searches ?? []).length > 0 && (
            <details className="mt-2">
              <summary className="cursor-pointer">Как DJ искал музыку</summary>
              {diagnostic.searches.map((search, index) => (
                <p key={index} className="mt-1 break-words">
                  {search.source.split("/")[0]} · «{search.query}» · {search.found} треков
                  {search.state === "timeout"
                    ? " · источник не уложился во время"
                    : search.state === "error"
                      ? " · источник недоступен"
                      : ""}
                </p>
              ))}
            </details>
          )}
        </div>
      )}
      <div className="mt-3 flex flex-wrap gap-3">
        {active ? (
          <button
            disabled={preparing || starting}
            onClick={() => void start()}
            className="rounded-lg bg-purple-500/20 px-3 py-2 text-purple-200 disabled:opacity-40"
          >
            {preparing ? "Подготавливает…" : "Подобрать музыку"}
          </button>
        ) : (
          <button onClick={() => navigate("/settings")} className="text-purple-300">
            Настроить AI DJ
          </button>
        )}
        {compact && (
          <button onClick={() => navigate("/collection/cryon-ai")} className="text-purple-300">
            Открыть AI-подборку
          </button>
        )}
      </div>
      {message &&
        !(
          message.startsWith("Поиск запущен") &&
          ["ready", "failed"].includes(diagnostic?.stage ?? "")
        ) && (
          <p role="status" className="mt-2 text-slate-300">
            {message}
          </p>
        )}
    </section>
  );
}
