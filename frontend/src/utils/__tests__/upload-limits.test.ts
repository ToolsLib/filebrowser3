import { describe, expect, it, vi } from "vitest";
import {
  formatUploadSize,
  parseUploadSize,
  oversizedUploadError,
  uploadResponseError,
} from "../upload-limits";

vi.mock("@/i18n", () => ({
  default: {
    global: {
      t: (key: string, values: unknown) => `${key} ${JSON.stringify(values)}`,
    },
  },
}));

describe("upload limits", () => {
  it("accepts readable sizes and rejects invalid or unsafe values", () => {
    expect(parseUploadSize("100 GiB")).toBe(100 * 1024 ** 3);
    expect(parseUploadSize("1.5GB")).toBe(1.5 * 1024 ** 3);
    expect(parseUploadSize("0")).toBe(0);
    expect(parseUploadSize("1 PiB")).toBe(1024 ** 5);
    for (const value of [
      "",
      "-1",
      "Infinity",
      "1.1 B",
      "1 XB",
      "2 PiB",
      "100 GB garbage",
    ])
      expect(parseUploadSize(value)).toBeNull();
  });
  it("explains the filename, actual size and effective limit", () => {
    expect(formatUploadSize(24 * 1024 ** 3)).toBe("24 GiB");
    expect(
      oversizedUploadError("large.iso", 24 * 1024 ** 3, 10 * 1024 ** 3).message
    ).toContain('"name":"large.iso","size":"24 GiB","limit":"10 GiB"');
  });
  it("distinguishes structured policy failures from proxy errors", () => {
    expect(
      uploadResponseError(
        '{"code":"upload_too_large","maxUploadSize":10737418240}',
        413
      ).message
    ).toContain("10 GiB");
    expect(uploadResponseError("<html>413</html>", 413).message).toContain(
      "uploadRejectedTooLarge"
    );
  });
});
