import { chromium } from "../../frontend/node_modules/@playwright/test/index.mjs";
import { mkdir, mkdtemp, writeFile, readFile } from "node:fs/promises";
import { spawn, execFileSync } from "node:child_process";
import path from "node:path";
import { gzipSync } from "node:zlib";
const parent = path.resolve("build/performance-browser");
await mkdir(parent, { recursive: true });
const screenshots = path.resolve("build/performance-screenshots");
await mkdir(screenshots, { recursive: true });
const work = await mkdtemp(path.join(parent, "run-"));
const root = path.join(work, "files");
await mkdir(root);
for (const count of [200, 2000]) {
  const dir = path.join(root, `folder-${count}`);
  await mkdir(dir);
  for (let start = 0; start < count; start += 100)
    await Promise.all(
      Array.from({ length: Math.min(100, count - start) }, (_, j) =>
        writeFile(
          path.join(dir, `item-${String(start + j).padStart(5, "0")}.txt`),
          "fixture",
        ),
      ),
    );
}
for (const count of [1000, 20000])
  await writeFile(
    path.join(root, `rows-${count}.csv`),
    "a,b,c,d,e,f,g,h\n" +
      Array.from({ length: count }, (_, i) => `${i},a,b,c,d,e,f,g\n`).join(""),
  );
await writeFile(
  path.join(root, "too-large.csv"),
  "a,b\n" + "x,y\n".repeat(1_500_000),
);
const binary = path.resolve(
  process.platform === "win32" ? "filebrowser.exe" : "filebrowser",
);
const password = "Performance-fixture-password-42!";
const hash = execFileSync(binary, ["hash", password], {
  encoding: "utf8",
}).trim();
const server = spawn(
  binary,
  [
    "--root",
    root,
    "--database",
    path.join(work, "test.db"),
    "--username",
    "perf",
    "--password",
    hash,
    "--port",
    "18091",
    "--insecureCookies",
  ],
  { stdio: "ignore", windowsHide: true },
);
let browser;
try {
  const base = "http://127.0.0.1:18091";
  for (let i = 0; i < 100; i++) {
    try {
      if ((await fetch(base + "/health")).ok) break;
    } catch {}
    await new Promise((r) => setTimeout(r, 100));
  }
  browser = await chromium.launch();
  const page = await browser.newPage({
    viewport: { width: 1440, height: 1000 },
  });
  const pageErrors = [];
  page.on("pageerror", (error) => pageErrors.push(error.message));
  const cdp = await page.context().newCDPSession(page);
  await cdp.send("Performance.enable");
  await page.goto(base + "/login");
  await page.getByLabel("Username", { exact: true }).fill("perf");
  await page.getByLabel("Password", { exact: true }).fill(password);
  await page.locator('input[type="submit"]').click();
  await page.waitForURL("**/files/**");
  const api = [];
  for (const count of [200, 2000])
    for (const paginated of [false, true]) {
      const samples = [];
      let bytes = 0,
        gzipBytes = 0,
        encoding = "";
      let lastWireBytes = 0;
      for (let n = 0; n < 3; n++) {
        const t = performance.now();
        const response = await page.request.get(
          base +
            `/api/resources/folder-${count}/` +
            (paginated ? "?limit=200" : ""),
          { headers: { "Accept-Encoding": "gzip, br" } },
        );
        const body = await response.body();
        samples.push(performance.now() - t);
        bytes = body.length;
        gzipBytes = gzipSync(body).length;
        encoding = response.headers()["content-encoding"] ?? "none";
        lastWireBytes = response.headers()["content-length"] || body.length;
        if (response.status() !== 200) throw Error("Listing failed");
      }
      api.push({
        items: count,
        paginated,
        wireBytes: Number(lastWireBytes),
        medianMs: samples.sort((a, b) => a - b)[1],
        bytes,
        gzipBytes,
        encoding,
      });
    }
  await page.goto(base + "/files/folder-2000/");
  await page.locator("#listing .item:not(.header)").first().waitFor();
  await page.locator("#loading").waitFor({ state: "detached" });
  if ((await page.locator("#listing .item:not(.header)").count()) > 200)
    throw Error("Listing DOM not bounded");
  await page.getByRole("button", { name: "Next", exact: true }).first().click();
  await page.waitForURL("**page=2");
  await page.locator('#listing .item[aria-label="item-00200.txt"]').waitFor();
  await page.locator('#listing .item[aria-label="item-00200.txt"]').dblclick();
  await page.locator("#editor").waitFor();
  await page.getByRole("button", { name: "Close", exact: true }).click();
  await page.waitForURL("**page=2");
  await page.locator('#listing .item[aria-label="item-00200.txt"]').waitFor();
  await page.screenshot({
    path: path.join(screenshots, "01-paginated-files.png"),
  });
  const initialRows = await page.locator("#listing .item:not(.header)").count();
  for (let n = 0; n < 18; n++) {
    await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
    await page.waitForTimeout(120);
  }
  const afterScrollRows = await page
    .locator("#listing .item:not(.header)")
    .count();
  const selectionStart = performance.now();
  await page.keyboard.press("Control+a");
  await page.evaluate(
    () =>
      new Promise((resolve) =>
        requestAnimationFrame(() => requestAnimationFrame(resolve)),
      ),
  );
  const selectAllMs = performance.now() - selectionStart;
  const selectedRenderedRows = await page
    .locator('#listing .item[aria-selected="true"]')
    .count();
  if (afterScrollRows > 200) throw Error("Scrolling grew listing DOM");
  const csv = [];
  await page.addInitScript(() => {
    window.perfLongTasks = [];
    new PerformanceObserver((list) => {
      for (const task of list.getEntries())
        window.perfLongTasks.push(task.duration);
    }).observe({ type: "longtask", buffered: true });
  });
  for (const count of [1000, 20000]) {
    const t = performance.now();
    await page.goto(base + `/files/rows-${count}.csv`);
    await page
      .locator(".csv-table tbody tr")
      .last()
      .waitFor({ timeout: 60000 });
    const metrics = (await cdp.send("Performance.getMetrics")).metrics;
    const rows = await page.locator(".csv-table tbody tr").count();
    if (rows > 100) throw Error("CSV DOM not bounded");
    csv.push({
      rows,
      sourceRows: count,
      inputBytes: (await readFile(path.join(root, `rows-${count}.csv`))).length,
      navigationToRowsMs: performance.now() - t,
      domNodes: await page.locator("*").count(),
      heapUsedBytes: metrics.find((x) => x.name === "JSHeapUsedSize")?.value,
      longTaskMs: await page.evaluate(() => window.perfLongTasks),
    });
    if (count === 20000) {
      await page.locator(".csv-table-container").evaluate((el) => {
        el.scrollTop = el.scrollHeight;
      });
      await page.screenshot({
        path: path.join(screenshots, "02-paginated-csv.png"),
      });
    }
  }
  const result = {
    environment: {
      platform: process.platform,
      node: process.version,
      browser: browser.version(),
    },
    api,
    listing: {
      initialRows,
      afterScrollRows,
      totalItems: 2000,
      selectAllMs,
      selectedRenderedRows,
    },
    csv,
  };
  await writeFile(
    "build/performance-browser-after.json",
    JSON.stringify(result, null, 2),
  );
  const oversizedResponse = page.waitForResponse((response) =>
    response.url().includes("/api/resources/too-large.csv?meta=true"),
  );
  await page.goto(base + "/files/too-large.csv");
  const response = await oversizedResponse;
  const oversizedBytes = (await response.body()).length;
  await page.waitForTimeout(1000);
  result.oversizedCsv = {
    downloadedBytes: oversizedBytes,
    status: response.status(),
    inputBytes: (await readFile(path.join(root, "too-large.csv"))).length,
    errorShown: await page.locator(".csv-error").count(),
    viewerText: (await page.locator("main").innerText()).slice(0, 220),
    pageErrors,
  };
  if (
    !result.oversizedCsv.errorShown ||
    oversizedBytes > 10000 ||
    pageErrors.length
  )
    throw Error("Preview regression: " + JSON.stringify(result.oversizedCsv));
  await writeFile(
    "build/performance-browser-after.json",
    JSON.stringify(result, null, 2),
  );
  console.log(JSON.stringify(result, null, 2));
} finally {
  await browser?.close();
  server.kill();
}
