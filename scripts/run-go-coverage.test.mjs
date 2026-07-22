import assert from "node:assert/strict";
import test from "node:test";

import { stripPnpmRunSeparator } from "./run-go-coverage.mjs";

test("removes pnpm's argument separator before invoking the Go coverage checker", () => {
  assert.deepEqual(
    stripPnpmRunSeparator(["--", "-package", "./internal/...", "-diff-base=origin/main"]),
    ["-package", "./internal/...", "-diff-base=origin/main"],
  );
});

test("preserves direct Node invocation arguments", () => {
  assert.deepEqual(stripPnpmRunSeparator(["-package", "./pkg/..."]), ["-package", "./pkg/..."]);
});
