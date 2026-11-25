import { afterEach, beforeEach, expect, it, vi } from "vitest";
import command from "../commands";

const harness = vi.hoisted(() => ({ fetch: vi.fn() }));
vi.mock("../utils", () => ({
  fetchURL: harness.fetch,
  removePrefix: (path: string) => path.replace(/^\/files/, ""),
}));
const encoder = new TextEncoder();
const abort = new AbortController();

function response(chunks: Uint8Array[], cancel = vi.fn()) {
  return new Response(
    new ReadableStream({
      start(controller) {
        for (const chunk of chunks) controller.enqueue(chunk);
        controller.close();
      },
      cancel,
    }),
    { headers: { "Content-Type": "application/x-ndjson; charset=utf-8" } }
  );
}

beforeEach(() => vi.clearAllMocks());
afterEach(() => vi.unstubAllGlobals());

it("posts a command once and parses split UTF-8, coalesced events and a nonzero exit", async () => {
  const bytes = encoder.encode(
    '{"type":"start"}\n{"type":"output","data":"🌍 partial"}\n{"type":"heartbeat"}\n{"type":"output","data":" line\\n"}\n{"type":"exit","code":7}\n'
  );
  // Split every byte, including within the UTF-8 encoding and JSON tokens.
  harness.fetch.mockResolvedValue(
    response(Array.from(bytes, (b) => new Uint8Array([b])))
  );
  const output = vi.fn();
  await expect(
    command("/files/Docs/", "example --flag", output, abort.signal)
  ).resolves.toBe(7);
  expect(output.mock.calls.flat().join("")).toBe("🌍 partial line\n");
  expect(harness.fetch).toHaveBeenCalledExactlyOnceWith("/api/command/Docs/", {
    method: "POST",
    body: "example --flag",
    signal: abort.signal,
    headers: {
      "Content-Type": "text/plain; charset=utf-8",
      Accept: "application/x-ndjson",
    },
  });
});

it("accepts many events coalesced into a chunk larger than the individual event limit", async () => {
  const text =
    '{"type":"start"}\n' +
    '{"type":"heartbeat"}\n'.repeat(4000) +
    '{"type":"exit","code":0}\n';
  harness.fetch.mockResolvedValue(response([encoder.encode(text)]));
  await expect(
    command("/files/", "example", vi.fn(), abort.signal)
  ).resolves.toBe(0);
});

it.each([
  [
    '{"type":"start"}\n{"type":"error","data":"Command timed out."}\n',
    "Command timed out.",
  ],
  [
    '{"type":"start"}\n{"type":"output","data":"partial"}\n',
    "ended before completion",
  ],
  ['{"type":"start"}\n{"type":"exit","code":0}', "ended before completion"],
  ['{"type":"output","data":"early"}\n', "did not start correctly"],
  ['{"type":"start"}\n{"type":"start"}\n', "Invalid command stream event"],
  [
    '{"type":"start"}\n{"type":"exit","code":"0"}\n',
    "Invalid command stream event",
  ],
  ['{"type":"start"}\n' + "x".repeat(65537), "event is too large"],
  ['{"type":"start"}\n' + "x".repeat(65537) + "\n", "event is too large"],
])("rejects a broken stream without retrying: %s", async (text, message) => {
  harness.fetch.mockResolvedValue(response([encoder.encode(text)]));
  await expect(
    command("/files/", "example", vi.fn(), abort.signal)
  ).rejects.toThrow(message);
  expect(harness.fetch).toHaveBeenCalledTimes(1);
});

it("cancels the response reader on a protocol error", async () => {
  const cancelled = vi.fn();
  harness.fetch.mockResolvedValue(
    new Response(
      new ReadableStream({
        start(controller) {
          controller.enqueue(encoder.encode('{"type":"unexpected"}\n'));
        },
        cancel: cancelled,
      }),
      { headers: { "Content-Type": "application/x-ndjson" } }
    )
  );
  await expect(
    command("/files/", "example", vi.fn(), abort.signal)
  ).rejects.toThrow();
  expect(cancelled).toHaveBeenCalledTimes(1);
});

it("aborts while waiting for output and releases the reader", async () => {
  const controller = new AbortController();
  let body!: ReadableStream;
  harness.fetch.mockImplementation(async (_url, opts) => {
    body = new ReadableStream({
      start(stream) {
        stream.enqueue(encoder.encode('{"type":"start"}\n'));
        opts.signal.addEventListener("abort", () =>
          stream.error(new DOMException("Aborted", "AbortError"))
        );
      },
    });
    return new Response(body, {
      headers: { "Content-Type": "application/x-ndjson" },
    });
  });
  const result = command("/files/", "example", vi.fn(), controller.signal);
  await vi.waitFor(() => expect(body.locked).toBe(true));
  controller.abort();
  await expect(result).rejects.toMatchObject({ name: "AbortError" });
  expect(body.locked).toBe(false);
  expect(harness.fetch).toHaveBeenCalledTimes(1);
});

it("reports a proxy HTML response instead of treating it as command output", async () => {
  harness.fetch.mockResolvedValue(
    new Response("<html>proxy error</html>", {
      headers: { "Content-Type": "text/html" },
    })
  );
  await expect(
    command("/files/", "example", vi.fn(), abort.signal)
  ).rejects.toThrow("proxy configuration");
});

it("propagates authentication errors without retrying commands", async () => {
  harness.fetch.mockRejectedValue(new Error("401 Unauthorized"));
  await expect(
    command("/files/", "example", vi.fn(), abort.signal)
  ).rejects.toThrow("401");
  expect(harness.fetch).toHaveBeenCalledTimes(1);
});
