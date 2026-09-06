#!/usr/bin/env node
import { chmodSync, copyFileSync, existsSync, mkdirSync, rmSync, statSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
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
const releaseOut = process.env.JFTRADE_PINETS_RELEASE_OUT || "dist/jftrade-api-rust";
const rustReleaseArtifact = process.env.JFTRADE_PINETS_RELEASE_RUST_ARTIFACT
  || join(process.env.CARGO_TARGET_DIR || "target", "release", "jftrade-api-rust");
const dryRun = process.env.JFTRADE_PINETS_RELEASE_DRY_RUN === "1";

let blocked = false;
if (!checkPinetsPackageAndLicense({ dryRun, verifyWorkspaceVisible: true })) {
  blocked = true;
}

run("node", ["scripts/quality/cargo-nextest.mjs", "run", "-p", "jftrade-integration-pine", "--all-targets"]);
run("node", ["scripts/quality/cargo-nextest.mjs", "run", "-p", "jftrade-engine", "--test", "strategy_pine_mcp_contract"]);
run("pnpm", ["run", "test:pineworker"]);
run("pnpm", ["run", "typecheck:pineworker"]);
run("pnpm", ["run", "check:pinets-compliance"]);
run("pnpm", ["run", "test:web"]);
run("pnpm", ["run", "typecheck:web"]);
run("pnpm", ["run", "build:frontend-assets"]);
run("git", ["diff", "--check"]);

if (!blocked) {
  run("pnpm", ["run", "build:pineworker"]);
  run("pnpm", ["run", "smoke:pinets-backtest"]);
  run("pnpm", ["run", "build:marketdata-sidecar"]);
  run("pnpm", ["run", "smoke:marketdata-sidecar"]);
  prepareReleaseArtifactPath();
  run("cargo", ["build", "--release", "-p", "jftrade-engine", "--bin", "jftrade-api-rust"]);
  copyRustReleaseArtifact();
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

function copyRustReleaseArtifact() {
  if (dryRun) {
    return;
  }
  if (!existsSync(rustReleaseArtifact)) {
    console.error(`Rust API release artifact is missing: ${rustReleaseArtifact}`);
    process.exit(1);
  }
  copyFileSync(rustReleaseArtifact, releaseOut);
  if (process.platform !== "win32") {
    chmodSync(releaseOut, 0o755);
  }
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
  if (command !== "cargo" || args[0] !== "build" || !args.includes("--release")) {
    return;
  }
  if (process.env.JFTRADE_PINETS_RELEASE_STUB_SKIP_ARTIFACT === "1") {
    return;
  }
  mkdirSync(dirname(releaseOut), { recursive: true });
  writeFileSync(releaseOut, "#!/bin/sh\nexit 0\n");
  chmodSync(releaseOut, 0o755);
}

function formatCommand(command, args, extraEnv) {
  const envPrefix = Object.entries(extraEnv).map(([key, value]) => `${key}=${value}`);
  return [...envPrefix, command, ...args].join(" ");
}
