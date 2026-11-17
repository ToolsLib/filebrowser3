import { test } from "node:test";
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { releaseMetadata } from "./metadata.mjs";
const revision = "a".repeat(40);

test("imports do not execute the CLI in a GitHub branch workflow", () => {
  const moduleURL = new URL("./metadata.mjs", import.meta.url).href;
  const child = spawnSync(
    process.execPath,
    ["--input-type=module", "-e", `await import(${JSON.stringify(moduleURL)})`],
    {
      encoding: "utf8",
      env: {
        ...process.env,
        GITHUB_REF_NAME: "main",
        GITHUB_SHA: revision,
        GITHUB_OUTPUT: "unused-github-output",
      },
    },
  );
  assert.equal(child.status, 0, child.stderr);
  assert.equal(child.stdout, "");
});
test("release candidates never produce stable or latest aliases", () => {
  const metadata = releaseMetadata("v1.0.0-rc.1", revision);
  assert.equal(metadata.candidate, "toolslib/filebrowser3:1.0.0-rc.1");
  assert.equal(metadata.prerelease, true);
  assert.deepEqual(metadata.aliases, []);
});
test("stable aliases are derived from an exact release version", () => {
  assert.deepEqual(releaseMetadata("v1.2.3", revision).aliases, [
    "toolslib/filebrowser3:1.2",
    "toolslib/filebrowser3:1",
    "toolslib/filebrowser3:latest",
  ]);
});
test("invalid, mutable and shell-like references are rejected", () => {
  for (const tag of [
    "main",
    "latest",
    "1.0.0",
    "v01.0.0",
    "v1.0.0-01",
    "v1.0.0\nlatest",
    "v1.0.0;echo bad",
  ]) {
    assert.throws(() => releaseMetadata(tag, revision));
  }
  assert.throws(() => releaseMetadata("v1.0.0", "unknown"));
});
