// Маппинг доменных сущностей backend (Go/Wails) в типы фронтенда.
// Backend Track и frontend Track различаются по форме, поэтому все
// данные из биндингов проходят через эти функции.

import type { AccentColor, ArtistPage, ArtistRelease, SourceId, Track } from "../types";
import type { domain } from "../../../wailsjs/go/models";

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

// Детерминированный акцент по строке (id трека), чтобы у одного
// и того же трека цвет не «прыгал» между рендерами.
function accentFor(seed: string): AccentColor {
  let hash = 0;
  for (let i = 0; i < seed.length; i++) {
    hash = (hash * 31 + seed.charCodeAt(i)) | 0;
  }
  return ACCENTS[Math.abs(hash) % ACCENTS.length];
}

const KNOWN_SOURCES: SourceId[] = [
  "youtube",
  "soundcloud",
  "spotify",
  "yandex",
  "vk",
  "local",
];

function toSourceId(service: string): SourceId {
  return (KNOWN_SOURCES as string[]).includes(service)
    ? (service as SourceId)
    : "local";
}

/** Преобразует backend Track в Track фронтенда. */
export function mapTrack(src: domain.Track): Track {
  const source = toSourceId(src.service);
  const artist =
    Array.isArray(src.artists) && src.artists.length > 0
      ? src.artists.join(", ")
      : "Неизвестный исполнитель";

  return {
    id: `${source}:${src.id}`,
    title: src.title || "Без названия",
    artist,
    album: src.album || undefined,
    source,
    duration: src.durationMs ? Math.round(src.durationMs / 1000) : 0,
    accent: accentFor(src.id || src.title || source),
    coverUrl: src.artworkUrl || undefined,
    playableKind: normalizeKind(src.playableKind),
    externalUrl: src.externalUrl || undefined,
  };
}

function normalizeKind(
  kind: string | undefined,
): Track["playableKind"] {
  if (kind === "embedded_web" || kind === "external_only") return kind;
  return "stream";
}

export function mapTracks(list: domain.Track[] | null | undefined): Track[] {
  if (!Array.isArray(list)) return [];
  return list.map(mapTrack);
}

function normalizeReleaseKind(kind: string | undefined): ArtistRelease["kind"] {
  if (kind === "single" || kind === "ep") return kind;
  return "album";
}

/** Преобразует backend Album в релиз страницы исполнителя. */
export function mapRelease(src: domain.Album): ArtistRelease {
  const service = toSourceId(src.service);
  return {
    id: src.id,
    service,
    title: src.title || "Без названия",
    artist: src.artist || undefined,
    year: src.year || undefined,
    coverUrl: src.artworkUrl || undefined,
    kind: normalizeReleaseKind(src.kind),
    trackCount: src.trackCount || undefined,
    externalUrl: src.externalUrl || undefined,
  };
}

function mapReleases(list: domain.Album[] | null | undefined): ArtistRelease[] {
  if (!Array.isArray(list)) return [];
  return list.map(mapRelease);
}

/** Преобразует backend ArtistInfo в страницу исполнителя фронтенда. */
export function mapArtistInfo(src: domain.ArtistInfo): ArtistPage {
  return {
    name: src.name || "",
    service: src.service ? toSourceId(src.service) : undefined,
    coverUrl: src.artworkUrl || undefined,
    topTracks: mapTracks(src.topTracks),
    albums: mapReleases(src.albums),
    singles: mapReleases(src.singles),
    appearsOn: mapReleases(src.appearsOn),
  };
}

/** Преобразует Track фронтенда обратно в backend Track (для AddFavorite). */
export function toBackendTrack(t: Track): domain.Track {
  const { source, rawId } = splitTrackId(t.id);
  return {
    id: rawId,
    service: source,
    title: t.title,
    artists: t.artist ? [t.artist] : [],
    album: t.album || "",
    durationMs: t.duration ? t.duration * 1000 : 0,
    artworkUrl: t.coverUrl || "",
    externalUrl: t.externalUrl || "",
    playableKind:
      t.playableKind ||
      (source === "youtube" ||
      source === "soundcloud" ||
      source === "yandex" ||
      source === "local"
        ? "stream"
        : "embedded_web"),
  } as domain.Track;
}

// Разбирает составной id вида "youtube:VIDEOID" обратно на источник и id,
// как ожидают бэкенд-методы (GetAudioStream, RemoveFavorite).
export function splitTrackId(compositeId: string): {
  source: SourceId;
  rawId: string;
} {
  const idx = compositeId.indexOf(":");
  if (idx === -1) {
    return { source: "local", rawId: compositeId };
  }
  return {
    source: toSourceId(compositeId.slice(0, idx)),
    rawId: compositeId.slice(idx + 1),
  };
}
