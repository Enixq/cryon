import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { usePlayerStore } from "../../store/playerStore";
import { startRadioAutoplay } from "./useRadioAutoplay";
import { listRelatedTracks } from "../api/client";
import type { Track } from "../types";

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
  usePlayerStore.setState({
    queue: [track("seed")],
    currentIndex: 0,
    radio: true,
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
