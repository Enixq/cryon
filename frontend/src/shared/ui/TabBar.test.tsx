import { useState } from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TabBar } from "./TabBar";

class TouchPointer extends MouseEvent {
  pointerType = "touch";
  isPrimary = true;
  pointerId = 1;
}
function Page() {
  const [tab, setTab] = useState("tracks");
  return (
    <div>
      <TabBar
        tabs={[
          { id: "tracks", label: "Треки" },
          { id: "albums", label: "Альбомы" },
          { id: "artists", label: "Исполнители" },
        ]}
        active={tab}
        onChange={setTab}
      />
      <div data-testid="content">Содержание</div>
      <input aria-label="Перемотка" type="range" />
    </div>
  );
}
function swipe(target: HTMLElement, dx: number, dy = 0) {
  fireEvent(target, new TouchPointer("pointerdown", { clientX: 200, clientY: 200, bubbles: true }));
  fireEvent(
    target,
    new TouchPointer("pointerup", {
      clientX: 200 + dx,
      clientY: 200 + dy,
      bubbles: true,
      cancelable: true,
    }),
  );
}
describe("page tab gestures", () => {
  afterEach(() => vi.unstubAllGlobals());
  it("switches from the page body and preserves vertical scrolling and sliders", () => {
    vi.stubGlobal("PointerEvent", TouchPointer);
    render(<Page />);
    swipe(screen.getByTestId("content"), -100, 5);
    expect(screen.getByRole("tab", { name: "Альбомы" }).getAttribute("aria-selected")).toBe("true");
    swipe(screen.getByTestId("content"), -100, 180);
    swipe(screen.getByRole("slider"), -100);
    expect(screen.getByRole("tab", { name: "Альбомы" }).getAttribute("aria-selected")).toBe("true");
    swipe(screen.getByTestId("content"), 100, 5);
    expect(screen.getByRole("tab", { name: "Треки" }).getAttribute("aria-selected")).toBe("true");
  });
});
