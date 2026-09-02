#!/usr/bin/env node
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

import { buildStage7Corpus } from "./generate-stage7-corpus.mjs";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));

export function stage7FixtureRoot(root = repositoryRoot) {
  return path.join(root, "tests/fixtures/rust-migration/stage7");
}

export function runStage7Process(command, args, options = {}) {
  const timeoutMs = options.timeoutMs ?? 180_000;
  const result = spawnSync(command, args, {
    cwd: options.cwd ?? repositoryRoot,
    encoding: "utf8",
    env: { ...process.env, ...options.env },
    stdio: ["ignore", "pipe", "pipe"],
    maxBuffer: 16 * 1024 * 1024,
    timeout: timeoutMs,
    killSignal: "SIGTERM",
  });
  if (result.error?.code === "ETIMEDOUT") throw new Error(`${command} timed out after ${timeoutMs}ms`);
  if (result.error) throw result.error;
  if (result.status !== 0) {
    throw new Error(`${command} ${args.join(" ")} failed:\n${result.stderr || result.stdout}`);
  }
  return result.stdout.trim();
}

export function assertStage7Equivalent(actual, expected) {
  assert.deepEqual(actual, expected, "Rust Stage 7 output differs from the pinned API/control-plane contract");
  assert.equal(actual.routes.length, 278);
  assert.equal(Object.keys(actual.routeGroups).length, 18);
  assert.equal(actual.routeProbes.at(-1).allowed, false);
  assert.equal(actual.transport.websocketLimit, 20);
  assert.match(actual.transport.sse, /^retry: 3000\n\n/);
  assert.equal(actual.security.applyListenerAfterPersist, true);
  assert.equal(actual.provider.activateBeforePersist, true);
  assert.deepEqual(actual.cleanup.preview.candidates, actual.cleanup.approvedCandidates);
}

export function runRustStage7Reference(root = repositoryRoot) {
  const stdout = runStage7Process("cargo", [
    "run", "--quiet", "-p", "jftrade-engine", "--bin", "jftrade-stage7-shadow", "--",
    "--input", path.join(stage7FixtureRoot(root), "api-control-plane-corpus.json"),
  ], { cwd: root, timeoutMs: 300_000 });
  return JSON.parse(stdout);
}

export function runStage7Differential(root = repositoryRoot) {
  const fixtures = stage7FixtureRoot(root);
  const corpus = JSON.parse(fs.readFileSync(path.join(fixtures, "api-control-plane-corpus.json"), "utf8"));
  const baseline = JSON.parse(fs.readFileSync(path.join(root, "contracts/openapi/openapi.json"), "utf8"));
  assert.deepEqual(corpus, buildStage7Corpus(baseline), "Stage 7 route corpus drifted from OpenAPI baseline");
  const expected = JSON.parse(fs.readFileSync(
    path.join(fixtures, "api-control-plane-corpus.expected.json"),
    "utf8",
  ));
  const actual = runRustStage7Reference(root);
  assertStage7Equivalent(actual, expected);
  return {
    routes: expected.routes.length,
    routeGroups: Object.keys(expected.routeGroups).length,
    routeProbes: expected.routeProbes.length,
  };
}

if (pathToFileURL(path.resolve(process.argv[1] ?? "")).href === import.meta.url) {
  const result = runStage7Differential();
  console.log(
    `Rust Stage 7 compatibility replay passed: ${result.routes} OpenAPI operations, ` +
      `${result.routeGroups} route groups and ${result.routeProbes} concrete route probes.`,
  );
}
