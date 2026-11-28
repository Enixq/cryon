import { useEffect, useState } from "react";
import { getDJStatus, saveDJConfig, rebuildDJProfile, type DJStatus } from "../api/intelligence";
import {
  getDJSources,
  saveDJSources,
  djSourceNames,
  djSourceStates,
  type DJSourcePolicy,
} from "../api/intelligence";

export function IntelligenceSettings() {
  const [status, setStatus] = useState<DJStatus>();
  const [enabled, setEnabled] = useState(false);
  const [provider, setProvider] = useState("openai");
  const [model, setModel] = useState("");
  const [baseUrl, setBaseUrl] = useState("");
  const [key, setKey] = useState("");
  const [budget, setBudget] = useState(100);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [sources, setSources] = useState<DJSourcePolicy>();
  const [sourceBusy, setSourceBusy] = useState(false);
  useEffect(() => {
    void getDJSources()
      .then(setSources)
      .catch(() => {});
  }, []);
  useEffect(() => {
    void getDJStatus()
      .then((s) => {
        setStatus(s);
        setEnabled(s.enabled);
        setProvider(s.provider);
        setModel(s.model);
        setBaseUrl(s.baseUrl);
        setBudget(s.dailySegments);
      })
      .catch(() => {});
  }, []);
  useEffect(() => {
    const timer = window.setInterval(() => {
      void getDJStatus()
        .then(setStatus)
        .catch(() => {});
    }, 4000);
    return () => window.clearInterval(timer);
  }, []);
  const save = async () => {
    setBusy(true);
    setMessage("");
    try {
      setStatus(
        await saveDJConfig({
          enabled,
          provider,
          model,
          baseUrl,
          apiKey: key,
          dailySegments: budget,
        }),
      );
      setKey("");
      setMessage("Настройки сохранены");
    } catch (e) {
      setMessage((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  const field = "mt-1 w-full rounded-lg border border-white/10 bg-black/30 p-2 text-sm text-white";
  const stateLabel: Record<string, string> = {
    disabled: "выключен",
    not_configured: "нужен ключ",
    ready: "готов к работе",
    preparing: "подготавливает музыку",
    fallback: "работает обычное радио",
  };
  return (
    <section className="mb-5 rounded-2xl border border-white/10 bg-white/[.025] p-5">
      <h2 className="text-lg font-semibold text-white">Cryon DJ</h2>
      <p className="mt-2 text-sm text-slate-400">
        Cryon DJ координирует поиск и ранжирует музыку локально. Модель необязательна: она помогает
        выбрать стратегию. Без ключа DJ использует профиль и музыкальные связи. При выключенном DJ
        работает обычное радио.
      </p>
      <label className="mt-4 flex gap-2 text-sm text-white">
        <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />
        Включить Cryon DJ
      </label>
      <div className="mt-4 grid gap-3 text-sm text-slate-300">
        <label>
          Провайдер
          <select
            className={field}
            value={provider}
            onChange={(e) => {
              setProvider(e.target.value);
              setKey("");
            }}
          >
            <option value="openai">OpenAI</option>
            <option value="deepseek">DeepSeek</option>
            <option value="anthropic">Claude / Anthropic</option>
            <option value="compatible">Совместимый API</option>
          </select>
        </label>
        <label>
          Модель
          <input
            className={field}
            value={model}
            onChange={(e) => setModel(e.target.value)}
            placeholder="Идентификатор модели у провайдера"
          />
        </label>
        <label>
          API endpoint
          <input
            className={field}
            value={baseUrl}
            onChange={(e) => {
              setBaseUrl(e.target.value);
              setKey("");
            }}
            placeholder="По умолчанию — адрес провайдера"
          />
        </label>
        <label>
          API-ключ
          <input
            type="password"
            autoComplete="off"
            className={field}
            value={key}
            onChange={(e) => setKey(e.target.value)}
            placeholder={status?.configured ? "Ключ сохранён (скрыт)" : "Введите свой API-ключ"}
          />
        </label>
        <label>
          Лимит запросов стратегии к модели в сутки
          <input
            type="number"
            min={1}
            max={500}
            className={field}
            value={budget}
            onChange={(e) => setBudget(Number(e.target.value))}
          />
        </label>
      </div>
      <p className="mt-2 text-sm text-slate-300" role="status">
        {status?.configured
          ? "Ключ сохранён на устройстве. Его значение скрыто; пустое поле оставляет сохранённый ключ для прежнего провайдера и endpoint."
          : "Ключ не сохранён. Cryon DJ может работать без модели."}
      </p>
      <p className="mt-3 text-xs text-slate-400">
        Ключ хранится на устройстве. Выбранному провайдеру передаются названия музыки, краткий
        профиль и реакции; запросы могут оплачиваться с вашего API-баланса. При смене провайдера или
        endpoint введите ключ заново.
      </p>
      <button
        disabled={busy}
        onClick={() => void save()}
        className="mt-4 rounded-lg bg-[var(--app-accent)] px-4 py-2 text-sm text-white disabled:opacity-40"
      >
        {busy ? "Сохраняем…" : "Сохранить"}
      </button>
      <p role="status" className="mt-2 text-sm text-slate-300">
        {message ||
          (status?.enabled
            ? `Cryon DJ: ${stateLabel[status.state] || status.state} · подготовлено ${status.prepared}`
            : "Cryon DJ выключен")}
      </p>
      {status?.diagnostic?.error && status.state === "fallback" && (
        <p className="mt-2 text-sm text-rose-300">{status.diagnostic.error}</p>
      )}
      {status?.enabled && (
        <p className="mt-2 text-xs text-slate-400">
          Сегодня: {status.usedToday ?? 0} / {status.dailySegments} запросов стратегии ·{" "}
          {status.tokensToday ?? 0} токенов по ответам API. Ранжирование выполняется на устройстве.
        </p>
      )}
      <button
        disabled={busy}
        className="mt-4 text-sm text-slate-300 underline disabled:opacity-40"
        onClick={() => {
          setBusy(true);
          void rebuildDJProfile()
            .then((p) =>
              setMessage(
                `Профиль пересчитан: ${p.preferences.length} исполнителей. Новые подборки учтут изменения.`,
              ),
            )
            .catch((e) => setMessage((e as Error).message))
            .finally(() => setBusy(false));
        }}
      >
        Пересчитать профиль из библиотеки и истории
      </button>
      {sources && (
        <div className="mt-5 border-t border-white/10 pt-4">
          <h3 className="font-medium text-white">Источники Cryon DJ</h3>
          <p className="mt-1 text-xs text-slate-400">
            Ищем версии выбранной музыки во всех включённых источниках. Порядок задаёт предпочтение
            воспроизведения; при ошибке пробуем следующий. Без своего порядка DJ учитывает
            совпадение записи и разнообразие источников.
          </p>
          <div className="mt-3 grid gap-2">
            {[
              ...sources.priority,
              ...sources.sources
                .map((s) => s.source)
                .filter((id) => !sources.priority.includes(id)),
            ].map((id, index, order) => (
              <div key={id} className="flex flex-wrap items-center gap-2 text-sm text-slate-300">
                <label className="flex min-w-0 flex-1 items-center gap-2">
                  <input
                    type="checkbox"
                    checked={sources.enabled[id] ?? true}
                    onChange={(e) =>
                      setSources({
                        ...sources,
                        enabled: { ...sources.enabled, [id]: e.target.checked },
                      })
                    }
                  />
                  {djSourceNames[id] ?? id}
                </label>
                <span className="text-xs text-slate-400">
                  {djSourceStates[sources.sources.find((s) => s.source === id)?.state ?? "ready"]}
                </span>
                <button
                  type="button"
                  aria-label={`Выше: ${djSourceNames[id]}`}
                  disabled={index === 0}
                  className="rounded border border-white/10 px-2 disabled:opacity-30"
                  onClick={() => {
                    const next = [...order];
                    [next[index - 1], next[index]] = [next[index], next[index - 1]];
                    setSources({ ...sources, priority: next });
                  }}
                >
                  ↑
                </button>
                <button
                  type="button"
                  aria-label={`Ниже: ${djSourceNames[id]}`}
                  disabled={index === order.length - 1}
                  className="rounded border border-white/10 px-2 disabled:opacity-30"
                  onClick={() => {
                    const next = [...order];
                    [next[index + 1], next[index]] = [next[index], next[index + 1]];
                    setSources({ ...sources, priority: next });
                  }}
                >
                  ↓
                </button>
              </div>
            ))}
          </div>
          <div className="mt-3 flex flex-wrap gap-3 text-sm">
            <button
              disabled={sourceBusy}
              className="rounded-lg bg-purple-500/20 px-3 py-2 text-purple-200 disabled:opacity-40"
              onClick={() => {
                setSourceBusy(true);
                void saveDJSources(sources)
                  .then((s) => {
                    setSources(s);
                    setMessage("Источники сохранены. Новая подборка учтёт изменения.");
                  })
                  .catch((e) => setMessage((e as Error).message))
                  .finally(() => setSourceBusy(false));
              }}
            >
              Сохранить источники
            </button>
            <button
              className="text-slate-400 underline"
              onClick={() => setSources({ ...sources, priority: [] })}
            >
              Автоматический порядок
            </button>
          </div>
        </div>
      )}
    </section>
  );
}
