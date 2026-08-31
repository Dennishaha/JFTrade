import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import {
  inspectPostReleaseSmokeReports,
  writePostReleaseSmokeReport,
  POST_RELEASE_BINDING_SCHEMA,
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
    releaseBinding: releaseBinding(),
    externalRequired: [
      "native package installation, upgrade, uninstall and rollback on the matching runner",
      "code-signing and notarization verification",
    ],
  };
}

function releaseBinding(overrides = {}) {
  const commitSha = "a".repeat(40);
  return {
    schemaVersion: POST_RELEASE_BINDING_SCHEMA,
    releaseTag: "v1.2.3",
    releaseRef: "refs/tags/v1.2.3",
    commitSha,
    releaseRun: {
      id: 81234,
      attempt: 2,
      workflow: "desktop-release-qualification.yml",
      ref: "refs/tags/v1.2.3",
      commitSha,
    },
    artifacts: [
      { path: "release/JFTrade-v1.2.3-macos-arm64.dmg", sha256: "a".repeat(64) },
      { path: "release/JFTrade-v1.2.3-linux-x64.AppImage", sha256: "b".repeat(64) },
      { path: "release/JFTrade-v1.2.3-windows-x64-setup.exe", sha256: "c".repeat(64) },
      { path: "release/JFTrade-v1.2.3-windows-arm64-setup.exe", sha256: "d".repeat(64) },
    ],
    ...overrides,
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
  assert.equal(result.releaseBinding.schemaVersion, POST_RELEASE_BINDING_SCHEMA);
  assert.equal(result.releaseBinding.releaseTag, "v1.2.3");
  assert.equal(result.artifactDigests.length, 4);
});

test("requires a release binding and fails closed for legacy smoke reports", () => {
  const reports = allReports();
  delete reports[0].releaseBinding;
  const result = inspectPostReleaseSmokeReports({ reports });
  assert.equal(result.valid, false);
  assert.match(result.errors.join("\n"), /report\[0\]\.releaseBinding must be an object/);
});

test("rejects release binding tag, commit, run and artifact mismatches", () => {
  const cases = [
    ["tag", (binding) => { binding.releaseTag = "v9.9.9"; }, /releaseRef must match releaseTag/],
    ["commit", (binding) => { binding.commitSha = "b".repeat(40); }, /releaseRun\.commitSha does not match commitSha/],
    ["run id", (binding) => { binding.releaseRun.id = 81235; }, /does not match the other post-release reports/],
    ["run attempt", (binding) => { binding.releaseRun.attempt = 3; }, /does not match the other post-release reports/],
    ["workflow", (binding) => { binding.releaseRun.workflow = "other.yml"; }, /does not match the other post-release reports/],
    ["run ref", (binding) => { binding.releaseRun.ref = "refs/tags/v1.2.4"; }, /releaseRun\.ref does not match releaseRef/],
    ["run commit", (binding) => { binding.releaseRun.commitSha = "b".repeat(40); }, /releaseRun\.commitSha does not match commitSha/],
    ["artifact", (binding) => { binding.artifacts[0].sha256 = "f".repeat(64); }, /does not match the other post-release reports/],
  ];
  for (const [name, mutate, expectedError] of cases) {
    const reports = allReports();
    mutate(reports[1].releaseBinding);
    const result = inspectPostReleaseSmokeReports({ reports });
    assert.equal(result.valid, false, `${name} mismatch should fail`);
    assert.match(result.errors.join("\n"), expectedError, name);
  }
});

test("matches every report against an expected binding and rejects cross-tag expectations", () => {
  const expected = releaseBinding();
  const accepted = inspectPostReleaseSmokeReports({ reports: allReports(), expectedBinding: expected });
  assert.equal(accepted.valid, true);

  const wrong = releaseBinding({ releaseTag: "v1.2.4", releaseRef: "refs/tags/v1.2.4" });
  const rejected = inspectPostReleaseSmokeReports({ reports: allReports(), expectedBinding: wrong });
  assert.equal(rejected.valid, false);
  assert.match(rejected.errors.join("\n"), /does not match expected release binding/);
});

test("CLI verifies an external artifact digest binding before accepting reports", (context) => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-post-release-binding-"));
  context.after(() => fs.rmSync(directory, { recursive: true, force: true }));
  const bindingPath = path.join(directory, "artifact-digests.json");
  fs.writeFileSync(bindingPath, JSON.stringify(releaseBinding()));
  const reportPaths = [];
  for (const [index, report] of allReports().entries()) {
    const reportPath = path.join(directory, `report-${index}.json`);
    fs.writeFileSync(reportPath, JSON.stringify(report));
    reportPaths.push(reportPath);
  }
  const scriptPath = path.resolve("scripts/rust-migration/check-post-release-smoke.mjs");
  const args = reportPaths.flatMap((reportPath) => ["--report", reportPath]).concat([
    "--expected-artifact-digests", bindingPath,
    "--expected-tag", "v1.2.3",
    "--expected-ref", "refs/tags/v1.2.3",
    "--expected-commit", "a".repeat(40),
    "--expected-run-id", "81234",
    "--expected-attempt", "2",
    "--expected-workflow", "desktop-release-qualification.yml",
  ]);
  const accepted = spawnSync(process.execPath, [scriptPath, ...args], { encoding: "utf8" });
  assert.equal(accepted.status, 0, accepted.stderr);

  const wrongBinding = releaseBinding();
  wrongBinding.artifacts[0].sha256 = "f".repeat(64);
  fs.writeFileSync(bindingPath, JSON.stringify(wrongBinding));
  const rejected = spawnSync(process.execPath, [scriptPath, ...args], { encoding: "utf8" });
  assert.equal(rejected.status, 1);
  assert.match(rejected.stdout, /does not match expected release binding/);
});

test("CLI can combine a digest-only manifest with explicit release metadata", (context) => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-post-release-digests-"));
  context.after(() => fs.rmSync(directory, { recursive: true, force: true }));
  const binding = releaseBinding();
  const digestPath = path.join(directory, "digests.json");
  fs.writeFileSync(digestPath, JSON.stringify({ artifactDigests: binding.artifacts }));
  const reportPaths = allReports().map((report, index) => {
    const reportPath = path.join(directory, `report-${index}.json`);
    fs.writeFileSync(reportPath, JSON.stringify(report));
    return reportPath;
  });
  const scriptPath = path.resolve("scripts/rust-migration/check-post-release-smoke.mjs");
  const result = spawnSync(process.execPath, [
    scriptPath,
    ...reportPaths.flatMap((reportPath) => ["--report", reportPath]),
    "--expected-artifact-digests", digestPath,
    "--expected-tag", binding.releaseTag,
    "--expected-ref", binding.releaseRef,
    "--expected-commit", binding.commitSha,
    "--expected-run-id", String(binding.releaseRun.id),
    "--expected-attempt", String(binding.releaseRun.attempt),
    "--expected-workflow", binding.releaseRun.workflow,
  ], { encoding: "utf8" });
  assert.equal(result.status, 0, result.stderr);
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
