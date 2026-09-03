import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";
import test from "node:test";

import { bindReleaseCandidateRehearsal } from "./bind-release-candidate-rehearsal.mjs";
import { inspectReleaseCandidateRehearsal } from "./check-release-candidate-rehearsal.mjs";

const commitSha = "a".repeat(40);
const candidateRef = "refs/heads/release/0.29.0-candidate";
const digest = (character) => `sha256:${character.repeat(64)}`;

function source() {
  const names = {
    "macos-arm64": "desktop-release-macos",
    "linux-x64": "desktop-release-linux",
    "windows-x64": "desktop-release-windows",
    "windows-arm64": "desktop-release-windows-arm64",
  };
  return {
    schemaVersion: "jftrade.release-candidate-rehearsal-source.v1",
    qualificationMode: "rehearsal",
    repository: "Dennishaha/jftrade",
    candidateRef,
    plannedReleaseTag: "v0.29.0",
    commitSha,
    workflowRun: {
      id: 201,
      attempt: 1,
      workflow: "desktop-release-evidence-source.yml",
      ref: candidateRef,
      commitSha,
    },
    sourceWorkflowRun: {
      id: 101,
      attempt: 1,
      workflow: "desktop-release.yml",
      ref: candidateRef,
      commitSha,
    },
    platforms: Object.fromEntries(Object.entries(names).map(([platform, name], index) => [
      platform,
      {
        status: "passed",
        artifact: { name, id: index + 1, digest: digest(String(index + 1)) },
        checks: Object.fromEntries([
          "package", "install", "firstStart", "upgrade", "databaseUpgrade",
          "runtimeSmoke", "uninstall", "backupRestore", "rollback", "zeroGo",
          "sbomZeroGo",
        ].map((check) => [check, "passed"])),
      },
    ])),
    limitations: {
      packageSigning: "not_run",
      notarization: "not_run",
      updaterSignature: "not_run",
      independentSecuritySignOff: "open",
    },
  };
}

const sourceArtifact = {
  name: "desktop-release-rehearsal-source",
  id: 301,
  digest: digest("f"),
};

test("binder creates a valid unsigned rehearsal receipt from the controlled source", () => {
  const receipt = bindReleaseCandidateRehearsal(source(), sourceArtifact);
  const result = inspectReleaseCandidateRehearsal(receipt, {
    expected: {
      candidateRef,
      plannedReleaseTag: "v0.29.0",
      commitSha,
      artifactName: sourceArtifact.name,
      artifactId: sourceArtifact.id,
      artifactDigest: sourceArtifact.digest,
    },
  });
  assert.equal(result.valid, true, result.errors.join("\n"));
  assert.equal(receipt.releaseQualified, false);
  assert.equal(receipt.status, "rehearsal_passed");
});

test("binder rejects cross-mode sources, drift, and missing native platforms", () => {
  const cases = [
    ["formal candidate mode", (value) => { value.qualificationMode = "candidate"; }, /schema or qualification mode/],
    ["artifact digest drift", (_value, artifact) => { artifact.digest = digest("e"); }, /does not match expected value/],
    ["missing platform", (value) => { delete value.platforms["windows-arm64"]; }, /windows-arm64 is required/],
  ];
  for (const [label, mutate, pattern] of cases) {
    const value = source();
    const artifact = structuredClone(sourceArtifact);
    mutate(value, artifact);
    assert.throws(
      () => bindReleaseCandidateRehearsal(value, artifact, {
        expected: { artifactDigest: sourceArtifact.digest },
      }),
      pattern,
      label,
    );
  }
});

test("CLI fails closed when the output receipt already exists", () => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-rehearsal-bind-"));
  const sourcePath = path.join(directory, "source.json");
  const outputPath = path.join(directory, "receipt.json");
  fs.writeFileSync(sourcePath, JSON.stringify(source()));
  fs.writeFileSync(outputPath, "occupied\n");
  const result = spawnSync(process.execPath, [
    "scripts/release/bind-release-candidate-rehearsal.mjs",
    "--source", sourcePath,
    "--output", outputPath,
    "--artifact-name", sourceArtifact.name,
    "--artifact-id", String(sourceArtifact.id),
    "--artifact-digest", sourceArtifact.digest,
  ], { encoding: "utf8" });
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /EEXIST|file already exists/i);
  assert.equal(fs.readFileSync(outputPath, "utf8"), "occupied\n");
});
