#!/usr/bin/env node
import { existsSync, mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawnSync } from "node:child_process";

const tempDir = mkdtempSync(join(tmpdir(), "jftrade-pineworker-dev-"));
try {
  const outDir = join(tempDir, "worker");
  const envFile = join(tempDir, "pineworker.env");
  const missing = runBuild({
    JFTRADE_PINEWORKER_DEV_OUT_DIR: outDir,
    JFTRADE_PINEWORKER_DEV_BUILD_DRY_RUN: "1",
    JFTRADE_PINETS_RELEASE_PINETS_STATUS: "1",
  });
  assert(missing.status !== 0, "dev worker build passed despite missing pinets");
  assert(missing.stderr.includes("PineTS dev worker build is blocked until the pinets package is installed"), "missing pinets blocker was not reported");

  const pass = runBuild({
    JFTRADE_PINEWORKER_DEV_OUT_DIR: outDir,
    JFTRADE_PINEWORKER_DEV_ENV_FILE: envFile,
    JFTRADE_PINEWORKER_DEV_BUILD_DRY_RUN: "1",
    JFTRADE_PINETS_RELEASE_PINETS_STATUS: "0",
    JFTRADE_PINETS_RELEASE_PINETS_LICENSE: "AGPL-3.0-only",
  });
  assert(pass.status === 0, `dev worker build failed: ${pass.stderr || pass.stdout}`);
  assert(pass.stdout.includes("pinets package license: AGPL-3.0-only"), "pinets package license was not reported");
  assert(pass.stdout.includes("DRY RUN bun build --compile"), "dev worker build did not invoke Bun compile path");
  assert(existsSync(envFile), "dev worker build did not write env file");
  assert(readFileSync(envFile, "utf8").includes("JFTRADE_PINEWORKER_BINARY="), "dev env file does not contain worker binary");

  const devAPI = runDevAPI({
    JFTRADE_PINEWORKER_DEV_OUT_DIR: outDir,
    JFTRADE_PINEWORKER_DEV_BUILD_DRY_RUN: "1",
    JFTRADE_DEV_API_PINEWORKER_DRY_RUN: "1",
    JFTRADE_PINETS_RELEASE_PINETS_STATUS: "0",
    JFTRADE_PINETS_RELEASE_PINETS_LICENSE: "AGPL-3.0-only",
  });
  assert(devAPI.status === 0, `dev api pineworker dry run failed: ${devAPI.stderr || devAPI.stdout}`);
  assert(devAPI.stdout.includes("DRY RUN JFTRADE_PINEWORKER_BINARY="), "dev api dry run did not configure worker binary");
  assert(devAPI.stdout.includes("JFTRADE_PINEWORKER_WORKERS=1"), "dev api dry run did not default to one worker");
  assert(devAPI.stdout.includes("go run ./cmd/jftrade-api"), "dev api dry run did not show Go API command");

  const devAPIWorkersOverride = runDevAPI({
    JFTRADE_PINEWORKER_DEV_OUT_DIR: outDir,
    JFTRADE_PINEWORKER_DEV_BUILD_DRY_RUN: "1",
    JFTRADE_DEV_API_PINEWORKER_DRY_RUN: "1",
    JFTRADE_PINEWORKER_WORKERS: "2",
    JFTRADE_PINETS_RELEASE_PINETS_STATUS: "0",
    JFTRADE_PINETS_RELEASE_PINETS_LICENSE: "AGPL-3.0-only",
  });
  assert(devAPIWorkersOverride.status === 0, `dev api pineworker workers override dry run failed: ${devAPIWorkersOverride.stderr || devAPIWorkersOverride.stdout}`);
  assert(devAPIWorkersOverride.stdout.includes("JFTRADE_PINEWORKER_WORKERS=2"), "dev api dry run did not preserve worker override");
} finally {
  rmSync(tempDir, { recursive: true, force: true });
}

function runBuild(extraEnv) {
  return spawnSync(process.execPath, ["scripts/build-pineworker-dev.mjs"], {
    cwd: process.cwd(),
    env: { ...process.env, ...extraEnv },
    encoding: "utf8",
  });
}

function runDevAPI(extraEnv) {
  return spawnSync(process.execPath, ["scripts/dev-api-pineworker.mjs"], {
    cwd: process.cwd(),
    env: { ...process.env, ...extraEnv },
    encoding: "utf8",
  });
}

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}
