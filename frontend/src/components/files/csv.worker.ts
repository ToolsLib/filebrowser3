import { parse } from "csv-parse/browser/esm";

// Preview limits bound result memory independently of the file's byte size.
const MAX_CELLS = 200_000,
  MAX_COLUMNS = 200,
  MAX_ROWS = 50_001;
self.onmessage = (
  event: MessageEvent<{
    content: ArrayBuffer | string;
    encoding: string;
    delimiter: string[];
  }>
) => {
  let cells = 0,
    truncated = false;
  const { content, encoding, delimiter } = event.data;
  try {
    const text =
      typeof content === "string"
        ? content
        : new TextDecoder(encoding).decode(content);
    parse(
      text,
      {
        delimiter,
        skip_empty_lines: true,
        relax_column_count: true,
        max_record_size: 262144,
        to: MAX_ROWS,
        on_record(record: string[]) {
          if (record.length > MAX_COLUMNS) truncated = true;
          const row = record.slice(0, MAX_COLUMNS);
          if (cells + row.length > MAX_CELLS) {
            truncated = true;
            return null;
          }
          cells += row.length;
          return row;
        },
      },
      (error, records: string[][]) => {
        if (error) self.postMessage({ error: error.message });
        else
          self.postMessage({
            headers: records[0] ?? [],
            rows: records.slice(1),
            truncated: truncated || records.length >= MAX_ROWS,
          });
      }
    );
  } catch (error) {
    self.postMessage({ error: String(error) });
  }
};
