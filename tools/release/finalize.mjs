import { writeFileSync } from "node:fs";
import { execFileSync } from "node:child_process";
import { releaseMetadata } from "./metadata.mjs";

const metadata = releaseMetadata(
  process.env.GITHUB_REF_NAME,
  process.env.GITHUB_SHA,
);
if (process.env.GITHUB_REPOSITORY.toLowerCase() !== "toolslib/filebrowser3")
  throw new Error("Unexpected release repository");
if (!/^sha256:[a-f0-9]{64}$/.test(process.env.IMAGE_DIGEST ?? ""))
  throw new Error("Missing image digest");
const immutableImage = `${metadata.image}@${process.env.IMAGE_DIGEST}`;
writeFileSync("image-digest.txt", immutableImage + "\n");
execFileSync(
  "gh",
  [
    "release",
    "upload",
    process.env.GITHUB_REF_NAME,
    "image-digest.txt",
    "--clobber",
  ],
  { stdio: "inherit" },
);
execFileSync(
  "gh",
  [
    "release",
    "edit",
    process.env.GITHUB_REF_NAME,
    "--draft=false",
    `--prerelease=${metadata.prerelease}`,
    `--latest=${!metadata.prerelease}`,
  ],
  { stdio: "inherit" },
);
if (metadata.aliases.length) {
  const tags = metadata.aliases.flatMap((tag) => ["--tag", tag]);
  execFileSync(
    "docker",
    ["buildx", "imagetools", "create", ...tags, immutableImage],
    { stdio: "inherit" },
  );
}
console.log(`Published ${process.env.GITHUB_REF_NAME}: ${immutableImage}`);
