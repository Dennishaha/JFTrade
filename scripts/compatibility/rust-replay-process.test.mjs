import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import { rustReplayInvocation } from "./rust-replay-process.mjs";

test("uses cargo run when no prebuilt replay directory is configured", () => {
  assert.deepEqual(rustReplayInvocation({
    root: "/repo",
    packageName: "jftrade-engine",
    binaryName: "jftrade-provider-runtime-replay",
    args: ["--input", "fixture.json"],
    env: {},
    platform: "linux",
  }), {
    command: "cargo",
    args: [
      "run", "--quiet", "-p", "jftrade-engine", "--bin",
      "jftrade-provider-runtime-replay", "--", "--input", "fixture.json",
    ],
  });
});

test("uses an existing prebuilt replay binary without invoking cargo", () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-replay-bin-"));
  try {
    const binaryDirectory = path.join(root, "target", "debug");
    fs.mkdirSync(binaryDirectory, { recursive: true });
    const binary = path.join(binaryDirectory, "jftrade-provider-runtime-replay");
    fs.writeFileSync(binary, "replay");
    assert.deepEqual(rustReplayInvocation({
      root,
      packageName: "jftrade-engine",
      binaryName: "jftrade-provider-runtime-replay",
      args: ["--input", "fixture.json"],
      env: { JFTRADE_COMPATIBILITY_BIN_DIR: "target/debug" },
      platform: "linux",
    }), {
      command: binary,
      args: ["--input", "fixture.json"],
    });
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("fails closed when the configured replay binary is missing", () => {
  assert.throws(() => rustReplayInvocation({
    root: "/repo",
    packageName: "jftrade-engine",
    binaryName: "jftrade-provider-runtime-replay",
    args: [],
    env: { JFTRADE_COMPATIBILITY_BIN_DIR: "target/debug" },
    platform: "linux",
  }), /prebuilt compatibility replay binary is missing/);
});
