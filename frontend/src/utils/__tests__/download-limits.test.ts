import { describe, expect, it, vi } from "vitest";
import { formatDownloadRate, parseDownloadRate } from "../download-limits";
vi.mock("@/i18n", () => ({ default: { global: { t: () => "" } } }));

describe("download speeds", () => {
  it("parses byte rates without confusing bits and bytes", () => {
    expect(parseDownloadRate("10 MiB/s")).toBe(10 * 1024 ** 2);
    expect(parseDownloadRate("500 KiB/s")).toBe(500 * 1024);
    expect(parseDownloadRate("1024")).toBe(1024);
    expect(parseDownloadRate("1 TiB/s")).toBe(1024 ** 4);
    for (const value of [
      "10 Mbps",
      "-1",
      "Infinity",
      "0.5 B/s",
      "2 TiB/s",
      "junk",
    ])
      expect(parseDownloadRate(value)).toBeNull();
  });
  it("preserves exact configured byte rates", () => {
    for (const rate of [1, 1024, 1025, 4097, 5 * 1024 ** 2, 1024 ** 4])
      expect(parseDownloadRate(formatDownloadRate(rate))).toBe(rate);
  });
});
