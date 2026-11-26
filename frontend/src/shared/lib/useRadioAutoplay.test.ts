import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { usePlayerStore } from "../../store/playerStore";
import { startRadioAutoplay } from "./useRadioAutoplay";
import { listRelatedTracks } from "../api/client";
import type { Track } from "../types";
import { hasApiV1Runtime } from "../api/v1";
import { djRadio } from "../api/intelligence";

vi.mock("../api/v1", async (original) => ({
  ...(await original<typeof import("../api/v1")>()),
  hasApiV1Runtime: vi.fn(() => false),
}));
vi.mock("../api/intelligence", async (original) => ({
  ...(await original<typeof import("../api/intelligence")>()),
  djRadio: vi.fn(),
}));

vi.mock("../api/client", async (original) => ({
  ...(await original<typeof import("../api/client")>()),
  listRelatedTracks: vi.fn(),
}));
const track = (id: string): Track => ({
  id,
  title: id,
  artist: id,
  source: "youtube",
  duration: 180,
  accent: "violet",
});
let stop: (() => void) | undefined;
beforeEach(() => {
  vi.useFakeTimers();
  vi.mocked(listRelatedTracks).mockReset();
  vi.mocked(hasApiV1Runtime).mockReturnValue(false);
  vi.mocked(djRadio).mockReset();
  usePlayerStore.setState({
    queue: [track("seed")],
    currentIndex: 0,
    radio: true,
    radioSeed: null,
    radioWaiting: false,
    isPlaying: true,
    playbackOnThisDevice: true,
    repeat: "off",
    shuffle: false,
  });
});
afterEach(() => {
  stop?.();
  stop = undefined;
  vi.useRealTimers();
});
describe("radio prefetch", () => {
  it("starts DJ with a full queue and promotes its result after the preloaded next track", async () => {
    vi.mocked(hasApiV1Runtime).mockReturnValue(true);
    const queue = Array.from({ length: 10 }, (_, i) => track(`base${i}`));
    const selected = { ...track("base7"), djPlanId: "plan" };
    usePlayerStore.setState({ queue, radioSeed: queue[0] });
    vi.mocked(djRadio)
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce([selected, { ...track("discovery"), djPlanId: "plan" }]);
    stop = startRadioAutoplay();
    await vi.advanceTimersByTimeAsync(0);
    expect(djRadio).toHaveBeenCalledWith(queue[0], queue[0], queue.slice(0, 2), 24, true);
    expect(listRelatedTracks).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(5000);
    expect(usePlayerStore.getState().queue.map((t) => t.id)).toEqual([
      "base0",
      "base1",
      "base7",
      "discovery",
      "base2",
      "base3",
      "base4",
      "base5",
      "base6",
      "base8",
      "base9",
    ]);
    expect(usePlayerStore.getState().currentIndex).toBe(0);
    await vi.advanceTimersByTimeAsync(10000);
    expect(djRadio).toHaveBeenCalledTimes(2);
  });
  it("keeps a full ordinary queue unchanged when DJ is disabled", async () => {
    vi.mocked(hasApiV1Runtime).mockReturnValue(true);
    vi.mocked(djRadio).mockResolvedValue(null);
    const queue = Array.from({ length: 10 }, (_, i) => track(`base${i}`));
    usePlayerStore.setState({ queue });
    stop = startRadioAutoplay();
    await vi.advanceTimersByTimeAsync(6000);
    expect(usePlayerStore.getState().queue).toEqual(queue);
    expect(listRelatedTracks).not.toHaveBeenCalled();
  });
  it("keeps track radio anchored to its seed and excludes the existing queue", async () => {
    const seed = track("original");
    usePlayerStore.getState().startTrackRadio(seed);
    vi.mocked(listRelatedTracks)
      .mockResolvedValueOnce(Array.from({ length: 8 }, (_, i) => track(`related${i}`)))
      .mockResolvedValueOnce([track("fresh")]);
    stop = startRadioAutoplay();
    await vi.advanceTimersByTimeAsync(0);
    usePlayerStore.getState().next();
    usePlayerStore.getState().next();
    await vi.advanceTimersByTimeAsync(0);
    expect(listRelatedTracks).toHaveBeenLastCalledWith(
      seed,
      24,
      expect.arrayContaining([seed, track("related7")]),
    );
    expect(usePlayerStore.getState().radioSeed?.id).toBe("original");
    expect(usePlayerStore.getState().queue.some((item) => item.id === "fresh")).toBe(true);
  });
  it("discards a previous track radio response after starting a new station", async () => {
    let resolve!: (tracks: Track[]) => void;
    usePlayerStore.getState().startTrackRadio(track("old"));
    vi.mocked(listRelatedTracks)
      .mockImplementationOnce(
        () =>
          new Promise((done) => {
            resolve = done;
          }),
      )
      .mockResolvedValue([]);
    stop = startRadioAutoplay();
    usePlayerStore.getState().startTrackRadio(track("new"));
    resolve([track("stale")]);
    await vi.advanceTimersByTimeAsync(0);
    expect(usePlayerStore.getState().queue.map((item) => item.id)).toEqual(["new"]);
  });
  it("continues a favorites queue when its track metadata is replaced while loading", async () => {
    let resolve!: (tracks: Track[]) => void;
    vi.mocked(listRelatedTracks)
      .mockImplementationOnce(
        () =>
          new Promise((done) => {
            resolve = done;
          }),
      )
      .mockResolvedValue([]);
    stop = startRadioAutoplay();
    const seed = usePlayerStore.getState().queue[0];
    usePlayerStore.getState().playTrack({ ...seed, liked: true }, [{ ...seed, liked: true }]);
    usePlayerStore.getState().next();
    resolve([track("fresh")]);
    await vi.advanceTimersByTimeAsync(0);
    expect(usePlayerStore.getState().currentTrack()?.id).toBe("fresh");
    expect(usePlayerStore.getState().isPlaying).toBe(true);
  });
  it("fills ahead, replenishes after next, and discards duplicate candidates", async () => {
    vi.mocked(listRelatedTracks)
      .mockResolvedValueOnce(Array.from({ length: 8 }, (_, i) => track(`a${i}`)))
      .mockResolvedValueOnce([track("a7"), track("fresh"), track("fresh")]);
    stop = startRadioAutoplay();
    await vi.advanceTimersByTimeAsync(0);
    expect(usePlayerStore.getState().queue).toHaveLength(9);
    usePlayerStore.getState().next();
    usePlayerStore.getState().next();
    await vi.advanceTimersByTimeAsync(0);
    expect(listRelatedTracks).toHaveBeenCalledTimes(2);
    expect(usePlayerStore.getState().queue.filter((t) => t.id === "fresh")).toHaveLength(1);
  });
  it("retries an empty reply and resumes a depleted queue", async () => {
    vi.mocked(listRelatedTracks)
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce([track("next")]);
    stop = startRadioAutoplay();
    await vi.advanceTimersByTimeAsync(0);
    usePlayerStore.getState().next();
    expect(usePlayerStore.getState().radioWaiting).toBe(true);
    await vi.advanceTimersByTimeAsync(5000);
    expect(usePlayerStore.getState().currentTrack()?.id).toBe("next");
    expect(usePlayerStore.getState().isPlaying).toBe(true);
  });
  it("does not resume when the user pauses while recommendations are loading", async () => {
    let resolve!: (tracks: Track[]) => void;
    vi.mocked(listRelatedTracks).mockImplementation(
      () =>
        new Promise((done) => {
          resolve = done;
        }),
    );
    stop = startRadioAutoplay();
    usePlayerStore.getState().next();
    usePlayerStore.getState().pause();
    resolve([track("next")]);
    await vi.advanceTimersByTimeAsync(0);
    expect(usePlayerStore.getState().isPlaying).toBe(false);
    expect(usePlayerStore.getState().currentIndex).toBe(0);
  });
  it("does not apply a response after radio is switched off", async () => {
    let resolve!: (tracks: Track[]) => void;
    vi.mocked(listRelatedTracks).mockImplementation(
      () =>
        new Promise((done) => {
          resolve = done;
        }),
    );
    stop = startRadioAutoplay();
    usePlayerStore.getState().toggleRadio();
    resolve([track("next")]);
    await vi.advanceTimersByTimeAsync(0);
    expect(usePlayerStore.getState().queue).toHaveLength(1);
  });
  it("leaves recommendation fetching to the selected playback device", async () => {
    usePlayerStore.setState({ playbackOnThisDevice: false });
    stop = startRadioAutoplay();
    await vi.advanceTimersByTimeAsync(10000);
    expect(listRelatedTracks).not.toHaveBeenCalled();
  });
});
