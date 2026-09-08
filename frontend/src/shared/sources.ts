import type { SourceId } from "./types";

export interface SourceMeta {
  id: SourceId;
  name: string;
  /** CSS-цвет точки-индикатора сервиса. */
  dot: string;
  short: string;
}

export const SOURCES: Record<SourceId, SourceMeta> = {
  youtube: { id: "youtube", name: "YouTube Music", dot: "#ff3b30", short: "YTM" },
  soundcloud: { id: "soundcloud", name: "SoundCloud", dot: "#ff7a00", short: "SC" },
  spotify: { id: "spotify", name: "Spotify", dot: "#1db954", short: "SP" },
  yandex: { id: "yandex", name: "Яндекс Музыка", dot: "#ffcc00", short: "ЯМ" },
  vk: { id: "vk", name: "VK Музыка", dot: "#4a76a8", short: "VK" },
  local: { id: "local", name: "Локальная музыка", dot: "#7c8cff", short: "LOC" },
};

export function sourceName(id: SourceId): string {
  return SOURCES[id].name;
}
