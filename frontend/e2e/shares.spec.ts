import { expect, test, type Page } from "@playwright/test";

async function login(
  page: Page,
  username = "reviewer",
  password = "Browser-test-password-42!"
) {
  await page.goto("login");
  await page.locator('input[type="text"]').fill(username);
  await page.locator('input[type="password"]').fill(password);
  const response = page.waitForResponse(
    (res) => res.url().endsWith("/api/login") && res.status() === 200
  );
  await page.locator('input[type="submit"]').click();
  const token = await (await response).text();
  await expect(page).toHaveURL(/\/files\//);
  return { "X-Auth": token };
}

test("Shares sidebar edits password and duration without changing the public URL", async ({
  page,
}) => {
  const headers = await login(page);
  const created = await page.request.post("api/share/review.md", {
    headers,
    data: { password: "Original-share-pass", expires: "1", unit: "days" },
  });
  expect(created.ok()).toBe(true);
  const initial = await created.json();
  try {
    await page
      .locator("#workspace-navigation")
      .getByRole("button", { name: "Shares", exact: true })
      .click();
    await expect(page).toHaveURL(/\/shares$/);
    await expect(
      page
        .locator("#workspace-navigation")
        .getByRole("button", { name: "Shares", exact: true })
    ).toHaveAttribute("aria-current", "page");
    const row = page.locator(`[data-share-hash="${initial.hash}"]`);
    const url = await row.getByRole("link").getAttribute("href");
    await row.getByRole("button", { name: "Edit share", exact: true }).click();
    await expect(page.locator("#edit-share-password-mode")).toBeFocused();
    await page.screenshot({
      path: "test-results/shares-desktop.png",
      fullPage: true,
      animations: "disabled",
    });
    await page.locator("#edit-share-password-mode").selectOption("set");
    await page.locator("#edit-share-password").fill("Replacement+share-pass");
    await page.locator("#edit-share-expiration").selectOption("duration");
    await page.locator("#edit-share-duration").fill("2");
    await page.locator(".share-editor select.right").selectOption("hours");
    await page.route(
      `**/api/share/${initial.hash}`,
      (route) =>
        route.fulfill({ status: 503, body: "Temporary share update failure" }),
      { times: 1 }
    );
    const failed = page.waitForResponse(
      (res) =>
        res.url().endsWith(`/api/share/${initial.hash}`) &&
        res.request().method() === "PUT"
    );
    await page.locator("#save-share").click();
    expect((await failed).status()).toBe(503);
    await expect(page.locator(".share-editor")).toBeVisible();
    await expect(row.getByRole("link")).toHaveAttribute("href", url!);
    await expect(page.locator("#save-share")).toBeEnabled();
    const saved = page.waitForResponse(
      (res) =>
        res.url().endsWith(`/api/share/${initial.hash}`) &&
        res.request().method() === "PUT"
    );
    await page.locator("#save-share").click();
    const changed = await (await saved).json();
    expect(changed.hash).toBe(initial.hash);
    expect(changed.hasPassword).toBe(true);
    expect(changed.expire).toBeGreaterThan(
      Math.floor(Date.now() / 1000) + 7190
    );
    await expect(page.locator(".share-editor")).toHaveCount(0);
    await expect(row.getByRole("link")).toHaveAttribute("href", url!);
    expect(
      (
        await page.request.get(`api/public/dl/${initial.hash}`, {
          headers: { "X-SHARE-PASSWORD": "Original-share-pass" },
        })
      ).status()
    ).toBe(401);
    expect(
      (
        await page.request.get(`api/public/dl/${initial.hash}`, {
          headers: {
            "X-SHARE-PASSWORD": encodeURIComponent("Replacement+share-pass"),
          },
        })
      ).status()
    ).toBe(200);
    await page.reload();
    await expect(row).toContainText("Protected");
    await row.getByRole("button", { name: "Edit share", exact: true }).click();
    await page.locator("#edit-share-password-mode").selectOption("remove");
    await page.locator("#edit-share-expiration").selectOption("permanent");
    const cleared = page.waitForResponse(
      (res) =>
        res.url().endsWith(`/api/share/${initial.hash}`) &&
        res.request().method() === "PUT"
    );
    await page.locator("#save-share").click();
    expect((await (await cleared).json()).expire).toBe(0);
    await expect(row).toContainText("No password");
    await expect(row).toContainText("Permanent");
    expect(
      (await page.request.get(`api/public/dl/${initial.hash}`)).status()
    ).toBe(200);
    await page.goto("settings/profile");
    await expect(
      page
        .locator("#nav")
        .getByRole("link", { name: "Share Management", exact: true })
    ).toHaveCount(0);
    await page.goto("settings/shares");
    await expect(page).toHaveURL(/\/shares$/);
    await expect(row).toBeVisible();
    await page.setViewportSize({ width: 390, height: 844 });
    await page.locator("header .menu-button").click();
    await page
      .locator("#workspace-navigation")
      .getByRole("button", { name: "Shares", exact: true })
      .click();
    await expect(page.locator("#workspace-navigation")).not.toHaveClass(
      /active/
    );
    await expect(row).toBeVisible();
    await expect
      .poll(() =>
        page
          .locator("#workspace-navigation")
          .evaluate((element) => element.getBoundingClientRect().right)
      )
      .toBeLessThanOrEqual(1);
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth
      )
    ).toBe(true);
    await page.screenshot({
      path: "test-results/shares-mobile.png",
      fullPage: true,
      animations: "disabled",
    });
    await expect(page.locator(".workspace-heading p")).toBeInViewport();
    await row.getByRole("button", { name: "Edit share", exact: true }).click();
    await page.locator("#edit-share-password-mode").selectOption("set");
    await page.locator("#edit-share-expiration").selectOption("duration");
    await expect(page.locator("#edit-share-password")).toBeInViewport();
    expect(
      await page
        .locator(".share-editor")
        .evaluate(
          (element) => element.getBoundingClientRect().right <= innerWidth
        )
    ).toBe(true);
    await page.screenshot({
      path: "test-results/shares-mobile-edit.png",
      fullPage: true,
      animations: "disabled",
    });
  } finally {
    await page.request.delete(`api/share/${initial.hash}`, { headers });
  }
});

test("ordinary users manage their own shares and lose access when sharing is revoked", async ({
  page,
  browser,
  baseURL,
}) => {
  const headers = await login(page);
  const configuration = await (
    await page.request.get("api/settings", { headers })
  ).json();
  const data = {
    ...configuration.defaults,
    id: 0,
    username: "share-manager-user",
    password: "Share-manager-password-42!",
    scope: ".",
    perm: {
      ...configuration.defaults.perm,
      admin: false,
      share: true,
      download: true,
    },
  };
  const created = await page.request.post("api/users", {
    headers,
    data: {
      what: "user",
      which: [],
      current_password: "Browser-test-password-42!",
      data,
    },
  });
  expect(created.status()).toBe(201);
  const users = await (await page.request.get("api/users", { headers })).json();
  const user = users.find(
    (entry: { username: string }) => entry.username === data.username
  );
  const ownerShare = await (
    await page.request.post("api/share/review.md", { headers, data: {} })
  ).json();
  const context = await browser.newContext({ baseURL });
  let ownHash = "";
  try {
    const userPage = await context.newPage();
    const userHeaders = await login(userPage, data.username, data.password);
    const own = await (
      await userPage.request.post("api/share/review.md", {
        headers: userHeaders,
        data: {},
      })
    ).json();
    ownHash = own.hash;
    await userPage
      .locator("#workspace-navigation")
      .getByRole("button", { name: "Shares", exact: true })
      .click();
    await expect(
      userPage.locator(`[data-share-hash="${ownHash}"]`)
    ).toBeVisible();
    await expect(
      userPage.locator(`[data-share-hash="${ownerShare.hash}"]`)
    ).toHaveCount(0);
    expect(
      (
        await userPage.request.put(`api/share/${ownerShare.hash}`, {
          headers: userHeaders,
          data: { expires: "" },
        })
      ).status()
    ).toBe(403);
    const revoked = await page.request.put(`api/users/${user.id}`, {
      headers,
      data: {
        what: "user",
        which: ["Perm"],
        current_password: "Browser-test-password-42!",
        data: { ...user, perm: { ...user.perm, share: false } },
      },
    });
    expect(revoked.ok()).toBe(true);
    await userPage.reload();
    await expect(userPage).toHaveURL(/\/403$/);
    await userPage.goto("files/");
    await expect(
      userPage
        .locator("#workspace-navigation")
        .getByRole("button", { name: "Shares", exact: true })
    ).toHaveCount(0);
  } finally {
    await context.close();
    if (ownHash) await page.request.delete(`api/share/${ownHash}`, { headers });
    await page.request.delete(`api/share/${ownerShare.hash}`, { headers });
    await page.request.delete(`api/users/${user.id}`, {
      headers,
      data: { current_password: "Browser-test-password-42!" },
    });
  }
});
