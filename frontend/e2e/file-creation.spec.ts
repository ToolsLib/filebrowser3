import { expect, test } from "@playwright/test";

test("New menu creates in the current folder with keyboard and mobile access", async ({
  page,
}) => {
  await page.goto("login");
  await page.getByLabel("Username", { exact: true }).fill("reviewer");
  await page
    .getByLabel("Password", { exact: true })
    .fill("Browser-test-password-42!");
  const loggedIn = page.waitForResponse(
    (res) => res.url().endsWith("/api/login") && res.status() === 200
  );
  await page.locator('input[type="submit"]').click();
  const headers = { "X-Auth": await (await loggedIn).text() };
  await page.goto("files/Documents/");
  const trigger = page
    .locator(".workspace-heading")
    .getByRole("button", { name: "New", exact: true });
  const menu = page.getByRole("menu", { name: "New", exact: true });
  const folder = page.getByRole("menuitem", {
    name: "New folder",
    exact: true,
  });
  const file = page.getByRole("menuitem", { name: "New file", exact: true });
  const createdPath = "Documents/Menu-created%20folder/";
  await expect(page.locator(".sidebar-create")).toHaveCount(0);
  await expect(trigger).toBeVisible();
  await expect(menu).toHaveCount(0);
  await trigger.press("ArrowDown");
  await expect(folder).toBeFocused();
  await page.keyboard.press("ArrowDown");
  await expect(file).toBeFocused();
  await page.keyboard.press("Home");
  await expect(folder).toBeFocused();
  await page.keyboard.press("End");
  await expect(file).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(trigger).toBeFocused();
  await expect(menu).toHaveCount(0);
  await trigger.press("ArrowUp");
  await expect(file).toBeFocused();
  await page.keyboard.press("Shift+Tab");
  await expect(menu).toHaveCount(0);
  await expect(trigger).toBeFocused();
  await trigger.press("ArrowDown");
  await page.keyboard.press("Tab");
  await expect(menu).toHaveCount(0);
  expect(
    await page.evaluate(() => document.activeElement !== document.body)
  ).toBe(true);
  await trigger.click();
  await trigger.click();
  await expect(menu).toHaveCount(0);
  await trigger.click();
  await page.getByRole("heading", { name: "Documents", exact: true }).click();
  await expect(menu).toHaveCount(0);
  try {
    await trigger.click();
    await folder.click();
    const dialog = page.getByRole("dialog");
    await dialog.locator("#focus-prompt").fill("Menu-created folder");
    await dialog.getByRole("button", { name: "Create", exact: true }).click();
    await expect(dialog).toHaveCount(0);
    await expect(trigger).toBeFocused();
    await page
      .locator('#listing .item[aria-label="Menu-created folder"]')
      .dblclick();
    await expect(page).toHaveURL(/Documents\/Menu-created%20folder\/$/);
    await expect(
      page.getByRole("heading", { name: "This folder is empty" })
    ).toBeVisible();
    await page.evaluate(() => (document.documentElement.className = "dark"));
    await trigger.click();
    await page.screenshot({
      path: "../build/new-menu-desktop.png",
      animations: "disabled",
    });
    await file.click();
    await dialog.locator("#focus-prompt").fill("menu-created.txt");
    await dialog.getByRole("button", { name: "Create", exact: true }).click();
    await expect(page).toHaveURL(
      /Documents\/Menu-created%20folder\/menu-created.txt$/
    );
    await expect(page.locator(".ace_editor")).toBeVisible();
    await expect(page.locator(".new-menu")).toHaveCount(0);
    const resource = await page.request.get(
      `api/resources/${createdPath}menu-created.txt`,
      { headers }
    );
    expect(resource.ok()).toBe(true);
    await page.goto(`files/${createdPath}`);
    await page.setViewportSize({ width: 320, height: 844 });
    await expect(trigger).toBeVisible();
    await trigger.click();
    const bounds = await menu.boundingBox();
    expect(bounds!.x).toBeGreaterThanOrEqual(0);
    expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(320);
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth
      )
    ).toBe(true);
    await page.screenshot({
      path: "../build/new-menu-mobile.png",
      animations: "disabled",
    });
    await page.keyboard.press("Escape");
    await expect(trigger).toBeFocused();
    await trigger.click();
    await folder.click();
    await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
    await expect(trigger).toBeFocused();
    await page.goto("settings/profile");
    await expect(menu).toHaveCount(0);
    await expect(page.locator(".new-menu")).toHaveCount(0);
  } finally {
    const removed = await page.request.delete(`api/resources/${createdPath}`, {
      headers,
    });
    expect([200, 204, 404]).toContain(removed.status());
  }
});
