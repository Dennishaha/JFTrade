#!/usr/bin/env node
import { chmodSync, existsSync, mkdirSync, rmSync, statSync, writeFileSync } from "node:fs";
import { dirname } from "node:path";
import { checkPinetsPackageAndLicense } from "./lib/pinets-package.mjs";
import { spawnChecked } from "./lib/spawn.mjs";

let allowBlocked = false;
for (const arg of process.argv.slice(2)) {
  if (arg === "--allow-blocked") {
    allowBlocked = true;
  } else {
    console.error(`unknown argument: ${arg}`);
    console.error("usage: node scripts/check-pinets-release.mjs [--allow-blocked]");
    process.exit(2);
  }
}

const runLog = process.env.JFTRADE_PINETS_RELEASE_RUN_LOG || "";
const releaseOut = process.env.JFTRADE_PINETS_RELEASE_OUT || "dist/trading-engine";
const dryRun = process.env.JFTRADE_PINETS_RELEASE_DRY_RUN === "1";

let blocked = false;
if (!checkPinetsPackageAndLicense({ dryRun, verifyNpmVisible: true })) {
  blocked = true;
}

run("go", ["test", "./internal/app/apiserver/servercore", "-run", "TestResolvePineWorkerRuntimeConfigDefaultsToRealPineTSWorker", "-v"]);
run("go", ["test", "./pkg/strategy/pineworker", "-run", "TestPineTSHardCutDoesNotExposeGoPineRuntime", "-v"]);
run("go", ["test", "./pkg/strategy/pineworker", "-run", "Test", "-cover"]);
run("go", ["test", "./pkg/strategy/pineworker", "-bench", "BenchmarkCheckPerformanceGate", "-run", "^$", "-benchmem"]);
run("npm", ["run", "test:pineworker"]);
run("npm", ["run", "typecheck:pineworker"]);
run("npm", ["run", "test:web"]);
run("npm", ["run", "typecheck:web"]);
run("npm", ["run", "build:frontend-assets"]);
run("go", ["test", "-tags", "release_assets", "./internal/frontendassets", "-run", "TestFileSystem"]);
run("git", ["diff", "--check"]);

if (!blocked) {
  run("go", ["test", "./pkg/strategy/pineworker", "-run", "TestWorkerManagerRealPineTSProcessSmoke", "-v"], {
    JFTRADE_PINEWORKER_REAL_PROCESS_SMOKE: "1",
  });
  run("npm", ["run", "build:pineworker"]);
  run("go", ["test", "-tags", "release_assets", "./internal/pineworkerassets", "-run", "Test"]);
  prepareReleaseArtifactPath();
  run("go", ["build", "-tags", "release_assets", "-o", releaseOut, "./cmd/jftrade-api"]);
  verifyReleaseArtifact();
} else {
  console.log("==> Skipping real PineTS process smoke and release asset build until pinets is installed");
}

if (blocked && !allowBlocked) {
  console.error("PineTS release acceptance is blocked; rerun with --allow-blocked only for migration progress checks.");
  process.exit(1);
}

console.log(blocked ? "PineTS release acceptance gates ran in blocked mode." : "PineTS release acceptance gates passed.");

function run(command, args, extraEnv = {}) {
  const printable = formatCommand(command, args, extraEnv);
  console.log(`==> ${printable}`);
  if (runLog) {
    writeFileSync(runLog, `${printable}\n`, { flag: "a" });
  }
  if (dryRun) {
    maybeWriteDryRunArtifact(command, args);
    return;
  }
  const status = spawnChecked(command, args, {
    env: { ...process.env, ...extraEnv },
  });
  if (status !== 0) {
    process.exit(status);
  }
}

function prepareReleaseArtifactPath() {
  mkdirSync(dirname(releaseOut), { recursive: true });
  rmSync(releaseOut, { force: true });
}

function verifyReleaseArtifact() {
  if (!existsSync(releaseOut) || statSync(releaseOut).size === 0) {
    console.error(`release artifact is missing or empty: ${releaseOut}`);
    process.exit(1);
  }
  if (process.platform !== "win32" && (statSync(releaseOut).mode & 0o111) === 0) {
    console.error(`release artifact is not executable: ${releaseOut}`);
    process.exit(1);
  }
}

function maybeWriteDryRunArtifact(command, args) {
  if (command !== "go" || args[0] !== "build") {
    return;
  }
  if (process.env.JFTRADE_PINETS_RELEASE_STUB_SKIP_ARTIFACT === "1") {
    return;
  }
  const outFlag = args.indexOf("-o");
  if (outFlag < 0 || !args[outFlag + 1]) {
    return;
  }
  const outPath = args[outFlag + 1];
  mkdirSync(dirname(outPath), { recursive: true });
  writeFileSync(outPath, "#!/bin/sh\nexit 0\n");
  chmodSync(outPath, 0o755);
}

function formatCommand(command, args, extraEnv) {
  const envPrefix = Object.entries(extraEnv).map(([key, value]) => `${key}=${value}`);
  return [...envPrefix, command, ...args].join(" ");
}
