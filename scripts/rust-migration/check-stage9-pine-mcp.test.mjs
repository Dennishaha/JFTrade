import assert from "node:assert/strict";
import test from "node:test";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

test("Stage 9 Pine MCP check runs the native strategy and engine contract tests", () => {
  const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));
  const result = spawnSync(process.execPath, ["scripts/rust-migration/check-stage9-pine-mcp.mjs"], {
    cwd: repositoryRoot,
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"],
    timeout: 300_000,
  });
  assert.equal(result.status, 0, result.stderr || result.stdout);
  assert.match(result.stdout, /support-matrix, v4\.0 score model, externalEngine\/saveHint field sets/);
  assert.match(result.stdout, /structural fail-closed/);
});
