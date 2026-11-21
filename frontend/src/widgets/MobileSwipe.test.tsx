import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MobileMiniPlayer } from "./MobileMiniPlayer";
import { MobileNowPlaying } from "./MobileNowPlaying";
import { usePlayerStore } from "../store/playerStore";
import type { Track } from "../shared/types";
import { MemoryRouter } from "react-router-dom";

vi.mock("../shared/lib/useRecoFeedback", () => ({ useRecoFeedback: () => ({ scoreFor: () => 0, rate: vi.fn(), saving: false }) }));
vi.mock("../shared/lib/useFavorites", () => ({ useFavoriteIds: () => new Set() }));
vi.mock("../shared/lib/useTrackCover", () => ({ useTrackCover: () => undefined }));
vi.mock("../shared/lib/useCoverPalette", () => ({ useCoverPalette: () => "#777" }));
vi.mock("../shared/lib/useLyrics", () => ({ useLyrics: () => ({ loading: false, instrumental: false, lines: [], found: false, plain: "" }) }));

const track = (id: string): Track => ({ id, source: "youtube", title: id, artist: "Artist", duration: 180, accent: "violet" });
const queue = [track("first"), track("second"), track("third")];

describe("mobile player swipes", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("changes tracks when swiping the large cover", () => {
    usePlayerStore.setState({ queue, currentIndex: 1, isPlaying: true, progress: 40, repeat: "off", shuffle: false });
    render(<MemoryRouter><MobileNowPlaying /></MemoryRouter>);
    const cover = screen.getByRole("group", { name: "Обложка: свайп для смены трека" });
    fireEvent.touchStart(cover, { touches: [{ clientX: 220, clientY: 300 }] });
    fireEvent.touchMove(cover, { touches: [{ clientX: 120, clientY: 305 }] });
    fireEvent.touchEnd(cover, { changedTouches: [{ clientX: 120, clientY: 305 }] });
    expect(usePlayerStore.getState().currentTrack()?.id).toBe("third");
    fireEvent.touchStart(cover, { touches: [{ clientX: 120, clientY: 300 }] });
    fireEvent.touchMove(cover, { touches: [{ clientX: 220, clientY: 305 }] });
    fireEvent.touchEnd(cover, { changedTouches: [{ clientX: 220, clientY: 305 }] });
    expect(usePlayerStore.getState().currentTrack()?.id).toBe("second");
  });

  it("changes tracks when swiping the mini player cover", () => {
    vi.stubGlobal("PointerEvent", MouseEvent);
    usePlayerStore.setState({ queue, currentIndex: 1, isPlaying: true, progress: 0, repeat: "off", shuffle: false });
    render(<MobileMiniPlayer />);
    const coverButton = screen.getByRole("button", { name: "Открыть плеер" });
    fireEvent.pointerDown(coverButton, { clientX: 220, clientY: 100, pointerId: 1 });
    fireEvent.pointerMove(coverButton, { clientX: 120, clientY: 103, pointerId: 1 });
    expect(coverButton.style.transform).toBe("translateX(-100px)");
    expect(usePlayerStore.getState().currentTrack()?.id).toBe("second");
    fireEvent.pointerUp(coverButton, { clientX: 120, clientY: 103, pointerId: 1 });
    expect(usePlayerStore.getState().currentTrack()?.id).toBe("third");
  });

  it("previews touch seeking and sends one seek when the thumb is released", () => {
    class Pointer extends MouseEvent { pointerId = 1; }
    vi.stubGlobal("PointerEvent", Pointer);
    usePlayerStore.setState({ queue, currentIndex: 1, isPlaying: true, progress: 40 });
    const initial = usePlayerStore.getState().seekNonce;
    const view = render(<MemoryRouter><MobileNowPlaying /></MemoryRouter>);
    const slider = screen.getByRole("slider", { name: "Перемотка воспроизведения" });
    vi.spyOn(slider, "getBoundingClientRect").mockReturnValue({ left: 0, width: 180 } as DOMRect);
    fireEvent.pointerDown(slider, { clientX: 40, button: 0 });
    fireEvent.pointerMove(slider, { clientX: 100 });
    expect(usePlayerStore.getState().seekNonce).toBe(initial);
    expect(slider.getAttribute("aria-valuenow")).toBe("100");
    fireEvent.pointerUp(slider, { clientX: 120 });
    expect(usePlayerStore.getState().seekNonce).toBe(initial + 1);
    expect(usePlayerStore.getState().seekTarget).toBe(120);
    view.unmount();
  });
});
