import { expect, test, type Page } from "@playwright/test";
import { mkdir } from "node:fs/promises";
import path from "node:path";

async function capture(page: Page, options: { path: string }) {
  await expect(page.locator("#loading")).toHaveCount(0);
  await page.evaluate(() => document.fonts.ready);
  await page.screenshot(options);
}

test("minimal workspace: keyboard, dialogs, shares, responsive layouts and themes", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  const screenshots = path.resolve("../build/ui-screenshots");
  await mkdir(screenshots, { recursive: true });
  await page.setViewportSize({ width: 1440, height: 960 });
  await page.emulateMedia({ colorScheme: "light", reducedMotion: "reduce" });
  await page.goto("login");
  await expect(page.getByLabel("Username", { exact: true })).toBeVisible();
  await capture(page, { path: path.join(screenshots, "01-login.png") });
  await page.getByLabel("Username", { exact: true }).fill("reviewer");
  await page
    .getByLabel("Password", { exact: true })
    .fill("Browser-test-password-42!");
  await page.locator('input[type="submit"]').click();
  await expect(
    page.locator("#listing .item:not(.header)").first()
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "My files", exact: true })
  ).toBeVisible();
  await page.evaluate(() => document.fonts.ready);
  // Default view is retained for existing users, rather than overwriting preferences.
  while (
    !(await page.locator("#listing").getAttribute("class"))?.includes("mosaic")
  )
    await page.locator("#view-button").click();
  await capture(page, { path: path.join(screenshots, "02-files-grid.png") });

  const newFolder = page
    .locator(".sidebar")
    .getByRole("button", { name: "New folder", exact: true });
  for (let i = 0; i < 3; i++) {
    await newFolder.click();
    const dialog = page.getByRole("dialog");
    await expect(dialog).toBeVisible();
    await expect(dialog.locator("#focus-prompt")).toBeFocused();
    await page.keyboard.press("Shift+Tab");
    await expect(
      dialog.getByRole("button", { name: "Create", exact: true })
    ).toBeFocused();
    await page.keyboard.press("Tab");
    await expect(dialog.locator("#focus-prompt")).toBeFocused();
    if (i === 0)
      await capture(page, {
        path: path.join(screenshots, "03-new-folder.png"),
      });
    await page.keyboard.press("Escape");
    await expect(dialog).toHaveCount(0);
    await expect(newFolder).toBeFocused();
  }

  const file = page.locator('#listing .item[aria-label="Getting started.txt"]');
  await file.focus();
  await page.keyboard.press("Space");
  await expect(file).toHaveAttribute("aria-selected", "true");
  for (const width of [320, 768, 1024, 1280]) {
    await page.setViewportSize({ width, height: 960 });
    await expect
      .poll(
        () =>
          page.evaluate(() => ({
            width: innerWidth,
            scroll: document.documentElement.scrollWidth,
          })),
        { message: `Viewport ${width} must fit the UI` }
      )
      .toEqual({ width, scroll: width });
    await expect(page.locator("#upload-button")).toBeVisible();
  }
  await page.setViewportSize({ width: 1440, height: 960 });
  await page
    .locator(".workspace-header")
    .getByRole("button", { name: "Share", exact: true })
    .click();
  const duration = page.getByLabel("Share Duration", { exact: true });
  await expect(duration).toHaveAttribute("type", "number");
  await duration.fill("-1");
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Share", exact: true })
    .click();
  await expect(duration).toBeVisible();
  expect(
    await duration.evaluate(
      (input: HTMLInputElement) => input.validity.rangeUnderflow
    )
  ).toBe(true);
  await duration.fill("0");
  const created = page.waitForResponse(
    (response) =>
      response.url().includes("/api/share/Getting") &&
      response.request().method() === "POST"
  );
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Share", exact: true })
    .click();
  const share = await (await created).json();
  await expect(
    page
      .getByRole("dialog")
      .getByRole("button", { name: "Copy to clipboard", exact: true })
  ).toBeVisible();
  await page.keyboard.press("Escape");
  await page.goto(`share/${share.hash}`);
  await expect(page.locator(".shared-file h1")).toHaveText(
    "Getting started.txt"
  );
  const download = await page.request.get(`api/public/dl/${share.hash}`);
  expect(download.status()).toBe(200);
  expect(await download.text()).toBe("Welcome to your workspace.\n");
  await capture(page, {
    path: path.join(screenshots, "04-public-share.png"),
  });
  await page.getByText("Show QR code", { exact: true }).click();
  await expect(page.locator(".share-qr canvas, .share-qr svg")).toBeVisible();

  await page.goto("files/");
  while (
    !(await page.locator("#listing").getAttribute("class"))
      ?.split(" ")
      .includes("list")
  )
    await page.locator("#view-button").click();
  const sortName = page.getByRole("button", {
    name: "Sort by name",
    exact: true,
  });
  await sortName.focus();
  const sorted = page.waitForResponse(
    (r) =>
      r.url().includes("/api/users/") &&
      r.request().method() === "PUT" &&
      r.status() === 200
  );
  await page.keyboard.press("Enter");
  await sorted;
  await capture(page, { path: path.join(screenshots, "05-files-list.png") });
  await page.evaluate(() => (document.documentElement.className = "dark"));
  await capture(page, { path: path.join(screenshots, "06-files-dark.png") });
  await page.evaluate(() => (document.documentElement.className = "light"));

  await page.goto("settings/profile");
  await expect(page.locator("#aceTheme option").first()).toBeAttached();
  await expect(page.locator(".spinner")).toHaveCount(0);
  await capture(page, { path: path.join(screenshots, "07-settings.png") });
  await page.goto("settings/global");
  await expect(page.locator("#minimumPasswordLength")).toHaveAttribute(
    "type",
    "number"
  );
  await page.locator("#minimumPasswordLength").fill("0");
  expect(
    await page
      .locator("#minimumPasswordLength")
      .evaluate((input: HTMLInputElement) => input.validity.valid)
  ).toBe(false);

  // Enter opens a focused folder; search results remain keyboard-dismissable.
  await page.goto("files/");
  await page.locator('#listing .item[aria-label="Documents"]').focus();
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(/files\/Documents\/$/);
  await expect(
    page.getByRole("heading", { name: "Documents", exact: true })
  ).toBeVisible();
  await page.goto("files/");
  await page.getByRole("textbox", { name: "Search...", exact: true }).click();
  await page.locator("#search input").fill("Getting");
  await page.locator("#search input").press("Enter");
  await expect(page.locator("#search li").first()).toContainText(
    "Getting started.txt"
  );
  await capture(page, { path: path.join(screenshots, "12-search.png") });
  await page.keyboard.press("Escape");
  await expect(page.locator("#search")).not.toHaveClass(/active/);
  await page.goto("files/Empty%20folder/");
  await expect(
    page.getByRole("heading", { name: "This folder is empty" })
  ).toBeVisible();
  await expect(
    page.locator(".empty-state").getByRole("button", { name: "Upload" })
  ).toBeVisible();
  await capture(page, {
    path: path.join(screenshots, "08-empty-folder.png"),
  });

  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("files/");
  await expect(
    page.locator("#listing .item:not(.header)").first()
  ).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth
    )
  ).toBe(true);
  await capture(page, { path: path.join(screenshots, "09-mobile.png") });
  await page.getByRole("button", { name: "Toggle sidebar" }).click();
  await expect(page.locator(".sidebar")).toHaveClass(/active/);
  await capture(page, {
    path: path.join(screenshots, "10-mobile-navigation.png"),
  });
  await page
    .locator(".sidebar")
    .getByRole("button", { name: "My files", exact: true })
    .click();
  await expect(page.locator(".sidebar")).not.toHaveClass(/active/);
  await page.getByRole("button", { name: "More", exact: true }).click();
  await expect(page.locator("#dropdown")).toHaveClass(/active/);
  await expect(page.locator("#dropdown #view-button")).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.locator("#dropdown")).not.toHaveClass(/active/);

  await page.setViewportSize({ width: 1440, height: 960 });
  await page.goto("settings/profile");
  await page.locator(".card select").first().selectOption("he");
  await page.locator('input[name="submitProfile"]').click();
  await expect(page.locator("html")).toHaveAttribute("dir", "rtl");
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth
    )
  ).toBe(true);
  await capture(page, {
    path: path.join(screenshots, "11-settings-rtl.png"),
  });
  expect(errors).toEqual([]);
});

test("public file and folder shares work without a signed-in session", async ({
  page,
  request,
}) => {
  const login = await request.post("api/login", {
    headers: { "X-Session-Transport": "header" },
    data: { username: "reviewer", password: "Browser-test-password-42!" },
  });
  expect(login.ok()).toBe(true);
  const headers = { "X-Auth": await login.text() };
  const create = await request.post("api/share/Getting%20started.txt", {
    headers,
    data: {},
  });
  expect(create.ok()).toBe(true);
  const fileShare = await create.json();
  await page.setViewportSize({ width: 1440, height: 960 });
  await page.emulateMedia({ colorScheme: "light", reducedMotion: "reduce" });
  await page.goto(`share/${fileShare.hash}`);
  expect(
    (await page.context().cookies()).filter(
      (cookie) => cookie.name === "fb_access" || cookie.name === "fb_refresh"
    )
  ).toHaveLength(0);
  await expect(page.locator(".sidebar .account-action")).toHaveCount(0);
  await expect(page.locator(".sidebar")).toHaveCount(0);
  await expect(page.locator(".shared-file h1")).toHaveText(
    "Getting started.txt"
  );
  const download = await page.request.get(`api/public/dl/${fileShare.hash}`, {
    headers: { Range: "bytes=0-6" },
  });
  expect(download.status()).toBe(206);
  expect(await download.text()).toBe("Welcome");
  await capture(page, {
    path: path.resolve("../build/ui-screenshots/04-public-share.png"),
  });
  const folder = await request.post("api/share/Documents", {
    headers,
    data: {},
  });
  expect(folder.ok()).toBe(true);
  const folderShare = await folder.json();
  await page.goto(`share/${folderShare.hash}`);
  const child = page.locator('#listing .item[aria-label="Meeting notes.md"]');
  await expect(child).toBeVisible();
  await capture(page, {
    path: path.resolve("../build/ui-screenshots/13-shared-folder.png"),
  });
  await child.focus();
  await page.keyboard.press("Enter");
  await expect(page.locator(".shared-file h1")).toHaveText("Meeting notes.md");
  const childLink = await page
    .locator(".shared-file-actions a")
    .first()
    .getAttribute("href");
  expect((await page.request.get(childLink!)).status()).toBe(200);
});

test("dragging a file into a folder uses its explicit destination", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 1100 });
  await page.emulateMedia({ reducedMotion: "reduce" });
  const login = await page.request.post("api/login", {
    headers: { "X-Requested-With": "FileBrowser" },
    data: { username: "reviewer", password: "Browser-test-password-42!" },
  });
  expect(login.ok()).toBe(true);
  await page.goto("files/");
  await expect(page.locator("#loading")).toHaveCount(0);
  const source = page.locator(
    '#listing .item[aria-label="Résumé — design notes.txt"]'
  );
  const destination = page.locator('#listing .item[aria-label="Projects"]');
  await expect(source).toBeVisible();
  await source.dragTo(destination);
  await expect(source).toHaveCount(0);
  await destination.focus();
  await page.keyboard.press("Enter");
  await expect(
    page.locator('#listing .item[aria-label="Résumé — design notes.txt"]')
  ).toBeVisible();
});
