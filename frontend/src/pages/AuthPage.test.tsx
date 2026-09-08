import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { AuthPage } from "./AuthPage";

const api = vi.hoisted(() => ({
  continueAsGuest: vi.fn(),
  loginAccount: vi.fn(),
  registerAccount: vi.fn(),
}));

vi.mock("../shared/api/client", () => api);

describe("AuthPage", () => {
  const onAuthenticated = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("выполняет вход и уведомляет родительский компонент", async () => {
    api.loginAccount.mockResolvedValue({ id: "1" });
    render(<AuthPage onAuthenticated={onAuthenticated} />);

    fireEvent.change(screen.getByLabelText("Логин"), { target: { value: "alex" } });
    fireEvent.change(screen.getByLabelText("Пароль"), { target: { value: "password1" } });
    fireEvent.click(screen.getByRole("button", { name: "Войти" }));

    await waitFor(() => expect(api.loginAccount).toHaveBeenCalledWith("alex", "password1"));
    expect(onAuthenticated).toHaveBeenCalledTimes(1);
  });

  it("показывает ошибку входа и не завершает авторизацию", async () => {
    api.loginAccount.mockRejectedValue(new Error("Неверный пароль"));
    render(<AuthPage onAuthenticated={onAuthenticated} />);

    fireEvent.change(screen.getByLabelText("Логин"), { target: { value: "alex" } });
    fireEvent.change(screen.getByLabelText("Пароль"), { target: { value: "password1" } });
    fireEvent.click(screen.getByRole("button", { name: "Войти" }));

    expect((await screen.findByRole("alert")).textContent).toContain("Неверный пароль");
    expect(onAuthenticated).not.toHaveBeenCalled();
  });

  it("переключается на регистрацию и передаёт все поля в API", async () => {
    api.registerAccount.mockResolvedValue({ id: "2" });
    render(<AuthPage onAuthenticated={onAuthenticated} />);

    fireEvent.click(screen.getByRole("button", { name: "Нет аккаунта? Создать" }));
    fireEvent.change(screen.getByLabelText("Email"), { target: { value: "alex@example.com" } });
    fireEvent.change(screen.getByLabelText("Логин"), { target: { value: "alex" } });
    fireEvent.change(screen.getByLabelText("Пароль"), { target: { value: "password1" } });
    fireEvent.click(screen.getByRole("button", { name: "Создать аккаунт" }));

    await waitFor(() => expect(api.registerAccount).toHaveBeenCalledWith("alex", "alex@example.com", "password1"));
    expect(onAuthenticated).toHaveBeenCalledTimes(1);
  });

  it("открывает гостевой режим", async () => {
    api.continueAsGuest.mockResolvedValue({ id: "guest", isGuest: true });
    render(<AuthPage onAuthenticated={onAuthenticated} />);

    fireEvent.click(screen.getByRole("button", { name: "Продолжить как гость" }));

    await waitFor(() => expect(api.continueAsGuest).toHaveBeenCalledOnce());
    expect(onAuthenticated).toHaveBeenCalledTimes(1);
  });
});