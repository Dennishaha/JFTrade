import test from "node:test";
import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { validateAiContext } from "./check-ai-context.mjs";

test("AI context map covers existing paths and instruction files", () => {
  assert.deepEqual(validateAiContext(), []);
});

test("AI context validation rejects stale package references", (t) => {
  const root = mkdtempSync(join(tmpdir(), "jftrade-ai-context-"));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  mkdirSync(join(root, ".github", "instructions"), { recursive: true });
  writeFileSync(join(root, "AGENTS.md"), "Use pkg/jftradeapi for sidecars.\n");
  writeFileSync(join(root, ".github", "instructions", "current.md"), "Current instructions.\n");

  const errors = validateAiContext(root, {
    modules: [],
    requiredInstructionFiles: ["AGENTS.md"],
    legacyPaths: ["pkg/jftradeapi"],
  });
  assert.deepEqual(errors, ["AGENTS.md 仍引用已删除路径 pkg/jftradeapi"]);
});

test("AI context validation requires every controlled source to have an owner or explicit ignore", (t) => {
  const root = mkdtempSync(join(tmpdir(), "jftrade-ai-context-"));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  mkdirSync(join(root, "internal", "known"), { recursive: true });

  const errors = validateAiContext(root, {
    sourceRoots: ["internal"],
    sourceExtensions: [".go"],
    ignoredSourcePaths: ["internal/generated"],
    modules: [{ id: "known", paths: ["internal/known"] }],
    requiredInstructionFiles: [],
  }, {
    trackedFiles: [
      "internal/known/service.go",
      "internal/generated/wire.go",
      "internal/missed/service.go",
      "internal/missed/README.md",
    ],
  });

  assert.deepEqual(errors, ["源码未归属任何模块且未显式忽略 internal/missed/service.go"]);
});
