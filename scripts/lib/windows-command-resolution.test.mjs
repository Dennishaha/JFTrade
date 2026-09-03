import assert from "node:assert/strict";
import test from "node:test";

import { resolveCommand } from "./spawn.mjs";

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
