import "@testing-library/jest-dom/vitest";
import { act, fireEvent, render, screen, waitFor, cleanup } from "@testing-library/react";
import { QueryClient, QueryClientProvider, useQuery, focusManager } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import { afterEach, expect, it, vi } from "vitest";
import { IntelligencePanel } from "./IntelligencePanel";
import {
  getDJCollectionView,
  getDJStatus,
  refreshDJCollection,
  type DJCollectionView,
} from "../api/intelligence";

vi.mock("../api/intelligence", async (original) => ({
  ...(await original<typeof import("../api/intelligence")>()),
  getDJStatus: vi.fn(),
  getDJCollectionView: vi.fn(),
  refreshDJCollection: vi.fn(),
}));
vi.mock("../api/v1", () => ({ hasApiV1Runtime: () => true }));
afterEach(() => {
  cleanup();
  focusManager.setFocused(undefined);
  vi.clearAllMocks();
});

function Tracks() {
  const { data } = useQuery({
    queryKey: ["djCollection"],
    queryFn: ({ signal }) => getDJCollectionView(signal),
    staleTime: Infinity,
  });
  return <div data-testid="tracks">{data?.resultId}</div>;
}

it("updates the result and status from the same collection snapshot, retaining old result during restart and failure", async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  vi.mocked(getDJStatus).mockResolvedValue({
    enabled: true,
    configured: true,
    model: "mock",
    usedToday: 0,
    dailySegments: 10,
    tokensToday: 0,
  } as Awaited<ReturnType<typeof getDJStatus>>);
  let view: DJCollectionView = {
    tracks: [],
    jobId: "old",
    resultId: "old-result",
    updatedAt: new Date().toISOString(),
    job: {
      jobId: "old",
      resultId: "old-result",
      stage: "ready",
      found: 1,
      candidates: 1,
      selected: 1,
      searches: [],
    },
  };
  vi.mocked(getDJCollectionView).mockImplementation(async () => view);
  vi.mocked(refreshDJCollection).mockImplementation(async () => {
    view = { ...view, job: { ...view.job, jobId: "new", resultId: undefined, stage: "searching" } };
  });
  render(
    <MemoryRouter>
      <QueryClientProvider client={client}>
        <IntelligencePanel />
        <Tracks />
      </QueryClientProvider>
    </MemoryRouter>,
  );
  await screen.findByText("Cryon DJ · AI-подборка готова");
  fireEvent.click(screen.getByText("Подобрать музыку"));
  await screen.findByText("Cryon DJ · Инструменты DJ собирают кандидатов…");
  expect(screen.getByTestId("tracks")).toHaveTextContent("old-result");
  expect(screen.getByText(/Предыдущая подборка/)).toBeInTheDocument();
  view = {
    ...view,
    job: { ...view.job, stage: "failed", error: "Кандидаты не прошли пороги качества" },
  };
  await act(async () => {
    await client.invalidateQueries({ queryKey: ["djCollection"] });
  });
  expect(await screen.findByText(/Cryon DJ · Кандидаты не прошли/)).toBeInTheDocument();
  expect(screen.getByTestId("tracks")).toHaveTextContent("old-result");
  view = {
    ...view,
    jobId: "new",
    resultId: "new-result",
    job: { ...view.job, stage: "ready", resultId: "new-result", error: undefined },
  };
  await act(async () => {
    await client.invalidateQueries({ queryKey: ["djCollection"] });
  });
  await waitFor(() => expect(screen.getByTestId("tracks")).toHaveTextContent("new-result"));
  expect(screen.getByText("Cryon DJ · AI-подборка готова")).toBeInTheDocument();
  expect(screen.queryByText(/Предыдущая подборка/)).not.toBeInTheDocument();
  client.clear();
});

it("refreshes an unfinished DJ job on focus despite the application's disabled focus refresh", async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: 60000, refetchOnWindowFocus: false } },
  });
  vi.mocked(getDJStatus).mockResolvedValue({
    enabled: true,
    configured: true,
    model: "mock",
    usedToday: 0,
    dailySegments: 10,
    tokensToday: 0,
  } as Awaited<ReturnType<typeof getDJStatus>>);
  let view: DJCollectionView = {
    tracks: [],
    jobId: "",
    resultId: "",
    job: {
      jobId: "active",
      stage: "planning",
      requestDeadlineAt: new Date(Date.now() + 60000).toISOString(),
      found: 0,
      candidates: 0,
      selected: 0,
      searches: [],
    },
  };
  vi.mocked(getDJCollectionView).mockImplementation(async () => view);
  render(
    <MemoryRouter>
      <QueryClientProvider client={client}>
        <IntelligencePanel />
      </QueryClientProvider>
    </MemoryRouter>,
  );
  await screen.findByText("Cryon DJ · DJ определяет стратегию поиска…");
  expect(screen.getByText(/Поиск по музыкальным источникам ещё не начат/)).toBeInTheDocument();
  act(() => focusManager.setFocused(false));
  view = {
    ...view,
    job: { ...view.job, stage: "failed", error: "Превышено время ожидания ответа модели" },
  };
  act(() => focusManager.setFocused(true));
  expect(
    await screen.findByText("Cryon DJ · Превышено время ожидания ответа модели"),
  ).toBeInTheDocument();
  client.clear();
});
