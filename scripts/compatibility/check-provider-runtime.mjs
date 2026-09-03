#!/usr/bin/env node
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));

export function providerRuntimeFixtureRoot(root = repositoryRoot) {
  return path.join(root, "tests/fixtures/compatibility/provider-runtime");
}

export function runProviderRuntimeProcess(command, args, options = {}) {
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

export function assertProviderRuntimeEquivalent(rustOutput, expected) {
  assert.deepEqual(
    rustOutput,
    expected,
    "provider runtime output differs from the pinned compatibility contract",
  );
}

export function runProviderRuntimeReference(root = repositoryRoot) {
  const stdout = runProviderRuntimeProcess("cargo", [
    "run",
    "--quiet",
    "-p",
    "jftrade-engine",
    "--bin",
    "jftrade-provider-runtime-replay",
    "--",
    "--input",
    path.join(providerRuntimeFixtureRoot(root), "provider-lifecycle-corpus.json"),
  ], { cwd: root, timeoutMs: 120_000 });
  return JSON.parse(stdout);
}

export function runProviderRuntimeReplay(root = repositoryRoot) {
  const expected = JSON.parse(fs.readFileSync(
    path.join(providerRuntimeFixtureRoot(root), "provider-lifecycle-corpus.expected.json"),
    "utf8",
  ));
  const rustOutput = runProviderRuntimeReference(root);
  assertProviderRuntimeEquivalent(rustOutput, expected);
  return {
    marketdataOperations: expected.marketdata.length,
    pineOperations: expected.pine.length,
    physicalSubscriptions: expected.futu.plan.physical.length,
    probes: expected.futu.probes.length,
  };
}

if (pathToFileURL(path.resolve(process.argv[1] ?? "")).href === import.meta.url) {
  const result = runProviderRuntimeReplay();
  console.log(
    `Provider runtime compatibility replay passed: ${result.marketdataOperations} market-data operations, ` +
      `${result.pineOperations} Pine lifecycle operations, ${result.physicalSubscriptions} OpenD subscriptions, ` +
      `${result.probes} health probes.`,
  );
}
