import { fetchURL, removePrefix } from "./utils";

const maxEventLength = 64 * 1024;

// No retries: repeating a command after a broken stream could execute it twice.
export default async function command(
  url: string,
  value: string,
  onOutput: (text: string) => void,
  signal: AbortSignal
): Promise<number> {
  const response = await fetchURL(`/api/command${removePrefix(url)}`, {
    method: "POST",
    headers: {
      "Content-Type": "text/plain; charset=utf-8",
      Accept: "application/x-ndjson",
    },
    body: value,
    signal,
  });
  if (
    !response.headers.get("Content-Type")?.startsWith("application/x-ndjson") ||
    !response.body
  ) {
    await response.body?.cancel();
    throw new Error(
      "The server did not return a command stream. Check your proxy configuration."
    );
  }
  const reader = response.body.getReader();
  const decoder = new TextDecoder("utf-8", { fatal: true });
  let pending = "";
  let started = false;
  try {
    while (true) {
      const { done, value } = await reader.read();
      pending += decoder.decode(value, { stream: !done });
      let newline: number;
      while ((newline = pending.indexOf("\n")) !== -1) {
        const line = pending.slice(0, newline);
        pending = pending.slice(newline + 1);
        if (line.length > maxEventLength)
          throw new Error("Command stream event is too large.");
        const event = JSON.parse(line);
        if (!event || typeof event !== "object")
          throw new Error("Invalid command stream event.");
        if (event.type === "start" && !started) {
          started = true;
        } else if (!started) {
          throw new Error("Command stream did not start correctly.");
        } else if (event.type === "output" && typeof event.data === "string") {
          onOutput(event.data);
        } else if (event.type === "heartbeat") {
          // Keepalives carry no console output.
        } else if (event.type === "error" && typeof event.data === "string") {
          throw new Error(event.data);
        } else if (event.type === "exit" && Number.isInteger(event.code)) {
          return event.code;
        } else {
          throw new Error("Invalid command stream event.");
        }
      }
      if (pending.length > maxEventLength)
        throw new Error("Command stream event is too large.");
      if (done)
        throw new Error(
          "Command stream ended before completion. The command may have run; it was not retried."
        );
    }
  } finally {
    // Also cancel on protocol errors or a terminal event, so a broken/malicious
    // stream cannot leave the request and server command running in the background.
    await reader.cancel().catch(() => {});
    reader.releaseLock();
  }
}
