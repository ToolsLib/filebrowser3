import { expect, test } from "@playwright/test";

test("upload settings honor deployment/user ceilings and reject large files before transferring", async ({
  page,
}) => {
  await page.goto("login");
  await page.locator('input[type="text"]').fill("reviewer");
  await page
    .locator('input[type="password"]')
    .fill("Browser-test-password-42!");
  const login = page.waitForResponse(
    (response) =>
      response.url().endsWith("/api/login") && response.status() === 200
  );
  await page.locator('input[type="submit"]').click();
  const token = await (await login).text();
  const headers = { "X-Auth": token };
  await expect(page).toHaveURL(/\/files\//);
  const original = await (
    await page.request.get("api/settings", { headers })
  ).json();
  const user = await (
    await page.request.get("api/users/1", { headers })
  ).json();
  try {
    await page.goto("settings/global");
    await expect(
      page.getByText(
        /FB_MAX_UPLOAD_SIZE enforces a deployment ceiling of 100 GiB/
      )
    ).toBeVisible();
    await page.locator("#global-upload-limit").fill("200 GiB");
    await expect(page.getByRole("status")).toContainText(
      "Effective global limit: 100 GiB"
    );
    const saved = page.waitForResponse(
      (response) =>
        response.url().endsWith("/api/settings") &&
        response.request().method() === "PUT"
    );
    await page
      .locator('form:has(#global-upload-limit) input[type="submit"]')
      .click();
    expect((await saved).status()).toBe(200);
    await page.reload();
    await expect(page.locator("#global-upload-limit")).toHaveValue("200 GiB");
    await expect(page.getByRole("status")).toContainText("100 GiB");

    // Configure small real chunks so the browser proves splitting and publication.
    const settings = {
      ...original,
      maxUploadSize: 64,
      tus: { ...original.tus, chunkSize: 3 },
    };
    expect(
      (await page.request.put("api/settings", { headers, data: settings })).ok()
    ).toBe(true);
    await page.goto("settings/users/1");
    await page.locator("#user-upload-limit").fill("8 B");
    await page
      .locator('form:has(#user-upload-limit) input[type="submit"]')
      .click();
    await page.locator("#focus-prompt").fill("Browser-test-password-42!");
    const userSaved = page.waitForResponse(
      (response) =>
        response.url().endsWith("/api/users/1") &&
        response.request().method() === "PUT"
    );
    await page.locator("#focus-prompt").press("Enter");
    expect((await userSaved).status()).toBe(200);
    await page.reload();
    await expect(page.locator("#user-upload-limit")).toHaveValue("8 B");
    await expect(
      page.getByText("Effective limit for this user: 8 B per file.")
    ).toBeVisible();
    await page.goto("files/");
    const requests: { method: string; offset?: string }[] = [];
    page.on("request", (request) => {
      if (request.url().includes("/api/tus/"))
        requests.push({
          method: request.method(),
          offset: request.headers()["upload-offset"],
        });
    });
    await page
      .locator("#upload-input")
      .first()
      .setInputFiles({
        name: "too-large.iso",
        mimeType: "application/octet-stream",
        buffer: Buffer.from("123456789"),
      });
    await expect(
      page.getByText(
        '"too-large.iso" is 9 B. Your maximum upload size is 8 B per file.'
      )
    ).toBeVisible();
    expect(requests).toHaveLength(0);
    const finished = page.waitForResponse(
      (response) =>
        response.url().includes("/api/tus/within-limit.bin") &&
        response.request().method() === "PATCH" &&
        response.headers()["upload-offset"] === "8"
    );
    await page
      .locator("#upload-input")
      .first()
      .setInputFiles({
        name: "within-limit.bin",
        mimeType: "application/octet-stream",
        buffer: Buffer.from("12345678"),
      });
    await finished;
    expect(
      requests
        .filter((request) => request.method === "PATCH")
        .map((request) => request.offset)
    ).toEqual(["0", "3", "6"]);
    expect(
      await (
        await page.request.get("api/raw/within-limit.bin", { headers })
      ).text()
    ).toBe("12345678");
  } finally {
    await page.request.put("api/users/1", {
      headers,
      data: {
        what: "user",
        which: ["MaxUploadSize"],
        current_password: "Browser-test-password-42!",
        data: { ...user, maxUploadSize: user.maxUploadSize || 0 },
      },
    });
    await page.request.put("api/settings", { headers, data: original });
  }
});
