#!/usr/bin/env node

import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));
const commandTimeoutMs = 300_000;

export const stage9MCPCommands = Object.freeze([
  Object.freeze({
    label: "Go MCP schema fixture reference",
    executable: "go",
    args: [
      "test",
      "./scripts/rust-migration",
      "-run",
      "^TestStage9MCPToolSchemasMatchGoReference$",
      "-count=1",
      "-timeout=300s",
    ],
  }),
  Object.freeze({
    label: "Rust MCP schema deep-equality replay",
    executable: "cargo",
    args: [
      "test",
      "-p",
      "jftrade-engine",
      "--lib",
      "product_mcp_server::tests::reviewed_mcp_schemas_match_canonical_go_fixture_deeply",
      "--locked",
      "--",
      "--nocapture",
    ],
  }),
]);

function run({ label, executable, args }) {
  console.log(`\n> ${label}\n> ${executable} ${args.join(" ")}`);
  const result = spawnSync(executable, args, {
    cwd: repositoryRoot,
    encoding: "utf8",
    stdio: "inherit",
    timeout: commandTimeoutMs,
  });
  if (result.error) {
    throw result.error;
  }
  if (result.status !== 0) {
    throw new Error(`${label} failed with exit ${result.status}`);
  }
}

export function main() {
  for (const command of stage9MCPCommands) {
    run(command);
  }
  console.log("Stage 9 MCP schema parity passed: all 69 Go input schemas deep-match Rust descriptors.");
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  try {
    main();
  } catch (error) {
    console.error(error instanceof Error ? error.message : error);
    process.exitCode = 1;
  }
}
