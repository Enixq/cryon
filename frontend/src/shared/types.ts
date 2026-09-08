// Единые доменные типы приложения Cryon2.
// На этом этапе используются моковыми данными, позже будут заполняться из backend.

export type SourceId =
  | "youtube"
  | "soundcloud"
  | "spotify"
  | "yandex"
  | "vk"
  | "local";

export type AccentColor =
  | "violet"
  | "cyan"
  | "pink"
  | "orange"
  | "rose"
  | "amber"
  | "slate"
  | "emerald";

export interface Track {
  id: string;
  title: string;
  artist: string;
  album?: string;
  source: SourceId;
  /** Длительность трека в секундах. */
  duration: number;
  accent: AccentColor;
  liked?: boolean;
  /** URL обложки. Если пусто — показывается градиентная заглушка. */
  coverUrl?: string;
  /**
   * Способ воспроизведения: "stream" — есть прямой поток (YouTube, SoundCloud,
   * Yandex с токеном, локальные); "embedded_web" — встроенный веб-плеер;
   * "external_only" — можно только открыть во внешнем сервисе (Spotify, треки
   * без прямого потока). Undefined трактуется как "stream" (моки).
   */
  playableKind?: "stream" | "embedded_web" | "external_only";
  /** Ссылка на трек во внешнем сервисе (для external_only). */
  externalUrl?: string;
}

export interface Playlist {
  id: string;
  title: string;
  description: string;
  source: SourceId;
  accent: AccentColor;
  trackCount: number;
  /** URL обложки. Если пусто — показывается градиентная заглушка. */
  coverUrl?: string;
}

export interface Album {
  id: string;
  title: string;
  artist: string;
  year: number;
  source: SourceId;
  accent: AccentColor;
  trackCount: number;
  /** URL обложки. Если пусто — показывается градиентная заглушка. */
  coverUrl?: string;
}

export interface Artist {
  id: string;
  name: string;
  accent: AccentColor;
  followers: string;
  /** URL аватара. Если пусто — показывается градиентная заглушка. */
  coverUrl?: string;
}

export interface QuickMix {
  id: string;
  title: string;
  subtitle: string;
  accent: AccentColor;
}

/**
 * Релиз на странице исполнителя (альбом/сингл/EP). В отличие от витринного
 * Album, несёт service + kind: они нужны, чтобы лениво подгрузить трек-лист
 * релиза по (service, id) и разложить релизы по вкладкам.
 */
export interface ArtistRelease {
  id: string;
  service: SourceId;
  title: string;
  artist?: string;
  year?: number;
  coverUrl?: string;
  kind: "album" | "single" | "ep";
  trackCount?: number;
  externalUrl?: string;
}

/** Агрегированная страница исполнителя: фото, топ-треки и релизы. */
export interface ArtistPage {
  name: string;
  service?: SourceId;
  coverUrl?: string;
  topTracks: Track[];
  albums: ArtistRelease[];
  singles: ArtistRelease[];
  /** Релизы других исполнителей, где артист лишь участвует («Встречается в»). */
  appearsOn: ArtistRelease[];
}

export interface ServiceState {
  id: SourceId;
  name: string;
  connected: boolean;
}

export interface HistoryEntry {
  track: Track;
  /** ISO-строка времени прослушивания. */
  playedAt: string;
}

export type RepeatMode = "off" | "all" | "one";
