// Ленивая загрузка обложек треков.
//
// Онлайн-источники отдают coverUrl прямо в треке. Локальные файлы — нет:
// обложка вшита в аудиофайл и достаётся отдельным вызовом backend
// (GetLocalCover -> data-URL). Этот хук по требованию подтягивает такую
// обложку и кэширует результат в памяти, чтобы не дёргать backend на
// каждый ре-рендер списка.

import { useEffect, useState } from "react";
import type { Track } from "../types";
import { getLocalCover } from "../api/client";

// Кэш обложек по id трека. Значение "" означает «обложки нет» —
// повторно не запрашиваем.
const coverCache = new Map<string, string>();
// Промисы уже идущих запросов, чтобы не запускать несколько сразу.
const inFlight = new Map<string, Promise<string>>();

function loadLocalCover(id: string): Promise<string> {
  const cached = coverCache.get(id);
  if (cached !== undefined) return Promise.resolve(cached);

  const existing = inFlight.get(id);
  if (existing) return existing;

  const p = getLocalCover(id)
    .then((url) => {
      coverCache.set(id, url || "");
      return url || "";
    })
    .catch(() => {
      coverCache.set(id, "");
      return "";
    })
    .finally(() => {
      inFlight.delete(id);
    });
  inFlight.set(id, p);
  return p;
}

/**
 * Возвращает URL обложки трека. Если у трека уже есть coverUrl —
 * отдаёт его сразу. Для локального трека без обложки в метаданных
 * подтягивает встроенную картинку из файла (с кэшем). Иначе — undefined,
 * и компонент показывает градиентную заглушку.
 */
export function useTrackCover(track: Track | undefined): string | undefined {
  const hasOwnCover = Boolean(track?.coverUrl);
  const isLocal = track?.source === "local";
  const id = track?.id;

  const [resolved, setResolved] = useState<string | undefined>(() => {
    if (hasOwnCover) return track!.coverUrl;
    if (isLocal && id) return coverCache.get(id) || undefined;
    return undefined;
  });

  useEffect(() => {
    if (hasOwnCover) {
      setResolved(track?.coverUrl);
      return;
    }
    if (!isLocal || !id) {
      setResolved(undefined);
      return;
    }
    let cancelled = false;
    loadLocalCover(id).then((url) => {
      if (!cancelled) setResolved(url || undefined);
    });
    return () => {
      cancelled = true;
    };
  }, [hasOwnCover, isLocal, id, track?.coverUrl]);

  return resolved;
}
