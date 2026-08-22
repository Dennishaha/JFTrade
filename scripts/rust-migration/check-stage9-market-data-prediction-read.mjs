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
  "^TestStage9MarketDataPredictionReadFixtureMatchesCurrentGoOwner$",
  "-count=1",
]);
run("cargo", [
  "test",
  "-p",
  "jftrade-engine",
  "--test",
  "stage9_market_data_prediction_read",
  "--",
  "--nocapture",
]);
for (const testName of [
  "product::tests::market_data_prediction_read_tests::market_data_prediction_read_routes_match_group_fixture_in_cutover_only",
  "product::tests::market_data_prediction_read_tests::market_data_prediction_read_routes_fail_closed_when_snapshot_is_unavailable",
  "product::tests::market_data_prediction_read_tests::market_data_prediction_read_routes_are_not_registered_without_snapshot_port",
]) {
  run("cargo", [
    "test",
    "-p",
    "jftrade-engine",
    testName,
    "--",
    "--exact",
  ]);
}
console.log("Stage 9 market-data prediction-read differential passed: Go fixture and Rust group replay agree.");
