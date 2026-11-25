import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { SearchPage } from "./SearchPage";
import { useUiStore } from "../store/uiStore";
import { listPlaylists, search, searchRemotePlaylists } from "../shared/api/client";

vi.mock("../shared/api/client", () => ({ search: vi.fn(), searchRemotePlaylists: vi.fn(), listPlaylists: vi.fn().mockResolvedValue([]) }));
vi.mock("../store/playerStore", () => ({ useTrackListPlayer: () => ({ playTrack: vi.fn(), isPlaying: false, activeTrackId: null }) }));
vi.mock("../shared/ui/GenreCard", () => ({ GenreCard: () => null }));
vi.mock("../shared/ui/TrackRow", () => ({ TrackRow: () => null }));
vi.mock("../shared/ui/PlaylistCard", () => ({ PlaylistCard: () => null }));
vi.mock("../shared/ui/ArtistLink", () => ({ ArtistLink: () => null }));

beforeEach(() => {
  useUiStore.setState({ searchQuery: "" });
  vi.mocked(search).mockResolvedValue([]);
  vi.mocked(searchRemotePlaylists).mockResolvedValue([]);
  vi.mocked(listPlaylists).mockResolvedValue([]);
});
afterEach(() => { cleanup(); vi.clearAllMocks(); });

function openVKSearch() {
  render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } })}><SearchPage /></QueryClientProvider></MemoryRouter>);
  fireEvent.click(screen.getByRole("button", { name: "VK Музыка" }));
  return screen.getByRole("textbox");
}

it("debounces typing and excludes other services' playlists from a VK-only search", async () => {
  const input = openVKSearch();
  fireEvent.change(input, { target: { value: "S" } });
  fireEvent.change(input, { target: { value: "So" } });
  fireEvent.change(input, { target: { value: "Song" } });
  expect(search).not.toHaveBeenCalled();
  await waitFor(() => expect(search).toHaveBeenCalledOnce(), { timeout: 2000 });
  expect(vi.mocked(search).mock.calls[0].slice(0, 2)).toEqual(["vk", "Song"]);
  expect(searchRemotePlaylists).not.toHaveBeenCalled();
  expect(await screen.findByText("Найдено 0 результатов")).toBeTruthy();
});

it("shows the actual VK refusal instead of claiming no tracks were found", async () => {
  vi.mocked(search).mockRejectedValue(new Error("ВК ограничил запросы (код 9)"));
  fireEvent.change(openVKSearch(), { target: { value: "Song" } });
  expect((await screen.findByRole("alert", {}, { timeout: 2000 })).textContent).toContain("код 9");
  expect(screen.queryByText("Найдено 0 результатов")).toBeNull();
  vi.mocked(search).mockResolvedValue([]);
  fireEvent.click(screen.getByRole("button", { name: "Повторить поиск" }));
  expect(await screen.findByText("Найдено 0 результатов")).toBeTruthy();
  expect(search).toHaveBeenCalledTimes(2);
  expect(searchRemotePlaylists).not.toHaveBeenCalled();
});
