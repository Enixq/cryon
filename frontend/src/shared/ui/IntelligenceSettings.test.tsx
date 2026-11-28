import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { IntelligenceSettings } from "./IntelligenceSettings";
import {
  getDJSources,
  getDJStatus,
  saveDJSources,
  saveDJConfig,
  type DJSourcePolicy,
} from "../api/intelligence";

vi.mock("../api/intelligence", async (original) => ({
  ...(await original<typeof import("../api/intelligence")>()),
  getDJSources: vi.fn(),
  getDJStatus: vi.fn(),
  saveDJSources: vi.fn(),
  saveDJConfig: vi.fn(),
}));
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});
it("saves source enablement and order independently of the model key", async () => {
  vi.mocked(getDJStatus).mockResolvedValue({
    enabled: true,
    provider: "openai",
    model: "mock",
    baseUrl: "",
    configured: true,
    dailySegments: 100,
  } as Awaited<ReturnType<typeof getDJStatus>>);
  const sources: DJSourcePolicy = {
    enabled: { yandex: true, vk: true, youtube: true },
    priority: [],
    sources: ["yandex", "vk", "youtube"].map((source) => ({
      source,
      enabled: true,
      auth: source === "vk" ? "missing" : "not_required",
      state: source === "vk" ? "no_auth" : "ready",
      searchAttempted: false,
      queries: 0,
      found: 0,
      matched: 0,
      playable: 0,
      chosen: 0,
      rejected: 0,
      reasons: {},
      elapsedMs: 0,
    })),
  };
  vi.mocked(getDJSources).mockResolvedValue(sources);
  vi.mocked(saveDJSources).mockImplementation(async (value) => ({ ...sources, ...value }));
  render(<IntelligenceSettings />);
  fireEvent.click(await screen.findByLabelText("YouTube"));
  fireEvent.click(screen.getByLabelText("Выше: YouTube"));
  fireEvent.click(screen.getByText("Сохранить источники"));
  await waitFor(() =>
    expect(saveDJSources).toHaveBeenCalledWith(
      expect.objectContaining({
        enabled: { yandex: true, vk: true, youtube: false },
        priority: ["yandex", "youtube", "vk"],
      }),
    ),
  );
  expect(saveDJConfig).not.toHaveBeenCalled();
  expect(
    await screen.findByText("Источники сохранены. Новая подборка учтёт изменения."),
  ).toBeInTheDocument();
});
