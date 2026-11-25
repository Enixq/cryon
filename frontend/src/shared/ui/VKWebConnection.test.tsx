import { fireEvent, render, screen, cleanup, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { VKWebConnection } from "./VKWebConnection";
import { request } from "../api/v1";

vi.mock("../api/v1", () => ({ hasApiV1Runtime: () => true, request: vi.fn() }));
vi.mock("../api/client", () => ({ openExternal: vi.fn() }));
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  delete window.CryonAndroid;
});

it("opens website login and only reports connected after the access check", async () => {
  let started = false;
  let polls = 0;
  const openVKLogin = vi.fn();
  window.CryonAndroid = { openVKLogin };
  vi.mocked(request).mockImplementation(async (_path, init) => {
    if (init?.method === "POST") {
      started = true;
      return { id: "attempt", state: "browser" } as never;
    }
    const state = ++polls > 2 ? "connected" : "checking";
    return {
      browserSupported: true,
      connected: started && state === "connected",
      session: started ? { id: "attempt", state } : undefined,
    } as never;
  });
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } })}
    >
      <VKWebConnection />
    </QueryClientProvider>,
  );
  fireEvent.click(await screen.findByRole("button", { name: "Войти через сайт ВК" }));
  await waitFor(() => expect(openVKLogin).toHaveBeenCalledWith("attempt"));
  await screen.findByText("Подключено", {}, { timeout: 3500 });
  expect(screen.queryByRole("textbox")).toBeNull();
  expect(screen.queryByLabelText("Пароль ВК")).toBeNull();
});

it("shows the failing VK stage", async () => {
  let started = false;
  vi.mocked(request).mockImplementation(async (_path, init) => {
    if (init?.method === "POST") {
      started = true;
      return { id: "attempt", state: "browser" } as never;
    }
    return {
      browserSupported: true,
      connected: false,
      session: started
        ? {
            id: "attempt",
            state: "error",
            error: "ВК: проверка доступа к музыке — ошибка API ВК, код 3.",
          }
        : undefined,
    } as never;
  });
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } })}
    >
      <VKWebConnection />
    </QueryClientProvider>,
  );
  fireEvent.click(await screen.findByRole("button", { name: "Войти через сайт ВК" }));
  expect((await screen.findByRole("alert")).textContent).toContain("код 3");
});

it("keeps login during flood control and lets the user open the VK website without reauthenticating", async () => {
  const openVKWebsite = vi.fn();
  window.CryonAndroid = { openVKWebsite };
  vi.mocked(request).mockResolvedValue({ connected: true, browserSupported: true, retryAfterSeconds: 900, limitMessage: "ВК ограничил запросы (код 9)" } as never);
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } })}><VKWebConnection /></QueryClientProvider>);
  expect(await screen.findByText("Подключено · запросы приостановлены")).toBeTruthy();
  expect((screen.getByRole("button", { name: "Сменить аккаунт" }) as HTMLButtonElement).disabled).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "Открыть сайт ВК" }));
  expect(openVKWebsite).toHaveBeenCalledOnce();
  expect(vi.mocked(request).mock.calls.some(([, init]) => init?.method === "POST")).toBe(false);
});
