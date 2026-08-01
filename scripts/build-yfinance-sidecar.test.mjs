#!/usr/bin/env node

import assert from "node:assert/strict";
import {
  existsSync,
  mkdirSync,
  mkdtempSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import process from "node:process";
import { spawnSync } from "node:child_process";
import test from "node:test";

test("build stages one host-specific PyInstaller onedir asset", () => {
  const tempDir = mkdtempSync(join(tmpdir(), "jftrade-yfinance-assets-"));
  try {
    const outDir = join(tempDir, "assets", "bin");
    mkdirSync(outDir, { recursive: true });
    writeFileSync(join(outDir, ".gitkeep"), "");
    if (hostAssetName() !== hostAssetBase()) {
      writeFileSync(join(outDir, hostAssetName()), "stale");
    }
    mkdirSync(join(outDir, hostAssetBase()), { recursive: true });
    writeFileSync(join(outDir, hostAssetBase(), "stale"), "stale");
    writeFileSync(join(outDir, "yfinance-sidecar-other-platform"), "keep");
    writeFileSync(join(outDir, "unrelated-file"), "keep");

    const python = join(tempDir, "Python Runtime", "python");
    const result = runBuild({
      JFTRADE_YFINANCE_ASSET_BUILD_DRY_RUN: "1",
      JFTRADE_YFINANCE_ASSET_OUT_DIR: outDir,
      JFTRADE_YFINANCE_BUILD_PYTHON: python,
    });

    assert.equal(result.status, 0, result.stderr || result.stdout);
    assert.match(
      result.stdout,
      new RegExp(`Building yfinance sidecar -> .*${escapeRegex(hostAssetBase())}`),
    );
    assert.match(result.stdout, /DRY RUN .* -m PyInstaller --clean --noconfirm/);
    assert.match(result.stdout, /yfinance-sidecar\.spec/);
    assert.ok(result.stdout.includes(JSON.stringify(python)));
    assert.ok(existsSync(join(outDir, ".gitkeep")));
    assert.ok(existsSync(join(outDir, "unrelated-file")));
    assert.ok(existsSync(join(outDir, "yfinance-sidecar-other-platform")));
    assert.ok(!existsSync(join(outDir, hostAssetName())));
    assert.ok(!existsSync(join(outDir, hostAssetBase())));
  } finally {
    rmSync(tempDir, { recursive: true, force: true });
  }
});

test("build rejects a target that PyInstaller cannot produce on this host", () => {
  const crossGOOS = process.platform === "win32" ? "linux" : "windows";
  const result = runBuild({
    GOOS: crossGOOS,
    JFTRADE_YFINANCE_ASSET_BUILD_DRY_RUN: "1",
  });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /PyInstaller builds must run on the target platform/);
});

function runBuild(extraEnv) {
  return spawnSync(process.execPath, ["scripts/build-yfinance-sidecar.mjs"], {
    cwd: process.cwd(),
    env: { ...process.env, GOOS: "", GOARCH: "", ...extraEnv },
    encoding: "utf8",
  });
}

function hostAssetName() {
  return `${hostAssetBase()}${process.platform === "win32" ? ".exe" : ""}`;
}

function hostAssetBase() {
  const goos =
    process.platform === "win32" ? "windows" : process.platform;
  const goarch = process.arch === "x64" ? "amd64" : process.arch;
  return `yfinance-sidecar-${goos}-${goarch}`;
}

function escapeRegex(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}
