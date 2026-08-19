#!/usr/bin/env node
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));

export function stage4FixtureRoot(root = repositoryRoot) {
  return path.join(root, "tests/fixtures/rust-migration/stage4");
}

export function runStage4Process(command, args, options = {}) {
  const timeoutMs = options.timeoutMs ?? 120_000;
  const result = spawnSync(command, args, {
    cwd: options.cwd ?? repositoryRoot,
    encoding: "utf8",
    env: { ...process.env, ...options.env },
    stdio: ["ignore", "pipe", "pipe"],
    maxBuffer: 8 * 1024 * 1024,
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
  return result.stdout.trim();
}

export function assertStage4Equivalent(rustOutput, expected) {
  assert.deepEqual(
    rustOutput,
    expected,
    "Rust Stage 4 output differs from the pinned Go compatibility contract",
  );
}

export function runGoStage4References(root = repositoryRoot) {
  const packages = [
    ["./internal/marketdata", "^TestRustMigrationStage4DemandAndProviderLifecycleMatchesCorpus$"],
    ["./internal/integration/futu", "^TestRustMigrationStage4OpenDFrameAndSubscriptionPlanMatchesCorpus$"],
    ["./pkg/strategy/pineworker", "^TestRustMigrationStage4PineLifecycleMatchesCorpus$"],
  ];
  for (const [pkg, testName] of packages) {
    runStage4Process("go", ["test", pkg, "-run", testName, "-count=1"], {
      cwd: root,
      timeoutMs: 120_000,
    });
  }
}

export function runRustStage4Reference(root = repositoryRoot) {
  const stdout = runStage4Process("cargo", [
    "run",
    "--quiet",
    "-p",
    "jftrade-engine",
    "--bin",
    "jftrade-stage4-shadow",
    "--",
    "--input",
    path.join(stage4FixtureRoot(root), "provider-lifecycle-corpus.json"),
  ], { cwd: root, timeoutMs: 120_000 });
  return JSON.parse(stdout);
}

export function runStage4Differential(root = repositoryRoot) {
  const expected = JSON.parse(fs.readFileSync(
    path.join(stage4FixtureRoot(root), "provider-lifecycle-corpus.expected.json"),
    "utf8",
  ));
  runGoStage4References(root);
  const rustOutput = runRustStage4Reference(root);
  assertStage4Equivalent(rustOutput, expected);
  return {
    marketdataOperations: expected.marketdata.length,
    pineOperations: expected.pine.length,
    physicalSubscriptions: expected.futu.plan.physical.length,
    probes: expected.futu.probes.length,
  };
}

if (pathToFileURL(path.resolve(process.argv[1] ?? "")).href === import.meta.url) {
  const result = runStage4Differential();
  console.log(
    `Go/Rust Stage 4 differential passed: ${result.marketdataOperations} market-data operations, ` +
      `${result.pineOperations} Pine lifecycle operations, ${result.physicalSubscriptions} OpenD subscriptions, ` +
      `${result.probes} health probes.`,
  );
}
