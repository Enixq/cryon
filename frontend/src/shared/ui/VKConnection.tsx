import { useEffect, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { hasApiV1Runtime, request } from "../api/v1";
import { ServiceIcon } from "./ServiceIcon";

type Login = {
  id: string;
  state: "loading" | "code" | "captcha" | "connected" | "error" | "cancelled";
  method?: string;
  canResend: boolean;
  error?: string;
  captchaImage?: string;
};
const path = "/v1/accounts/vk/login";
const field =
  "w-full rounded-xl border border-white/10 bg-black/20 px-3 py-2.5 text-sm text-white outline-none focus:border-[var(--app-accent)]";
const button =
  "rounded-xl bg-[var(--app-accent)] px-4 py-2.5 text-sm font-medium text-white disabled:opacity-50";

export function VKConnection() {
  const client = useQueryClient();
  const [phone, setPhone] = useState("");
  const [password, setPassword] = useState("");
  const [code, setCode] = useState("");
  const [token, setToken] = useState("");
  const [manual, setManual] = useState(false);
  const [sessionId, setSessionId] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const { data, refetch } = useQuery({
    queryKey: ["vk-login", sessionId],
    queryFn: () =>
      request<{ connected: boolean; session?: Login }>(
        `${path}?id=${encodeURIComponent(sessionId)}`,
      ),
    enabled: hasApiV1Runtime(),
    refetchInterval: (query) =>
      ["loading", "code", "captcha"].includes(query.state.data?.session?.state || "")
        ? 1000
        : false,
    retry: false,
  });
  useEffect(() => {
    if (data?.session?.state === "connected") {
      void client.invalidateQueries({ queryKey: ["sourceStatus"] });
    }
  }, [data?.session?.state, client]);
  const send = async (body: object, method = "POST") => {
    setBusy(true);
    setError("");
    try {
      const result = await request<Login>(path, {
        method,
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });
      if (result.id) setSessionId(result.id);
      setPassword("");
      setCode("");
      setToken("");
      await refetch();
      void client.invalidateQueries({ queryKey: ["sourceStatus"] });
    } catch (failure) {
      setPassword("");
      setError((failure as Error).message);
    } finally {
      setBusy(false);
    }
  };
  const session = data?.session;
  const captcha = session?.state === "captcha";
  const waiting = session?.state === "code" || captcha;
  const loading = session?.state === "loading";
  return (
    <section className="mb-5 break-inside-avoid rounded-2xl border border-white/5 bg-white/[0.025] p-5">
      <div className="mb-3 flex items-center gap-3">
        <ServiceIcon id="vk" size={32} badge />
        <h3 className="font-semibold text-white">ВК Музыка</h3>
      </div>
      {data?.connected ? (
        <div className="flex items-center justify-between gap-3">
          <span className="text-sm text-emerald-300">Подключено</span>
          <button
            type="button"
            disabled={busy}
            className={button}
            onClick={() => void send({ token: "" }, "PUT")}
          >
            Отключить
          </button>
        </div>
      ) : (
        <>
          <p className="mb-4 text-xs leading-relaxed text-slate-400">
            Войдите в ВК для поиска и воспроизведения доступной вашему аккаунту музыки. Пароль не
            сохраняется. Срок действия подключения зависит от ВК.
          </p>
          {!waiting && !loading && !manual && (
            <form
              className="flex flex-col gap-3"
              onSubmit={(event) => {
                event.preventDefault();
                void send({ phone, password });
              }}
            >
              <input
                aria-label="Номер телефона ВК"
                autoComplete="tel"
                type="tel"
                placeholder="Телефон ВК, +7…"
                value={phone}
                onChange={(event) => setPhone(event.target.value)}
                className={field}
              />
              <input
                aria-label="Пароль ВК"
                autoComplete="current-password"
                type="password"
                placeholder="Пароль ВК, если установлен"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                className={field}
              />
              <button className={button} disabled={busy || !phone.trim() || !hasApiV1Runtime()}>
                Войти в ВК Музыку
              </button>
            </form>
          )}
          {loading && (
            <p role="status" className="text-sm text-slate-300">
              Ожидаем ответ ВК…
            </p>
          )}
          {waiting && (
            <form
              className="flex flex-col gap-3"
              onSubmit={(event) => {
                event.preventDefault();
                void send({ id: sessionId, code });
              }}
            >
              {captcha ? (
                <>
                  <p className="text-sm text-slate-300">Введите символы с картинки ВК.</p>
                  <img
                    src={session.captchaImage}
                    alt="Проверка безопасности ВК"
                    className="max-h-28 max-w-full self-start rounded-lg bg-white"
                    referrerPolicy="no-referrer"
                  />
                </>
              ) : (
                <p className="text-sm text-slate-300">
                  Введите код{" "}
                  {session?.method === "email"
                    ? "из письма ВК"
                    : session?.method === "sms" || session?.method === "sms_inbox"
                      ? "из SMS"
                      : "из уведомления ВК"}
                  .
                </p>
              )}
              <input
                aria-label={captcha ? "Символы с картинки ВК" : "Код подтверждения ВК"}
                autoComplete={captcha ? "off" : "one-time-code"}
                inputMode={captcha ? "text" : "numeric"}
                value={code}
                onChange={(event) => setCode(event.target.value)}
                className={field}
              />
              <button disabled={busy || !code.trim()} className={button}>
                Подтвердить
              </button>
              {session?.canResend && (
                <button
                  type="button"
                  disabled={busy}
                  onClick={() => void send({ id: sessionId, resend: true })}
                  className="text-xs text-slate-400"
                >
                  Получить код другим способом
                </button>
              )}
            </form>
          )}
          {(loading || waiting) && (
            <button
              type="button"
              className="mt-3 text-xs text-slate-400"
              onClick={() => {
                void request(`${path}?id=${sessionId}`, { method: "DELETE" }).then(() => refetch());
              }}
            >
              Отменить вход
            </button>
          )}
          {!loading && !waiting && (
            <button
              type="button"
              className="mt-4 text-xs text-slate-400"
              onClick={() => setManual(!manual)}
            >
              {manual ? "Войти по телефону" : "У меня есть токен ВК Музыки"}
            </button>
          )}
          {manual && (
            <p className="mt-3 text-xs leading-relaxed text-slate-400">
              Токен ВК Музыки получается автоматически после входа по телефону в Cryon. Это
              отдельный токен музыкального API; обычный токен VK API не подходит. Ручное поле
              предназначено для уже полученного вами токена.{" "}
              <a
                className="underline"
                href="https://github.com/oklookat/vkmauth"
                target="_blank"
                rel="noreferrer"
              >
                Описание используемого способа входа
              </a>
            </p>
          )}
          {manual && !waiting && !loading && (
            <form
              className="mt-3 flex flex-col gap-3"
              onSubmit={(event) => {
                event.preventDefault();
                void send({ token }, "PUT");
              }}
            >
              <input
                aria-label="Токен ВК Музыки"
                autoComplete="off"
                type="password"
                placeholder="Токен API ВК Музыки"
                value={token}
                onChange={(event) => setToken(event.target.value)}
                className={field}
              />
              <button className={button} disabled={busy || !token.trim() || !hasApiV1Runtime()}>
                Подключить
              </button>
            </form>
          )}
        </>
      )}
      {(error || session?.error) && (
        <p role="alert" className="mt-3 text-sm text-rose-300">
          {error || session?.error}
        </p>
      )}
    </section>
  );
}
