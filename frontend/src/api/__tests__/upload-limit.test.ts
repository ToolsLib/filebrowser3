import { beforeEach, expect, it, vi } from "vitest";
import { post } from "../files";

const harness = vi.hoisted(() => ({ fetch: vi.fn(), upload: vi.fn() }));
vi.mock("@/api/utils", () => ({
  fetchURL: harness.fetch,
  removePrefix: (path: string) => path,
}));
vi.mock("@/api/tus", () => ({ upload: harness.upload, useTus: () => true }));
vi.mock("@/stores/auth", () => ({ useAuthStore: () => ({}) }));
vi.mock("@/stores/layout", () => ({ useLayoutStore: () => ({}) }));
vi.mock("@/utils/constants", () => ({ baseURL: "" }));
vi.mock("@/i18n", () => ({
  default: {
    global: { t: (_key: string, values: unknown) => JSON.stringify(values) },
  },
}));

beforeEach(() => {
  vi.clearAllMocks();
  harness.fetch.mockResolvedValue({ json: async () => ({ maxUploadSize: 5 }) });
  vi.stubGlobal("window", { location: { protocol: "https:" } });
});

it("rejects an oversized file before creating an upload", async () => {
  await expect(
    post("/files/large.iso", new File(["123456"], "large.iso"))
  ).rejects.toThrow("large.iso");
  expect(harness.upload).not.toHaveBeenCalled();
});

it("uses chunked uploads at the exact allowed limit", async () => {
  const file = new File(["12345"], "allowed.iso");
  await post("/files/allowed.iso", file);
  expect(harness.upload).toHaveBeenCalledWith(
    "/files/allowed.iso",
    file,
    false,
    expect.any(Function)
  );
});
