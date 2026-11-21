import type { domain } from "../../../wailsjs/go/models";
import type { SourceId, Track } from "../types";
import { mapTracks, toBackendTrack } from "./mappers";
import { hasApiV1Runtime, request } from "./v1";

export interface SavedCollection {
  id: string;
  kind: "album" | "playlist";
  title: string;
  service: SourceId;
  catalogId?: string;
  artist?: string;
  artworkUrl?: string;
  tracks: Track[];
}
type WireCollection = Omit<SavedCollection, "tracks"> & { tracks: domain.Track[] };
const previewKey = "cryon.preview.savedCollections";
export async function listSavedCollections(): Promise<SavedCollection[]> {
  if (!hasApiV1Runtime()) {
    try {
      return JSON.parse(localStorage.getItem(previewKey) || "[]") as SavedCollection[];
    } catch {
      return [];
    }
  }
  return (await request<WireCollection[]>("/v1/library/collections")).map((item) => ({
    ...item,
    tracks: mapTracks(item.tracks),
  }));
}
export async function saveCollection(item: SavedCollection): Promise<void> {
  if (!hasApiV1Runtime()) {
    const saved = await listSavedCollections();
    localStorage.setItem(
      previewKey,
      JSON.stringify([item, ...saved.filter((value) => value.id !== item.id)]),
    );
    return;
  }
  await request("/v1/library/collections", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ ...item, tracks: item.tracks.slice(0, 1000).map(toBackendTrack) }),
  });
}
export async function removeSavedCollection(id: string): Promise<void> {
  if (!hasApiV1Runtime()) {
    localStorage.setItem(
      previewKey,
      JSON.stringify((await listSavedCollections()).filter((item) => item.id !== id)),
    );
    return;
  }
  await request(`/v1/library/collections?id=${encodeURIComponent(id)}`, { method: "DELETE" });
}
export async function catalogCollection(kind: string): Promise<Track[]> {
  if (!hasApiV1Runtime()) return [];
  return mapTracks(
    await request<domain.Track[]>(
      `/v1/catalog/collection?kind=${encodeURIComponent(kind)}&limit=50`,
    ),
  );
}
