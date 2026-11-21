import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { getLyrics } from "../api/client";
import { hasApiV1Runtime, request } from "../api/v1";
import { splitTrackId } from "../api/mappers";
import type { Track } from "../types";
import { parseLrc, type LyricLine } from "./lyrics";
export type LyricsProvider = "auto" | "lrclib" | "ovh" | "source";
type LyricsData = {
  synced: string;
  plain: string;
  found: boolean;
  instrumental: boolean;
  provider?: string;
};
export interface UseLyricsResult {
  loading: boolean;
  found: boolean;
  instrumental: boolean;
  lines: LyricLine[];
  plain: string;
  provider: LyricsProvider;
  setProvider: (provider: LyricsProvider) => void;
  sourceName?: string;
  error?: string;
}
const EMPTY: LyricsData = { synced: "", plain: "", found: false, instrumental: false };
export function useLyrics(track: Track | undefined): UseLyricsResult {
  const [choice, setChoice] = useState<{ trackId: string; provider: LyricsProvider }>({
    trackId: "",
    provider: "auto",
  });
  const provider = choice.trackId === track?.id ? choice.provider : "auto";
  const {
    data = EMPTY,
    isFetching,
    error,
  } = useQuery<LyricsData>({
    queryKey: ["lyrics", track?.id, provider],
    queryFn: async () => {
      const artist = track!.primaryArtist || track!.artists?.[0] || track!.artist;
      if (!hasApiV1Runtime())
        return getLyrics(artist, track!.title, track!.album || "", track!.duration);
      const { source, rawId } = splitTrackId(track!.id);
      const params = new URLSearchParams({
        artist,
        title: track!.title,
        album: track!.album || "",
        duration: String(track!.duration || 0),
        provider,
        service: source,
        id: rawId,
      });
      return request<LyricsData>(`/v1/lyrics?${params}`);
    },
    enabled: Boolean(track),
    staleTime: 60 * 60 * 1000,
    gcTime: 60 * 60 * 1000,
    retry: false,
  });
  return {
    loading: isFetching,
    found: data.found,
    instrumental: data.instrumental,
    lines: parseLrc(data.synced),
    plain: data.plain,
    provider,
    setProvider: (value) => setChoice({ trackId: track?.id || "", provider: value }),
    sourceName: data.provider,
    error: error?.message,
  };
}
