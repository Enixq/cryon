import { hasApiV1Runtime, request } from "./v1";
import { toBackendTrack, mapTracks } from "./mappers";
import type { Track } from "../types";
import type { domain } from "../../../wailsjs/go/models";

export const djPollingOptions = {
  staleTime: 0,
  refetchInterval: 2000,
  refetchIntervalInBackground: true,
  refetchOnWindowFocus: "always" as const,
  refetchOnReconnect: "always" as const,
};

export type DJDiagnostic = {
  sources?: DJSourceDiagnostic[];
  requestDeadlineAt?: string;
  strategySource?: string;
  planFromCache?: boolean;
  llmCalls?: number;
  engine?: string;
  localAdmitted?: number;
  aiSelected?: number;
  warning?: string;
  jobId?: string;
  resultId?: string;
  sessionId?: string;
  mode?: string;
  iteration?: number;
  stage: string;
  error?: string;
  found: number;
  candidates: number;
  selected: number;
  updatedAt?: string;
  searches: {
    iteration?: number;
    unique?: number;
    accepted?: number;
    elapsedMs?: number;
    source: string;
    query: string;
    found: number;
    state: string;
  }[];
};
export type DJSourceDiagnostic = {
  source: string;
  enabled: boolean;
  auth: string;
  searchAttempted: boolean;
  queries: number;
  cached?: number;
  found: number;
  matched: number;
  playable: number;
  unchecked?: number;
  chosen: number;
  rejected: number;
  reasons: Record<string, number>;
  elapsedMs: number;
  state: string;
};
export type DJSourcePolicy = {
  enabled: Record<string, boolean>;
  priority: string[];
  sources: DJSourceDiagnostic[];
};
export const djSourceNames: Record<string, string> = {
  yandex: "Яндекс Музыка",
  vk: "VK Музыка",
  soundcloud: "SoundCloud",
  youtube: "YouTube",
  local: "Локальная библиотека",
};
export const djSourceStates: Record<string, string> = {
  ready: "готов к проверке",
  disabled: "выключен",
  unsupported: "адаптер недоступен",
  no_auth: "требуется вход",
  selected: "используется в очереди",
  playable: "поток проверен",
  search_error: "ошибка поиска",
  empty: "совпадений нет",
  identity_rejected: "другая композиция или запись",
  unplayable: "поток недоступен",
  matched_unchecked: "найдено; поток ещё не проверен",
};
export const djSourceReasons: Record<string, string> = {
  probe_budget_expired: "не проверено в пределах бюджета потоков",
  search_timeout: "истёк срок запроса",
  rate_limited: "источник ограничил запросы",
  search_error: "ошибка поиска",
  search_budget_expired: "истёк бюджет поиска",
  stream_unavailable: "поток не прошёл проверку",
  no_direct_stream: "нет прямого потока",
  artist_mismatch: "другой исполнитель",
  title_mismatch: "другое название",
  recording_variant: "другая версия записи",
  duration_mismatch: "другая длительность",
  invalid_source_id: "некорректный ID",
};
export function getDJSources(): Promise<DJSourcePolicy> {
  return request("/v1/intelligence/sources");
}
export function saveDJSources(
  value: Pick<DJSourcePolicy, "enabled" | "priority">,
): Promise<DJSourcePolicy> {
  return request("/v1/intelligence/sources", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(value),
  });
}
export type DJStatus = {
  diagnostic?: DJDiagnostic;
  usedToday: number;
  tokensToday: number;
  events: Record<string, number>;
  enabled: boolean;
  configured: boolean;
  provider: string;
  model: string;
  baseUrl: string;
  dailySegments: number;
  state: string;
  prepared: number;
  fallbacks: number;
  updatedAt?: string;
  trackCount: number;
};
let sessionId = "";
let generation = 0;
let enabled = false;
const sessions = new Map<string, string>();
const plans = new Map<string, string>();
export function canUseDJ(): boolean {
  return hasApiV1Runtime() && enabled;
}
export async function getDJStatus(): Promise<DJStatus> {
  const status = await request<DJStatus>("/v1/intelligence/config");
  enabled = status.enabled;
  return status;
}
export async function saveDJConfig(value: {
  enabled: boolean;
  provider: string;
  model: string;
  baseUrl: string;
  apiKey: string;
  dailySegments: number;
}): Promise<DJStatus> {
  const status = await request<DJStatus>("/v1/intelligence/config", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(value),
  });
  enabled = status.enabled;
  resetDJSession();
  return status;
}
export function resetDJSession(): void {
  const old = sessionId;
  sessionId = "";
  generation++;
  if (old && hasApiV1Runtime())
    void request("/v1/intelligence/end", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ sessionId: old }),
    }).catch(() => {});
}
export async function djRadio(
  seed: Track | null,
  anchor: Track,
  exclude: Track[],
  limit: number,
  preparedOnly = false,
): Promise<Track[] | null> {
  if (!hasApiV1Runtime()) return null;
  if (!enabled) {
    try {
      await getDJStatus();
    } catch {
      return null;
    }
  }
  if (!enabled) return null;
  const token = generation;
  const result = await request<{ sessionId: string; tracks: domain.Track[] }>(
    "/v1/intelligence/radio",
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        sessionId,
        mode: seed ? "track" : "infinite",
        track: toBackendTrack(seed ?? anchor),
        exclude: exclude.slice(-200).map(toBackendTrack),
        limit,
        preparedOnly,
      }),
    },
  );
  if (token !== generation) return [];
  sessionId = result.sessionId;
  const tracks = mapTracks(result.tracks);
  tracks.forEach((track, index) => {
    const raw = result.tracks[index] as domain.Track & { djPlanId?: string };
    if (sessionId) sessions.set(track.id, sessionId);
    if (raw.djPlanId) plans.set(track.id, raw.djPlanId);
  });
  if (sessions.size > 1000) {
    for (const key of [...sessions.keys()].slice(0, 500)) {
      sessions.delete(key);
      plans.delete(key);
    }
  }
  return tracks;
}
export function reportDJEvent(
  track: Track,
  kind:
    | "track_queued"
    | "playback_started"
    | "shown"
    | "like"
    | "dislike"
    | "skip"
    | "completed"
    | "replay"
    | "playback_error",
  playedMs = 0,
): void {
  const session = track.djSessionId || sessions.get(track.id);
  if (!hasApiV1Runtime()) return;
  if (!session) {
    if (["like", "dislike", "skip", "completed", "replay"].includes(kind)) {
      void request("/v1/intelligence/behavior", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          track: toBackendTrack(track),
          event: {
            id: crypto.randomUUID(),
            track_id: track.id,
            kind,
            played_ms: Math.max(0, Math.round(playedMs)),
            duration_ms: Math.max(0, Math.round(track.duration * 1000)),
            at: new Date().toISOString(),
          },
        }),
      }).catch(() => {});
    }
    return;
  }
  void request("/v1/intelligence/feedback", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      id: crypto.randomUUID(),
      session_id: session,
      plan_id: track.djPlanId || plans.get(track.id) || "",
      track_id: track.id,
      kind,
      played_ms: Math.max(0, Math.round(playedMs)),
      duration_ms: Math.max(0, Math.round(track.duration * 1000)),
      at: new Date().toISOString(),
    }),
  }).catch(() => {});
}

export type DJCollectionView = {
  tracks: Track[];
  job: DJDiagnostic;
  jobId: string;
  resultId: string;
  updatedAt?: string;
};
export async function getDJCollectionView(signal?: AbortSignal): Promise<DJCollectionView> {
  const result = await request<{
    tracks: domain.Track[];
    job: DJDiagnostic;
    jobId: string;
    resultId: string;
    updatedAt?: string;
  }>("/v1/intelligence/collection", { signal });
  return { ...result, tracks: mapTracks(result.tracks) };
}
export async function getDJCollection(): Promise<Track[]> {
  if (!hasApiV1Runtime()) return [];
  const result = await request<{ tracks: domain.Track[] }>("/v1/intelligence/collection");
  return mapTracks(result.tracks);
}
export async function refreshDJCollection(): Promise<void> {
  await request("/v1/intelligence/collection", { method: "POST" });
}

export async function rebuildDJProfile(): Promise<{
  preferences: { name: string; weight: number }[];
}> {
  return request("/v1/intelligence/profile");
}
