import { expect, test } from "@playwright/test";

test("bounded pages, folder selection, CSV editing and deferred dependencies", async ({
  page,
}) => {
  const errors: string[] = [];
  const scripts: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  page.on("request", (request) => {
    if (request.resourceType() === "script") scripts.push(request.url());
  });
  await page.goto("login");
  await page.getByLabel("Username", { exact: true }).fill("reviewer");
  await page
    .getByLabel("Password", { exact: true })
    .fill("Browser-test-password-42!");
  await page.locator('input[type="submit"]').click();
  await expect(page).toHaveURL(/\/files\//);
  await page.goto("settings/profile");
  await expect(page.locator("#aceTheme option").first()).toBeAttached();
  expect(scripts.some((s) => /ext-themelist|ace-builds/.test(s))).toBe(false);
  await page.goto("files/Large%20folder/");
  await page.locator("#search input").click();
  await page.locator("#search input").fill("item-");
  await page.locator("#search input").press("Enter");
  await expect(page.locator("#result li")).toHaveCount(200);
  await page
    .locator("#result")
    .getByRole("button", { name: "Next", exact: true })
    .click();
  await expect(page.locator("#result li")).toHaveCount(50);
  await page.keyboard.press("Escape");

  const rows = page.locator("#listing .item:not(.header)");
  await expect(rows).toHaveCount(200);
  await page.getByRole("button", { name: "Next", exact: true }).first().click();
  await expect(page).toHaveURL(/page=2/);
  await expect(rows).toHaveCount(50);
  await page.locator('#listing .item[aria-label="item-200.txt"]').dblclick();
  await expect(page.locator(".ace_editor")).toBeVisible();
  expect(scripts.some((s) => /markdown-|UTIF-/.test(s))).toBe(false);
  await page.getByRole("button", { name: "Close", exact: true }).click();
  await expect(page).toHaveURL(/page=2/);
  await expect(rows).toHaveCount(50);
  await page
    .getByRole("button", { name: "Select entire folder", exact: true })
    .click();
  await expect(
    page.locator('#listing .item[aria-selected="true"]')
  ).toHaveCount(200);
  await page.getByRole("button", { name: "Next", exact: true }).first().click();
  await expect(
    page.locator('#listing .item[aria-selected="true"]')
  ).toHaveCount(50);
  await page.goto("files/performance-markdown.md");
  await expect(page.locator(".ace_editor")).toBeVisible();
  await page.getByRole("button", { name: "Preview", exact: true }).click();
  await expect(page.locator("#preview-container h1")).toHaveText(
    "Markdown fixture"
  );
  await expect(page.locator("#preview-container math")).toBeAttached();
  expect(await page.locator("#preview-container").innerHTML()).not.toContain(
    "onerror"
  );
  await page.goto("files/encoded-preview.csv");
  await expect(page.locator(".csv-table tbody tr")).toHaveCount(1);
  await page.locator(".selected-encoding").click();
  await page.locator('input[name="encoding"]').fill("windows-1252");
  await page.getByRole("button", { name: "windows-1252", exact: true }).click();
  await expect(page.locator(".csv-table")).toContainText("café");
  await page
    .locator("#columnSeparator")
    .selectOption({ label: "Semicolon (;)" });
  await expect(page.locator(".csv-table thead th")).toHaveCount(2);
  await page.goto("files/large-preview.csv");
  await expect(page.locator(".csv-table tbody tr")).toHaveCount(100);
  expect(await page.locator("*").count()).toBeLessThan(2000);
  await page
    .locator(".csv-viewer")
    .getByRole("button", { name: "Next", exact: true })
    .click();
  await expect(page.locator(".csv-table tbody tr").first()).toContainText(
    "100"
  );
  await page.goto("files/review.csv?edit=true");
  await expect(page.locator(".ace_editor")).toContainText("review,42");
  let rawRequests = 0;
  page.on("request", (request) => {
    if (
      request.url().includes("oversized-preview.csv") &&
      !request.url().includes("meta=true") &&
      request.url().includes("/api/resources")
    )
      rawRequests++;
  });
  await page.goto("files/oversized-preview.csv");
  await expect(page.locator(".csv-error")).toContainText("too large");
  expect(rawRequests).toBe(0);
  await page.goto("files/review.tiff");
  await expect
    .poll(() =>
      page
        .locator(".image-ex-img")
        .evaluate(
          (img: HTMLImageElement) => img.complete && img.naturalWidth > 0
        )
    )
    .toBe(true);
  expect(errors).toEqual([]);
});
