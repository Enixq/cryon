import { beforeEach, describe, expect, it, vi } from "vitest";
import { reportDJEvent } from "../shared/api/intelligence";
import type { Track } from "../shared/types";
import { consumeRadioCompletion, rankRadioTracks, recordRadioSignal, resetRadioSession } from "./radioSession";

vi.mock("../shared/api/intelligence", () => ({ reportDJEvent: vi.fn() }));

function track(id: string, artist: string): Track {
  return { id, artist, title: id, source: "youtube", duration: 180, accent: "violet" };
}

describe("radio session", () => {
  beforeEach(() => { resetRadioSession(); vi.mocked(reportDJEvent).mockClear(); });

  it("does not duplicate native completion observations", () => {
    const item = track("native", "Artist");
    recordRadioSignal(item, "completed", 0, true);
    expect(reportDJEvent).not.toHaveBeenCalled();
    expect(consumeRadioCompletion(item)).toBe(true);
    recordRadioSignal(item, "completed");
    expect(reportDJEvent).toHaveBeenCalledWith(item, "completed", 0);
  });

  it("removes skipped tracks and favours artists completed in this session", () => {
    recordRadioSignal(track("skipped", "Other"), "skipped");
    recordRadioSignal(track("completed", "Favourite"), "completed");
    expect(rankRadioTracks([track("skipped", "Other"), track("neutral", "Neutral"), track("new", "Favourite")]).map((item) => item.id)).toEqual(["new", "neutral"]);
  });

  it("forgets temporary feedback when the session is reset", () => {
    const candidate = track("skipped", "Artist");
    recordRadioSignal(candidate, "skipped");
    resetRadioSession();
    expect(rankRadioTracks([candidate])).toEqual([candidate]);
  });

  it("excludes an artist after two consecutive skips until the session resets", () => {
    recordRadioSignal(track("one", "Artist"), "skipped");
    recordRadioSignal(track("two", "artist"), "skipped");
    const candidates = [track("three", "Artist"), track("other", "Other")];
    expect(rankRadioTracks(candidates).map((item) => item.id)).toEqual(["other"]);
    resetRadioSession();
    expect(rankRadioTracks(candidates)).toEqual(candidates);
  });

  it("does not treat separated skips as consecutive", () => {
    recordRadioSignal(track("one", "Artist"), "skipped");
    recordRadioSignal(track("two", "Other"), "completed");
    recordRadioSignal(track("three", "Artist"), "skipped");
    expect(rankRadioTracks([track("four", "Artist")])).toHaveLength(1);
  });

  it("does not count repeated events for the same track as two skips", () => {
    recordRadioSignal(track("one", "Artist"), "skipped");
    recordRadioSignal(track("one", "Artist"), "skipped");
    expect(rankRadioTracks([track("two", "Artist")])).toHaveLength(1);
  });

  it("interleaves artists and limits repeats", () => {
    const candidates = [track("a1", "A"), track("a2", "A"), track("a3", "A"), track("b1", "B")];
    expect(rankRadioTracks(candidates).map((item) => item.id)).toEqual(["a1", "b1", "a2"]);
  });

  it("consumes a completion only once when advancing automatically", () => {
    const candidate = track("one", "Artist");
    recordRadioSignal(candidate, "completed");
    expect(consumeRadioCompletion(candidate)).toBe(true);
    expect(consumeRadioCompletion(candidate)).toBe(false);
  });
});
