import { beforeEach, describe, expect, it } from "vitest";
import type { Track } from "../shared/types";
import { rankRadioTracks, recordRadioSignal, resetRadioSession } from "./radioSession";

function track(id: string, artist: string): Track {
  return { id, artist, title: id, source: "youtube", duration: 180, accent: "violet" };
}

describe("radio session", () => {
  beforeEach(resetRadioSession);

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
});
