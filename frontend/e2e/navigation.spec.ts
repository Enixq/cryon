import { expect, test } from "@playwright/test";

test.beforeEach(async ({ page }) => {
  // В web fallback онбординг хранится в localStorage; отключаем его, чтобы
  // сценарии проверяли навигацию, а не приветственное окно.
  await page.addInitScript(() => localStorage.setItem("cryon.setting.ui.onboardingCompleted", "true"));
  await page.goto("/#/");
});

async function playSweaterWeather(page: import("@playwright/test").Page) {
  const search = page.getByPlaceholder("Начните вводить название трека, исполнителя, альбома или плейлист");
  await search.fill("sweater");
  await expect(page.getByRole("heading", { name: "Треки", exact: true })).toBeVisible();

  // Ограничиваемся TrackRow поисковой выдачи: одинаковый заголовок может быть
  // одновременно в панели «Сейчас играет» и в результате поиска.
  const firstTrack = page
    .getByRole("main")
    .getByText("Sweater Weather", { exact: true })
    .locator("xpath=ancestor::div[contains(@class, 'group grid')][1]");
  await firstTrack.hover();
  await firstTrack.getByRole("button", { name: "Воспроизвести" }).click();
  return { search, firstTrack };
}

test("главная открывается и навигация ведёт на основные рабочие экраны", async ({ page }) => {
  await expect(page.getByText(/Доброе|Добрый|Доброй/).first()).toBeVisible();

  const destinations = [
    ["Поиск", /^Поиск$/i],
    ["Библиотека", /Библиотека/i],
    ["Плейлисты", /Плейлисты/i],
    ["Избранное", /Избранное/i],
    ["История", /История/i],
  ] as const;

  for (const [label, heading] of destinations) {
    await page.getByRole("link", { name: label }).click();
    await expect(page.getByText(heading).first()).toBeVisible();
  }
});

test("сервисы и элементы нижней панели открывают настройки", async ({ page }) => {
  await page.getByRole("button", { name: "Настроить сервисы" }).click();
  await expect(page).toHaveURL(/#\/settings$/);

  await page.getByRole("link", { name: "Главная" }).click();
  await page.getByRole("button", { name: "Оформление" }).click();
  await expect(page).toHaveURL(/#\/settings$/);
});

test("базовый путь: поиск, воспроизведение, избранное и создание плейлиста", async ({ page }) => {
  await page.getByRole("link", { name: "Поиск" }).click();

  const { search, firstTrack } = await playSweaterWeather(page);
  await page.waitForTimeout(50);
  await expect(page.getByLabel("Пауза").first()).toBeVisible();

  await firstTrack.getByRole("button", { name: "Быстрые действия" }).click();
  const quickActions = page.locator("div.absolute.right-0.top-9");
  await quickActions.getByRole("button", { name: "В избранное" }).click();
  await page.getByRole("link", { name: "Избранное" }).click();
  await expect(page.getByText("Избранное").first()).toBeVisible();

  await page.getByRole("link", { name: "Поиск" }).click();
  await search.fill("sweater");
  await firstTrack.getByRole("button", { name: "Быстрые действия" }).click();
  await quickActions.getByRole("button", { name: "Добавить в плейлист" }).click();
  await page.getByPlaceholder("Новый плейлист").fill("E2E плейлист");
  await page.getByRole("button", { name: "Создать и добавить" }).click();

  await page.getByRole("link", { name: "Плейлисты" }).click();
  await expect(page.getByText("E2E плейлист")).toBeVisible();
});

test("воспроизведённый трек появляется в истории и историю можно очистить", async ({ page }) => {
  await page.getByRole("link", { name: "Поиск" }).click();
  await playSweaterWeather(page);
  await page.waitForTimeout(50);

  await page.getByRole("link", { name: "История" }).click();
  const history = page.getByRole("main");
  await expect(history.getByText("История прослушивания", { exact: true })).toBeVisible();
  await expect(history.getByRole("button", { name: "Очистить историю" })).toBeVisible();
  await expect(history.getByText("Sweater Weather", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Очистить историю" }).click();
  await expect(page.getByText("Здесь появятся треки, которые вы прослушаете.")).toBeVisible();
});