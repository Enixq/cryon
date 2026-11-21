import { fireEvent, render } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { Slider } from "./Slider";

class TestPointerEvent extends MouseEvent {
  pointerId: number;
  constructor(type: string, init: PointerEventInit) { super(type, init); this.pointerId = init.pointerId ?? 1; }
}
afterEach(() => vi.unstubAllGlobals());

it("keeps the drag preview while engine position changes and commits once", () => {
  vi.stubGlobal("PointerEvent", TestPointerEvent);
  const change = vi.fn(), commit = vi.fn();
  const props = { value: 10, max: 100, onChange: change, onCommit: commit, visibleThumb: true };
  const view = render(<Slider {...props} />);
  const slider = view.getByRole("slider");
  vi.spyOn(slider, "getBoundingClientRect").mockReturnValue({ left: 0, width: 100 } as DOMRect);
  expect(slider.style.touchAction).toBe("none");
  fireEvent.pointerDown(slider, { clientX: 10, pointerId: 1, button: 0 });
  fireEvent.pointerMove(slider, { clientX: 60, pointerId: 1 });
  view.rerender(<Slider {...props} value={11} />);
  expect(slider.getAttribute("aria-valuenow")).toBe("60");
  expect(commit).not.toHaveBeenCalled();
  fireEvent.pointerUp(slider, { clientX: 70, pointerId: 1 });
  expect(commit).toHaveBeenCalledTimes(1);
  expect(commit).toHaveBeenCalledWith(70);
  view.unmount();
});

it("cancels an interrupted touch without committing a seek", () => {
  vi.stubGlobal("PointerEvent", TestPointerEvent);
  const commit = vi.fn(), cancel = vi.fn();
  const view = render(<Slider value={10} max={100} onChange={() => {}} onCommit={commit} onCancel={cancel} />);
  const slider = view.getByRole("slider");
  vi.spyOn(slider, "getBoundingClientRect").mockReturnValue({ left: 0, width: 100 } as DOMRect);
  fireEvent.pointerDown(slider, { clientX: 60, pointerId: 2, button: 0 });
  fireEvent.pointerCancel(slider, { pointerId: 2 });
  expect(commit).not.toHaveBeenCalled();
  expect(cancel).toHaveBeenCalledOnce();
  expect(slider.getAttribute("aria-valuenow")).toBe("10");
  view.unmount();
});
