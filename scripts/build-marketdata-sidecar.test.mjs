#!/usr/bin/env node

import assert from "node:assert/strict";
import {
  chmodSync,
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
  const tempDir = mkdtempSync(join(tmpdir(), "jftrade-marketdata-assets-"));
  try {
    const outDir = join(tempDir, "assets", "bin");
    mkdirSync(outDir, { recursive: true });
    writeFileSync(join(outDir, ".gitkeep"), "");
    if (hostAssetName() !== hostAssetBase()) {
      writeFileSync(join(outDir, hostAssetName()), "stale");
    }
    mkdirSync(join(outDir, hostAssetBase()), { recursive: true });
    writeFileSync(join(outDir, hostAssetBase(), "stale"), "stale");
    writeFileSync(join(outDir, "marketdata-sidecar-other-platform"), "keep");
    writeFileSync(join(outDir, "unrelated-file"), "keep");

    const python = createFakePython(tempDir, "3.14.9");
    const result = runBuild({
      JFTRADE_MARKETDATA_ASSET_BUILD_DRY_RUN: "1",
      JFTRADE_MARKETDATA_ASSET_OUT_DIR: outDir,
      JFTRADE_MARKETDATA_BUILD_PYTHON: python,
    });

    assert.equal(result.status, 0, result.stderr || result.stdout);
    assert.match(
      result.stdout,
      new RegExp(`Building market-data sidecar -> .*${escapeRegex(hostAssetBase())}`),
    );
    assert.match(result.stdout, /DRY RUN .* -m PyInstaller --clean --noconfirm/);
    assert.match(result.stdout, /marketdata-sidecar\.spec/);
    assert.ok(result.stdout.includes(JSON.stringify(python)));
    assert.ok(existsSync(join(outDir, ".gitkeep")));
    assert.ok(existsSync(join(outDir, "unrelated-file")));
    assert.ok(existsSync(join(outDir, "marketdata-sidecar-other-platform")));
    assert.ok(!existsSync(join(outDir, hostAssetName())));
    assert.ok(!existsSync(join(outDir, hostAssetBase())));
  } finally {
    rmSync(tempDir, { recursive: true, force: true });
  }
});

for (const version of ["3.13.9", "3.15.0"]) {
  test(`build rejects CPython ${version} without deleting staged assets`, () => {
    const tempDir = mkdtempSync(join(tmpdir(), "jftrade-marketdata-version-"));
    try {
      const outDir = join(tempDir, "assets", "bin");
      const stagedDir = join(outDir, hostAssetBase());
      mkdirSync(stagedDir, { recursive: true });
      const sentinel = join(stagedDir, "keep-existing");
      writeFileSync(sentinel, "keep");
      const result = runBuild({
        JFTRADE_MARKETDATA_ASSET_BUILD_DRY_RUN: "1",
        JFTRADE_MARKETDATA_ASSET_OUT_DIR: outDir,
        JFTRADE_MARKETDATA_BUILD_PYTHON: createFakePython(tempDir, version),
      });

      assert.notEqual(result.status, 0);
      assert.match(result.stderr, /require CPython 3\.14\.x/);
      assert.match(result.stderr, new RegExp(version.replaceAll(".", "\\.")));
      assert.ok(existsSync(sentinel));
    } finally {
      rmSync(tempDir, { recursive: true, force: true });
    }
  });
}

test("build rejects invalid and unavailable Python probes", () => {
  const tempDir = mkdtempSync(join(tmpdir(), "jftrade-marketdata-probe-"));
  try {
    const invalid = createFakePython(tempDir, "invalid", "not-json");
    const invalidResult = runBuild({
      JFTRADE_MARKETDATA_ASSET_BUILD_DRY_RUN: "1",
      JFTRADE_MARKETDATA_BUILD_PYTHON: invalid,
    });
    assert.notEqual(invalidResult.status, 0);
    assert.match(invalidResult.stderr, /Could not parse market-data build Python version/);

    const missingResult = runBuild({
      JFTRADE_MARKETDATA_ASSET_BUILD_DRY_RUN: "1",
      JFTRADE_MARKETDATA_BUILD_PYTHON: join(tempDir, "missing-python"),
    });
    assert.notEqual(missingResult.status, 0);
    assert.match(missingResult.stderr, /Could not execute market-data build Python/);
  } finally {
    rmSync(tempDir, { recursive: true, force: true });
  }
});

test("build rejects a target that PyInstaller cannot produce on this host", () => {
  const crossGOOS = process.platform === "win32" ? "linux" : "windows";
  const result = runBuild({
    GOOS: crossGOOS,
    JFTRADE_MARKETDATA_ASSET_BUILD_DRY_RUN: "1",
  });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /PyInstaller builds must run on the target platform/);
});

test("legacy yfinance build variables remain lower-priority aliases", () => {
  const tempDir = mkdtempSync(join(tmpdir(), "jftrade-marketdata-legacy-env-"));
  try {
    const genericOut = join(tempDir, "generic", "bin");
    const legacyOut = join(tempDir, "legacy", "bin");
    const python = createFakePython(tempDir, "3.14.9");
    const legacy = runBuild({
      JFTRADE_MARKETDATA_ASSET_BUILD_DRY_RUN: "",
      JFTRADE_MARKETDATA_ASSET_OUT_DIR: "",
      JFTRADE_MARKETDATA_BUILD_PYTHON: "",
      JFTRADE_YFINANCE_ASSET_BUILD_DRY_RUN: "1",
      JFTRADE_YFINANCE_ASSET_OUT_DIR: legacyOut,
      JFTRADE_YFINANCE_BUILD_PYTHON: python,
    });
    assert.equal(legacy.status, 0, legacy.stderr || legacy.stdout);
    assert.ok(legacy.stdout.includes(legacyOut));

    const preferred = runBuild({
      JFTRADE_MARKETDATA_ASSET_BUILD_DRY_RUN: "1",
      JFTRADE_MARKETDATA_ASSET_OUT_DIR: genericOut,
      JFTRADE_MARKETDATA_BUILD_PYTHON: python,
      JFTRADE_YFINANCE_ASSET_BUILD_DRY_RUN: "1",
      JFTRADE_YFINANCE_ASSET_OUT_DIR: legacyOut,
      JFTRADE_YFINANCE_BUILD_PYTHON: python,
    });
    assert.equal(preferred.status, 0, preferred.stderr || preferred.stdout);
    assert.ok(preferred.stdout.includes(genericOut));
    assert.ok(!preferred.stdout.includes(legacyOut));
  } finally {
    rmSync(tempDir, { recursive: true, force: true });
  }
});

function runBuild(extraEnv) {
  return spawnSync(process.execPath, ["scripts/build-marketdata-sidecar.mjs"], {
    cwd: process.cwd(),
    env: { ...process.env, GOOS: "", GOARCH: "", ...extraEnv },
    encoding: "utf8",
  });
}

function createFakePython(root, version, output) {
  const directory = join(root, `Python Runtime ${version}`);
  mkdirSync(directory, { recursive: true });
  if (process.platform === "win32") {
    const path = join(directory, "python.cmd");
    writeFileSync(
      path,
      `@echo off\r\necho ${output ?? `{\"implementation\":\"cpython\",\"version\":[${version.split(".").join(",")}]}`}\r\n`,
    );
    return path;
  }
  const path = join(directory, "python");
  writeFileSync(
    path,
    `#!/bin/sh\nprintf '%s\\n' '${output ?? `{\"implementation\":\"cpython\",\"version\":[${version.split(".").join(",")}]}`}'\n`,
  );
  chmodSync(path, 0o755);
  return path;
}

function hostAssetName() {
  return `${hostAssetBase()}${process.platform === "win32" ? ".exe" : ""}`;
}

function hostAssetBase() {
  const goos =
    process.platform === "win32" ? "windows" : process.platform;
  const goarch = process.arch === "x64" ? "amd64" : process.arch;
  return `marketdata-sidecar-${goos}-${goarch}`;
}

function escapeRegex(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}
