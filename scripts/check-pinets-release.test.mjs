#!/usr/bin/env node
import { existsSync, mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawnSync } from "node:child_process";

const tempDir = mkdtempSync(join(tmpdir(), "jftrade-pinets-release-"));
try {
  const runLog = join(tempDir, "run.log");
  const releaseOut = join(tempDir, "dist", "trading-engine");

  const strict = runCheck([], {
    JFTRADE_PINETS_RELEASE_RUN_LOG: runLog,
    JFTRADE_PINETS_RELEASE_OUT: releaseOut,
    JFTRADE_PINETS_RELEASE_DRY_RUN: "1",
    JFTRADE_PINETS_RELEASE_PINETS_STATUS: "1",
    JFTRADE_PINETS_RELEASE_PINETS_LICENSE: "AGPL-3.0-only",
  });
  assert(strict.status !== 0, "strict release check passed despite missing pinets");
  assert(strict.stderr.includes("PineTS release acceptance is blocked"), "strict release check did not report blocked acceptance");

  const blocked = runCheck(["--allow-blocked"], {
    JFTRADE_PINETS_RELEASE_RUN_LOG: runLog,
    JFTRADE_PINETS_RELEASE_OUT: releaseOut,
    JFTRADE_PINETS_RELEASE_DRY_RUN: "1",
    JFTRADE_PINETS_RELEASE_PINETS_STATUS: "1",
    JFTRADE_PINETS_RELEASE_PINETS_LICENSE: "AGPL-3.0-only",
  });
  assert(blocked.status === 0, `blocked release check failed: ${blocked.stderr}`);
  const blockedLog = readFileSync(runLog, "utf8");
  assert(!blockedLog.includes("pnpm run build:pineworker"), "blocked release check should skip release asset build");
  assert(blockedLog.includes("go test ./pkg/strategy/pineworker -run Test -cover"), "blocked release check did not run focused Pine worker coverage gate");
  assert(blockedLog.includes("pnpm run check:pinets-compliance"), "blocked release check did not run PineTS compliance gate");
  assert(blockedLog.includes("pnpm run test:web"), "blocked release check did not run frontend test gate");
  assert(blockedLog.includes("pnpm run typecheck:web"), "blocked release check did not run frontend typecheck gate");
  assert(blockedLog.includes("pnpm run build:frontend-assets"), "blocked release check did not rebuild frontend release assets");
  assert(blockedLog.includes("git diff --check"), "blocked release check did not run git diff whitespace gate");

  const pass = runCheck([], {
    JFTRADE_PINETS_RELEASE_RUN_LOG: runLog,
    JFTRADE_PINETS_RELEASE_OUT: releaseOut,
    JFTRADE_PINETS_RELEASE_DRY_RUN: "1",
    JFTRADE_PINETS_RELEASE_PINETS_STATUS: "0",
    JFTRADE_PINETS_RELEASE_PINETS_LICENSE: "AGPL-3.0-only",
  });
  assert(pass.status === 0, `unblocked release check failed: ${pass.stderr || pass.stdout}`);
  assert(pass.stdout.includes("pinets package license: AGPL-3.0-only"), "unblocked release check did not report pinets package license");
  const passLog = readFileSync(runLog, "utf8");
  assert(passLog.includes("JFTRADE_PINEWORKER_REAL_PROCESS_SMOKE=1 go test ./pkg/strategy/pineworker -run TestWorkerManagerRealPineTSProcessSmoke -v"), "unblocked release check did not run real PineTS process smoke");
  assert(passLog.includes("pnpm run build:pineworker"), "unblocked release check did not build worker assets");
  assert(passLog.includes(`go build -tags release_assets -o ${releaseOut} ./cmd/jftrade-api`), "unblocked release check did not build release_assets API binary");
  assert(existsSync(releaseOut), "unblocked release check did not leave an artifact");

  const missingArtifact = runCheck([], {
    JFTRADE_PINETS_RELEASE_RUN_LOG: runLog,
    JFTRADE_PINETS_RELEASE_OUT: releaseOut,
    JFTRADE_PINETS_RELEASE_DRY_RUN: "1",
    JFTRADE_PINETS_RELEASE_PINETS_STATUS: "0",
    JFTRADE_PINETS_RELEASE_PINETS_LICENSE: "AGPL-3.0-only",
    JFTRADE_PINETS_RELEASE_STUB_SKIP_ARTIFACT: "1",
  });
  assert(missingArtifact.status !== 0, "release check passed despite missing release artifact");
  assert(missingArtifact.stderr.includes("release artifact is missing or empty"), "release check did not report missing release artifact");
} finally {
  rmSync(tempDir, { recursive: true, force: true });
}

function runCheck(args, extraEnv) {
  return spawnSync(process.execPath, ["scripts/check-pinets-release.mjs", ...args], {
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
