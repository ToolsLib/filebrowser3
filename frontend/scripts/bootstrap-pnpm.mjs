import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import {
  mkdtempSync,
  readFileSync,
  rmSync,
  rmdirSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { pathToFileURL } from "node:url";

export function packageManagerPin(value) {
  const match = /^pnpm@(\d+\.\d+\.\d+)\+sha512\.([a-f0-9]{128})$/.exec(value);
  assert.ok(
    match && match[0] === value,
    "packageManager must pin an exact pnpm version and SHA-512"
  );
  return {
    version: match[1],
    integrity: match[2],
    url: `https://registry.npmjs.org/pnpm/-/pnpm-${match[1]}.tgz`,
  };
}

export function verifyArchive(archive, pin) {
  assert.equal(
    createHash("sha512").update(archive).digest("hex"),
    pin.integrity,
    "pnpm archive SHA-512 does not match package.json"
  );
}

async function bootstrap() {
  const args = process.argv.slice(2);
  assert.ok(
    args.length === 0 ||
      (args.length === 2 && args[0] === "--prefix" && args[1]),
    "Usage: npm run setup:pnpm -- [--prefix directory]"
  );
  const npmCLI = process.env.npm_execpath;
  assert.ok(npmCLI, "Run this bootstrap through npm run setup:pnpm");
  const manifest = JSON.parse(
    readFileSync(new URL("../package.json", import.meta.url), "utf8")
  );
  const pin = packageManagerPin(manifest.packageManager);
  const response = await fetch(pin.url, {
    signal: AbortSignal.timeout(60_000),
    redirect: "error",
  });
  assert.ok(response.ok, `pnpm download failed: HTTP ${response.status}`);
  const archive = Buffer.from(await response.arrayBuffer());
  verifyArchive(archive, pin);
  const directory = mkdtempSync(path.join(tmpdir(), "filebrowser3-pnpm-"));
  const filename = path.join(directory, "pnpm.tgz");
  try {
    writeFileSync(filename, archive, { mode: 0o600, flag: "wx" });
    execFileSync(
      process.execPath,
      [
        npmCLI,
        "install",
        "--global",
        "--ignore-scripts",
        "--no-audit",
        "--no-fund",
        ...args,
        filename,
      ],
      { stdio: "inherit", windowsHide: true }
    );
    const root = execFileSync(
      process.execPath,
      [npmCLI, "root", "--global", ...args],
      {
        encoding: "utf8",
        windowsHide: true,
      }
    ).trim();
    const version = execFileSync(
      process.execPath,
      [path.join(root, "pnpm", "bin", "pnpm.cjs"), "--version"],
      { encoding: "utf8", windowsHide: true }
    ).trim();
    assert.equal(
      version,
      pin.version,
      "Installed pnpm version differs from the pin"
    );
    console.log(`Installed pnpm ${version}; archive SHA-512 verified`);
  } finally {
    rmSync(filename, { force: true });
    rmdirSync(directory);
  }
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(process.argv[1]).href
) {
  await bootstrap();
}
