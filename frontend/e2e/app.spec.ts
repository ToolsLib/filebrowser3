import { expect, test } from "@playwright/test";

test("production login, local Ace, save and lazy previews work under CSP", async ({
  page,
}) => {
  const errors: string[] = [];
  const violations: string[] = [];
  const externalScripts: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  page.on("console", (message) => {
    if (
      message.type() === "error" &&
      /Content Security Policy|Error initializing|Failed to fetch dynamically/.test(
        message.text()
      )
    )
      violations.push(message.text());
  });
  page.on("request", (request) => {
    if (
      ["script", "worker"].includes(request.resourceType()) &&
      !request.url().startsWith("http://127.0.0.1:18089/")
    )
      externalScripts.push(request.url());
  });
  await page.goto("login");
  await page.locator('input[type="text"]').fill("reviewer");
  await page
    .locator('input[type="password"]')
    .fill("Browser-test-password-42!");
  const loginResponse = page.waitForResponse(
    (response) =>
      response.url().endsWith("/api/login") && response.status() === 200
  );
  await page.locator('input[type="submit"]').click();
  const accessToken = await (await loginResponse).text();
  const cookies = await page.context().cookies();
  for (const name of ["fb_access", "fb_refresh"]) {
    const cookie = cookies.find((cookie) => cookie.name === name);
    expect(cookie?.httpOnly).toBe(true);
    expect(cookie?.sameSite).toBe("Strict");
    expect(cookie?.path.startsWith("/test-app/")).toBe(true);
  }
  expect(await page.evaluate(() => localStorage.getItem("jwt"))).toBeNull();
  expect(await page.evaluate(() => document.cookie)).not.toMatch(
    /fb_access|fb_refresh/
  );
  await expect(page).toHaveURL(/\/files\//);
  await page.goto("files/review.md");
  await expect(page.locator(".ace_editor")).toBeVisible();
  await expect(page.locator(".ace_editor")).toContainText("Review fixture");
  await page.locator(".ace_text-input").fill("# Saved in browser\n");
  await page.locator("#save-button").click();
  await page.reload();
  await expect(page.locator(".ace_editor")).toContainText("Saved in browser");
  await page.goto("files/review.csv");
  await expect(page.locator(".csv-viewer")).toContainText("42");
  await page.goto("files/review.png");
  await expect(page.locator(".image-ex-img")).toBeVisible();
  await expect
    .poll(() =>
      page
        .locator(".image-ex-img")
        .evaluate(
          (img: HTMLImageElement) => img.complete && img.naturalWidth > 0
        )
    )
    .toBe(true);
  await page.goto("files/review.epub");
  await expect(
    page.frameLocator(".epub-reader iframe").getByText("Offline EPUB chapter")
  ).toBeVisible();
  // Produce a real video fixture without a network download or codec tool.
  const video = await page.evaluate(async () => {
    const canvas = document.createElement("canvas");
    canvas.width = canvas.height = 64;
    canvas.getContext("2d")!.fillRect(0, 0, 64, 64);
    const stream = canvas.captureStream(10);
    const recorder = new MediaRecorder(stream, {
      mimeType: "video/webm;codecs=vp8",
    });
    const chunks: Blob[] = [];
    recorder.ondataavailable = (event) => chunks.push(event.data);
    const result = new Promise<number[]>((resolve) => {
      recorder.onstop = async () =>
        resolve(
          Array.from(new Uint8Array(await new Blob(chunks).arrayBuffer()))
        );
    });
    recorder.start();
    setTimeout(() => recorder.stop(), 300);
    return result;
  });
  const upload = await page.request.post("api/resources/review.webm", {
    headers: { "X-Auth": accessToken },
    data: Buffer.from(video),
  });
  expect(upload.ok()).toBe(true);
  await page.goto("files/review.webm");
  await expect(page.locator(".video-js")).toBeVisible();
  await expect
    .poll(() =>
      page
        .locator("video")
        .evaluate((video: HTMLVideoElement) => video.readyState)
    )
    .toBeGreaterThan(0);
  expect(errors).toEqual([]);
  expect(violations).toEqual([]);
  expect(externalScripts).toEqual([]);
  const otherLogin = await page.request.post("api/login", {
    headers: { "X-Session-Transport": "header" },
    data: { username: "reviewer", password: "Browser-test-password-42!" },
  });
  expect(otherLogin.ok()).toBe(true);
  const otherAccess = await otherLogin.text();
  await page.goto("settings/profile");
  await page.getByRole("button", { name: "Sign out of all sessions" }).click();
  await expect(page).toHaveURL(/\/login$/);
  for (const token of [accessToken, otherAccess]) {
    const revoked = await page.request.get("api/resources/", {
      headers: { "X-Auth": token },
    });
    expect(revoked.status()).toBe(401);
  }
});
