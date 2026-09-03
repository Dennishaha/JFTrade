#!/usr/bin/env node
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

import { rustReplayInvocation } from "./rust-replay-process.mjs";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));
export function backtestFixtureRoot(root = repositoryRoot) {
  return path.join(root, "tests/fixtures/compatibility/backtest");
}

export function runProcess(command, args, options = {}) {
  const timeout = options.timeoutMs ?? 120_000;
  const result = spawnSync(command, args, {
    cwd: options.cwd ?? repositoryRoot,
    encoding: "utf8",
    env: { ...process.env, ...options.env },
    stdio: ["ignore", "pipe", "pipe"],
    maxBuffer: 8 * 1024 * 1024,
    timeout,
    killSignal: "SIGTERM",
  });
  if (result.error?.code === "ETIMEDOUT") {
    throw new Error(`${command} timed out after ${timeout}ms`);
  }
  if (result.error) throw result.error;
  if (result.status !== 0) {
    throw new Error(command + " " + args.join(" ") + " failed:\n" + (result.stderr || result.stdout));
  }
  return result.stdout.trim();
}

export function assertBacktestEquivalent(rustOutput, expected) {
  assert.deepEqual(rustOutput, expected, "backtest output differs from the pinned compatibility golden");
}

export function loadBacktestExpected(root = repositoryRoot) {
  return JSON.parse(fs.readFileSync(
    path.join(backtestFixtureRoot(root), "backtest-corpus.expected.json"),
    "utf8",
  ));
}

export function runRustReference(root = repositoryRoot) {
  const invocation = rustReplayInvocation({
    root,
    packageName: "jftrade-backtest",
    binaryName: "jftrade-backtest-replay",
    args: [
      "--input",
      path.join(backtestFixtureRoot(root), "backtest-corpus.json"),
    ],
  });
  return JSON.parse(runProcess(invocation.command, invocation.args, { cwd: root, timeoutMs: 120_000 }));
}

export function runBacktestReplay(root = repositoryRoot) {
  const expected = loadBacktestExpected(root);
  const rustOutput = runRustReference(root);
  assertBacktestEquivalent(rustOutput, expected);
  return {
    cases: expected.cases.length,
    fills: expected.cases.reduce((total, item) => total + item.totalFills, 0),
    resultHashes: expected.cases.map((item) => item.resultHash),
  };
}

if (pathToFileURL(path.resolve(process.argv[1] ?? "")).href === import.meta.url) {
  const result = runBacktestReplay();
  console.log(
    `Rust backtest compatibility replay passed: ${result.cases} cases, ${result.fills} fills.`,
  );
}
