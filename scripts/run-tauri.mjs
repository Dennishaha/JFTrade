#!/usr/bin/env node
import { spawnSync } from "node:child_process";
import path from "node:path";
import { fileURLToPath } from "node:url";

import {
  tauriCommandOptions,
  tauriDevelopmentEnvironment,
  tauriPreparation,
  tauriReleaseBuild,
} from "./lib/tauri-runtime.mjs";
import { spawnChecked } from "./lib/spawn.mjs";

const repositoryRoot = fileURLToPath(new URL("..", import.meta.url));
const projectRoot = path.join(repositoryRoot, "apps/desktop/src-tauri");
const cli = path.join(repositoryRoot, "node_modules/@tauri-apps/cli/tauri.js");
const command = process.argv[2];

if (command !== "dev" && command !== "build") {
  console.error("Usage: node scripts/run-tauri.mjs <dev|build> [tauri options]");
  process.exit(2);
}

let releaseBuild = null;
try {
  releaseBuild = command === "build" ? tauriReleaseBuild(process.env) : null;
} catch (error) {
  console.error(error instanceof Error ? error.message : String(error));
  process.exit(1);
}
const preparation = tauriPreparation(command);
if (preparation !== null) {
  const preparedStatus = spawnChecked(preparation[0], preparation[1], {
    cwd: repositoryRoot,
    env: process.env,
    stdio: "inherit",
  });
  if (preparedStatus !== 0) process.exit(preparedStatus);
}
const environment =
  command === "dev"
    ? tauriDevelopmentEnvironment(repositoryRoot, process.env, process.execPath)
    : releaseBuild.environment;
const result = spawnSync(
  process.execPath,
  [cli, command, ...tauriCommandOptions(command, process.argv.slice(3), releaseBuild)],
  { cwd: projectRoot, env: environment, stdio: "inherit" },
);
if (result.error) throw result.error;
process.exit(result.status ?? 1);
