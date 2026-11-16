import { randomBytes } from "node:crypto";
import { execFileSync } from "node:child_process";
import { chmodSync, mkdirSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "../..",
);
const destination = path.resolve(process.argv[2] || path.join(root, "secrets"));
const binary = path.join(
  root,
  process.platform === "win32" ? "filebrowser.exe" : "filebrowser",
);
const password = randomBytes(32).toString("hex");
const hash = execFileSync(binary, ["hash", password], {
  encoding: "utf8",
  windowsHide: true,
}).trim();
// Refuse to overwrite any existing directory or credential files.
mkdirSync(destination, { mode: 0o700 });
// The private parent protects host access. File-based Compose secrets preserve
// source permissions, so files must be readable by the two non-root containers.
writeFileSync(
  path.join(destination, "redis_password"),
  randomBytes(32).toString("hex") + "\n",
  { mode: 0o644, flag: "wx" },
);
writeFileSync(
  path.join(destination, "filebrowser_password_hash"),
  hash + "\n",
  { mode: 0o644, flag: "wx" },
);
if (process.platform !== "win32") {
  chmodSync(destination, 0o700);
  chmodSync(path.join(destination, "redis_password"), 0o644);
  chmodSync(path.join(destination, "filebrowser_password_hash"), 0o644);
}
process.stdout.write(
  `Created private secrets in ${destination}\nInitial admin password (save it now): ${password}\n`,
);
