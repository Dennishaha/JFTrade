#!/usr/bin/env node
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));

function run(command, args, timeoutMs = 300_000) {
  const result = spawnSync(command, args, {
    cwd: repositoryRoot,
    encoding: "utf8",
    env: process.env,
    stdio: ["ignore", "pipe", "pipe"],
    timeout: timeoutMs,
    killSignal: "SIGTERM",
  });
  if (result.error?.code === "ETIMEDOUT") {
    throw new Error(`${command} timed out after ${timeoutMs}ms`);
  }
  if (result.error) throw result.error;
  if (result.status !== 0) {
    throw new Error(`${command} ${args.join(" ")} failed:\n${result.stderr || result.stdout}`);
  }
}

run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "--test",
  "product_plugins_write_tests",
  "--",
  "--nocapture",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "--lib",
  "plugins_write",
  "--",
  "--nocapture",
]);
console.log("Stage 9 plugins-write differential passed: Go fixture, authenticated rehearsal, Rust leaf, and Rust product replay agree.");
