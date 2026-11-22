import { fireEvent, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { expect, it, vi } from "vitest";
import { VKConnection } from "./VKConnection";

const api = vi.hoisted(() => ({
  session: undefined as
    | undefined
    | { id: string; state: string; canResend: boolean; captchaImage?: string; method?: string },
  request: vi.fn(),
}));
vi.mock("../api/v1", () => ({ hasApiV1Runtime: () => true, request: api.request }));

it("accepts a VK image challenge and then prompts for an SMS inbox code in the same session", async () => {
  api.request.mockImplementation(async (_path, options) => {
    if (options?.method === "POST") {
      const input = JSON.parse(options.body);
      if (input.phone)
        api.session = {
          id: "fixture-session",
          state: "captcha",
          canResend: false,
          captchaImage: "https://api.vk.com/captcha.php?sid=fixture",
        };
      else {
        expect(input).toEqual({ id: "fixture-session", code: "abcde" });
        api.session = {
          id: "fixture-session",
          state: "code",
          canResend: false,
          method: "sms_inbox",
        };
      }
      return api.session;
    }
    return { connected: false, session: api.session };
  });
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <VKConnection />
    </QueryClientProvider>,
  );
  fireEvent.change(screen.getByRole("textbox", { name: "Номер телефона ВК" }), {
    target: { value: "+70000000000" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Войти в ВК Музыку" }));
  const answer = await screen.findByRole("textbox", { name: "Символы с картинки ВК" });
  expect(
    screen.getByRole("img", { name: "Проверка безопасности ВК" }).getAttribute("src"),
  ).toContain("https://api.vk.com/");
  fireEvent.change(answer, { target: { value: "abcde" } });
  fireEvent.click(screen.getByRole("button", { name: "Подтвердить" }));
  await screen.findByRole("textbox", { name: "Код подтверждения ВК" });
  expect(screen.getByText(/из SMS/)).toBeTruthy();
});
