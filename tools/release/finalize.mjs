import { writeFileSync } from "node:fs";
import { execFileSync } from "node:child_process";
import { pathToFileURL } from "node:url";
import { releaseMetadata } from "./metadata.mjs";

export function finalizeRelease(
  env = process.env,
  { run = execFileSync, write = writeFileSync } = {},
) {
  const metadata = releaseMetadata(env.GITHUB_REF_NAME, env.GITHUB_SHA);
  if (env.GITHUB_REPOSITORY?.toLowerCase() !== "toolslib/filebrowser3")
    throw new Error("Unexpected release repository");
  if (!/^sha256:[a-f0-9]{64}$/.test(env.IMAGE_DIGEST ?? ""))
    throw new Error("Missing image digest");
  const immutableImage = `${metadata.image}@${env.IMAGE_DIGEST}`;
  write("image-digest.txt", immutableImage + "\n");
  run(
    "gh",
    [
      "release",
      "upload",
      env.GITHUB_REF_NAME,
      "image-digest.txt",
      "--clobber",
      "--repo",
      "ToolsLib/filebrowser3",
    ],
    { stdio: "inherit" },
  );
  run(
    "gh",
    [
      "release",
      "edit",
      env.GITHUB_REF_NAME,
      "--draft=false",
      `--prerelease=${metadata.prerelease}`,
      `--latest=${!metadata.prerelease}`,
      "--repo",
      "ToolsLib/filebrowser3",
    ],
    { stdio: "inherit" },
  );
  if (metadata.aliases.length) {
    const tags = metadata.aliases.flatMap((tag) => ["--tag", tag]);
    run("docker", ["buildx", "imagetools", "create", ...tags, immutableImage], {
      stdio: "inherit",
    });
  }
  return { tag: env.GITHUB_REF_NAME, image: immutableImage };
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(process.argv[1]).href
) {
  const published = finalizeRelease();
  console.log(`Published ${published.tag}: ${published.image}`);
}
