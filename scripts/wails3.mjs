import { spawnSync } from "node:child_process";
import process from "node:process";

const args = process.argv.slice(2);

function run(command, commandArgs, options = {}) {
  return spawnSync(command, commandArgs, {
    stdio: "inherit",
    shell: process.platform === "win32",
    ...options,
  });
}

const result = run("go", ["tool", "wails3", ...args]);

if (result.error) {
  console.error(result.error.message);
  process.exit(1);
}

process.exit(result.status ?? 0);
