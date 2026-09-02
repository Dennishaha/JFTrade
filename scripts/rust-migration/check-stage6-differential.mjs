#!/usr/bin/env node
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));

export function stage6FixtureRoot(root = repositoryRoot) {
  return path.join(root, "tests/fixtures/rust-migration/stage6");
}

export function runStage6Process(command, args, options = {}) {
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

export function assertStage6Equivalent(rustOutput, expected) {
  assert.deepEqual(
    rustOutput,
    expected,
    "Rust Stage 6 output differs from the pinned Go Assistant compatibility contract",
  );
  assert.equal(rustOutput.rig.recordTelemetryContent, false);
  assert.equal(rustOutput.approval.replayResolutionChanged, false);
  assert.equal(rustOutput.input.replayResolutionChanged, false);
  assert.equal(rustOutput.claims.outcomeUnknownError, "TOOL_OUTCOME_UNKNOWN");
  assert.equal(rustOutput.provider.attempts, 2);
}

export function runRustStage6Reference(root = repositoryRoot) {
  const stdout = runStage6Process("cargo", [
    "run",
    "--quiet",
    "-p",
    "jftrade-engine",
    "--bin",
    "jftrade-stage6-shadow",
    "--",
    "--input",
    path.join(stage6FixtureRoot(root), "assistant-rig-corpus.json"),
  ], { cwd: root, timeoutMs: 180_000 });
  return JSON.parse(stdout);
}

export function runStage6Differential(root = repositoryRoot) {
  const expected = JSON.parse(fs.readFileSync(
    path.join(stage6FixtureRoot(root), "assistant-rig-corpus.expected.json"),
    "utf8",
  ));
  const rustOutput = runRustStage6Reference(root);
  assertStage6Equivalent(rustOutput, expected);
  return {
    statuses: expected.statuses.length,
    transitions: expected.transitions.length,
    invalidInputs: expected.invalidInputs.length,
    claims: expected.claims.invocations.length,
    tasks: expected.workflow.tasks.length,
    artifacts: expected.artifacts.versions,
    streamDeltas: expected.provider.deltas.length,
  };
}

if (pathToFileURL(path.resolve(process.argv[1] ?? "")).href === import.meta.url) {
  const result = runStage6Differential();
  console.log(
    `Rust Stage 6 compatibility replay passed: ${result.statuses} statuses, ` +
      `${result.transitions} transitions, ${result.invalidInputs} rejected input prompts, ` +
      `${result.claims} durable claims, ${result.tasks} workflow tasks, ` +
      `${result.artifacts} artifact versions and ${result.streamDeltas} stream deltas.`,
  );
}
