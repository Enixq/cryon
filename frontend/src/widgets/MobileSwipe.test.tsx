import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MobileMiniPlayer } from "./MobileMiniPlayer";
import { MobileNowPlaying, MobileNowPlayingSheet } from "./MobileNowPlaying";
import { usePlayerStore } from "../store/playerStore";
import type { Track } from "../shared/types";
import { MemoryRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useUiStore } from "../store/uiStore";
import { useAndroidBackButton } from "../shared/lib/useAndroidBackButton";

vi.mock("../shared/lib/useRecoFeedback", () => ({
  useRecoFeedback: () => ({ scoreFor: () => 0, rate: vi.fn(), saving: false }),
}));
vi.mock("../shared/lib/useFavorites", () => ({ useFavoriteIds: () => new Set() }));
vi.mock("../shared/lib/useTrackCover", () => ({ useTrackCover: () => undefined }));
vi.mock("../shared/lib/useCoverPalette", () => ({ useCoverPalette: () => "#777" }));
vi.mock("../shared/lib/useLyrics", () => ({
  useLyrics: () => ({ loading: false, instrumental: false, lines: [], found: false, plain: "" }),
}));

const track = (id: string): Track => ({
  id,
  source: "youtube",
  title: id,
  artist: "Artist",
  duration: 180,
  accent: "violet",
});
const queue = [track("first"), track("second"), track("third")];

describe("mobile player swipes", () => {
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
    useUiStore.setState({ nowPlayingOpen: false, nowPlayingView: "cover", shareOpen: false });
  });

  it("follows a vertical drag in both directions and cancels without navigating", () => {
    vi.useFakeTimers();
    usePlayerStore.setState({ queue, currentIndex: 1 });
    const mounted = render(
      <MemoryRouter>
        <MobileNowPlaying />
      </MemoryRouter>,
    );
    const cover = screen.getByRole("group", { name: "Обложка: свайп для смены трека" });
    const page = mounted.container.querySelector<HTMLElement>("[data-player-page=cover]")!;
    const sheet = mounted.container.querySelector<HTMLElement>("[data-player-sheet]")!;
    act(() => {
      vi.advanceTimersByTime(32);
    });
    fireEvent.touchStart(cover, { touches: [{ clientX: 180, clientY: 420 }] });
    fireEvent.touchMove(cover, { touches: [{ clientX: 180, clientY: 300 }] });
    expect(page.style.transform).not.toBe("translate3d(0, 0%, 0)");
    expect(page.style.transition).toBe("none");
    fireEvent.touchMove(cover, { touches: [{ clientX: 180, clientY: 445 }] });
    expect(sheet.style.transform).toContain("25px");
    fireEvent.touchCancel(cover);
    expect(page.style.transform).toBe("translate3d(0, 0%, 0)");
    expect(page.style.transition).toContain("360ms");
    expect(useUiStore.getState().nowPlayingView).toBe("cover");
    expect(usePlayerStore.getState().currentIndex).toBe(1);
  });

  it("pulls the queue back to the cover but preserves scrolling inside a long queue", () => {
    usePlayerStore.setState({ queue, currentIndex: 1 });
    useUiStore.setState({ nowPlayingOpen: true, nowPlayingView: "queue" });
    const mounted = render(
      <MemoryRouter>
        <MobileNowPlaying />
      </MemoryRouter>,
    );
    const list = mounted.container.querySelector<HTMLElement>("[data-player-scroll]")!;
    const extra = mounted.container.querySelector<HTMLElement>("[data-player-page=extra]")!;
    list.scrollTop = 80;
    fireEvent.touchStart(list, { touches: [{ clientX: 180, clientY: 300 }] });
    fireEvent.touchMove(list, { touches: [{ clientX: 180, clientY: 460 }] });
    fireEvent.touchEnd(list);
    expect(useUiStore.getState().nowPlayingView).toBe("queue");
    list.scrollTop = 0;
    fireEvent.touchStart(list, { touches: [{ clientX: 180, clientY: 300 }] });
    fireEvent.touchMove(list, { touches: [{ clientX: 180, clientY: 460 }] });
    expect(extra.style.transform).not.toBe("translate3d(0, 0%, 0)");
    fireEvent.touchEnd(list);
    expect(useUiStore.getState()).toMatchObject({ nowPlayingOpen: true, nowPlayingView: "cover" });
    expect(usePlayerStore.getState().currentIndex).toBe(1);
  });

  it("keeps the sheet mounted while closing and can reopen during its exit", () => {
    vi.useFakeTimers();
    usePlayerStore.setState({ queue, currentIndex: 1 });
    const mounted = render(
      <MemoryRouter>
        <MobileNowPlayingSheet open />
      </MemoryRouter>,
    );
    mounted.rerender(
      <MemoryRouter>
        <MobileNowPlayingSheet open={false} />
      </MemoryRouter>,
    );
    expect(
      mounted.container.querySelector<HTMLElement>("[data-player-sheet]")!.style.transform,
    ).toContain("100%");
    act(() => {
      vi.advanceTimersByTime(200);
    });
    mounted.rerender(
      <MemoryRouter>
        <MobileNowPlayingSheet open />
      </MemoryRouter>,
    );
    act(() => {
      vi.advanceTimersByTime(400);
    });
    expect(mounted.container.querySelector("[data-player-sheet]")).not.toBeNull();
    mounted.rerender(
      <MemoryRouter>
        <MobileNowPlayingSheet open={false} />
      </MemoryRouter>,
    );
    act(() => {
      vi.advanceTimersByTime(380);
    });
    expect(mounted.container.querySelector("[data-player-sheet]")).toBeNull();
  });

  it("shares the current track and offers the existing track actions", () => {
    usePlayerStore.setState({ queue, currentIndex: 1 });
    render(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter>
          <MobileNowPlaying />
        </MemoryRouter>
      </QueryClientProvider>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Поделиться треком" }));
    expect(useUiStore.getState().shareTrack?.id).toBe("second");
    fireEvent.click(screen.getByRole("button", { name: "Другие действия" }));
    expect(screen.getByRole("menu")).toBeTruthy();
    expect(screen.getByText("Сменить источник трека")).toBeTruthy();
  });

  it("opens the queue on an upward swipe and Android back returns to the player first", () => {
    window.CryonAndroid = {};
    const BackHandler = () => {
      useAndroidBackButton();
      return null;
    };
    usePlayerStore.setState({ queue, currentIndex: 1, isPlaying: false });
    useUiStore.setState({ nowPlayingOpen: true, nowPlayingView: "cover" });
    const mounted = render(
      <MemoryRouter initialEntries={["/library"]}>
        <BackHandler />
        <MobileNowPlaying />
      </MemoryRouter>,
    );
    const cover = screen.getByRole("group", { name: "Обложка: свайп для смены трека" });
    fireEvent.touchStart(cover, { touches: [{ clientX: 180, clientY: 420 }] });
    fireEvent.touchMove(cover, { touches: [{ clientX: 183, clientY: 270 }] });
    fireEvent.touchEnd(cover, { changedTouches: [{ clientX: 183, clientY: 270 }] });
    expect(useUiStore.getState()).toMatchObject({ nowPlayingOpen: true, nowPlayingView: "queue" });
    expect(usePlayerStore.getState().currentIndex).toBe(1);
    const back = () =>
      (window as Window & { __cryonAndroidBack?: () => boolean }).__cryonAndroidBack?.();
    act(() => {
      expect(back()).toBe(true);
    });
    expect(useUiStore.getState()).toMatchObject({ nowPlayingOpen: true, nowPlayingView: "cover" });
    act(() => {
      expect(back()).toBe(true);
    });
    expect(useUiStore.getState().nowPlayingOpen).toBe(false);
    mounted.unmount();
    delete window.CryonAndroid;
  });

  it("changes tracks when swiping the large cover", () => {
    vi.useFakeTimers();
    usePlayerStore.setState({
      queue,
      currentIndex: 1,
      isPlaying: true,
      progress: 40,
      repeat: "off",
      shuffle: false,
    });
    render(
      <MemoryRouter>
        <MobileNowPlaying />
      </MemoryRouter>,
    );
    const cover = screen.getByRole("group", { name: "Обложка: свайп для смены трека" });
    fireEvent.touchStart(cover, { touches: [{ clientX: 220, clientY: 300 }] });
    fireEvent.touchMove(cover, { touches: [{ clientX: 120, clientY: 305 }] });
    fireEvent.touchEnd(cover, { changedTouches: [{ clientX: 120, clientY: 305 }] });
    act(() => {
      vi.advanceTimersByTime(180);
    });
    expect(usePlayerStore.getState().currentTrack()?.id).toBe("third");
    act(() => {
      vi.advanceTimersByTime(300);
    });
    fireEvent.touchStart(cover, { touches: [{ clientX: 120, clientY: 300 }] });
    fireEvent.touchMove(cover, { touches: [{ clientX: 220, clientY: 305 }] });
    fireEvent.touchEnd(cover, { changedTouches: [{ clientX: 220, clientY: 305 }] });
    act(() => {
      vi.advanceTimersByTime(180);
    });
    expect(usePlayerStore.getState().currentTrack()?.id).toBe("second");
  });

  it("changes tracks when swiping the mini player cover", () => {
    vi.stubGlobal("PointerEvent", MouseEvent);
    usePlayerStore.setState({
      queue,
      currentIndex: 1,
      isPlaying: true,
      progress: 0,
      repeat: "off",
      shuffle: false,
    });
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
    class Pointer extends MouseEvent {
      pointerId = 1;
    }
    vi.stubGlobal("PointerEvent", Pointer);
    usePlayerStore.setState({ queue, currentIndex: 1, isPlaying: true, progress: 40 });
    const initial = usePlayerStore.getState().seekNonce;
    const view = render(
      <MemoryRouter>
        <MobileNowPlaying />
      </MemoryRouter>,
    );
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
