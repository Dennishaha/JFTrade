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
  "--lib",
  "auth_session",
  "--",
  "--nocapture",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-api",
  "--test",
  "auth_session_transport_contracts",
  "--",
  "--nocapture",
]);
console.log(
  "Stage 9 auth-session differential passed: Go fixture, authenticated rehearsal, Rust product replay, and transport contract agree.",
);
