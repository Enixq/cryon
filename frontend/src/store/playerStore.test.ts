import { describe, it, expect, beforeEach } from "vitest";
import { usePlayerStore } from "./playerStore";
import type { Track } from "../shared/types";

function makeTrack(id: string): Track {
  return {
    id,
    title: `Track ${id}`,
    artist: "Artist",
    source: "youtube",
    duration: 200,
    accent: "violet",
  };
}

const q = [makeTrack("a"), makeTrack("b"), makeTrack("c")];

describe("playerStore queue navigation", () => {
  beforeEach(() => {
    usePlayerStore.setState({
      queue: q,
      currentIndex: 0,
      isPlaying: false,
      progress: 0,
      repeat: "off",
      shuffle: false,
    });
  });

  it("next переходит к следующему треку", () => {
    usePlayerStore.getState().next();
    expect(usePlayerStore.getState().currentIndex).toBe(1);
    expect(usePlayerStore.getState().isPlaying).toBe(true);
  });

  it("next в конце очереди без repeat остаётся на месте", () => {
    usePlayerStore.setState({ currentIndex: 2 });
    usePlayerStore.getState().next();
    expect(usePlayerStore.getState().currentIndex).toBe(2);
  });

  it("next в конце очереди с repeat=all уходит в начало", () => {
    usePlayerStore.setState({ currentIndex: 2, repeat: "all" });
    usePlayerStore.getState().next();
    expect(usePlayerStore.getState().currentIndex).toBe(0);
  });

  it("next с repeat=one не меняет индекс, сбрасывает прогресс", () => {
    usePlayerStore.setState({ currentIndex: 1, repeat: "one", progress: 50 });
    usePlayerStore.getState().next();
    expect(usePlayerStore.getState().currentIndex).toBe(1);
    expect(usePlayerStore.getState().progress).toBe(0);
  });

  it("previous после 3с перематывает в начало трека", () => {
    usePlayerStore.setState({ currentIndex: 1, progress: 10 });
    usePlayerStore.getState().previous();
    expect(usePlayerStore.getState().currentIndex).toBe(1);
    expect(usePlayerStore.getState().progress).toBe(0);
  });

  it("previous в начале трека переходит к предыдущему", () => {
    usePlayerStore.setState({ currentIndex: 2, progress: 1 });
    usePlayerStore.getState().previous();
    expect(usePlayerStore.getState().currentIndex).toBe(1);
  });

  it("seek ограничивает цель длительностью и инкрементит nonce", () => {
    const before = usePlayerStore.getState().seekNonce;
    usePlayerStore.getState().seek(500);
    const s = usePlayerStore.getState();
    expect(s.seekTarget).toBe(200);
    expect(s.seekNonce).toBe(before + 1);
  });
});

describe("playerStore volume", () => {
  beforeEach(() => {
    usePlayerStore.setState({ volume: 50, muted: false });
  });

  it("setVolume ограничивает 0..100 и снимает mute", () => {
    usePlayerStore.setState({ muted: true });
    usePlayerStore.getState().setVolume(150);
    expect(usePlayerStore.getState().volume).toBe(100);
    expect(usePlayerStore.getState().muted).toBe(false);
  });

  it("nudgeVolume не выходит за границы", () => {
    usePlayerStore.getState().nudgeVolume(-80);
    expect(usePlayerStore.getState().volume).toBe(0);
  });
});

describe("playerStore queue management", () => {
  beforeEach(() => {
    usePlayerStore.setState({ queue: [...q], currentIndex: 1 });
  });

  it("removeFromQueue корректирует currentIndex при удалении до текущего", () => {
    usePlayerStore.getState().removeFromQueue("a");
    expect(usePlayerStore.getState().currentIndex).toBe(0);
    expect(usePlayerStore.getState().queue.map((t) => t.id)).toEqual(["b", "c"]);
  });

  it("removeFromQueue останавливает плеер при удалении последнего текущего трека", () => {
    usePlayerStore.setState({ queue: [makeTrack("a")], currentIndex: 0, isPlaying: true, progress: 42, engineDuration: 200 });
    usePlayerStore.getState().removeFromQueue("a");
    const state = usePlayerStore.getState();
    expect(state.queue).toEqual([]);
    expect(state.currentIndex).toBe(0);
    expect(state.isPlaying).toBe(false);
    expect(state.progress).toBe(0);
    expect(state.engineDuration).toBe(0);
  });

  it("addToQueue не дублирует существующий трек", () => {
    usePlayerStore.getState().addToQueue(makeTrack("b"));
    expect(usePlayerStore.getState().queue.length).toBe(3);
    usePlayerStore.getState().addToQueue(makeTrack("d"));
    expect(usePlayerStore.getState().queue.length).toBe(4);
  });

  it("playTrack с новой очередью встаёт на нужный индекс", () => {
    usePlayerStore.getState().playTrack(makeTrack("c"), q);
    expect(usePlayerStore.getState().currentIndex).toBe(2);
    expect(usePlayerStore.getState().isPlaying).toBe(true);
  });
});
