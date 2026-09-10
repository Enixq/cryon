// HTTP-мост: делает Android/веб-сборку клиентом внешнего backend Cryon.
//
// Задача. На десктопе фронтенд общается с Go-бэкендом через Wails-биндинги
// (window.go.main.App.*) и рантайм (window.runtime.*). В Android-оболочке
// (Capacitor WebView) этого моста нет — раньше приложение падало в мок-режим и
// показывало демо-данные. Здесь мы САМИ ставим на window.go / window.runtime
// совместимые заглушки, которые гонят те же вызовы по HTTP на сервер Cryon
// (десктоп-приложение, запущенное в той же сети, или встроенный сервер).
//
// Благодаря этому ВЕСЬ существующий client.ts работает без единого изменения:
// isWailsRuntime() становится true, а App.* уходит по сети. Активируется,
// только если задан адрес сервера (localStorage) и мы ещё НЕ внутри настоящего
// Wails. Иначе — no-op: десктоп и обычная веб-разработка ведут себя как раньше.

const BASE_URL_KEY = "cryon.server.baseUrl";

type WindowBridge = {
  go?: { main?: { App?: unknown } };
  runtime?: unknown;
  __CRYON_BASE__?: string;
};

function win(): WindowBridge | undefined {
  return typeof window === "undefined" ? undefined : (window as unknown as WindowBridge);
}

/** Настоящий Wails уже присутствует (десктоп) — мост не нужен. */
function isRealWails(): boolean {
  const w = win();
  return Boolean(w?.go?.main?.App);
}

/** Сохранённый адрес backend (пусто — мост выключен, мок-режим). */
export function configuredBaseUrl(): string {
  if (typeof window === "undefined") return "";
  try {
    return (window.localStorage.getItem(BASE_URL_KEY) ?? "").trim().replace(/\/+$/, "");
  } catch {
    return "";
  }
}

/**
 * Адрес, ВНЕДРённый прямо в страницу (standalone Android): встроенный на
 * устройстве сервер отдаёт index.html со скриптом
 * `window.__CRYON_BASE__ = "<свой origin>"`. Имеет приоритет над сохранённым
 * вручную адресом. В обычной веб-разработке (vite) переменной нет → мок-режим.
 */
function injectedBaseUrl(): string {
  const w = win();
  const v = w?.__CRYON_BASE__;
  return typeof v === "string" ? v.trim().replace(/\/+$/, "") : "";
}

/** Задать/очистить адрес backend. Применяется после перезапуска приложения. */
export function setConfiguredBaseUrl(url: string): void {
  if (typeof window === "undefined") return;
  try {
    const v = url.trim().replace(/\/+$/, "");
    if (v) window.localStorage.setItem(BASE_URL_KEY, v);
    else window.localStorage.removeItem(BASE_URL_KEY);
  } catch {
    // приватный режим WebView — просто не сохранится
  }
}

/** RPC-вызов метода App по HTTP. Совпадает с JSON-контрактом Wails-биндингов. */
async function rpc(base: string, method: string, args: unknown[]): Promise<unknown> {
  const resp = await fetch(base + "/api/call", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ method, args }),
  });
  if (!resp.ok) {
    let msg = `RPC ${method}: HTTP ${resp.status}`;
    try {
      const j = (await resp.json()) as { error?: string };
      if (j && j.error) msg = j.error;
    } catch {
      // тело не JSON — оставляем статусное сообщение
    }
    throw new Error(msg);
  }
  const data = (await resp.json()) as { ok?: boolean; result?: unknown; error?: string };
  if (data && data.ok === false) throw new Error(data.error || `RPC ${method} failed`);
  return data ? data.result : undefined;
}

type EventHandler = (...data: unknown[]) => void;

/**
 * Ставит мост, если задан адрес сервера и мы вне настоящего Wails. Вызывается
 * один раз в main.tsx ДО рендера React. Возвращает true, если мост установлен.
 */
export function installHttpBridge(): boolean {
  const w = win();
  if (!w) return false;
  if (isRealWails()) return false; // десктоп — ничего не трогаем

  // Адрес backend: сперва внедрённый страницей (встроенный сервер Android),
  // затем сохранённый вручную (легаси-режим LAN). Пусто — остаёмся в мок-режиме.
  const base = injectedBaseUrl() || configuredBaseUrl();
  if (!base) return false;

  // --- Мост App.* (биндинги) ---
  const appProxy = new Proxy(
    {},
    {
      get(_t, prop) {
        if (typeof prop !== "string" || prop === "then") return undefined;
        return (...args: unknown[]) => {
          // Плеер mpv — только десктоп. На телефоне звук идёт через HTML5 <audio>
          // (/stream). Гасим весь Player*, чтобы не управлять mpv на ПК по сети.
          if (prop === "PlayerBackendAvailable") return Promise.resolve(false);
          if (prop.startsWith("Player")) return Promise.resolve(undefined);
          // Системные диалоги открылись бы на ПК и подвесили бы вызов с телефона.
          if (prop.startsWith("Pick")) return Promise.resolve("");
          return rpc(base, prop, args);
        };
      },
    },
  );

  // --- Мост window.runtime.* ---
  const listeners = new Map<string, Set<EventHandler>>();
  let es: EventSource | null = null;
  let esFailed = false;
  const ensureEvents = () => {
    if (es || esFailed || typeof EventSource === "undefined") return;
    try {
      es = new EventSource(base + "/api/events");
      es.onmessage = (ev: MessageEvent) => {
        try {
          const msg = JSON.parse(ev.data as string) as { event?: string; data?: unknown[] };
          if (!msg || !msg.event) return;
          const set = listeners.get(msg.event);
          if (set) set.forEach((cb) => cb(...(msg.data ?? [])));
        } catch {
          // некорректное событие игнорируем
        }
      };
      es.onerror = () => {
        // Сервер v1 может не отдавать события — не долбимся переподключением.
        esFailed = true;
        try {
          es?.close();
        } catch {
          // already closed
        }
        es = null;
      };
    } catch {
      esFailed = true;
    }
  };

  const eventsOnMultiple = (name: string, cb: EventHandler): (() => void) => {
    let set = listeners.get(name);
    if (!set) {
      set = new Set();
      listeners.set(name, set);
    }
    set.add(cb);
    ensureEvents();
    return () => {
      set?.delete(cb);
    };
  };

  const runtimeKnown: Record<string, unknown> = {
    EventsOnMultiple: (name: string, cb: EventHandler) => eventsOnMultiple(name, cb),
    EventsOn: (name: string, cb: EventHandler) => eventsOnMultiple(name, cb),
    EventsOnce: (name: string, cb: EventHandler) => {
      const off = eventsOnMultiple(name, (...d) => {
        off();
        cb(...d);
      });
      return off;
    },
    EventsOff: (name: string) => listeners.delete(name),
    EventsOffAll: () => listeners.clear(),
    EventsEmit: () => undefined,
    BrowserOpenURL: (url: string) => {
      try {
        window.open(url, "_blank", "noopener,noreferrer");
      } catch {
        // popup-блокировка — не критично
      }
    },
    LogPrint: (m: string) => console.log(m),
    LogTrace: (m: string) => console.log(m),
    LogDebug: (m: string) => console.debug(m),
    LogInfo: (m: string) => console.info(m),
    LogWarning: (m: string) => console.warn(m),
    LogError: (m: string) => console.error(m),
    LogFatal: (m: string) => console.error(m),
  };
  const runtimeProxy = new Proxy(runtimeKnown, {
    get(t, prop) {
      if (typeof prop === "string" && prop in t) return t[prop];
      // Десктоп-only рантайм (управление окном и т.п.) — безопасная заглушка.
      return () => undefined;
    },
  });

  w.go = { main: { App: appProxy } };
  w.runtime = runtimeProxy;
  w.__CRYON_BASE__ = base;

  // Backend на устройстве не имеет своего браузера: вместо runtime.BrowserOpenURL
  // он присылает событие "browser:open", а мы открываем ссылку средствами WebView
  // (на Android — системный intent). Подписка заодно поднимает поток /api/events.
  eventsOnMultiple("browser:open", (url) => {
    if (typeof url === "string" && url) {
      try {
        window.open(url, "_blank", "noopener,noreferrer");
      } catch {
        // popup-блокировка — не критично
      }
    }
  });

  return true;
}
