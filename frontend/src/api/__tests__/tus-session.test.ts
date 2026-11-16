import { describe, expect, it, vi } from "vitest";
import { upload } from "../tus";

const harness = vi.hoisted(() => ({
  options: null as any,
  store: { jwt: "first" },
  renew: vi.fn(),
}));
vi.mock("tus-js-client", () => ({
  Upload: class {
    constructor(_content: unknown, options: unknown) {
      harness.options = options;
    }
    start() {}
  },
}));
vi.mock("@/utils/constants", () => ({
  baseURL: "/app",
  tusEndpoint: "/api/tus",
  origin: "https://example.test",
  tusSettings: { chunkSize: 10, retryCount: 1 },
}));
vi.mock("@/stores/auth", () => ({ useAuthStore: () => harness.store }));
vi.mock("@/api/utils", () => ({ removePrefix: () => "/file" }));
vi.mock("@/utils/auth", () => ({ renew: harness.renew }));

describe("resumable upload sessions", () => {
  it("uses renewed credentials for subsequent chunks and consumes renewal hints", async () => {
    const complete = upload(
      "/files/file",
      new Blob(["content"]),
      false,
      undefined
    );
    const options = harness.options;
    expect(options.endpoint).toBe(
      "https://example.test/app/api/tus/file?override=false"
    );
    const request = { setHeader: vi.fn() };
    options.onBeforeRequest(request);
    expect(request.setHeader).toHaveBeenLastCalledWith("X-Auth", "first");
    harness.store.jwt = "renewed";
    options.onBeforeRequest(request);
    expect(request.setHeader).toHaveBeenLastCalledWith("X-Auth", "renewed");
    await options.onAfterResponse(request, { getHeader: () => "true" });
    expect(harness.renew).toHaveBeenCalledOnce();
    expect(
      options.onShouldRetry({ originalResponse: { getStatus: () => 401 } })
    ).toBe(false);
    options.onSuccess();
    await complete;
  });
});
