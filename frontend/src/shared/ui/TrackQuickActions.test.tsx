import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { TrackQuickActions } from "./TrackQuickActions";
import type { Track } from "../types";

vi.mock("../lib/useFavorites", () => ({ useFavoriteIds: () => new Set() }));
vi.mock("./AddToPlaylistButton", () => ({ AddToPlaylistButton: () => <div>Плейлисты</div> }));

const track: Track = { id: "youtube:one", source: "youtube", title: "Название", artist: "Исполнитель", duration: 180, accent: "violet" };

describe("track actions menu", () => {
  it("opens above a button near the bottom and closes on Escape", () => {
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (this: HTMLElement) {
      const bounds = this.getAttribute("role") === "menu"
        ? { top: 0, left: 0, right: 224, bottom: 250, width: 224, height: 250 }
        : { top: 740, left: 360, right: 396, bottom: 776, width: 36, height: 36 };
      return { ...bounds, x: bounds.left, y: bounds.top, toJSON: () => ({}) };
    });
    Object.defineProperty(window, "innerHeight", { configurable: true, value: 800 });
    Object.defineProperty(window, "innerWidth", { configurable: true, value: 1280 });
    render(<TrackQuickActions track={track} />);
    fireEvent.click(screen.getByRole("button", { name: "Другие действия" }));
    expect(screen.getByRole("menu").style.top).toBe("484px");
    fireEvent.keyDown(document, { key: "Escape" });
    expect(screen.queryByRole("menu")).toBeNull();
    vi.restoreAllMocks();
  });
});
