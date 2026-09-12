// Поделиться треком без сервера и регистрации: трек упаковывается в компактную
// ссылку `cryon://track/<данные>`, которую можно отправить другу в любом
// мессенджере. Друг вставляет ссылку в Cryon — трек добавляется к нему.
//
// Почему так, а не чат: полноценный чат/друзья требуют сервера и учёток, а это
// намеренно отложено («без регистрации пока»). Ссылка решает саму задачу —
// «поделиться треком» — и работает офлайн между двумя копиями приложения.
//
// Воспроизведение у получателя зависит от его источников: движок при запуске
// вызывает resolvePlayableTrack и подбирает играбельный эквивалент по
// метаданным (название/исполнитель), даже если исходный поток недоступен.

import type { AccentColor, SourceId, Track } from "../types";

const PREFIX = "cryon://track/";

const ACCENTS: AccentColor[] = [
  "violet",
  "cyan",
  "pink",
  "orange",
  "rose",
  "amber",
  "slate",
  "emerald",
];
const SOURCES: SourceId[] = ["youtube", "soundcloud", "spotify", "yandex", "vk", "local"];

// Компактное представление трека (короткие ключи — короче ссылка).
interface SharePayload {
  v: 1;
  i?: string;
  t: string;
  a: string;
  al?: string;
  s?: SourceId;
  d?: number;
  c?: string;
  k?: Track["playableKind"];
  u?: string;
  ac?: AccentColor;
}

function toBase64Url(input: string): string {
  const bytes = new TextEncoder().encode(input);
  let binary = "";
  bytes.forEach((b) => {
    binary += String.fromCharCode(b);
  });
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

function fromBase64Url(input: string): string {
  const base64 = input.replace(/-/g, "+").replace(/_/g, "/");
  const padded = base64.length % 4 ? base64 + "=".repeat(4 - (base64.length % 4)) : base64;
  const binary = atob(padded);
  const bytes = Uint8Array.from(binary, (c) => c.charCodeAt(0));
  return new TextDecoder().decode(bytes);
}

// Уникальный id с деградацией по возможностям среды. crypto.randomUUID есть не
// везде: на старом Android WebView (< 92) его нет вовсе, а вне защищённого
// контекста он бросает исключение. Раньше это роняло весь разбор ссылки (и,
// шире, экран). Пробуем randomUUID → getRandomValues → Math.random.
function safeRandomId(): string {
  try {
    const c = globalThis.crypto as Crypto | undefined;
    if (c && typeof c.randomUUID === "function") return c.randomUUID();
    if (c && typeof c.getRandomValues === "function") {
      const buf = new Uint8Array(16);
      c.getRandomValues(buf);
      return Array.from(buf, (b) => b.toString(16).padStart(2, "0")).join("");
    }
  } catch {
    // недоступно/бросило — уходим на Math.random ниже
  }
  return `${Date.now().toString(16)}${Math.random().toString(16).slice(2, 10)}`;
}

/** Упаковывает трек в ссылку `cryon://track/…` для передачи другу. */
export function encodeTrackShare(track: Track): string {
  const payload: SharePayload = {
    v: 1,
    i: track.id,
    t: track.title,
    a: track.artist,
    al: track.album,
    s: track.source,
    d: track.duration,
    c: track.coverUrl,
    k: track.playableKind,
    u: track.externalUrl,
    ac: track.accent,
  };
  // Пустые поля выкидываем, чтобы ссылка была короче.
  (Object.keys(payload) as (keyof SharePayload)[]).forEach((key) => {
    const value = payload[key];
    if (value === undefined || value === "" || (key === "d" && !value)) delete payload[key];
  });
  return PREFIX + toBase64Url(JSON.stringify(payload));
}

/**
 * Разбирает ссылку/код, полученные от друга, обратно в трек. Терпима к вводу:
 * принимает полную ссылку, код без префикса и лишние пробелы/переводы строк.
 * Возвращает null, если это не похоже на ссылку Cryon.
 */
export function decodeTrackShare(input: string): Track | null {
  if (!input) return null;
  let token = input.trim();
  // Вытаскиваем токен из возможного окружающего текста (напр. из сообщения).
  const at = token.indexOf(PREFIX);
  if (at >= 0) token = token.slice(at + PREFIX.length);
  // Обрезаем всё после первого пробела/переноса — на случай прилипшего текста.
  token = token.split(/\s/)[0];
  if (!token) return null;

  try {
    const data = JSON.parse(fromBase64Url(token)) as Partial<SharePayload>;
    if (!data || typeof data.t !== "string" || typeof data.a !== "string") return null;
    const source: SourceId = data.s && SOURCES.includes(data.s) ? data.s : "youtube";
    const accent: AccentColor = data.ac && ACCENTS.includes(data.ac) ? data.ac : "violet";
    const track: Track = {
      id: data.i && typeof data.i === "string" ? data.i : `shared:${safeRandomId()}`,
      title: data.t,
      artist: data.a,
      album: typeof data.al === "string" ? data.al : undefined,
      source,
      duration: typeof data.d === "number" && data.d > 0 ? data.d : 0,
      accent,
      coverUrl: typeof data.c === "string" ? data.c : undefined,
      playableKind: data.k,
      externalUrl: typeof data.u === "string" ? data.u : undefined,
    };
    return track;
  } catch {
    return null;
  }
}

/** Быстрая проверка, что строка похожа на ссылку Cryon (для автоподхвата). */
export function looksLikeTrackShare(input: string): boolean {
  return typeof input === "string" && input.includes(PREFIX);
}
