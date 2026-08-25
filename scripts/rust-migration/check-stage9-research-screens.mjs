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

run("go", [
  "test",
  "scripts/rust-migration/stage9_research_screens_reference_test.go",
  "-run",
  "^TestStage9ResearchScreensFixtureMatchesCurrentGoOwner$",
  "-count=1",
]);
run("go", [
  "test",
  "./internal/app/apiserver/servercoretest",
  "-run",
  "^TestResearchScreensWriteRehearsalFencesOwnersAndRecoversAcrossRestart$",
  "-count=1",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "--test",
  "stage9_research_screens",
  "--",
  "--nocapture",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "--lib",
  "product::tests::research_screen_write_product_tests",
  "--",
  "--nocapture",
]);
console.log(
  "Stage 9 research-screens differential passed: Go fixture, Rust leaf/product replay, and authenticated rehearsal agree for 1 route and 22 cases.",
);
