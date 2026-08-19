#!/usr/bin/env node
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));

export function stage8FixtureRoot(root = repositoryRoot) {
  return path.join(root, "tests/fixtures/rust-migration/stage8");
}

export function runStage8Process(command, args, options = {}) {
  const timeoutMs = options.timeoutMs ?? 300_000;
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

export function assertStage8Equivalent(actual, expected) {
  assert.deepEqual(actual, expected, "Rust Stage 8 output differs from the pinned desktop shell contract");
  assert.equal(actual.tauriVersion, "2.11.5");
  assert.equal(actual.profiles.length, 3);
  assert.equal(actual.links.filter((link) => link.accepted).length, 3);
  assert.deepEqual(actual.runtimePlan.startOrder, ["engine", "pine-worker", "marketdata-sidecar"]);
  assert.deepEqual(actual.runtimePlan.shutdownOrder, ["marketdata-sidecar", "pine-worker", "engine"]);
  assert.equal(actual.successfulStart.ready, true);
  assert.equal(actual.failedStart.ready, false);
  assert.equal(actual.failedStart.failureRole, "marketdata-sidecar");
  assert.equal(actual.commands.length, 10);
  assert.equal(actual.events.length, 4);
}

export function assertStage8Configuration(config, facadeSource, expected) {
  assert.equal(config.identifier, "com.jftrade.desktop");
  assert.equal(config.build.frontendDist, "../../web/dist");
  assert.equal(config.bundle.active, false, "native packaging must remain disabled during shadow");
  assert.notEqual(config.app.security.csp, null, "Tauri CSP must fail closed");
  assert.match(config.app.security.csp["connect-src"], /ipc:/);
  for (const contractName of [...expected.commands, ...expected.events]) {
    assert.match(facadeSource, new RegExp(contractName.replaceAll(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
}

export function runGoStage8Reference(root = repositoryRoot) {
  runStage8Process("go", [
    "test", "./cmd/jftrade-desktop", "./internal/desktop", "-count=1",
    "-run", "^(TestDevelopmentDesktopBuildProfile|TestDesktopBuildChannelsCanCoexist|TestNormalizeDesktopDocsURL|TestNormalizeDesktopDocsURLRejectsUnsafePaths|TestSanitizeDesktopExternalURL|TestDesktopShutdownCancelsStartupAndReclaimsLateResources|TestProductDataDirByPlatform)$",
  ], { cwd: root });
}

export function runRustStage8Reference(root = repositoryRoot) {
  const stdout = runStage8Process("cargo", [
    "run", "--quiet", "-p", "jftrade-desktop", "--bin", "jftrade-stage8-shadow", "--",
    "--input", path.join(stage8FixtureRoot(root), "desktop-shell-corpus.json"),
  ], { cwd: root });
  return JSON.parse(stdout);
}

export function runStage8Differential(root = repositoryRoot) {
  runGoStage8Reference(root);
  const expected = JSON.parse(fs.readFileSync(
    path.join(stage8FixtureRoot(root), "desktop-shell-corpus.expected.json"),
    "utf8",
  ));
  const actual = runRustStage8Reference(root);
  assertStage8Equivalent(actual, expected);
  assertStage8Configuration(
    JSON.parse(fs.readFileSync(path.join(root, "apps/desktop/src-tauri/tauri.conf.json"), "utf8")),
    fs.readFileSync(path.join(root, "apps/web/src/composables/shared/desktopFacade.ts"), "utf8"),
    expected,
  );
  return {
    platforms: expected.profiles.length,
    links: expected.links.length,
    commands: expected.commands.length,
    events: expected.events.length,
  };
}

if (pathToFileURL(path.resolve(process.argv[1] ?? "")).href === import.meta.url) {
  const result = runStage8Differential();
  console.log(
    `Go/Rust Stage 8 differential passed: ${result.platforms} platform profiles, ` +
      `${result.links} link cases, ${result.commands} facade commands and ${result.events} events.`,
  );
}
