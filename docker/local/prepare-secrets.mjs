import { randomBytes } from "node:crypto";
import { execFileSync } from "node:child_process";
import {
  chmodSync,
  existsSync,
  lstatSync,
  mkdirSync,
  writeFileSync,
} from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "../..",
);
const args = process.argv.slice(2);
const withRedis = args.includes("--redis");
const imageIndex = args.indexOf("--image");
const image = imageIndex < 0 ? undefined : args[imageIndex + 1];
if (
  imageIndex >= 0 &&
  (!image || !/^[A-Za-z0-9][A-Za-z0-9._/:@-]*$/.test(image))
)
  throw new Error("--image requires a valid image reference");
const paths = args.filter(
  (arg, index) =>
    arg !== "--redis" &&
    (imageIndex < 0 || (index !== imageIndex && index !== imageIndex + 1)),
);
if (paths.length > 1 || paths.some((arg) => arg.startsWith("-"))) {
  throw new Error(
    "Usage: node docker/local/prepare-secrets.mjs [--redis] [--image image] [directory]",
  );
}
const destination = path.resolve(paths[0] || path.join(root, "secrets"));
const adminFile = path.join(destination, "filebrowser_password_hash");
const redisFile = path.join(destination, "redis_password");

function writeSecret(file, value) {
  // Compose file secrets preserve source permissions. The private parent
  // protects host access while both non-root container users can read mounts.
  writeFileSync(file, value + "\n", { mode: 0o644, flag: "wx" });
  if (process.platform !== "win32") chmodSync(file, 0o644);
}

if (existsSync(destination)) {
  const directory = lstatSync(destination);
  if (
    !directory.isDirectory() ||
    directory.isSymbolicLink() ||
    (process.platform !== "win32" && (directory.mode & 0o077) !== 0)
  ) {
    throw new Error(
      "Existing secrets directory must be private and must not be a symlink",
    );
  }
  if (
    !withRedis ||
    !existsSync(adminFile) ||
    !lstatSync(adminFile).isFile() ||
    existsSync(redisFile)
  ) {
    throw new Error(
      "Refusing to overwrite existing secrets; --redis can add a missing Redis secret to an existing admin setup",
    );
  }
  writeSecret(redisFile, randomBytes(32).toString("hex"));
  process.stdout.write(
    `Added Redis secret in ${destination}; existing admin credentials preserved\n`,
  );
  process.exit(0);
}
const binary = path.join(
  root,
  process.platform === "win32" ? "filebrowser.exe" : "filebrowser",
);
const password = randomBytes(32).toString("hex");
const command = image ? "docker" : binary;
const commandArgs = image
  ? [
      "run",
      "--rm",
      "-i",
      "--network",
      "none",
      "--entrypoint",
      "/bin/sh",
      image,
      "-c",
      'IFS= read -r password; exec /usr/local/bin/filebrowser hash "$password"',
    ]
  : ["hash", password];
const hash = execFileSync(command, commandArgs, {
  input: image ? password + "\n" : undefined,
  encoding: "utf8",
  windowsHide: true,
}).trim();
mkdirSync(destination, { mode: 0o700 });
writeSecret(adminFile, hash);
if (withRedis) writeSecret(redisFile, randomBytes(32).toString("hex"));
process.stdout.write(
  `Created private secrets in ${destination}\nInitial admin password (save it now): ${password}\n`,
);
