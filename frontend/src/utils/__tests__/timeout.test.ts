import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { setSafeTimeout } from "../timeout";

describe("long authentication timers", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.stubGlobal("window", globalThis);
  });
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });
  it("cancels the current segment after the first day", () => {
    const expired = vi.fn();
    const cancel = setSafeTimeout(expired, 3 * 86_400_000);
    vi.advanceTimersByTime(86_400_001);
    cancel();
    vi.advanceTimersByTime(5 * 86_400_000);
    expect(expired).not.toHaveBeenCalled();
  });
  it("allows renewal to replace a long timer without stale logout", () => {
    const oldLogout = vi.fn();
    const currentLogout = vi.fn();
    const cancel = setSafeTimeout(oldLogout, 2 * 86_400_000);
    vi.advanceTimersByTime(86_400_000);
    cancel();
    setSafeTimeout(currentLogout, 3 * 86_400_000);
    vi.advanceTimersByTime(2 * 86_400_000);
    expect(oldLogout).not.toHaveBeenCalled();
    expect(currentLogout).not.toHaveBeenCalled();
    vi.advanceTimersByTime(86_400_000);
    expect(currentLogout).toHaveBeenCalledOnce();
  });
});
