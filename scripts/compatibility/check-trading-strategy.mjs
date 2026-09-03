#!/usr/bin/env node
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

import { rustReplayInvocation } from "./rust-replay-process.mjs";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));

export function tradingStrategyFixtureRoot(root = repositoryRoot) {
  return path.join(root, "tests/fixtures/compatibility/trading-strategy");
}

export function runTradingStrategyProcess(command, args, options = {}) {
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

export function assertTradingStrategyEquivalent(rustOutput, expected) {
  assert.deepEqual(
    rustOutput,
    expected,
    "trading and strategy output differs from the pinned compatibility contract",
  );
  assert.equal(hasTrueDispatch(rustOutput), false, "compatibility replay attempted a real dispatch");
}

export function runTradingStrategyReference(root = repositoryRoot) {
  const invocation = rustReplayInvocation({
    root,
    packageName: "jftrade-engine",
    binaryName: "jftrade-trading-strategy-replay",
    args: [
      "--input",
      path.join(tradingStrategyFixtureRoot(root), "trading-strategy-corpus.json"),
    ],
  });
  const stdout = runTradingStrategyProcess(
    invocation.command,
    invocation.args,
    { cwd: root, timeoutMs: 120_000 },
  );
  return JSON.parse(stdout);
}

export function runTradingStrategyReplay(root = repositoryRoot) {
  const expected = JSON.parse(fs.readFileSync(
    path.join(tradingStrategyFixtureRoot(root), "trading-strategy-corpus.expected.json"),
    "utf8",
  ));
  const rustOutput = runTradingStrategyReference(root);
  assertTradingStrategyEquivalent(rustOutput, expected);
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
  const result = runTradingStrategyReplay();
  console.log(
    `Trading and strategy compatibility replay passed: ${result.statuses} statuses, ` +
      `${result.transitions} transitions, ${result.commands} command plans, ` +
      `${result.events} update events, ${result.positionRefreshes} position refreshes, ` +
      `${result.strategies} strategy scenarios; zero dispatches.`,
  );
}
