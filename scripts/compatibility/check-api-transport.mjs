#!/usr/bin/env node
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

import { buildApiTransportCorpus } from "./generate-api-transport-corpus.mjs";
import { rustReplayInvocation } from "./rust-replay-process.mjs";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));

export function apiTransportFixtureRoot(root = repositoryRoot) {
  return path.join(root, "tests/fixtures/compatibility/api-transport");
}

export function runApiTransportProcess(command, args, options = {}) {
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

export function assertApiTransportEquivalent(actual, expected) {
  assert.deepEqual(actual, expected, "API transport output differs from the pinned compatibility contract");
  assert.equal(actual.routes.length, 278);
  assert.equal(Object.keys(actual.routeGroups).length, 18);
  assert.equal(actual.routeProbes.at(-1).allowed, false);
  assert.equal(actual.transport.websocketLimit, 20);
  assert.match(actual.transport.sse, /^retry: 3000\n\n/);
  assert.equal(actual.security.applyListenerAfterPersist, true);
  assert.equal(actual.provider.activateBeforePersist, true);
  assert.deepEqual(actual.cleanup.preview.candidates, actual.cleanup.approvedCandidates);
}

export function runApiTransportReference(root = repositoryRoot) {
  const invocation = rustReplayInvocation({
    root,
    packageName: "jftrade-engine",
    binaryName: "jftrade-api-transport-replay",
    args: ["--input", path.join(apiTransportFixtureRoot(root), "api-control-plane-corpus.json")],
  });
  const stdout = runApiTransportProcess(
    invocation.command,
    invocation.args,
    { cwd: root, timeoutMs: 300_000 },
  );
  return JSON.parse(stdout);
}

export function runApiTransportReplay(root = repositoryRoot) {
  const fixtures = apiTransportFixtureRoot(root);
  const corpus = JSON.parse(fs.readFileSync(path.join(fixtures, "api-control-plane-corpus.json"), "utf8"));
  const baseline = JSON.parse(fs.readFileSync(path.join(root, "contracts/openapi/openapi.json"), "utf8"));
  assert.deepEqual(corpus, buildApiTransportCorpus(baseline), "API route corpus drifted from OpenAPI baseline");
  const expected = JSON.parse(fs.readFileSync(
    path.join(fixtures, "api-control-plane-corpus.expected.json"),
    "utf8",
  ));
  const actual = runApiTransportReference(root);
  assertApiTransportEquivalent(actual, expected);
  return {
    routes: expected.routes.length,
    routeGroups: Object.keys(expected.routeGroups).length,
    routeProbes: expected.routeProbes.length,
  };
}

if (pathToFileURL(path.resolve(process.argv[1] ?? "")).href === import.meta.url) {
  const result = runApiTransportReplay();
  console.log(
    `API transport compatibility replay passed: ${result.routes} OpenAPI operations, ` +
      `${result.routeGroups} route groups and ${result.routeProbes} concrete route probes.`,
  );
}
