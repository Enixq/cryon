import type { domain } from "../../../wailsjs/go/models";
import type { Track } from "../types";
import { mapTrack, mapTracks, toBackendTrack } from "./mappers";

type SearchResponse = { tracks?: domain.Track[] };
type PlaybackResolveResponse = { track: domain.Track; streamUrl: string; mimeType?: string };
type PlaybackFallbackResponse = { track: domain.Track; streamUrl: string; reason: string };
type HistoryResponse = { track: domain.Track; playedAtMs: number };

export type HistoryEntryV1 = {
  track: Track;
  playedAtMs: number;
};

export type PlaylistV1 = {
  id: string;
  title: string;
  description: string;
  trackCount: number;
  coverUrls?: string[];
};

export type SourceStatusV1 = {
  id: string;
  connected: boolean;
  level: "ok" | "limited" | "off";
  error?: string;
};

export type BackgroundJob = {
  id: string;
  kind: string;
  state: "queued" | "running" | "retrying" | "blocked" | "failed" | "completed" | "cancelled";
  priority: number;
  payloadJson: string;
  attempt: number;
  maxAttempts: number;
  progress: number;
  message: string;
  error?: string;
  runAfterMs: number;
  createdAtMs: number;
  updatedAtMs: number;
  startedAtMs?: number;
  finishedAtMs?: number;
};

export type PlaybackStatusV1 = {
  backend: "mpv" | "none";
  available: boolean;
  state: "idle" | "playing" | "paused";
  trackId?: string;
  positionS: number;
  durationS: number;
  volume: number;
  eof: boolean;
  message?: string;
};

export type PlaybackPreferencesV1 = {
  preferLocal: boolean;
  preferExplicit: boolean;
  officialOnly: boolean;
  sourcePriority: Record<string, number>;
};

export function apiV1BaseUrl(): string {
  if (typeof window === "undefined") return "";
  const base = (window as Window & { __CRYON_BASE__?: string }).__CRYON_BASE__;
  return typeof base === "string" ? base.replace(/\/+$/, "") : "";
}

export function hasApiV1Runtime(): boolean {
  if (typeof window === "undefined") return false;
  if (apiV1BaseUrl()) return true;
  return Boolean((window as Window & { go?: { main?: { App?: unknown } } }).go?.main?.App);
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const base = apiV1BaseUrl();
  if (!hasApiV1Runtime()) throw new Error("API v1 недоступен в этом runtime");
  const response = await fetch(base + path, init);
  if (!response.ok) {
    let message = `API v1: HTTP ${response.status}`;
    try {
      const body = (await response.json()) as { error?: string };
      if (body.error) message = body.error;
    } catch {
      // Keep the HTTP status when an upstream proxy returned a non-JSON body.
    }
    throw new Error(message);
  }
  if (response.status === 204) return undefined as T;
  return response.json() as Promise<T>;
}

export async function searchV1(query: string, service?: string): Promise<Track[]> {
  const params = new URLSearchParams({ q: query });
  if (service && service !== "all") params.set("service", service);
  const response = await request<SearchResponse>(`/v1/search?${params}`);
  return mapTracks(response.tracks);
}

function recommendationPath(path: string, limit: number): string {
  const params = new URLSearchParams({ limit: String(limit) });
  return `${path}?${params}`;
}

export async function recommendationsV1(limit: number): Promise<Track[]> {
  return mapTracks(await request<domain.Track[]>(recommendationPath("/v1/recommendations", limit)));
}

export async function dailyMixV1(limit: number): Promise<Track[]> {
  return mapTracks(await request<domain.Track[]>(recommendationPath("/v1/recommendations/daily-mix", limit)));
}

export async function weeklyDiscoveriesV1(limit: number): Promise<Track[]> {
  return mapTracks(await request<domain.Track[]>(recommendationPath("/v1/recommendations/weekly-discoveries", limit)));
}

export async function newReleasesV1(limit: number): Promise<Track[]> {
  return mapTracks(await request<domain.Track[]>(recommendationPath("/v1/recommendations/new-releases", limit)));
}

export async function relatedTracksV1(track: Track, limit: number): Promise<Track[]> {
  const response = await request<domain.Track[]>("/v1/recommendations/related", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ track: toBackendTrack(track), limit }),
  });
  return mapTracks(response);
}

export async function favoriteTracksV1(): Promise<Track[]> {
  return mapTracks(await request<domain.Track[]>("/v1/library/favorites"));
}

export async function localTracksV1(): Promise<Track[]> {
  return mapTracks(await request<domain.Track[]>("/v1/library/local/tracks"));
}

export async function localFoldersV1(): Promise<string[]> {
  return request<string[]>("/v1/library/local/folders");
}

export async function listSourcesV1(): Promise<SourceStatusV1[]> {
  return request<SourceStatusV1[]>("/v1/sources");
}

export async function validateSourceV1(id: string): Promise<void> {
  await request<void>(`/v1/sources/${encodeURIComponent(id)}/validate`, { method: "POST" });
}

export async function listHistoryV1(): Promise<HistoryEntryV1[]> {
  const history = await request<HistoryResponse[]>("/v1/library/history");
  return history.map((entry) => ({ track: mapTrack(entry.track), playedAtMs: entry.playedAtMs }));
}

export async function addHistoryV1(track: Track): Promise<void> {
  await request<void>("/v1/library/history", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ track: toBackendTrack(track) }),
  });
}

export async function clearHistoryV1(): Promise<void> {
  await request<void>("/v1/library/history", { method: "DELETE" });
}

export async function listPlaylistsV1(): Promise<PlaylistV1[]> {
  return request<PlaylistV1[]>("/v1/playlists");
}

export async function createPlaylistV1(title: string, description: string): Promise<PlaylistV1> {
  return request<PlaylistV1>("/v1/playlists", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ title, description }),
  });
}

export async function updatePlaylistV1(id: string, title: string, description: string): Promise<void> {
  await request<void>(`/v1/playlists/${encodeURIComponent(id)}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ title, description }),
  });
}

export async function deletePlaylistV1(id: string): Promise<void> {
  await request<void>(`/v1/playlists/${encodeURIComponent(id)}`, { method: "DELETE" });
}

export async function playlistTracksV1(id: string): Promise<Track[]> {
  return mapTracks(await request<domain.Track[]>(`/v1/playlists/${encodeURIComponent(id)}/tracks`));
}

export async function addPlaylistTrackV1(id: string, track: Track): Promise<void> {
  await request<void>(`/v1/playlists/${encodeURIComponent(id)}/tracks`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ track: toBackendTrack(track) }),
  });
}

export async function removePlaylistTrackV1(id: string, service: string, trackId: string): Promise<void> {
  const params = new URLSearchParams({ service, trackId });
  await request<void>(`/v1/playlists/${encodeURIComponent(id)}/tracks?${params}`, { method: "DELETE" });
}

export async function reorderPlaylistTracksV1(
  id: string,
  tracks: Array<{ service: string; trackId: string }>,
): Promise<void> {
  await request<void>(`/v1/playlists/${encodeURIComponent(id)}/tracks/order`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ tracks }),
  });
}

export async function resolvePlaybackV1(track: Track, native = false): Promise<{ track: Track; streamUrl: string; mimeType?: string } | null> {
  const response = await request<PlaybackResolveResponse>("/v1/playback/resolve", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ track: toBackendTrack(track), native }),
  });
  return response.streamUrl ? { track: mapTrack(response.track), streamUrl: response.streamUrl, mimeType: response.mimeType } : null;
}

export async function resolvePlaybackFallbackV1(
  track: Track,
  failedService = track.source,
): Promise<{ track: Track; streamUrl: string; reason: string } | null> {
  const response = await request<PlaybackFallbackResponse>("/v1/playback/fallback", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ track: toBackendTrack(track), failedService }),
  });
  if (!response.streamUrl) return null;
  return { track: mapTrack(response.track), streamUrl: response.streamUrl, reason: response.reason };
}

export async function playbackStatusV1(): Promise<PlaybackStatusV1> {
  return request<PlaybackStatusV1>("/v1/playback/status");
}

export function playbackPreferencesV1(): Promise<PlaybackPreferencesV1> {
  return request<PlaybackPreferencesV1>("/v1/playback/preferences");
}

export async function setPlaybackPreferencesV1(preferences: PlaybackPreferencesV1): Promise<void> {
  await request<void>("/v1/playback/preferences", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(preferences),
  });
}

async function playbackCommandV1(path: string, body?: unknown): Promise<void> {
  await request<void>(path, {
    method: "POST",
    headers: body === undefined ? undefined : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
}

export function playbackPlayV1(trackId: string, streamUrl: string): Promise<void> {
  return playbackCommandV1("/v1/playback/play", { trackId, streamUrl });
}

export function playbackPauseV1(): Promise<void> {
  return playbackCommandV1("/v1/playback/pause");
}

export function playbackResumeV1(): Promise<void> {
  return playbackCommandV1("/v1/playback/resume");
}

export function playbackStopV1(): Promise<void> {
  return playbackCommandV1("/v1/playback/stop");
}

export function playbackSeekV1(positionS: number): Promise<void> {
  return playbackCommandV1("/v1/playback/seek", { positionS });
}

export function playbackSetVolumeV1(volume: number): Promise<void> {
  return playbackCommandV1("/v1/playback/volume", { volume });
}

export async function enqueueJobV1(kind: string, priority = 0, payloadJson = ""): Promise<BackgroundJob> {
  return request<BackgroundJob>("/v1/jobs", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ kind, priority, payloadJson }),
  });
}

export async function listJobsV1(): Promise<BackgroundJob[]> {
  return request<BackgroundJob[]>("/v1/jobs");
}

export async function spotifyFileImportV1(file: File, action: "preview" | "import", trackIds: string[] = []): Promise<domain.Track[] | { source: string; found: number; imported: number; skipped: number }> {
  const form = new FormData();
  form.append("file", file);
  form.append("action", action);
  if (action === "import") form.append("trackIds", JSON.stringify(trackIds));
  return request("/v1/imports/spotify", { method: "POST", body: form });
}
