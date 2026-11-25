import { useEffect, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { hasApiV1Runtime, request } from "../api/v1";
import { ServiceIcon } from "./ServiceIcon";
import { openExternal } from "../api/client";

type LoginState = {
  id: string;
  state: "browser" | "checking" | "connected" | "error" | "cancelled";
  error?: string;
};
type Connection = { connected: boolean; browserSupported: boolean; session?: LoginState; retryAfterSeconds?: number; limitMessage?: string };
const path = "/v1/accounts/vk/login";
const button =
  "rounded-xl bg-[var(--app-accent)] px-4 py-2.5 text-sm font-medium text-white disabled:opacity-50";

export function VKWebConnection() {
  const client = useQueryClient();
  const [sessionId, setSessionId] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const { data, refetch } = useQuery({
    queryKey: ["vk-web-login", sessionId],
    queryFn: () => request<Connection>(`${path}?id=${encodeURIComponent(sessionId)}`),
    enabled: hasApiV1Runtime(),
    refetchInterval: (query) =>
      sessionId &&
      (!query.state.data?.session ||
        ["browser", "checking"].includes(query.state.data.session.state))
        ? 1000
        : query.state.data?.retryAfterSeconds ? 10_000 : false,
    retry: false,
  });
  const state = data?.session?.state;
  const waiting = Boolean(sessionId) && (!state || state === "browser" || state === "checking");
  useEffect(() => {
    if (state === "connected") {
      void client.invalidateQueries({ queryKey: ["sourceStatus"] });
      void client.invalidateQueries({ queryKey: ["search"] });
    }
  }, [client, state]);

  const start = async (fresh = false) => {
    setBusy(true);
    setError("");
    try {
      const login = await request<LoginState>(path, { method: "POST" });
      setSessionId(login.id);
      if (fresh && window.CryonAndroid?.openVKFreshLogin) window.CryonAndroid.openVKFreshLogin(login.id);
      else window.CryonAndroid?.openVKLogin?.(login.id);
    } catch (failure) {
      setError((failure as Error).message);
    } finally {
      setBusy(false);
    }
  };
  const openWebsite = async () => {
    if (window.CryonAndroid?.openVKWebsite) window.CryonAndroid.openVKWebsite();
    else await openExternal("https://vk.ru/audio");
  };
  const stop = async (disconnect: boolean) => {
    setBusy(true);
    setError("");
    try {
      await request(
        `${path}?id=${encodeURIComponent(sessionId)}&disconnect=${disconnect ? "1" : "0"}`,
        { method: "DELETE" },
      );
      await refetch();
      void client.invalidateQueries({ queryKey: ["sourceStatus"] });
    } catch (failure) {
      setError((failure as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="mb-5 break-inside-avoid rounded-2xl border border-white/5 bg-white/[0.025] p-5">
      <div className="mb-3 flex items-center gap-3">
        <ServiceIcon id="vk" size={32} badge />
        <h3 className="font-semibold text-white">ВК Музыка</h3>
      </div>
      <p className="mb-4 text-xs leading-relaxed text-slate-400">
        Войдите на сайте ВК в отдельном окне. Можно использовать QR-код или другой способ, который
        предлагает ВК. После входа Cryon проверит доступ к музыке.
      </p>
      {data?.connected && !waiting ? (
        <div className="flex items-center justify-between gap-3">
          <span className="text-sm text-emerald-300">{data.retryAfterSeconds ? "Подключено · запросы приостановлены" : "Подключено"}</span>
          <button className={button} disabled={busy} onClick={() => void stop(true)}>
            Отключить
          </button>
        </div>
      ) : waiting ? (
        <div>
          <p role="status" className="text-sm text-slate-300">
            {state === "checking"
              ? "Проверяем доступ к музыке ВК…"
              : "Завершите вход в открывшемся окне ВК."}
          </p>
          <button
            className="mt-3 text-xs text-slate-400"
            disabled={busy}
            onClick={() => void stop(false)}
          >
            Отменить вход
          </button>
        </div>
      ) : (
        <button
          className={button}
          disabled={busy || !hasApiV1Runtime() || data?.browserSupported === false || Boolean(data?.retryAfterSeconds)}
          onClick={() => void start()}
        >
          Войти через сайт ВК
        </button>
      )}
      {!waiting && (
        <div className="mt-3 flex flex-wrap gap-4 text-sm text-slate-300">
          <button onClick={() => void openWebsite().catch(failure => setError((failure as Error).message))}>Открыть сайт ВК</button>
          {data?.connected && <button disabled={busy || Boolean(data.retryAfterSeconds)} className="disabled:opacity-50" onClick={() => void start(true)}>Сменить аккаунт</button>}
        </div>
      )}
      {data?.limitMessage && <p role="status" className="mt-3 text-sm text-amber-300">{data.limitMessage}</p>}
      {(error || data?.session?.error) && (
        <p role="alert" className="mt-3 text-sm text-rose-300">
          {error || data?.session?.error}
        </p>
      )}
    </section>
  );
}
