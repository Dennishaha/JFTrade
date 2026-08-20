#!/usr/bin/env node
import { spawnSync } from "node:child_process";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { tauriDevelopmentEnvironment, tauriPreparation } from "./lib/tauri-runtime.mjs";

const repositoryRoot = fileURLToPath(new URL("..", import.meta.url));
const projectRoot = path.join(repositoryRoot, "apps/desktop/src-tauri");
const cli = path.join(repositoryRoot, "node_modules/@tauri-apps/cli/tauri.js");
const command = process.argv[2];

if (command !== "dev" && command !== "build") {
  console.error("Usage: node scripts/run-tauri.mjs <dev|build> [tauri options]");
  process.exit(2);
}

const defaultOptions = command === "dev" ? ["--config", "tauri.dev.conf.json"] : [];
const preparation = tauriPreparation(command);
if (preparation !== null) {
  const prepared = spawnSync(preparation[0], preparation[1], {
    cwd: repositoryRoot,
    env: process.env,
    stdio: "inherit",
  });
  if (prepared.error) throw prepared.error;
  if (prepared.status !== 0) process.exit(prepared.status ?? 1);
}
const environment =
  command === "dev"
    ? tauriDevelopmentEnvironment(repositoryRoot, process.env, process.execPath)
    : process.env;
const result = spawnSync(
  process.execPath,
  [cli, command, ...defaultOptions, ...process.argv.slice(3)],
  { cwd: projectRoot, env: environment, stdio: "inherit" },
);
if (result.error) throw result.error;
process.exit(result.status ?? 1);
