import assert from "node:assert/strict";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import test from "node:test";

import {
  checkUntrackedWhitespace,
  diffCheckArgs,
  untrackedDiffArgs,
} from "./check-diff.mjs";

test("checks the complete working projection against the resolved merge-base", () => {
  assert.deepEqual(diffCheckArgs("merge-base-commit"), ["diff", "--check", "merge-base-commit"]);
  assert.deepEqual(untrackedDiffArgs("/tmp/new-file.ts", "darwin"), [
    "diff",
    "--no-index",
    "--check",
    "/dev/null",
    "/tmp/new-file.ts",
  ]);
});

test("checks untracked text files without touching the Git index", async (t) => {
  const root = await mkdtemp(path.join(tmpdir(), "jftrade-diff-test-"));
  t.after(() => rm(root, { recursive: true, force: true }));

  await writeFile(path.join(root, "clean.ts"), "const value = 1;\n");
  assert.equal(checkUntrackedWhitespace(root, [path.join(root, "clean.ts")]), true);

  await writeFile(path.join(root, "bad.ts"), "const value = 1;\n\n");
  assert.equal(checkUntrackedWhitespace(root, [path.join(root, "bad.ts")]), false);
});
