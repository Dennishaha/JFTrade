import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import {
  inspectPostReleaseSmokeReports,
  writePostReleaseSmokeReport,
} from "./check-post-release-smoke.mjs";

const targets = [
  ["darwin", "arm64", "jftrade-desktop"],
  ["linux", "amd64", "jftrade-desktop"],
  ["windows", "amd64", "jftrade-desktop.exe"],
  ["windows", "arm64", "jftrade-desktop.exe"],
];

function smokeReport(platform, architecture, executable) {
  const windows = platform === "windows" || platform.startsWith("windows-");
  return {
    schemaVersion: "jftrade.tauri-runtime-smoke.v1",
    target: { platform, architecture },
    executable: `/release/${executable}`,
    scope: [
      "packaged runtime resource presence and startup integrity validation",
      "unauthenticated API fail-closed response",
      "startup and graceful shutdown with retained child cleanup",
    ],
    readiness: { status: 401, errorCode: "WEB_AUTH_REQUIRED", readyMs: 120 },
    shutdown: { code: 0, signal: null, shutdownMs: 40 },
    orphanCheck: windows ? "not-applicable-on-windows" : "passed",
    externalRequired: [
      "native package installation, upgrade, uninstall and rollback on the matching runner",
      "code-signing and notarization verification",
    ],
  };
}

function allReports() {
  return targets.map(([platform, architecture, executable]) =>
    smokeReport(platform, architecture, executable));
}

test("validates all four runtime smoke targets without qualifying release evidence", () => {
  const result = inspectPostReleaseSmokeReports({ reports: allReports() });
  assert.equal(result.valid, true);
  assert.equal(result.status, "inputs_verified");
  assert.equal(result.releaseQualified, false);
  assert.equal(result.releaseQualification, "external_post_release_observation_required");
  assert.deepEqual(result.platforms.map((entry) => entry.platform), [
    "macos-arm64",
    "linux-x64",
    "windows-x64",
    "windows-arm64",
  ]);
  assert.deepEqual(result.missingPlatforms, []);
});

test("accepts canonical target names emitted by a normalized runner", () => {
  const reports = [
    smokeReport("macos-arm64", "arm64", "jftrade-desktop"),
    smokeReport("linux-x64", "amd64", "jftrade-desktop"),
    smokeReport("windows-x64", "amd64", "jftrade-desktop.exe"),
    smokeReport("windows-arm64", "arm64", "jftrade-desktop.exe"),
  ];
  const result = inspectPostReleaseSmokeReports({ reports });
  assert.equal(result.valid, true);
});

test("rejects missing readiness fields and invalid shutdown status", () => {
  const reports = allReports();
  delete reports[0].readiness.errorCode;
  reports[1].shutdown.code = 1;
  const result = inspectPostReleaseSmokeReports({ reports });
  assert.equal(result.valid, false);
  assert.match(result.errors.join("\n"), /readiness\.errorCode must be WEB_AUTH_REQUIRED/);
  assert.match(result.errors.join("\n"), /shutdown\.code must be 0/);
});

test("rejects wrong orphan policy for a Windows target", () => {
  const reports = allReports();
  reports[2].orphanCheck = "passed";
  const result = inspectPostReleaseSmokeReports({ reports });
  assert.equal(result.valid, false);
  assert.match(result.errors.join("\n"), /orphanCheck must be not-applicable-on-windows/);
});

test("rejects duplicate and missing platform reports", () => {
  const reports = allReports().slice(0, 3);
  reports[1] = reports[0];
  const result = inspectPostReleaseSmokeReports({ reports });
  assert.equal(result.valid, false);
  assert.match(result.errors.join("\n"), /duplicate post-release smoke target: macos-arm64/);
  assert.match(result.errors.join("\n"), /missing post-release smoke target\(s\): linux-x64, windows-arm64/);
});

test("rejects unknown target and stale fixture-shaped reports", () => {
  const reports = allReports();
  reports[0].target = { platform: "freebsd", architecture: "amd64" };
  reports[1].scope = ["synthetic fixture"];
  const result = inspectPostReleaseSmokeReports({ reports });
  assert.equal(result.valid, false);
  assert.match(result.errors.join("\n"), /not a supported Tauri smoke target/);
  assert.match(result.errors.join("\n"), /scope is missing/);
});

test("writes an input report and leaves the closeout manifest untouched", (context) => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-post-release-smoke-"));
  context.after(() => fs.rmSync(directory, { recursive: true, force: true }));
  const manifestPath = path.resolve("tests/fixtures/rust-migration/stage9/closeout-evidence.json");
  const before = fs.readFileSync(manifestPath);
  const outputPath = path.join(directory, "reports/post-release.json");
  const result = writePostReleaseSmokeReport(outputPath, { reports: allReports() });
  assert.equal(result.valid, true);
  assert.equal(JSON.parse(fs.readFileSync(outputPath, "utf8")).releaseQualified, false);
  assert.deepEqual(fs.readFileSync(manifestPath), before);
});

test("CLI writes an incomplete report instead of treating one local target as four", (context) => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-post-release-smoke-cli-"));
  context.after(() => fs.rmSync(directory, { recursive: true, force: true }));
  const inputPath = path.join(directory, "mac.json");
  const outputPath = path.join(directory, "post-release.json");
  fs.writeFileSync(inputPath, JSON.stringify(smokeReport("darwin", "arm64", "jftrade-desktop")));
  const scriptPath = path.resolve("scripts/rust-migration/check-post-release-smoke.mjs");
  const result = spawnSync(process.execPath, [
    scriptPath,
    "--report", inputPath,
    "--output", outputPath,
  ], { encoding: "utf8" });
  assert.equal(result.status, 1);
  const report = JSON.parse(fs.readFileSync(outputPath, "utf8"));
  assert.equal(report.status, "incomplete_inputs");
  assert.deepEqual(report.missingPlatforms, ["linux-x64", "windows-x64", "windows-arm64"]);
});
