import { formatUploadSize, parseUploadSize } from "./upload-limits";

export const MAX_DOWNLOAD_RATE = 1024 ** 4;

export function parseDownloadRate(text: string): number | null {
  const bytes = parseUploadSize(text.trim().replace(/\/s$/i, ""));
  return bytes !== null && bytes <= MAX_DOWNLOAD_RATE ? bytes : null;
}

export function formatDownloadRate(rate: number): string {
  const formatted = formatUploadSize(rate);
  return `${parseDownloadRate(formatted) === rate ? formatted : `${rate} B`}/s`;
}
