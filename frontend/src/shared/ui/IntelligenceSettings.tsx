import { useEffect, useState } from "react";
import { getDJStatus, saveDJConfig, type DJStatus } from "../api/intelligence";

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
    const timer = window.setInterval(() => { void getDJStatus().then(setStatus).catch(() => {}); },4000);
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
      <h2 className="text-lg font-semibold text-white">Cryon Intelligence · AI DJ</h2>
      <p className="mt-2 text-sm text-slate-400">
        AI DJ координирует поиск музыки, открывает направления и оценивает кандидатов. Без
        подключения продолжает работать обычное радио Cryon.
      </p>
      <label className="mt-4 flex gap-2 text-sm text-white">
        <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />
        Включить AI DJ
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
            placeholder={
              status?.configured
                ? "Ключ сохранён; пустое поле сохраняет его для прежнего endpoint"
                : "Введите свой API-ключ"
            }
          />
        </label>
        <label>
          Лимит подготовки сегментов в сутки
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
            ? `AI DJ: ${stateLabel[status.state] || status.state} · подготовлено ${status.prepared}`
            : "AI DJ выключен")}
      </p>
      {status?.enabled && (
        <p className="mt-2 text-xs text-slate-400">
          Сегодня: {status.usedToday ?? 0} / {status.dailySegments} сегментов ·{" "}
          {status.tokensToday ?? 0} токенов по ответам API. Две AI-фазы на сегмент. Стоимость
          зависит от модели.
        </p>
      )}
    </section>
  );
}
