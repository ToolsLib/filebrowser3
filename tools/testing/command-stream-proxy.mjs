// Local integration check against actual Nginx and Traefik. Build the app first.
// Only generated fixtures and uniquely named temporary containers are changed.
import assert from "node:assert/strict";
import { execFileSync, spawn } from "node:child_process";
import { access, mkdir, mkdtemp, readFile, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { setTimeout as delay } from "node:timers/promises";

const root = fileURLToPath(new URL("../../", import.meta.url));
const scratch = path.join(root, "build", "command-proxy-tests");
await mkdir(scratch, { recursive: true });
const directory = await mkdtemp(path.join(scratch, "run-"));
const origin = "http://127.0.0.1:18090/test-app";
const prefix = `fb3-command-proxy-${process.pid}`;
const containers = [];
const docker = (...args) =>
  execFileSync("docker", args, {
    encoding: "utf8",
    windowsHide: true,
    stdio: ["ignore", "pipe", "pipe"],
  }).trim();
const fixtureName = `command-proxy-${process.pid}.json`;
const app = spawn(process.execPath, ["e2e/server.mjs"], {
  cwd: path.join(root, "frontend"),
  windowsHide: true,
  stdio: ["ignore", "ignore", "pipe"],
  env: {
    ...process.env,
    FB_E2E_PORT: "18090",
    FB_E2E_ADDRESS: "0.0.0.0",
    FB_E2E_FIXTURE: fixtureName,
    FB_E2E_RESULTS: directory,
  },
});
let appLog = "";
app.stderr.on("data", (chunk) => {
  appLog = (appLog + chunk).slice(-12000);
});
async function ready(url) {
  for (let i = 0; i < 150; i++) {
    try {
      if (
        (await fetch(`${url}/health`, { signal: AbortSignal.timeout(500) })).ok
      )
        return;
    } catch {}
    if (app.exitCode !== null) throw new Error(`Fixture exited: ${appLog}`);
    await delay(100);
  }
  throw new Error(`Fixture/proxy not ready: ${url}`);
}
async function json(url, token, opts = {}) {
  const response = await fetch(url, {
    ...opts,
    headers: {
      "Content-Type": "application/json",
      "X-Auth": token,
      "X-Session-Transport": "header",
    },
  });
  assert(
    response.ok,
    `API status ${response.status}: ${await response.clone().text()}`,
  );
  return response;
}
function startContainer(name, image, config, destination, ports, args = []) {
  const host =
    process.platform === "linux"
      ? ["--add-host", "host.docker.internal:host-gateway"]
      : [];
  docker(
    "run",
    "-d",
    "--name",
    name,
    ...host,
    ...ports.flatMap((port) => ["-p", `127.0.0.1:${port}:${port}`]),
    "--mount",
    `type=bind,source=${config},target=${destination},readonly`,
    image,
    ...args,
  );
  containers.push(name);
}
async function* events(response) {
  assert.equal(response.status, 200);
  assert.match(response.headers.get("content-type"), /application\/x-ndjson/);
  let pending = "";
  for await (const chunk of response.body.pipeThrough(
    new TextDecoderStream(),
  )) {
    pending += chunk;
    let boundary;
    while ((boundary = pending.indexOf("\n")) !== -1) {
      yield JSON.parse(pending.slice(0, boundary));
      pending = pending.slice(boundary + 1);
    }
  }
  assert.equal(pending, "");
}
try {
  await ready(origin);
  const fixture = JSON.parse(
    await readFile(path.join(directory, fixtureName), "utf8"),
  );
  const quote = (value) => JSON.stringify(value.replaceAll("\\", "/"));
  const command = (mode, marker) =>
    `${quote(fixture.node)} ${quote(fixture.helper)} ${mode} ${quote(marker)}`;
  const login = await json(`${origin}/api/login`, "", {
    method: "POST",
    body: JSON.stringify({
      username: "reviewer",
      password: "Browser-test-password-42!",
      recaptcha: "",
    }),
  });
  const token = await login.text();
  const user = await (await json(`${origin}/api/users/1`, token)).json();
  await json(`${origin}/api/users/1`, token, {
    method: "PUT",
    body: JSON.stringify({
      what: "user",
      which: ["Commands"],
      current_password: "Browser-test-password-42!",
      data: { ...user, commands: [fixture.node.replaceAll("\\", "/")] },
    }),
  });
  const nginxConfig = path.join(directory, "nginx.conf");
  await writeFile(
    nginxConfig,
    `events {}
http {
  access_log off;
  server { listen 18091;
    location / { proxy_pass http://host.docker.internal:18090; proxy_http_version 1.1; proxy_buffering on; proxy_read_timeout 30s; }
  }
  server { listen 18092;
    location / { proxy_pass http://host.docker.internal:18090; proxy_http_version 1.1; proxy_buffering off; proxy_ignore_headers X-Accel-Buffering; proxy_read_timeout 30s; }
  }
}
`,
  );
  startContainer(
    `${prefix}-nginx`,
    "nginx:stable-alpine@sha256:8eb2b5540eae779603cc3db84b3986a15ecb5308b54303146eee9837c137b51a",
    nginxConfig,
    "/etc/nginx/nginx.conf",
    [18091, 18092],
  );
  const traefikConfig = path.join(directory, "traefik.yaml");
  await writeFile(
    traefikConfig,
    `http:
  routers:
    app:
      rule: PathPrefix(\x60/\x60)
      entryPoints: [web]
      service: app
    buffered:
      rule: PathPrefix(\x60/\x60)
      entryPoints: [buffered]
      middlewares: [buffered]
      service: app
  middlewares:
    buffered:
      buffering:
        memResponseBodyBytes: 1048576
        maxResponseBodyBytes: 2097152
  services:
    app:
      loadBalancer:
        servers:
          - url: http://host.docker.internal:18090
`,
  );
  startContainer(
    `${prefix}-traefik`,
    process.env.FB_TEST_TRAEFIK_IMAGE || "traefik:v3.8.0",
    traefikConfig,
    "/etc/traefik/dynamic.yaml",
    [18093, 18094],
    [
      "--entrypoints.web.address=:18093",
      "--entrypoints.buffered.address=:18094",
      "--providers.file.filename=/etc/traefik/dynamic.yaml",
    ],
  );
  const targets = [
    { name: "nginx", port: 18091, streaming: true },
    { name: "nginx-explicit-streaming", port: 18092, streaming: true },
    { name: "traefik", port: 18093, streaming: true },
    { name: "traefik-forced-buffering", port: 18094, streaming: false },
  ];
  for (const target of targets) {
    const base = `http://127.0.0.1:${target.port}/test-app`;
    await ready(base);
    const marker = path.join(directory, `${target.name}-release`);
    let released = false;
    const release = target.streaming
      ? null
      : delay(1000).then(async () => {
          released = true;
          await writeFile(marker, "release");
        });
    let earlyOutput = false,
      code;
    const response = await fetch(`${base}/api/command/`, {
      method: "POST",
      headers: { "X-Auth": token, "Content-Type": "text/plain" },
      body: command("wait", marker),
      signal: AbortSignal.timeout(6000),
    });
    for await (const event of events(response)) {
      if (event.type === "output" && event.data.includes("first")) {
        earlyOutput = !released;
        if (target.streaming) {
          released = true;
          await writeFile(marker, "release");
        }
      }
      if (event.type === "exit") code = event.code;
    }
    await release;
    assert.equal(code, 0, `${target.name}: exit`);
    assert.equal(earlyOutput, target.streaming, `${target.name}: early output`);
    console.log(
      `${target.name}: ${earlyOutput ? "incremental output before completion" : "negative control confirmed forced buffering"}`,
    );
    if (!target.streaming) continue;
    const escaped = path.join(directory, `${target.name}-escaped`);
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), 6000);
    try {
      const running = await fetch(`${base}/api/command/`, {
        method: "POST",
        headers: { "X-Auth": token, "Content-Type": "text/plain" },
        body: command("tree", escaped),
        signal: controller.signal,
      });
      let childReady = false;
      for await (const event of events(running)) {
        if (event.type === "output" && event.data.includes("child-ready")) {
          childReady = true;
          break;
        }
      }
      assert(childReady, "child process did not start");
      controller.abort();
      await delay(1700);
      await assert.rejects(access(escaped));
      console.log(
        `${target.name}: client abort stopped command and descendant`,
      );
    } finally {
      clearTimeout(timer);
      controller.abort();
    }
  }
  console.log("Command proxy checks passed.");
} finally {
  try {
    for (const name of containers.reverse()) docker("rm", "-f", name);
  } finally {
    app.kill("SIGTERM");
  }
}
