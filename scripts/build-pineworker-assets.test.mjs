#!/usr/bin/env node
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawnSync } from "node:child_process";

const tempDir = mkdtempSync(join(tmpdir(), "jftrade-pineworker-assets-"));
try {
  const outDir = join(tempDir, "assets", "bin");
  const missing = runBuild({
    JFTRADE_PINEWORKER_ASSET_OUT_DIR: outDir,
    JFTRADE_PINEWORKER_ASSET_BUILD_DRY_RUN: "1",
    JFTRADE_PINETS_RELEASE_PINETS_STATUS: "1",
  });
  assert(missing.status !== 0, "worker asset build passed despite missing pinets");
  assert(missing.stderr.includes("PineTS worker asset build is blocked until the pinets package is installed"), "missing pinets blocker was not reported");

  mkdirSync(outDir, { recursive: true });
  writeFileSync(join(outDir, ".gitkeep"), "");
  writeFileSync(join(outDir, "stale-worker"), "stale");

  const pass = runBuild({
    JFTRADE_PINEWORKER_ASSET_OUT_DIR: outDir,
    JFTRADE_PINEWORKER_ASSET_BUILD_DRY_RUN: "1",
    JFTRADE_PINETS_RELEASE_PINETS_STATUS: "0",
    JFTRADE_PINETS_RELEASE_PINETS_LICENSE: "AGPL-3.0-only",
  });
  assert(pass.status === 0, `worker asset build failed: ${pass.stderr || pass.stdout}`);
  assert(pass.stdout.includes("pinets package license: AGPL-3.0-only"), "pinets package license was not reported");
  assert((pass.stdout.match(/DRY RUN esbuild/g) ?? []).length === 1, "worker asset build did not produce one platform-independent Node bundle");
  assert(pass.stdout.includes("--platform=node") && pass.stdout.includes("--format=esm"), "worker asset build did not target Node ESM");
  assert(!/\bbun\b/i.test(pass.stdout), "worker asset build still references Bun");
  assert(existsSync(join(outDir, ".gitkeep")), ".gitkeep should be preserved");
  assert(!existsSync(join(outDir, "stale-worker")), "stale worker artifact should be deleted");
} finally {
  rmSync(tempDir, { recursive: true, force: true });
}

function runBuild(extraEnv) {
  return spawnSync(process.execPath, ["scripts/build-pineworker-assets.mjs"], {
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
