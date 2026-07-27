#!/usr/bin/env node
import { spawnSync } from "node:child_process";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const scriptPath = fileURLToPath(import.meta.url);
const repoRoot = resolve(dirname(scriptPath), "..");

const result = spawnSync(
  "go",
  ["run", "./scripts/go-test-quality", ...process.argv.slice(2)],
  {
    cwd: repoRoot,
    stdio: "inherit",
  },
);

if (result.error) {
  console.error(`unable to run the Go test assertion analyzer: ${result.error.message}`);
  process.exitCode = 1;
} else if (result.signal) {
  console.error(`Go test assertion analyzer terminated by ${result.signal}`);
  process.exitCode = 1;
} else {
  process.exitCode = result.status ?? 1;
}
