import { test } from "node:test";
import assert from "node:assert/strict";
import { finalizeRelease } from "./finalize.mjs";

const env = {
  GITHUB_REF_NAME: "v1.0.0",
  GITHUB_SHA: "a".repeat(40),
  GITHUB_REPOSITORY: "ToolsLib/filebrowser3",
  IMAGE_DIGEST: `sha256:${"b".repeat(64)}`,
};
const image = `toolslib/filebrowser3@${env.IMAGE_DIGEST}`;

test("stable release promotes aliases from the verified digest only after publication", () => {
  let uploaded = false,
    published = false;
  const promoted = [];
  const result = finalizeRelease(env, {
    write(filename, content) {
      assert.equal(filename, "image-digest.txt");
      assert.equal(content, image + "\n");
    },
    run(command, args) {
      if (command === "gh") {
        assert.equal(args[args.indexOf("--repo") + 1], "ToolsLib/filebrowser3");
        if (args[1] === "upload") uploaded = true;
        else {
          assert.ok(uploaded);
          assert.ok(args.includes("--draft=false"));
          assert.ok(args.includes("--prerelease=false"));
          assert.ok(args.includes("--latest=true"));
          published = true;
        }
      } else {
        assert.equal(command, "docker");
        assert.ok(
          published,
          "aliases must follow successful GitHub publication",
        );
        assert.deepEqual(args.slice(0, 3), ["buildx", "imagetools", "create"]);
        assert.equal(
          args.at(-1),
          image,
          "promote the existing digest without rebuilding",
        );
        for (let index = 3; index < args.length - 1; index += 2) {
          assert.equal(args[index], "--tag");
          promoted.push(args[index + 1]);
        }
      }
    },
  });
  assert.deepEqual(promoted, [
    "toolslib/filebrowser3:1.0",
    "toolslib/filebrowser3:1",
    "toolslib/filebrowser3:latest",
  ]);
  assert.deepEqual(result, { tag: "v1.0.0", image });
});

test("prereleases never publish stable aliases or become the latest GitHub release", () => {
  finalizeRelease(
    { ...env, GITHUB_REF_NAME: "v1.0.0-rc.4" },
    {
      write() {},
      run(command, args) {
        assert.equal(
          command,
          "gh",
          "prereleases must never invoke Docker alias promotion",
        );
        if (args[1] === "edit") {
          assert.ok(args.includes("--prerelease=true"));
          assert.ok(args.includes("--latest=false"));
        }
      },
    },
  );
});

test("failed digest upload or release publication blocks alias promotion", () => {
  for (const failingStep of ["upload", "edit"]) {
    let promoted = false;
    assert.throws(
      () =>
        finalizeRelease(env, {
          write() {},
          run(command, args) {
            if (command === "docker") promoted = true;
            if (command === "gh" && args[1] === failingStep)
              throw new Error("publication failed");
          },
        }),
      /publication failed/,
    );
    assert.equal(promoted, false);
  }
});

test("invalid release metadata cannot write files or publish anything", () => {
  for (const invalid of [
    { GITHUB_REPOSITORY: "other/repository" },
    { IMAGE_DIGEST: "latest" },
    { GITHUB_REF_NAME: "main" },
  ]) {
    let touched = false;
    assert.throws(() =>
      finalizeRelease(
        { ...env, ...invalid },
        {
          write() {
            touched = true;
          },
          run() {
            touched = true;
          },
        },
      ),
    );
    assert.equal(touched, false);
  }
});
