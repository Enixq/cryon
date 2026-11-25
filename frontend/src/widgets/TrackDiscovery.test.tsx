import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import { afterEach, expect, it, vi } from "vitest";
import { TrackDiscovery } from "./TrackDiscovery";
import { useUiStore } from "../store/uiStore";
import { djRadio, getDJStatus, resetDJSession } from "../shared/api/intelligence";
import { mapTrack } from "../shared/api/mappers";
import type { domain } from "../../wailsjs/go/models";

vi.mock("../shared/api/client", async (original) => ({
  ...(await original<typeof import("../shared/api/client")>()),
  listRelatedTracks: vi.fn(async () => []),
}));
vi.mock("../shared/ui/TrackRow", () => ({ TrackRow: () => null }));

afterEach(() => {
  cleanup();
  useUiStore.getState().openRadioPreview(null);
  resetDJSession();
  vi.unstubAllGlobals();
  delete (window as Window & { __CRYON_BASE__?: string }).__CRYON_BASE__;
});

it("opening another station preview preserves the playing AI radio session", async () => {
  (window as Window & { __CRYON_BASE__?: string }).__CRYON_BASE__ = "http://localhost:9999";
  const fetcher = vi.fn(async (url: string, init?: RequestInit) => {
    if (url.endsWith("/radio")) {
      const body = JSON.parse(init!.body as string);
      return new Response(
        JSON.stringify({ sessionId: body.sessionId || `session-${body.track.id}`, tracks: [] }),
      );
    }
    return new Response(JSON.stringify({ enabled: true, configured: true }));
  });
  vi.stubGlobal("fetch", fetcher);
  const active = mapTrack({
    id: "active",
    service: "soundcloud",
    title: "Active",
    artists: ["A"],
  } as domain.Track);
  const preview = mapTrack({
    id: "preview",
    service: "soundcloud",
    title: "Preview",
    artists: ["B"],
  } as domain.Track);
  await getDJStatus();
  await djRadio(active, active, [], 12);
  useUiStore.getState().openRadioPreview(preview);
  render(
    <MemoryRouter>
      <QueryClientProvider
        client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
      >
        <TrackDiscovery />
      </QueryClientProvider>
    </MemoryRouter>,
  );
  await waitFor(() =>
    expect(
      screen.getByText("Треки не найдены. Попробуйте другой источник или трек."),
    ).toBeInTheDocument(),
  );
  await djRadio(active, active, [], 12);
  const calls = fetcher.mock.calls.filter(([url]) => url.endsWith("/radio"));
  expect(calls).toHaveLength(2);
  expect(JSON.parse(calls[1][1]!.body as string).sessionId).toBe("session-active");
  expect(fetcher.mock.calls.some(([url]) => url.endsWith("/end"))).toBe(false);
});
