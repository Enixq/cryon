// Загрузка текста песни для текущего трека с кэшированием (react-query).
//
// Ключ кэша — источник+id трека: у одной композиции текст один, повторные
// открытия панели берут его из кэша. Запрос уходит только когда трек задан.
// Backend (lrclib.net) не требует ключа, вне Wails вернётся «не найдено».

import { useQuery } from "@tanstack/react-query";
import { getLyrics, type LyricsResult } from "../api/client";
import type { Track } from "../types";
import { parseLrc, type LyricLine } from "./lyrics";

export interface UseLyricsResult {
  loading: boolean;
  found: boolean;
  instrumental: boolean;
  /** Синхронизированные строки (могут быть пустыми, даже если found). */
  lines: LyricLine[];
  /** Обычный текст (фолбэк, когда синхронизации нет). */
  plain: string;
}

const EMPTY: LyricsResult = { synced: "", plain: "", instrumental: false, found: false };

export function useLyrics(track: Track | undefined): UseLyricsResult {
  const { data = EMPTY, isFetching } = useQuery({
    queryKey: ["lyrics", track?.source, track?.id],
    queryFn: () =>
      getLyrics(track!.artist, track!.title, track!.album ?? "", track!.duration ?? 0),
    enabled: Boolean(track),
    staleTime: 60 * 60 * 1000, // текст неизменен — час в кэше без перезапроса
    gcTime: 60 * 60 * 1000,
    retry: false,
  });

  const lines = parseLrc(data.synced);
  return {
    loading: isFetching,
    found: data.found,
    instrumental: data.instrumental,
    lines,
    plain: data.plain,
  };
}
