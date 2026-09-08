import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import { resolveCommand, spawnChecked } from "./spawn.mjs";

test("Windows resolves pnpm through the command interpreter", () => {
  assert.deepEqual(
    resolveCommand("pnpm", ["run", "prepare:tauri-release"], {
      platform: "win32",
      environment: { ComSpec: "C:\\Windows\\System32\\cmd.exe" },
    }),
    {
      command: "C:\\Windows\\System32\\cmd.exe",
      args: ["/d", "/s", "/c", "pnpm run prepare:tauri-release"],
    },
  );
});

test("native executables bypass Windows command wrapping", () => {
  assert.deepEqual(
    resolveCommand("node.exe", ["tauri.js", "build"], {
      platform: "win32",
      environment: {},
    }),
    { command: "node.exe", args: ["tauri.js", "build"] },
  );
});

test("spawnChecked retries shebang-less script under /bin/sh on POSIX when kernel returns ENOEXEC", (t) => {
  if (process.platform === "win32") {
    t.skip("ENOEXEC fallback is specific to POSIX systems");
    return;
  }
  const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), "spawn-test-"));
  const scriptPath = path.join(tempDir, "shebangless.sh");
  fs.writeFileSync(scriptPath, 'echo "ok $1"', { mode: 0o755 });
  try {
    const status = spawnChecked(scriptPath, ["test-arg"], { stdio: "ignore" });
    assert.equal(status, 0);
  } finally {
    fs.rmSync(tempDir, { recursive: true, force: true });
  }
});

