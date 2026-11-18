import i18n from "@/i18n";

export const MAX_UPLOAD_SIZE = 1024 ** 5;
const units = ["B", "KiB", "MiB", "GiB", "TiB", "PiB"];

export function formatUploadSize(bytes: number): string {
  let unit = 0;
  while (bytes >= 1024 && unit < units.length - 1) {
    bytes /= 1024;
    unit++;
  }
  return `${Number(bytes.toFixed(2))} ${units[unit]}`;
}

export function parseUploadSize(text: string): number | null {
  const match = /^(\d+(?:\.\d+)?)\s*(B|[KMGTP]i?B|[KMGTP])?$/i.exec(
    text.trim()
  );
  if (!match) return null;
  const unit = (match[2] || "B").toUpperCase().replace("I", "");
  const power = unit === "B" ? 0 : "KMGTP".indexOf(unit[0]) + 1;
  const bytes = Number(match[1]) * 1024 ** power;
  return Number.isSafeInteger(bytes) && bytes >= 0 && bytes <= MAX_UPLOAD_SIZE
    ? bytes
    : null;
}

export function oversizedUploadError(
  name: string,
  size: number,
  limit: number
) {
  return new Error(
    i18n.global.t("errors.uploadTooLarge", {
      name,
      size: formatUploadSize(size),
      limit: formatUploadSize(limit),
    })
  );
}

export function uploadResponseError(body: string, status: number): Error {
  if (status !== 413) return new Error(body || `Upload failed (${status}).`);
  try {
    const data = JSON.parse(body);
    if (
      data.code === "upload_too_large" &&
      Number.isSafeInteger(data.maxUploadSize) &&
      data.maxUploadSize > 0
    ) {
      return new Error(
        i18n.global.t("errors.uploadLimitExceeded", {
          limit: formatUploadSize(data.maxUploadSize),
        })
      );
    }
  } catch {
    /* A reverse proxy may return HTML or plain text. */
  }
  return new Error(i18n.global.t("errors.uploadRejectedTooLarge"));
}
