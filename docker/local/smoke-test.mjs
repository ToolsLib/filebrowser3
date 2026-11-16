// Real Compose regressions for the default memory cache and optional Redis.
// Run after building the host binary: node docker/local/smoke-test.mjs
import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import { randomBytes } from "node:crypto";
import {
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  realpathSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "../..",
);
const build = path.join(root, "build");
mkdirSync(build, { recursive: true });
const fixture = mkdtempSync(path.join(build, "compose-smoke-"));
const suffix = `${process.pid}-${randomBytes(4).toString("hex")}`;
const image = `filebrowser-next:smoke-${suffix}`;
const projects = [];
const env = { ...process.env, FB_HTTP_PORT: "0", FB_USERNAME: "compose-smoke" };

function run(command, args, options = {}) {
  return execFileSync(command, args, {
    cwd: root,
    env,
    encoding: "utf8",
    windowsHide: true,
    timeout: 180_000,
    maxBuffer: 32 << 20,
    stdio: ["ignore", "pipe", "pipe"],
    ...options,
  }).trim();
}

function prepare(args) {
  return run(process.execPath, [
    path.join(root, "docker/local/prepare-secrets.mjs"),
    ...args,
  ]);
}

function inspect(id) {
  return JSON.parse(run("docker", ["inspect", id]))[0];
}

async function exercise(redis) {
  const mode = redis ? "redis" : "app-only";
  const directory = path.join(fixture, mode);
  mkdirSync(directory);
  const secrets = path.join(directory, "secrets");
  // The Redis case tests adding a secret without regenerating admin credentials.
  const prepared = prepare([secrets]);
  const password = prepared.match(
    /Initial admin password \(save it now\): ([a-f0-9]+)/,
  )?.[1];
  assert.ok(password, "secret helper did not return the fixture password");
  const hashFile = path.join(secrets, "filebrowser_password_hash");
  const originalHash = readFileSync(hashFile, "utf8");
  assert.equal(existsSync(path.join(secrets, "redis_password")), false);
  if (redis) {
    prepare(["--redis", secrets]);
    assert.equal(readFileSync(hashFile, "utf8"), originalHash);
    assert.match(
      readFileSync(path.join(secrets, "redis_password"), "utf8").trim(),
      /^[a-f0-9]{64}$/,
    );
    assert.throws(
      () => prepare(["--redis", secrets]),
      "existing Redis secret was overwritten",
    );
  } else {
    assert.throws(
      () => prepare([secrets]),
      "existing admin credentials were overwritten",
    );
  }

  const override = path.join(directory, "fixture.yaml");
  writeFileSync(
    override,
    `services:\n  filebrowser:\n    image: ${image}\nsecrets:\n  filebrowser_password_hash:\n    file: ${JSON.stringify(hashFile)}\n${redis ? `  redis_password:\n    file: ${JSON.stringify(path.join(secrets, "redis_password"))}\n` : ""}`,
  );
  const project = `fb-compose-smoke-${suffix}-${mode}`;
  const args = [
    "compose",
    "-p",
    project,
    "-f",
    path.join(root, "compose.yaml"),
  ];
  if (redis) args.push("-f", path.join(root, "compose.redis.yaml"));
  args.push("-f", override);
  const compose = (...command) => run("docker", [...args, ...command]);
  projects.push(compose);

  const config = JSON.parse(compose("config", "--format", "json"));
  assert.deepEqual(
    Object.keys(config.services).sort(),
    redis ? ["filebrowser", "redis"] : ["filebrowser"],
  );
  assert.deepEqual(
    config.services.filebrowser.secrets.map((secret) => secret.source).sort(),
    redis
      ? ["filebrowser_password_hash", "redis_password"]
      : ["filebrowser_password_hash"],
  );
  if (!redis) {
    assert.equal(config.services.filebrowser.depends_on, undefined);
    assert.equal(config.secrets.redis_password, undefined);
    assert.equal(config.volumes["redis-data"], undefined);
    assert.equal(
      config.services.filebrowser.environment.FB_REDIS_PASSWORD_FILE,
      undefined,
    );
  }
  console.log(`${mode}: starting isolated stack`);
  compose("up", "-d", "--wait", "--wait-timeout", "90");
  const app = inspect(compose("ps", "-q", "filebrowser"));
  assert.equal(app.Config.User, "1000:1000");
  assert.equal(app.HostConfig.ReadonlyRootfs, true);
  assert.ok(app.HostConfig.CapDrop.includes("ALL"));
  assert.ok(app.HostConfig.SecurityOpt.includes("no-new-privileges:true"));
  const binding = app.NetworkSettings.Ports["8080/tcp"][0];
  assert.equal(binding.HostIp, "127.0.0.1");
  let origin = `http://127.0.0.1:${binding.HostPort}`;

  async function request(route, status, options = {}) {
    const response = await fetch(new URL(route, origin), {
      ...options,
      signal: AbortSignal.timeout(15_000),
    });
    assert.equal(
      response.status,
      status,
      `${mode}: ${options.method || "GET"} ${route}`,
    );
    return response;
  }
  await request("/health", 200);
  const login = await request("/api/login", 200, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "X-Session-Transport": "header",
    },
    body: JSON.stringify({ username: "compose-smoke", password }),
  });
  const token = await login.text();
  const headers = { "X-Auth": token, "Tus-Resumable": "1.0.0" };
  await request("/api/resources/completed.txt", 200, {
    method: "POST",
    headers,
    body: "completed file",
  });
  const initial = await request("/api/raw/completed.txt", 200, { headers });
  assert.equal(await initial.text(), "completed file");

  async function createUpload() {
    const created = await request("/api/tus/pending.txt", 201, {
      method: "POST",
      headers: { ...headers, "Upload-Length": "5" },
    });
    const location = created.headers.get("Location");
    assert.ok(location?.includes("uploadID="));
    return location;
  }
  let location = await createUpload();
  await request(location, 204, {
    method: "PATCH",
    headers: {
      ...headers,
      "Upload-Offset": "0",
      "Content-Type": "application/offset+octet-stream",
    },
    body: "hel",
  });
  await request("/api/raw/pending.txt", 404, { headers });

  if (redis) {
    const id = compose("ps", "-q", "redis");
    const redisInfo = inspect(id);
    assert.equal(redisInfo.Config.User, "999:999");
    assert.equal(redisInfo.HostConfig.ReadonlyRootfs, true);
    assert.equal(
      Object.keys(redisInfo.HostConfig.PortBindings || {}).length,
      0,
    );
    const auth =
      'REDISCLI_AUTH="$(cat /run/secrets/redis_password)" redis-cli --user filebrowser';
    assert.match(
      run("docker", ["exec", id, "sh", "-c", `${auth} CONFIG GET dir`]),
      /NOPERM/,
    );
    assert.match(
      run("docker", ["exec", id, "sh", "-c", `${auth} GET other-keyspace`]),
      /NOPERM/,
    );
    assert.match(run("docker", ["exec", id, "redis-cli", "ping"]), /NOAUTH/);
  }

  // Stop/start preserves volumes and respects Redis health ordering.
  compose("stop");
  compose("up", "-d", "--wait", "--wait-timeout", "90");
  // Docker may assign a different ephemeral host port when a container starts.
  const restarted = inspect(compose("ps", "-q", "filebrowser"));
  origin = `http://127.0.0.1:${restarted.NetworkSettings.Ports["8080/tcp"][0].HostPort}`;
  const persisted = await request("/api/raw/completed.txt", 200, { headers });
  assert.equal(
    await persisted.text(),
    "completed file",
    "file or session lost after restart",
  );
  if (redis) {
    const head = await request(location, 200, { method: "HEAD", headers });
    assert.equal(head.headers.get("Upload-Offset"), "3");
    await request(location, 204, {
      method: "PATCH",
      headers: {
        ...headers,
        "Upload-Offset": "3",
        "Content-Type": "application/offset+octet-stream",
      },
      body: "lo",
    });
  } else {
    await request(location, 404, { method: "HEAD", headers });
    location = await createUpload();
    await request(location, 204, {
      method: "PATCH",
      headers: {
        ...headers,
        "Upload-Offset": "0",
        "Content-Type": "application/offset+octet-stream",
      },
      body: "hello",
    });
  }
  const finished = await request("/api/raw/pending.txt", 200, { headers });
  assert.equal(await finished.text(), "hello");
  console.log(
    `${mode}: passed health, login, isolation, direct/TUS uploads, durable files/sessions and ${redis ? "resumption plus Redis ACLs" : "safe restart of an unfinished upload"}`,
  );
}

let failed = false;
try {
  console.log(
    "Building Dockerfile.source (build output: build/compose-smoke-build.log)",
  );
  const built = spawnSync(
    "docker",
    ["build", "-f", "Dockerfile.source", "-t", image, "."],
    {
      cwd: root,
      env,
      encoding: "utf8",
      windowsHide: true,
      timeout: 600_000,
      maxBuffer: 32 << 20,
      stdio: ["ignore", "pipe", "pipe"],
    },
  );
  writeFileSync(
    path.join(build, "compose-smoke-build.log"),
    String(built.stdout || "") + String(built.stderr || ""),
  );
  if (built.error || built.status !== 0) {
    throw new Error(
      "Source image build failed; inspect build/compose-smoke-build.log",
    );
  }
  // Explicit Redis selection must fail closed if its file is unavailable.
  let missingSecretRejected = false;
  try {
    run("docker", [
      "run",
      "--rm",
      "-e",
      "FB_REDIS_PASSWORD_FILE=/missing/redis_password",
      image,
    ]);
  } catch (error) {
    missingSecretRejected =
      error.status === 1 &&
      String(error.stderr).includes("/missing/redis_password");
  }
  assert.ok(
    missingSecretRejected,
    "explicit Redis selection silently fell back without its secret",
  );
  // Also exercise the one-command fresh Redis secret setup.
  const fresh = path.join(fixture, "fresh-redis-secrets");
  prepare(["--redis", fresh]);
  assert.ok(existsSync(path.join(fresh, "redis_password")));
  assert.ok(existsSync(path.join(fresh, "filebrowser_password_hash")));
  await exercise(false);
  await exercise(true);
} catch (error) {
  failed = true;
  console.error(error.message);
} finally {
  let cleaned = true;
  for (const compose of projects.reverse()) {
    try {
      compose("down", "--volumes", "--remove-orphans");
    } catch {
      cleaned = false;
      failed = true;
      console.error(
        "Fixture cleanup failed; inspect Docker projects prefixed fb-compose-smoke-" +
          suffix,
      );
    }
  }
  try {
    run("docker", ["image", "rm", image]);
  } catch {
    /* Build may have failed before creating the image. */
  }
  if (cleaned) {
    const resolved = realpathSync(fixture);
    assert.equal(path.dirname(resolved), realpathSync(build));
    assert.ok(path.basename(resolved).startsWith("compose-smoke-"));
    rmSync(resolved, { recursive: true });
  }
}
if (failed) process.exit(1);
console.log(
  "Both Compose setups passed; disposable containers, volumes and secrets removed.",
);
