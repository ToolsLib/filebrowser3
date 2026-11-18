import { expect, test } from "@playwright/test";

test("download speed settings persist global, inherited, custom and unlimited policies", async ({
  page,
}) => {
  await page.goto("login");
  await page.locator('input[type="text"]').fill("reviewer");
  await page
    .locator('input[type="password"]')
    .fill("Browser-test-password-42!");
  const login = page.waitForResponse(
    (res) => res.url().endsWith("/api/login") && res.status() === 200
  );
  await page.locator('input[type="submit"]').click();
  const headers = { "X-Auth": await (await login).text() };
  await expect(page).toHaveURL(/\/files\//);
  const original = await (
    await page.request.get("api/settings", { headers })
  ).json();
  const user = await (
    await page.request.get("api/users/1", { headers })
  ).json();
  async function saveUser() {
    await page
      .locator('form:has(#user-download-rate-mode) input[type="submit"]')
      .click();
    await page.locator("#focus-prompt").fill("Browser-test-password-42!");
    const saved = page.waitForResponse(
      (res) =>
        res.url().endsWith("/api/users/1") && res.request().method() === "PUT"
    );
    await page.locator("#focus-prompt").press("Enter");
    expect((await saved).status()).toBe(200);
    await page.reload();
  }
  try {
    await page.goto("settings/global");
    await page.locator("#global-download-rate-mode").selectOption("limited");
    await page.locator("#global-download-rate").fill("8 KiB/s");
    await expect(
      page.getByText(/shared budget applies within one File Browser container/)
    ).toBeVisible();
    const saved = page.waitForResponse(
      (res) =>
        res.url().endsWith("/api/settings") && res.request().method() === "PUT"
    );
    await page
      .locator('form:has(#global-download-rate-mode) input[type="submit"]')
      .click();
    expect((await saved).status()).toBe(200);
    await page.reload();
    await expect(page.locator("#global-download-rate")).toHaveValue("8 KiB/s");
    await page.goto("settings/users/1");
    await expect(page.locator("#user-download-rate-mode")).toHaveValue(
      "inherit"
    );
    await expect(
      page.getByText(
        "Effective download speed: 8 KiB/s, shared across all transfers."
      )
    ).toBeVisible();
    await page.locator("#user-download-rate-mode").selectOption("limited");
    await page.locator("#user-download-rate").fill("4 KiB/s");
    await saveUser();
    await expect(page.locator("#user-download-rate")).toHaveValue("4 KiB/s");
    await page.locator("#user-download-rate-mode").selectOption("unlimited");
    await saveUser();
    await expect(page.locator("#user-download-rate-mode")).toHaveValue(
      "unlimited"
    );
    const updated = await (
      await page.request.get("api/users/1", { headers })
    ).json();
    expect(updated.downloadRate).toBe(0);
    await page.locator("#user-download-rate-mode").selectOption("inherit");
    await saveUser();
    await expect(page.locator("#user-download-rate-mode")).toHaveValue(
      "inherit"
    );
    expect(
      (await (await page.request.get("api/users/1", { headers })).json())
        .downloadRate
    ).toBeNull();
  } finally {
    await page.request.put("api/users/1", {
      headers,
      data: {
        what: "user",
        which: ["DownloadRate"],
        current_password: "Browser-test-password-42!",
        data: { ...user, downloadRate: user.downloadRate ?? null },
      },
    });
    await page.request.put("api/settings", { headers, data: original });
  }
});
