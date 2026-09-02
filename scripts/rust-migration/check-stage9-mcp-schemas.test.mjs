import assert from "node:assert/strict";
import test from "node:test";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

import { stage9MCPCommands } from "./check-stage9-mcp-schemas.mjs";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));

test("Stage 9 MCP schema checker runs only the Rust compatibility replay", () => {
  assert.deepEqual(
    stage9MCPCommands.map(({ executable }) => executable),
    ["cargo"],
  );
  assert.match(
    stage9MCPCommands[0].args.join(" "),
    /reviewed_mcp_schemas_match_canonical_go_fixture_deeply/,
  );
});

test("Stage 9 MCP schema checker is executable", () => {
  const result = spawnSync(
    process.execPath,
    ["scripts/rust-migration/check-stage9-mcp-schemas.mjs"],
    { cwd: repositoryRoot, encoding: "utf8", timeout: 300_000 },
  );
  assert.equal(result.status, 0, result.stderr || result.stdout);
  assert.match(result.stdout, /all 69 pinned input schemas deep-match Rust descriptors/);
});
