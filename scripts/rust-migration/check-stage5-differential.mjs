#!/usr/bin/env node
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));

export function stage5FixtureRoot(root = repositoryRoot) {
  return path.join(root, "tests/fixtures/rust-migration/stage5");
}

export function runStage5Process(command, args, options = {}) {
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

export function assertStage5Equivalent(rustOutput, expected) {
  assert.deepEqual(
    rustOutput,
    expected,
    "Rust Stage 5 output differs from the pinned Go compatibility contract",
  );
  assert.equal(hasTrueDispatch(rustOutput), false, "Stage 5 shadow attempted a dispatch");
}

export function runGoStage5References(root = repositoryRoot) {
  const packages = [
    ["./internal/trading", "^TestRustMigrationStage5TradingStatusAndRiskMatchCorpus$"],
    ["./internal/strategy/runtimecontrol", "^TestRustMigrationStage5StrategyRiskAndNotificationPlansMatchCorpus$"],
    ["./pkg/futu", "^TestRustMigrationStage5OpenDTradeProtocolGateMatchesCorpus$"],
  ];
  for (const [pkg, testName] of packages) {
    runStage5Process("go", ["test", pkg, "-run", testName, "-count=1"], {
      cwd: root,
      timeoutMs: 120_000,
    });
  }
}

export function runRustStage5Reference(root = repositoryRoot) {
  const stdout = runStage5Process("cargo", [
    "run",
    "--quiet",
    "-p",
    "jftrade-engine",
    "--bin",
    "jftrade-stage5-shadow",
    "--",
    "--input",
    path.join(stage5FixtureRoot(root), "trading-strategy-corpus.json"),
  ], { cwd: root, timeoutMs: 120_000 });
  return JSON.parse(stdout);
}

export function runStage5Differential(root = repositoryRoot) {
  const expected = JSON.parse(fs.readFileSync(
    path.join(stage5FixtureRoot(root), "trading-strategy-corpus.expected.json"),
    "utf8",
  ));
  runGoStage5References(root);
  const rustOutput = runRustStage5Reference(root);
  assertStage5Equivalent(rustOutput, expected);
  return {
    statuses: expected.statuses.length,
    transitions: expected.transitions.length,
    commands: expected.commands.length,
    events: expected.events.length,
    positionRefreshes: expected.positionRefreshes.length,
    strategies: expected.strategies.length,
  };
}

function hasTrueDispatch(value) {
  if (Array.isArray(value)) return value.some(hasTrueDispatch);
  if (value && typeof value === "object") {
    if (value.dispatch === true) return true;
    return Object.values(value).some(hasTrueDispatch);
  }
  return false;
}

if (pathToFileURL(path.resolve(process.argv[1] ?? "")).href === import.meta.url) {
  const result = runStage5Differential();
  console.log(
    `Go/Rust Stage 5 differential passed: ${result.statuses} statuses, ` +
      `${result.transitions} transitions, ${result.commands} command plans, ` +
      `${result.events} update events, ${result.positionRefreshes} position refreshes, ` +
      `${result.strategies} strategy scenarios; zero dispatches.`,
  );
}
