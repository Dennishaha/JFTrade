import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import {
  evaluateReleaseSource,
  main,
  RELEASE_SOURCE_ADMISSION_SCHEMA,
  validateReleaseConfiguration,
} from "./check-release-source-admission.mjs";

const commitSha = "a".repeat(40);

function passingOptions(overrides = {}) {
  return {
    sourceRef: "refs/heads/release/0.29.0-candidate",
    commitSha,
    plannedReleaseTag: "v0.29.0",
    ciStatus: "success",
    ciUrl: "https://github.com/Dennishaha/jftrade/actions/runs/123",
    runChecks: false,
    ...overrides,
  };
}

test("admits an exact source SHA only after Build & Test succeeds", () => {
  const result = evaluateReleaseSource(passingOptions());
  assert.equal(result.schemaVersion, RELEASE_SOURCE_ADMISSION_SCHEMA);
  assert.equal(result.status, "admitted");
  assert.equal(result.releaseQualified, false);
  assert.deepEqual(result.errors, []);
});

test("rejects source ref version SHA and CI mismatches", () => {
  for (const [field, value, expected] of [
    ["sourceRef", "release/0.29.0-candidate", /exact branch ref/],
    ["commitSha", "abc", /40-character/],
    ["plannedReleaseTag", "0.29.0", /vX.Y.Z/],
    ["ciStatus", "failure", /Build & Test/],
    ["ciUrl", "https://example.test/run/1", /GitHub Actions URL/],
  ]) {
    const result = evaluateReleaseSource(passingOptions({ [field]: value }));
    assert.equal(result.status, "blocked", field);
    assert.match(result.errors.join("\n"), expected, field);
  }
});

test("fails when zero-Go or contract checks fail", () => {
  let calls = 0;
  const result = evaluateReleaseSource(passingOptions({
    runChecks: true,
    runner: (_executable, args) => {
      calls += 1;
      return args[0].endsWith("check-zero-go.mjs")
        ? { status: 1, stdout: "", stderr: "Go artifact found" }
        : { status: 0, stdout: "ok", stderr: "" };
    },
  }));
  assert.equal(calls, 2);
  assert.equal(result.status, "blocked");
  assert.equal(result.checks.zeroGo, "failed");
  assert.match(result.errors.join("\n"), /Go artifact found/);
});

test("validates pinned release configuration without a migration manifest", () => {
  assert.deepEqual(validateReleaseConfiguration(), []);
});

test("repository-only CLI never claims release qualification", () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-source-admission-"));
  try {
    for (const file of ["package.json", "rust-toolchain.toml", "Cargo.lock"]){
      fs.copyFileSync(path.resolve(file), path.join(root, file));
    }
    fs.mkdirSync(path.join(root, "apps/desktop/src-tauri"), { recursive: true });
    fs.copyFileSync(
      path.resolve("apps/desktop/src-tauri/tauri.conf.json"),
      path.join(root, "apps/desktop/src-tauri/tauri.conf.json"),
    );
    const output = [];
    const original = console.log;
    console.log = (value) => output.push(String(value));
    try {
      assert.equal(main([
        "--repository-only",
        "--source-ref", "refs/heads/release/0.29.0-candidate",
        "--commit-sha", commitSha,
        "--planned-release-tag", "v0.29.0",
      ], { root, runChecks: false }), 0);
    } finally {
      console.log = original;
    }
    const receipt = JSON.parse(output.join("\n"));
    assert.equal(receipt.status, "repository_checks_passed");
    assert.equal(receipt.releaseQualified, false);
    assert.equal(receipt.requiredCheck.status, "not_verified_repository_only");
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});
