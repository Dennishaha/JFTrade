#!/usr/bin/env node

import {
  chmodSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  rmSync,
  statSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import process from "node:process";
import { spawnSync } from "node:child_process";

import { materializeDirectorySymlinks } from "./lib/materialize-directory-symlinks.mjs";

const rootDir = resolve(import.meta.dirname, "..");
const sidecarDir = join(rootDir, "workers/yfinance-sidecar");
const specPath = join(sidecarDir, "yfinance-sidecar.spec");
const hostGOOS = resolveGOOS(process.platform);
const hostGOARCH = resolveGOARCH(process.arch);
const targetGOOS = normalizedTarget("GOOS", hostGOOS);
const targetGOARCH = normalizedTarget("GOARCH", hostGOARCH);

assertSupportedTarget(targetGOOS, targetGOARCH);
if (targetGOOS !== hostGOOS || targetGOARCH !== hostGOARCH) {
  fail(
    `PyInstaller builds must run on the target platform ` +
      `(${targetGOOS}/${targetGOARCH} requested on ${hostGOOS}/${hostGOARCH}).`,
  );
}

const outDir = resolve(
  process.env.JFTRADE_YFINANCE_ASSET_OUT_DIR?.trim() ||
    join(rootDir, "internal/yfinanceassets/assets/bin"),
);
const virtualenvPython =
  process.platform === "win32"
    ? join(sidecarDir, ".venv", "Scripts", "python.exe")
    : join(sidecarDir, ".venv", "bin", "python");
const python =
  process.env.JFTRADE_YFINANCE_BUILD_PYTHON?.trim() ||
  (existsSync(virtualenvPython)
    ? virtualenvPython
    : process.platform === "win32"
      ? "python"
      : "python3");
const dryRun = process.env.JFTRADE_YFINANCE_ASSET_BUILD_DRY_RUN === "1";
const binaryBase = `yfinance-sidecar-${targetGOOS}-${targetGOARCH}`;
const outputName = `${binaryBase}${targetGOOS === "windows" ? ".exe" : ""}`;
const outputDir = join(outDir, binaryBase);
const outputPath = join(outputDir, outputName);
const legacyOutputPath = join(outDir, outputName);
const tempDir = mkdtempSync(join(tmpdir(), "jftrade-yfinance-build-"));

try {
  mkdirSync(outDir, { recursive: true });
  rmSync(outputDir, { recursive: true, force: true });
  rmSync(legacyOutputPath, { force: true });

  const args = [
    "-m",
    "PyInstaller",
    "--clean",
    "--noconfirm",
    "--distpath",
    outDir,
    "--workpath",
    join(tempDir, "work"),
    specPath,
  ];
  console.log(`Building yfinance sidecar -> ${outputDir}`);
  if (dryRun) {
    console.log(`DRY RUN ${formatCommand(python, args)}`);
  } else {
    run(python, args, {
      ...process.env,
      JFTRADE_YFINANCE_BINARY_NAME: binaryBase,
    });
    const materializedLinks = materializeDirectorySymlinks(outputDir);
    verifyOutput(outputPath, targetGOOS);
    console.log(
      `Materialized ${materializedLinks} bundle symlink(s) as regular files`,
    );
    console.log(`Staged yfinance sidecar at ${outputPath}`);
  }
} finally {
  rmSync(tempDir, { recursive: true, force: true });
}

function run(command, args, env) {
  const result = spawnSync(command, args, {
    cwd: sidecarDir,
    env,
    stdio: "inherit",
    shell: process.platform === "win32" && /\.(?:cmd|bat)$/i.test(command),
  });
  if (result.error) {
    throw result.error;
  }
  if (result.status !== 0) {
    throw new Error(
      `PyInstaller exited with status ${result.status ?? "unknown"}`,
    );
  }
}

function verifyOutput(path, goos) {
  const outputDir = statSync(join(path, ".."), { throwIfNoEntry: false });
  const output = statSync(path, { throwIfNoEntry: false });
  if (!outputDir?.isDirectory() || !output?.isFile() || output.size === 0) {
    fail(`PyInstaller onedir output is missing or empty: ${path}`);
  }
  if (goos !== "windows") {
    chmodSync(path, 0o755);
  }
}

function normalizedTarget(name, fallback) {
  return String(process.env[name] || fallback).trim().toLowerCase();
}

function resolveGOOS(platform) {
  const aliases = { darwin: "darwin", linux: "linux", win32: "windows" };
  return aliases[platform] || platform;
}

function resolveGOARCH(arch) {
  const aliases = { arm64: "arm64", x64: "amd64" };
  return aliases[arch] || arch;
}

function assertSupportedTarget(goos, goarch) {
  if (!["darwin", "linux", "windows"].includes(goos)) {
    fail(`Unsupported yfinance sidecar target OS: ${goos}`);
  }
  if (!["amd64", "arm64"].includes(goarch)) {
    fail(`Unsupported yfinance sidecar target architecture: ${goarch}`);
  }
}

function formatCommand(command, args) {
  return [command, ...args].map(quoteArgument).join(" ");
}

function quoteArgument(value) {
  const text = String(value);
  return /^[A-Za-z0-9_./:\\-]+$/.test(text)
    ? text
    : JSON.stringify(text);
}

function fail(message) {
  throw new Error(message);
}
