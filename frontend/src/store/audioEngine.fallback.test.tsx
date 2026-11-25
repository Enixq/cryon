import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Track } from "../shared/types";
import { useAudioEngine } from "./audioEngine";
import { usePlayerStore } from "./playerStore";
import { apiV1BaseUrl, resolvePlaybackFallbackV1, resolvePlaybackV1 } from "../shared/api/v1";
import { getAudioStreamInfo, getPlaybackUrlForHtml5, playerBackendAvailable, playerPlay, playerSetEqualizer, playerSetNormalize, playerStatus, playerStop, playerResume, resolvePlayableTrack, addHistory } from "../shared/api/client";

vi.mock("../shared/api/v1", () => ({
  apiV1BaseUrl: vi.fn().mockReturnValue("http://localhost:1234"),
  resolvePlaybackV1: vi.fn(),
  resolvePlaybackFallbackV1: vi.fn(),
}));

vi.mock("../shared/api/client", async (importOriginal) => ({
  ...await importOriginal<typeof import("../shared/api/client")>(),
  playerBackendAvailable: vi.fn().mockResolvedValue(false),
  playerSetEqualizer: vi.fn().mockResolvedValue(undefined),
  playerSetNormalize: vi.fn().mockResolvedValue(undefined),
  resolvePlayableTrack: vi.fn().mockImplementation(async (track: Track) => track),
  addHistory: vi.fn().mockResolvedValue(undefined),
  getAudioStreamInfo: vi.fn(),
  getPlaybackUrlForHtml5: vi.fn(),
  playerPlay: vi.fn().mockResolvedValue(undefined),
  playerStop: vi.fn().mockResolvedValue(undefined),
  playerResume: vi.fn().mockResolvedValue(undefined),
  playerStatus: vi.fn(),
}));

const track = (id: string): Track => ({ id, source: id.split(":")[0] as Track["source"], title: "Song", artist: "Artist", duration: 180, accent: "violet" });

describe("Android playback fallback", () => {
  it("does not echo foreground metadata updates or let old broadcasts undo pause", async () => {
    const first = track("youtube:first");
    const nativePlay = vi.fn(), nativePause = vi.fn(), nativeResume = vi.fn();
    window.CryonAndroid = { nativePlay, nativePause, nativeResume, nativeSetQueue: vi.fn(), nativeStop: vi.fn(), nativeSetVolume: vi.fn() };
    vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(() => {});
    vi.spyOn(HTMLMediaElement.prototype, "load").mockImplementation(() => {});
    vi.mocked(resolvePlaybackV1).mockImplementation(async item => ({ track: item, streamUrl: "/stream/audio" }));
    usePlayerStore.setState({ queue: [first], currentIndex: 0, isPlaying: true, playbackOnThisDevice: true });
    const { unmount } = renderHook(() => useAudioEngine());
    await waitFor(() => expect(nativePlay).toHaveBeenCalledTimes(1));
    const playAck = nativePlay.mock.calls[0][9];
    const baseline = nativeResume.mock.calls.length;
    for (let i = 0; i < 20; i++) act(() => window.dispatchEvent(new CustomEvent("cryon:native-playback", {
      detail: { trackId: first.id, queue: [{ ...first, quality: `quality-${i}` }], currentIndex: 0, state: "ready", playing: true, autoPlay: true, commandId: playAck, positionMs: i * 500, durationMs: 180000 },
    })));
    expect(nativeResume).toHaveBeenCalledTimes(baseline);
    act(() => usePlayerStore.getState().pause());
    const pauseAck = nativePause.mock.calls[nativePause.mock.calls.length - 1][0];
    act(() => window.dispatchEvent(new CustomEvent("cryon:native-playback", {
      detail: { trackId: first.id, queue: [first], currentIndex: 0, state: "ready", playing: true, autoPlay: true, commandId: playAck, positionMs: 11000, durationMs: 180000 },
    })));
    expect(usePlayerStore.getState().isPlaying).toBe(false);
    act(() => window.dispatchEvent(new CustomEvent("cryon:native-playback", {
      detail: { trackId: first.id, queue: [first], currentIndex: 0, state: "ready", playing: false, autoPlay: false, commandId: pauseAck, positionMs: 11000, durationMs: 180000 },
    })));
    expect(usePlayerStore.getState().isPlaying).toBe(false);
    expect(nativePlay).toHaveBeenCalledTimes(1);
    unmount();
  });
  beforeEach(() => {
    vi.mocked(playerBackendAvailable).mockResolvedValue(false);
    vi.mocked(playerSetEqualizer).mockResolvedValue(undefined);
    vi.mocked(playerSetNormalize).mockResolvedValue(undefined);
    vi.mocked(resolvePlayableTrack).mockImplementation(async item => item);
    vi.mocked(addHistory).mockResolvedValue(undefined);
    vi.mocked(playerStop).mockResolvedValue(undefined);
  });
  afterEach(() => {
    delete window.CryonAndroid;
    vi.restoreAllMocks();
    usePlayerStore.setState({ playbackOnThisDevice: true });
  });

  it("mirrors a native background transition without reloading its stream", async () => {
    const first = track("youtube:first"); const second = track("youtube:second");
    const nativePlay = vi.fn();
    window.CryonAndroid = { nativePlay, nativeSetQueue: vi.fn(), nativeStop: vi.fn(), nativeResume: vi.fn(), nativeSetVolume: vi.fn() };
    vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(() => {});
    vi.spyOn(HTMLMediaElement.prototype, "load").mockImplementation(() => {});
    vi.mocked(resolvePlaybackV1).mockImplementation(async item => ({ track: item, streamUrl: "/stream/audio" }));
    usePlayerStore.setState({ queue: [first, second], currentIndex: 0, isPlaying: true, playbackOnThisDevice: true });
    const { unmount } = renderHook(() => useAudioEngine());
    await waitFor(() => expect(nativePlay).toHaveBeenCalledTimes(1));
    act(() => window.dispatchEvent(new CustomEvent("cryon:native-playback", { detail: { trackId: first.id, queue: [first, second], currentIndex: 0, state: "ready", playing: true, autoPlay: true, positionMs: 1000, durationMs: 180000 } })));
    act(() => window.dispatchEvent(new CustomEvent("cryon:native-playback", { detail: { trackId: second.id, queue: [first, second], currentIndex: 1, state: "ready", playing: true, autoPlay: true, positionMs: 1500, durationMs: 180000 } })));
    expect(usePlayerStore.getState().currentIndex).toBe(1);
    expect(usePlayerStore.getState().progress).toBe(1.5);
    expect(nativePlay).toHaveBeenCalledTimes(1);
    unmount();
  });

  it("keeps a remote playing queue silent on this Android device", async () => {
    const nativePlay = vi.fn(); const nativeStop = vi.fn();
    window.CryonAndroid = { nativePlay, nativeSetQueue: vi.fn(), nativeStop };
    vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(() => {});
    vi.spyOn(HTMLMediaElement.prototype, "load").mockImplementation(() => {});
    usePlayerStore.setState({ queue: [track("youtube:first")], currentIndex: 0, isPlaying: true, playbackOnThisDevice: false });
    const { unmount } = renderHook(() => useAudioEngine());
    await waitFor(() => expect(nativeStop).toHaveBeenCalled());
    expect(nativePlay).not.toHaveBeenCalled();
    expect(usePlayerStore.getState().isPlaying).toBe(true);
    unmount();
  });

  it("continues from the original position after two failed versions", async () => {
    const first = track("youtube:first");
    const second = track("soundcloud:second");
    const third = track("youtube:third");
    const nativePlay = vi.fn();
    window.CryonAndroid = { nativePlay, nativeStop: vi.fn(), nativeResume: vi.fn(), nativeSetVolume: vi.fn() };
    vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(() => {});
    vi.spyOn(HTMLMediaElement.prototype, "load").mockImplementation(() => {});
    vi.mocked(resolvePlaybackV1).mockImplementation(async (item) => ({ track: item, streamUrl: "/stream/audio" }));
    vi.mocked(resolvePlaybackFallbackV1)
      .mockResolvedValueOnce({ track: second, streamUrl: "/stream/second", reason: "Вторая версия" })
      .mockResolvedValueOnce({ track: third, streamUrl: "/stream/third", reason: "Третья версия" });
    usePlayerStore.setState({ queue: [first], currentIndex: 0, isPlaying: true, progress: 0 });
    const { unmount } = renderHook(() => useAudioEngine());
    await waitFor(() => expect(nativePlay).toHaveBeenCalledTimes(1));
    act(() => window.dispatchEvent(new CustomEvent("cryon:native-playback", { detail: { trackId: first.id, state: "error", playing: false, positionMs: 42000, durationMs: 180000 } })));
    await waitFor(() => expect(nativePlay).toHaveBeenCalledTimes(2));
    expect(nativePlay.mock.calls[1][7]).toBe(42000);
    act(() => window.dispatchEvent(new CustomEvent("cryon:native-playback", { detail: { trackId: second.id, state: "error", playing: false, positionMs: 0, durationMs: 180000 } })));
    await waitFor(() => expect(nativePlay).toHaveBeenCalledTimes(3));
    expect(nativePlay.mock.calls[2][7]).toBe(42000);
    expect(resolvePlaybackFallbackV1).toHaveBeenLastCalledWith(first, second.source, [first.id, second.id]);
    unmount();
  });

  it("keeps the user's seek when the failed source reports position zero", async () => {
    const first = track("youtube:first");
    const second = track("soundcloud:second");
    const nativePlay = vi.fn();
    window.CryonAndroid = { nativePlay, nativeStop: vi.fn(), nativeResume: vi.fn(), nativeSeek: vi.fn(), nativeSetVolume: vi.fn() };
    vi.mocked(playerBackendAvailable).mockResolvedValue(false);
    vi.mocked(playerSetEqualizer).mockResolvedValue(undefined);
    vi.mocked(playerSetNormalize).mockResolvedValue(undefined);
    vi.mocked(resolvePlayableTrack).mockImplementation(async (item) => item);
    vi.mocked(addHistory).mockResolvedValue(undefined);
    vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(() => {});
    vi.spyOn(HTMLMediaElement.prototype, "load").mockImplementation(() => {});
    vi.mocked(resolvePlaybackV1).mockImplementation(async (item) => ({ track: item, streamUrl: "/stream/audio" }));
    vi.mocked(resolvePlaybackFallbackV1).mockReset().mockResolvedValueOnce({ track: second, streamUrl: "/stream/second", reason: "Другая версия" });
    usePlayerStore.setState({ queue: [first], currentIndex: 0, isPlaying: true, progress: 0 });
    const { unmount } = renderHook(() => useAudioEngine());
    await waitFor(() => expect(nativePlay).toHaveBeenCalledTimes(1));
    act(() => usePlayerStore.getState().seek(71));
    act(() => window.dispatchEvent(new CustomEvent("cryon:native-playback", { detail: { trackId: first.id, state: "error", playing: false, positionMs: 0, durationMs: 180000 } })));
    await waitFor(() => expect(nativePlay).toHaveBeenCalledTimes(2));
    expect(nativePlay.mock.calls[1][7]).toBe(71000);
    unmount();
  });
});

describe("desktop playback", () => {
  afterEach(() => {
    vi.mocked(playerBackendAvailable).mockResolvedValue(false);
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("plays the saved VK file returned by the resolver without requesting a VK stream", async () => {
    const saved = track("vk:web_1_2");
    const audio = document.createElement("audio");
    vi.stubGlobal("Audio", vi.fn(() => audio));
    vi.mocked(apiV1BaseUrl).mockReturnValue("http://localhost:1234");
    vi.mocked(playerBackendAvailable).mockResolvedValue(false);
    vi.mocked(playerSetEqualizer).mockResolvedValue(undefined);
    vi.mocked(playerSetNormalize).mockResolvedValue(undefined);
    vi.mocked(playerStop).mockResolvedValue(undefined);
    vi.mocked(resolvePlayableTrack).mockImplementation(async item => item);
    vi.mocked(resolvePlaybackV1).mockResolvedValue({
      track: saved,
      streamUrl: "/v1/library/downloads/file?key=saved",
      mimeType: "audio/mpeg",
    });
    vi.mocked(getPlaybackUrlForHtml5).mockClear();
    vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(() => {});
    vi.spyOn(HTMLMediaElement.prototype, "load").mockImplementation(() => {});
    usePlayerStore.setState({ queue: [saved], currentIndex: 0, isPlaying: false, playbackOnThisDevice: true });
    const { unmount } = renderHook(() => useAudioEngine());
    await waitFor(() => expect(audio.src).toBe("http://localhost:1234/v1/library/downloads/file?key=saved"));
    expect(getPlaybackUrlForHtml5).not.toHaveBeenCalled();
    unmount();
  });

  it("searches for another version when mpv cannot open a stream", async () => {
    const first = track("soundcloud:first");
    const second = track("youtube:second");
    vi.mocked(playerBackendAvailable).mockResolvedValue(true);
    vi.mocked(playerSetEqualizer).mockResolvedValue(undefined);
    vi.mocked(playerSetNormalize).mockResolvedValue(undefined);
    vi.mocked(resolvePlayableTrack).mockImplementation(async (item) => item);
    vi.mocked(addHistory).mockResolvedValue(undefined);
    vi.mocked(playerPlay).mockResolvedValue(undefined);
    vi.mocked(playerStop).mockResolvedValue(undefined);
    vi.mocked(playerResume).mockResolvedValue(undefined);
    vi.mocked(getAudioStreamInfo).mockResolvedValue({ url: "https://example.test/audio", format: "audio/mpeg", quality: "", duration: 180 });
    vi.mocked(getPlaybackUrlForHtml5).mockResolvedValue("/stream/youtube/second");
    vi.mocked(playerStatus).mockResolvedValue({ state: "error", positionS: 0, durationS: 0, eof: false, message: "Поток недоступен" });
    vi.mocked(resolvePlaybackFallbackV1).mockReset().mockResolvedValueOnce({ track: second, streamUrl: "/stream/youtube/second", reason: "Другая версия" });
    vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(() => {});
    vi.spyOn(HTMLMediaElement.prototype, "load").mockImplementation(() => {});
    vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue(undefined);
    usePlayerStore.setState({ queue: [], currentIndex: 0, isPlaying: true, progress: 0 });
    const { unmount } = renderHook(() => useAudioEngine());
    await waitFor(() => expect(playerBackendAvailable).toHaveBeenCalled());
    await act(async () => { await Promise.resolve(); await Promise.resolve(); });
    act(() => usePlayerStore.getState().playTrack(first, [first]));
    await waitFor(() => expect(playerPlay).toHaveBeenCalledWith(first.id, "https://example.test/audio", 0));
    await waitFor(() => expect(resolvePlaybackFallbackV1).toHaveBeenCalledWith(first, first.source, [first.id]), { timeout: 3000 });
    unmount();
  });

  it("starts the replacement in mpv at the position selected before failure", async () => {
    const first = track("youtube:first");
    const second = track("soundcloud:second");
    const audio = document.createElement("audio");
    vi.stubGlobal("Audio", vi.fn(() => audio));
    vi.mocked(playerBackendAvailable).mockResolvedValue(true);
    vi.mocked(playerSetEqualizer).mockResolvedValue(undefined);
    vi.mocked(playerSetNormalize).mockResolvedValue(undefined);
    vi.mocked(resolvePlayableTrack).mockImplementation(async (item) => item);
    vi.mocked(addHistory).mockResolvedValue(undefined);
    vi.mocked(playerPlay).mockReset().mockResolvedValue(undefined);
    vi.mocked(playerStop).mockResolvedValue(undefined);
    vi.mocked(playerResume).mockResolvedValue(undefined);
    vi.mocked(playerStatus).mockResolvedValue(null);
    vi.mocked(getAudioStreamInfo).mockResolvedValue({ url: "https://example.test/audio", format: "audio/mpeg", quality: "", duration: 180 });
    vi.mocked(getPlaybackUrlForHtml5).mockResolvedValue("/stream/youtube/first");
    vi.mocked(resolvePlaybackFallbackV1).mockReset().mockResolvedValueOnce({ track: second, streamUrl: "/stream/second", reason: "Другая версия" });
    vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(() => {});
    vi.spyOn(HTMLMediaElement.prototype, "load").mockImplementation(() => {});
    vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue(undefined);
    usePlayerStore.setState({ queue: [], currentIndex: 0, isPlaying: true, progress: 0 });
    const { unmount } = renderHook(() => useAudioEngine());
    await waitFor(() => expect(playerBackendAvailable).toHaveBeenCalled());
    await act(async () => { await Promise.resolve(); });
    act(() => usePlayerStore.getState().playTrack(first, [first]));
    await waitFor(() => expect(audio.onerror).not.toBeNull());
    act(() => usePlayerStore.getState().seek(71));
    act(() => audio.dispatchEvent(new Event("error")));
    await act(async () => { await new Promise((resolve) => window.setTimeout(resolve, 300)); });
    act(() => audio.dispatchEvent(new Event("error")));
    await waitFor(() => expect(playerPlay).toHaveBeenCalledWith(second.id, "https://example.test/audio", 71));
    unmount();
  });
});
