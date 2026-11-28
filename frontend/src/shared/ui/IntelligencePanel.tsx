import { pluralWithCount } from "../lib/format";
import { useState, useEffect } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import {
  getDJStatus,
  getDJCollectionView,
  refreshDJCollection,
  djPollingOptions,
  djSourceNames,
  djSourceStates,
  djSourceReasons,
} from "../api/intelligence";
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
  const { data: view, error: viewError } = useQuery({
    ...djPollingOptions,
    queryKey: ["djCollection"],
    queryFn: ({ signal }) => getDJCollectionView(signal),
    enabled: hasApiV1Runtime(),
    refetchInterval: 2000,
  });
  const job = view?.job;
  const preparingStages = ["planning", "searching", "ranking", "resolving", "saving"];
  const active = status?.enabled;
  const preparing = preparingStages.includes(job?.stage ?? "");
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    if (!preparing) return;
    setNow(Date.now());
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [preparing]);
  const remaining = job?.requestDeadlineAt
    ? Math.max(0, Math.ceil((Date.parse(job.requestDeadlineAt) - now) / 1000))
    : undefined;
  const diagnostic = job;
  const stages: Record<string, string> = {
    planning: active ? "DJ определяет стратегию поиска…" : "Строим персональный план…",
    searching: "Инструменты DJ собирают кандидатов…",
    ranking: "Cryon оценивает кандидатов локально…",
    resolving: "Проверяем версии и потоки в музыкальных источниках…",
    saving: "Сохраняем подборку…",
  };
  const label = !status
    ? "Проверяем подключение AI DJ…"
    : !active
      ? "Персональные рекомендации Cryon · без AI API"
      : viewError
        ? "Не удалось обновить подборку. Проверяем соединение…"
        : preparing
          ? stages[diagnostic?.stage ?? ""] || "AI подготавливает музыку…"
          : ["failed", "cancelled"].includes(job?.stage ?? "")
            ? diagnostic?.error ||
              "Предыдущая попытка не завершилась. Запустите поиск для подробной диагностики"
            : view?.updatedAt
              ? job?.engine === "local_ai_strategy"
                ? "Подборка готова · стратегия AI, оценка Cryon"
                : "Персональная подборка готова"
              : "AI подключён. Запустите подборку или радио";
  const start = async () => {
    setStarting(true);
    setMessage("");
    try {
      await refreshDJCollection();
      setMessage("Задача запущена. Здесь появится результат или причина ошибки.");
      await Promise.all([
        client.invalidateQueries({ queryKey: ["intelligenceStatus"] }),
        client.invalidateQueries({ queryKey: ["djCollection"] }),
      ]);
    } catch (e) {
      setMessage((e as Error).message);
    } finally {
      setStarting(false);
    }
  };
  return (
    <section className="rounded-2xl border border-purple-400/20 bg-purple-500/5 p-4 text-sm">
      <div className="font-semibold text-white">Cryon DJ · {label}</div>
      {preparing && remaining !== undefined && Number.isFinite(remaining) && (
        <p role="status" className="mt-1 text-slate-400">
          {remaining > 0
            ? `Ожидаем ответ модели · осталось до ${remaining} с`
            : "Время ожидания модели истекло. Обновляем статус задачи…"}
          {job?.stage === "planning" && remaining > 0
            ? ". Поиск по музыкальным источникам ещё не начат."
            : ""}
        </p>
      )}
      {job?.stage && (
        <p className="mt-1 text-slate-400">
          Стратегия:{" "}
          {job.strategySource === "model"
            ? "AI"
            : job.strategySource === "cache"
              ? "сохранённый план AI"
              : "Cryon"}{" "}
          · запросов к модели в задаче: {job.llmCalls ?? 0} · ранжирование в Go
        </p>
      )}
      {job?.warning && <p className="mt-2 text-slate-400">{job.warning}</p>}
      {view?.updatedAt && (
        <p className="mt-1 text-slate-400">
          {job?.jobId && job.jobId !== view.jobId ? "Предыдущая подборка" : "Cryon DJ"} ·{" "}
          {pluralWithCount(view.tracks.length, "трек", "трека", "треков")} ·{" "}
          {new Date(view.updatedAt).toLocaleString()}
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
          Итерация: {diagnostic.iteration ?? 1} · найдено: {diagnostic.found} · допущено к оценке:{" "}
          {diagnostic.candidates} · отбор Cryon: {diagnostic.localAdmitted ?? "—"} · отбор AI: не
          используется · в результате: {diagnostic.selected}
          {!!diagnostic.sources?.length && (
            <details className="mt-2">
              <summary className="cursor-pointer">Источники воспроизведения</summary>
              <p className="mt-1 text-xs">
                Проверяем поток выбранной версии; остальные совпадения сохраняем для переключения
                при ошибке. Время — сумма операций источника.
              </p>
              <div className="mt-2 grid gap-2 sm:grid-cols-2">
                {diagnostic.sources.map((source) => (
                  <div
                    key={source.source}
                    className="min-w-0 rounded-lg border border-white/10 p-2 text-xs"
                  >
                    <p className="font-medium text-slate-200">
                      {djSourceNames[source.source] ?? source.source} ·{" "}
                      {djSourceStates[source.state] ?? source.state}
                    </p>
                    <p>
                      Включён: {source.enabled ? "да" : "нет"} · вход:{" "}
                      {source.auth === "missing" || source.auth === "expired"
                        ? "нужен"
                        : source.auth === "session_present"
                          ? "сессия сохранена"
                          : source.auth === "verified"
                            ? "проверен"
                            : "не требуется"}
                    </p>
                    <p>
                      Поиск: {source.searchAttempted ? "запущен" : "не запускался"} · запросов:{" "}
                      {source.queries} · из кэша: {source.cached ?? 0} · найдено: {source.found} ·
                      совпало: {source.matched}
                    </p>
                    <p>
                      Поток проверен: {source.playable} · без проверки: {source.unchecked ?? 0} · в
                      очереди: {source.chosen} · отклонено: {source.rejected} · {source.elapsedMs}{" "}
                      мс
                    </p>
                    {Object.entries(source.reasons ?? {}).map(([reason, count]) => (
                      <p key={reason}>
                        {djSourceReasons[reason] ?? reason}: {count}
                      </p>
                    ))}
                  </div>
                ))}
              </div>
            </details>
          )}
          {(diagnostic.searches ?? []).length > 0 && (
            <details className="mt-2">
              <summary className="cursor-pointer">Как DJ искал музыку</summary>
              {diagnostic.searches.map((search, index) => (
                <p key={index} className="mt-1 break-words">
                  {search.source.split("/")[0]} · «{search.query}» · {search.found} треков ·
                  уникальных: {search.unique ?? search.found} · прошли отбор: {search.accepted ?? 0}{" "}
                  · {search.elapsedMs ?? 0} мс
                  {search.state === "timeout"
                    ? " · источник не уложился во время"
                    : search.state === "cancelled"
                      ? " · ожидание остановлено: кандидатов достаточно"
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
        {status ? (
          <button
            disabled={starting}
            onClick={() => void start()}
            className="rounded-lg bg-purple-500/20 px-3 py-2 text-purple-200 disabled:opacity-40"
          >
            {preparing ? "Перезапустить поиск" : "Подобрать музыку"}
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
          message.startsWith("Задача запущена") &&
          ["ready", "failed", "cancelled"].includes(diagnostic?.stage ?? "")
        ) && (
          <p role="status" className="mt-2 text-slate-300">
            {message}
          </p>
        )}
    </section>
  );
}
