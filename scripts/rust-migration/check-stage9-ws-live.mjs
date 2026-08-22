#!/usr/bin/env node
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";

const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), "../..");

function run(command, args) {
  const result = spawnSync(command, args, {
    cwd: repositoryRoot,
    encoding: "utf8",
    env: process.env,
    stdio: ["ignore", "pipe", "pipe"],
    timeout: 300_000,
    killSignal: "SIGTERM",
  });
  if (result.error) throw result.error;
  if (result.status !== 0) {
    throw new Error(`${command} ${args.join(" ")} failed:\n${result.stderr || result.stdout}`);
  }
}

run("go", [
  "test",
  "./scripts/rust-migration",
  "-run",
  "^TestStage9WSLiveFixtureMatchesCurrentGoOwner$",
  "-count=1",
]);
run("cargo", ["test", "-p", "jftrade-engine", "--test", "stage9_ws_live"]);
console.log("Stage 9 ws-live Go/Rust fixture differential passed.");
