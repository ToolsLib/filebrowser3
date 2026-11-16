import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  workers: 1,
  timeout: 45_000,
  use: {
    baseURL: "http://127.0.0.1:18089/test-app/",
    locale: "en-US",
    trace: "retain-on-failure",
  },
  webServer: {
    command: "node e2e/server.mjs",
    url: "http://127.0.0.1:18089/test-app/health",
    timeout: 60_000,
    reuseExistingServer: false,
  },
});
