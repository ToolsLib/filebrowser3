import { fetchURL, removePrefix, StatusError } from "./utils";
import url from "../utils/url";

export default async function search(
  base: string,
  query: string,
  signal: AbortSignal,
  callback: (item: ResourceItem) => void,
  cursor = 0
) {
  let nextCursor = 0;
  const receive = (line: string) => {
    const item = JSON.parse(line);
    if (item.type === "page") {
      nextCursor = item.nextCursor;
      return;
    }
    item.url =
      `/files${base}` + url.encodePath(item.path) + (item.dir ? "/" : "");
    callback(item);
  };
  base = removePrefix(base);
  query = encodeURIComponent(query);

  if (!base.endsWith("/")) {
    base += "/";
  }

  const res = await fetchURL(
    `/api/search${base}?query=${query}&limit=200&cursor=${cursor}`,
    { signal }
  );
  if (!res.body) {
    throw new StatusError("000 No connection", 0);
  }
  try {
    // Try streaming approach first (modern browsers)
    if (res.body && typeof res.body.pipeThrough === "function") {
      const reader = res.body.pipeThrough(new TextDecoderStream()).getReader();
      let buffer = "";
      while (true) {
        const { done, value } = await reader.read();
        if (value) {
          buffer += value;
        }
        const lines = buffer.split(/\n/);
        let lastLine = lines.pop();
        // Save incomplete last line
        if (!lastLine) {
          lastLine = "";
        }
        buffer = lastLine;

        for (const line of lines) {
          if (line) {
            receive(line);
          }
        }
        if (done) break;
      }
    } else {
      // Fallback for browsers without streaming support (e.g., Safari)
      const text = await res.text();
      const lines = text.split(/\n/);
      for (const line of lines) {
        if (line) {
          receive(line);
        }
      }
    }
    return nextCursor;
  } catch (e) {
    // Check if the error is an intentional cancellation
    if (e instanceof Error && e.name === "AbortError") {
      throw new StatusError("000 No connection", 0, true);
    }
    throw e;
  }
}
