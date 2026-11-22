import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { hasApiV1Runtime, request } from "../api/v1";
import { configuredBaseUrl } from "../lib/httpBridge";

type Config = {
  enabled: boolean;
  peerUrl: string;
  paired: boolean;
  lastSync?: string;
  message?: string;
};
const path = "/v1/library/sync/config";
export function LibrarySyncSettings() {
  const [enabled, setEnabled] = useState(false);
  const [url, setUrl] = useState("");
  const [peerKey, setPeerKey] = useState("");
  const [ownKey, setOwnKey] = useState("");
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [hydrated, setHydrated] = useState(false);
  const { data, refetch } = useQuery({
    queryKey: ["library-sync-config"],
    queryFn: () => request<Config>(path),
    enabled: hasApiV1Runtime(),
    refetchInterval: 15000,
    retry: false,
  });
  useEffect(() => {
    if (data && !hydrated) {
      setEnabled(data.enabled);
      setUrl(data.peerUrl || configuredBaseUrl());
      setHydrated(true);
    }
  }, [data, hydrated]);
  const action = async (operation: "save" | "sync" | "pairing-key" | "rotate-key") => {
    setBusy(true);
    setMessage("");
    try {
      const result = await request<{ key?: string }>(path, {
        method: operation === "save" ? "PUT" : "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(
          operation === "save"
            ? { enabled, peerUrl: url.trim(), peerKey: peerKey.trim() }
            : { action: operation },
        ),
      });
      if (result.key) setOwnKey(result.key);
      else
        setMessage(operation === "sync" ? "Библиотеки синхронизированы." : "Настройки сохранены.");
      setPeerKey("");
      await refetch();
    } catch (error) {
      setMessage((error as Error).message);
    } finally {
      setBusy(false);
    }
  };
  return (
    <section className="mb-5 break-inside-avoid rounded-2xl border border-white/5 bg-white/[0.025] p-5">
      <h3 className="mb-2 font-semibold text-white">Библиотека по Wi-Fi</h3>
      <p className="mb-4 text-xs leading-relaxed text-slate-400">
        Избранные треки синхронизируются напрямую между Cryon на ПК и телефоне. Изменения, сделанные
        без сети, передадутся после подключения. Оба приложения должны работать и находиться в одной
        сети. Локальные аудиофайлы остаются на своём устройстве.
      </p>
      <label className="mb-4 flex items-center justify-between gap-3 text-sm text-white">
        Разрешить обмен библиотекой
        <input
          type="checkbox"
          checked={enabled}
          onChange={(event) => setEnabled(event.target.checked)}
        />
      </label>
      <div className="flex flex-col gap-3">
        <input
          aria-label="Адрес устройства для библиотеки"
          value={url}
          onChange={(event) => setUrl(event.target.value)}
          placeholder="На телефоне: http://192.168.1.50:8899"
          className="w-full rounded-xl border border-white/10 bg-black/20 px-3 py-2.5 text-sm text-white"
        />
        <input
          aria-label="Ключ сопряжения библиотеки"
          type="password"
          autoComplete="off"
          value={peerKey}
          onChange={(event) => setPeerKey(event.target.value)}
          placeholder={
            data?.paired
              ? "Ключ сохранён; введите новый для замены"
              : "Вставьте ключ библиотеки с другого устройства"
          }
          className="w-full rounded-xl border border-white/10 bg-black/20 px-3 py-2.5 text-sm text-white"
        />
        <p className="text-xs text-slate-500">
          На ПК включите обмен, оставьте адрес пустым и покажите ключ. На телефоне укажите адрес ПК
          и вставьте этот ключ. Для управления плеером используется отдельная настройка устройства.
        </p>
        <div className="flex flex-wrap gap-2">
          <button
            type="button"
            disabled={busy || !hasApiV1Runtime()}
            onClick={() => void action("save")}
            className="rounded-xl bg-[var(--app-accent)] px-4 py-2 text-sm text-white disabled:opacity-50"
          >
            Сохранить
          </button>
          <button
            type="button"
            disabled={busy || !data?.enabled || !data?.peerUrl}
            onClick={() => void action("sync")}
            className="rounded-xl bg-white/10 px-4 py-2 text-sm text-white disabled:opacity-50"
          >
            Синхронизировать сейчас
          </button>
          <button
            type="button"
            disabled={busy || !hasApiV1Runtime()}
            onClick={() => void action("pairing-key")}
            className="rounded-xl bg-white/10 px-4 py-2 text-sm text-white disabled:opacity-50"
          >
            Показать ключ этого устройства
          </button>
        </div>
        {ownKey && (
          <div className="rounded-xl bg-black/20 p-3">
            <input
              aria-label="Ключ библиотеки этого устройства"
              readOnly
              value={ownKey}
              onFocus={(event) => event.target.select()}
              className="w-full bg-transparent font-mono text-xs text-white"
            />
            <div className="mt-2 flex gap-3">
              <button
                type="button"
                onClick={() => {
                  void navigator.clipboard
                    .writeText(ownKey)
                    .then(() => setMessage("Ключ скопирован."));
                }}
                className="text-xs text-slate-300"
              >
                Копировать
              </button>
              <button
                type="button"
                disabled={busy}
                onClick={() => void action("rotate-key")}
                className="text-xs text-slate-400"
              >
                Сменить ключ
              </button>
              <button
                type="button"
                onClick={() => setOwnKey("")}
                className="text-xs text-slate-400"
              >
                Скрыть
              </button>
            </div>
          </div>
        )}
        {data?.lastSync && (
          <p className="text-xs text-slate-500">
            Последний обмен: {new Date(data.lastSync).toLocaleString("ru")}
          </p>
        )}
        {(message || data?.message) && (
          <p role="status" className="text-sm text-slate-300">
            {message || data?.message}
          </p>
        )}
      </div>
    </section>
  );
}
