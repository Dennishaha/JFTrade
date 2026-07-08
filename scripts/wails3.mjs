import { spawnSync } from "node:child_process";
import process from "node:process";

const args = process.argv.slice(2);
const pinnedVersion = "v3.0.0-alpha2.115";

function run(command, commandArgs, options = {}) {
  return spawnSync(command, commandArgs, {
    stdio: "inherit",
    shell: process.platform === "win32",
    ...options,
  });
}

const probe = spawnSync("wails3", ["version"], {
  stdio: "ignore",
  shell: process.platform === "win32",
});

const result =
  probe.status === 0
    ? run("wails3", args)
    : run("go", ["run", `github.com/wailsapp/wails/v3/cmd/wails3@${pinnedVersion}`, ...args]);

if (result.error) {
  console.error(result.error.message);
  process.exit(1);
}

process.exit(result.status ?? 0);
