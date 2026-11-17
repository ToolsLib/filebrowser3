import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { test } from "node:test";
import { packageManagerPin, verifyArchive } from "./bootstrap-pnpm.mjs";

test("only an exact pnpm version with a full SHA-512 is accepted", () => {
  const hash = "a".repeat(128);
  const pin = packageManagerPin(`pnpm@10.33.4+sha512.${hash}`);
  assert.equal(pin.version, "10.33.4");
  assert.equal(pin.integrity, hash);
  assert.equal(pin.url, "https://registry.npmjs.org/pnpm/-/pnpm-10.33.4.tgz");
  for (const value of [
    "pnpm@latest",
    "pnpm@^10.33.4",
    "pnpm@10.33.4",
    `npm@10.33.4+sha512.${hash}`,
    "pnpm@10.33.4+sha512.abc",
    `pnpm@10.33.4+sha512.${hash}\n`,
  ]) {
    assert.throws(() => packageManagerPin(value));
  }
});

test("a changed archive fails verification before installation", () => {
  const archive = Buffer.from("fixture package archive");
  const pin = packageManagerPin(
    `pnpm@10.33.4+sha512.${createHash("sha512").update(archive).digest("hex")}`
  );
  assert.doesNotThrow(() => verifyArchive(archive, pin));
  assert.throws(() => verifyArchive(Buffer.from("tampered package"), pin));
});
