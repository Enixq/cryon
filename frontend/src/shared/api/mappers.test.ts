import { describe, it, expect } from "vitest";
import { mapTrack, mapTracks, toBackendTrack, splitTrackId } from "./mappers";
import type { domain } from "../../../wailsjs/go/models";

function backendTrack(over: Partial<domain.Track> = {}): domain.Track {
  return {
    id: "abc123",
    service: "youtube",
    title: "Песня",
    artists: ["Исполнитель"],
    album: "Альбом",
    durationMs: 185000,
    artworkUrl: "https://example.com/cover.jpg",
    externalUrl: "https://youtube.com/watch?v=abc123",
    playableKind: "stream",
    ...over,
  } as domain.Track;
}

describe("mapTrack", () => {
  it("строит составной id source:rawId", () => {
    expect(mapTrack(backendTrack()).id).toBe("youtube:abc123");
  });

  it("переводит длительность из мс в секунды", () => {
    expect(mapTrack(backendTrack()).duration).toBe(185);
    expect(mapTrack(backendTrack({ durationMs: 0 })).duration).toBe(0);
  });

  it("склеивает исполнителей через запятую", () => {
    expect(mapTrack(backendTrack({ artists: ["A", "B"] })).artist).toBe("A, B");
  });

  it("подставляет заглушку при пустом списке исполнителей", () => {
    expect(mapTrack(backendTrack({ artists: [] })).artist).toBe(
      "Неизвестный исполнитель",
    );
  });

  it("нормализует неизвестный playableKind в stream", () => {
    expect(mapTrack(backendTrack({ playableKind: "" })).playableKind).toBe(
      "stream",
    );
    expect(
      mapTrack(backendTrack({ playableKind: "external_only" })).playableKind,
    ).toBe("external_only");
  });

  it("даёт детерминированный accent для одного id", () => {
    expect(mapTrack(backendTrack()).accent).toBe(mapTrack(backendTrack()).accent);
  });

  it("неизвестный источник трактуется как local", () => {
    expect(mapTrack(backendTrack({ service: "tidal" })).source).toBe("local");
  });
});

describe("mapTracks", () => {
  it("возвращает пустой массив на null/undefined", () => {
    expect(mapTracks(null)).toEqual([]);
    expect(mapTracks(undefined)).toEqual([]);
  });

  it("маппит список", () => {
    expect(mapTracks([backendTrack(), backendTrack({ id: "x" })]).length).toBe(2);
  });
});

describe("splitTrackId", () => {
  it("разбирает составной id", () => {
    expect(splitTrackId("youtube:abc")).toEqual({
      source: "youtube",
      rawId: "abc",
    });
  });

  it("id без разделителя считает локальным", () => {
    expect(splitTrackId("plainid")).toEqual({ source: "local", rawId: "plainid" });
  });

  it("сохраняет двоеточия внутри rawId (Yandex track:album)", () => {
    expect(splitTrackId("yandex:123:456")).toEqual({
      source: "yandex",
      rawId: "123:456",
    });
  });
});

describe("toBackendTrack", () => {
  it("делает round-trip source и rawId", () => {
    const fe = mapTrack(backendTrack());
    const be = toBackendTrack(fe);
    expect(be.id).toBe("abc123");
    expect(be.service).toBe("youtube");
  });

  it("сохраняет playableKind и externalUrl", () => {
    const fe = mapTrack(backendTrack({ playableKind: "external_only" }));
    const be = toBackendTrack(fe);
    expect(be.playableKind).toBe("external_only");
    expect(be.externalUrl).toBe("https://youtube.com/watch?v=abc123");
  });

  it("для yandex по умолчанию ставит stream", () => {
    const be = toBackendTrack({
      id: "yandex:1:2",
      title: "t",
      artist: "a",
      source: "yandex",
      duration: 10,
      accent: "violet",
    });
    expect(be.playableKind).toBe("stream");
  });
});
