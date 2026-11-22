import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import { afterEach, expect, it, vi } from "vitest";
import { ShareTrackModal } from "./ShareTrackModal";
import { ReceiveTrackButton } from "../shared/ui/ReceiveTrackButton";
import { useUiStore } from "../store/uiStore";
import { usePlayerStore } from "../store/playerStore";
import { encodeTrackShare } from "../shared/lib/trackShare";
import { addFavorite } from "../shared/api/client";
import type { Track } from "../shared/types";

vi.mock("../shared/api/client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../shared/api/client")>()),
  addFavorite: vi.fn().mockResolvedValue(undefined),
  openExternal: vi.fn(),
}));
afterEach(() =>
  act(() => {
    useUiStore.setState({ shareOpen: false, shareTrack: null });
  }),
);

it("opens receiving without a current track and previews a Cryon link without playing", async () => {
  const client = new QueryClient();
  const invalidate = vi.spyOn(client, "invalidateQueries");
  const sample: Track = {
    id: "youtube:sample",
    source: "youtube",
    title: "Привет",
    artist: "Артист",
    duration: 180,
    accent: "violet",
  };
  usePlayerStore.setState({ queue: [], currentIndex: -1, isPlaying: false });
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <ReceiveTrackButton />
        <ShareTrackModal />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  fireEvent.click(screen.getByRole("button", { name: "Получить трек по ссылке" }));
  expect(screen.getByRole("dialog", { name: "Получить трек" })).toBeTruthy();
  fireEvent.change(screen.getByRole("textbox", { name: "Получить трек по ссылке" }), {
    target: { value: encodeTrackShare(sample) },
  });
  expect(screen.getByText("Привет")).toBeTruthy();
  expect(usePlayerStore.getState().queue).toHaveLength(0);
  await act(async () => {
    fireEvent.click(screen.getByRole("button", { name: "В избранное" }));
  });
  await waitFor(() => expect(addFavorite).toHaveBeenCalledWith(sample));
  expect(invalidate).toHaveBeenCalledWith({ queryKey: ["favorites"] });
  expect(usePlayerStore.getState().isPlaying).toBe(false);
});
