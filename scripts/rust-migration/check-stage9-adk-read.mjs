#!/usr/bin/env node
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));

function run(command, args, timeoutMs = 300_000) {
  const result = spawnSync(command, args, {
    cwd: repositoryRoot,
    encoding: "utf8",
    env: { ...process.env, GIN_MODE: "release" },
    stdio: ["ignore", "pipe", "pipe"],
    timeout: timeoutMs,
    killSignal: "SIGTERM",
  });
  if (result.error?.code === "ETIMEDOUT") {
    throw new Error(`${command} timed out after ${timeoutMs}ms`);
  }
  if (result.error) {
    throw result.error;
  }
  if (result.status !== 0) {
    throw new Error(`${command} ${args.join(" ")} failed:\n${result.stderr || result.stdout}`);
  }
}

run("go", [
  "test",
  "./scripts/rust-migration",
  "-run",
  "^TestStage9ADKRead(SSE)?FixtureMatchesCurrentGoOwner$",
  "-count=1",
  "-timeout=300s",
]);
run("go", [
  "test",
  "./internal/app/apiserver/servercoretest",
  "-run",
  "^TestADKReadStreamRehearsalPreservesAuthenticatedSSEAndRecoversAcrossRestart$",
  "-count=1",
  "-timeout=300s",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "--lib",
  "product::tests::adk_read_tests::",
  "--",
  "--nocapture",
]);
console.log(
  "Stage 9 adk-read differential passed: Go JSON/SSE fixtures, authenticated GET-sidecar rehearsal, and Rust leaf/product replay agree.",
);
