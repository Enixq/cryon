import "@testing-library/jest-dom/vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { RecoTrackCard } from "./RecoTrackCard";
import { useRecoFeedback } from "../lib/useRecoFeedback";
import type { Track } from "../types";

// Мокаем только сетевую часть клиента: recoFeedbackKey — чистая функция, её
// поведение (нижний регистр, «артист|название») обязано совпадать с Go, поэтому
// повторяем её в моке, а не подменяем заглушкой.
const api = vi.hoisted(() => ({
  recoFeedbackKey: (artist: string, title: string) =>
    `${artist.trim()}|${title.trim()}`.toLowerCase(),
  recoFeedbackState: vi.fn(),
  setRecoFeedback: vi.fn(),
  // «Нравится» на карточке кладёт трек в избранное, снятие лайка — убирает
  // (см. useRecoFeedback). Без этих экспортов в моке путь лайка падал с
  // «No addFavorite export is defined on the mock».
  addFavorite: vi.fn(),
  removeFavorite: vi.fn(),
}));

vi.mock("../api/client", () => api);

const track: Track = {
  id: "t1",
  title: "Nightfall",
  artist: "Aurora",
  source: "soundcloud",
  duration: 210,
  accent: "violet",
};

// Небольшая обёртка: карточка сама оценок не хранит, их держит хук.
function Harness() {
  const feedback = useRecoFeedback();
  return (
    <RecoTrackCard
      track={track}
      onPlay={() => {}}
      score={feedback.scoreFor(track)}
      onRate={feedback.rate}
      disabled={feedback.saving}
    />
  );
}

function renderCard() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    // В карточке есть ArtistLink (useNavigate) — нужен Router вокруг.
    <MemoryRouter>
      <QueryClientProvider client={client}>
        <Harness />
      </QueryClientProvider>
    </MemoryRouter>,
  );
}

describe("RecoTrackCard + useRecoFeedback", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.recoFeedbackState.mockResolvedValue({});
    // Избранное обязано вернуть промис: путь лайка вызывает .catch() на нём.
    api.addFavorite.mockResolvedValue(undefined);
    api.removeFavorite.mockResolvedValue(undefined);
  });

  it("отправляет оценку «нравится» и подсвечивает кнопку", async () => {
    api.setRecoFeedback.mockResolvedValue({ "aurora|nightfall": 1 });
    renderCard();

    fireEvent.click(screen.getByRole("button", { name: "Нравится — больше такого" }));

    await waitFor(() =>
      expect(api.setRecoFeedback).toHaveBeenCalledWith("Aurora", "Nightfall", 1),
    );
    // Лайк одновременно сохраняет трек в избранное («Мне нравится»).
    expect(api.addFavorite).toHaveBeenCalledWith(track);
    // Кнопка становится нажатой, а подпись — про снятие оценки.
    expect(await screen.findByRole("button", { name: "Убрать «нравится»" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
  });

  it("повторный клик по той же кнопке снимает оценку", async () => {
    api.recoFeedbackState.mockResolvedValue({ "aurora|nightfall": -1 });
    api.setRecoFeedback.mockResolvedValue({});
    renderCard();

    const off = await screen.findByRole("button", { name: "Убрать «не нравится»" });
    fireEvent.click(off);

    await waitFor(() =>
      expect(api.setRecoFeedback).toHaveBeenCalledWith("Aurora", "Nightfall", 0),
    );
  });

  it("не мешает воспроизведению: кнопка Play остаётся отдельным элементом", async () => {
    const onPlay = vi.fn();
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <MemoryRouter>
        <QueryClientProvider client={client}>
          <RecoTrackCard track={track} onPlay={onPlay} score={0} onRate={() => {}} />
        </QueryClientProvider>
      </MemoryRouter>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Воспроизвести Aurora — Nightfall" }));
    expect(onPlay).toHaveBeenCalledWith(track);
  });
});
