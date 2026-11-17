import assert from "node:assert/strict";
import { appendFileSync } from "node:fs";

export function releaseMetadata(tag, revision) {
  const match =
    /^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9]\d*|\d*[A-Za-z-][0-9A-Za-z-]*))*))?$/.exec(
      tag,
    );
  assert.ok(
    match,
    "Release tag must be a semantic vMAJOR.MINOR.PATCH tag, optionally with a prerelease",
  );
  assert.match(revision, /^[0-9a-f]{40}$/, "A full source commit is required");
  const version = tag.slice(1),
    prerelease = Boolean(match[4]),
    image = "toolslib/filebrowser3";
  return {
    version,
    revision,
    prerelease,
    image,
    candidate: `${image}:${version}`,
    aliases: prerelease
      ? []
      : [
          `${image}:${match[1]}.${match[2]}`,
          `${image}:${match[1]}`,
          `${image}:latest`,
        ],
  };
}

if (process.env.GITHUB_REF_NAME && process.env.GITHUB_OUTPUT) {
  const metadata = releaseMetadata(
    process.env.GITHUB_REF_NAME,
    process.env.GITHUB_SHA,
  );
  for (const key of ["version", "revision", "prerelease", "image", "candidate"])
    appendFileSync(process.env.GITHUB_OUTPUT, `${key}=${metadata[key]}\n`);
  console.log(JSON.stringify(metadata, null, 2));
}
