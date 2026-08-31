import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import {
  RELEASE_EVIDENCE_INPUTS_SCHEMA,
  TRUSTED_EVIDENCE_WORKFLOWS,
  TRUSTED_PAYLOAD_WORKFLOWS,
  TRUSTED_SOURCE_WORKFLOWS,
  validateExternalEvidenceManifest,
} from "./check-release-evidence-inputs.mjs";

const contracts = {
  "signed-updater-inputs": ["signed-updater", "jftrade.release.signed-updater.v2", {
    feed: { entryCount: 4 },
    artifacts: [{ archive: "JFTrade-v1.2.3.zip", archiveSha256: "1".repeat(64), signatureSha256: "2".repeat(64) }],
    publicKeyConfigured: true,
    publicKeySha256: "3".repeat(64),
    endpoint: "https://updates.example.test/feed.json",
  }],
  "sbom-provenance-inputs": ["sbom-provenance", "jftrade.release.sbom-provenance.v2", {
    subjects: [
      { platform: "macos-arm64", sha256: "a".repeat(64) },
      { platform: "linux-x64", sha256: "b".repeat(64) },
      { platform: "linux-x64", sha256: "c".repeat(64) },
      { platform: "linux-x64", sha256: "d".repeat(64) },
      { platform: "windows-x64", sha256: "e".repeat(64) },
      { platform: "windows-arm64", sha256: "f".repeat(64) },
    ],
  }],
  "rollback-artifact-pair": ["rollback-artifact", "jftrade.release.rollback-artifact.v2", {
    current: { version: "1.2.3", platforms: { "macos-arm64": {} }, updaterMetadata: {} },
    previous: { version: "1.2.2", platforms: { "macos-arm64": {} }, updaterMetadata: {} },
    rollbackInstructions: "rollback from 1.2.3 to 1.2.2",
  }],
  "backup-restore-drill": ["backup-restore", "jftrade.release.backup-restore.v2", {
    priorVersion: "1.2.2",
    nativeDrill: { status: "verified" },
  }],
  "security-review-inputs": ["security-review", "jftrade.release.security-review.v2", {
    independentReview: {
      independent: true,
      status: "signed_off",
      reviewer: "independent-reviewer",
      approvedAt: "2026-08-31T00:00:00Z",
    },
  }],
};

function digest(text) {
  return createHash("sha256").update(text).digest("hex");
}

function createFixture() {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-evidence-v2-"));
  const artifact = { name: "release-evidence-v2", id: 9001, digest: `sha256:${"1".repeat(64)}` };
  const binding = {
    repository: "example/jftrade",
    releaseRef: "refs/tags/v1.2.3",
    ref: "refs/tags/v1.2.3",
    commitSha: "a".repeat(40),
    workflow: TRUSTED_EVIDENCE_WORKFLOWS[0],
    runId: 81234,
    attempt: 2,
    artifact,
  };
  const sourceBinding = {
    ...binding,
    ref: "refs/heads/release-evidence/v1.2.3",
    workflow: TRUSTED_SOURCE_WORKFLOWS[0],
    runId: 81235,
    attempt: 1,
    artifact: { name: "payload-evidence", id: 9002, digest: `sha256:${"2".repeat(64)}` },
  };
  const evidence = {};
  for (const [id, [kind, schemaVersion, extra]] of Object.entries(contracts)) {
    const report = { schemaVersion, status: id === "security-review-inputs" ? "independent_review_signed_off" : "verified", binding: sourceBinding, ...extra };
    const relative = `reports/${id}.json`;
    const text = `${JSON.stringify(report)}\n`;
    fs.mkdirSync(path.dirname(path.join(root, relative)), { recursive: true });
    fs.writeFileSync(path.join(root, relative), text);
    evidence[id] = {
      kind,
      status: "passed",
      files: [{
        path: relative,
        sha256: digest(text),
        size: Buffer.byteLength(text),
        kind,
        schemaVersion,
      }],
    };
  }
  return {
    root,
    artifact,
    binding,
    manifest: {
      $schema: "./release-evidence-inputs.schema.json",
      schemaVersion: RELEASE_EVIDENCE_INPUTS_SCHEMA,
      repository: binding.repository,
      releaseRef: binding.releaseRef,
      ref: binding.ref,
      commitSha: binding.commitSha,
      workflow: binding.workflow,
      runId: binding.runId,
      attempt: binding.attempt,
      artifact,
      sourceBinding,
      evidence,
    },
    expected: { ...binding, sourceBinding },
    releaseArtifactDigests: {
      "macos-arm64": "a".repeat(64),
      "linux-x64": ["b".repeat(64), "c".repeat(64), "d".repeat(64)],
      "windows-x64": "e".repeat(64),
      "windows-arm64": "f".repeat(64),
    },
  };
}

function inspect(value) {
  return validateExternalEvidenceManifest(value.manifest, {
    baseDirectory: value.root,
    expected: value.expected,
    expectedArtifactMetadata: value.artifact,
    releaseArtifactDigests: value.releaseArtifactDigests,
  });
}

test("accepts a fully bound v2 manifest and report set", (context) => {
  const value = createFixture();
  context.after(() => fs.rmSync(value.root, { recursive: true, force: true }));
  const result = inspect(value);
  assert.equal(result.valid, true, result.errors.join("; "));
});

test("rejects arbitrary text and local checker markers", (context) => {
  const value = createFixture();
  context.after(() => fs.rmSync(value.root, { recursive: true, force: true }));
  const file = value.manifest.evidence["signed-updater-inputs"].files[0];
  fs.writeFileSync(path.join(value.root, file.path), "external_release_runner_evidence_required\n");
  const result = inspect(value);
  assert.equal(result.valid, false);
  assert.match(result.errors.join("\n"), /JSON report|local checker marker/);
});

test("requires semantic updater and rollback evidence fields", (context) => {
  const value = createFixture();
  context.after(() => fs.rmSync(value.root, { recursive: true, force: true }));
  const updater = structuredClone(value);
  delete updater.manifest.evidence["signed-updater-inputs"].files[0].schemaVersion;
  fs.writeFileSync(
    path.join(value.root, "reports/signed-updater-inputs.json"),
    JSON.stringify({ schemaVersion: "jftrade.release.signed-updater.v2", status: "verified", binding: value.manifest.sourceBinding }),
  );
  updater.manifest.evidence["signed-updater-inputs"].files[0].sha256 = digest(
    fs.readFileSync(path.join(value.root, "reports/signed-updater-inputs.json")),
  );
  updater.manifest.evidence["signed-updater-inputs"].files[0].size = fs.statSync(
    path.join(value.root, "reports/signed-updater-inputs.json"),
  ).size;
  assert.match(inspect(updater).errors.join("\n"), /schemaVersion|feed|artifacts/);

  const rollback = structuredClone(value);
  const rollbackReport = rollback.manifest.evidence["rollback-artifact-pair"].files[0];
  fs.writeFileSync(path.join(value.root, rollbackReport.path), JSON.stringify({
    schemaVersion: "jftrade.release.rollback-artifact.v2",
    status: "verified",
    binding: value.manifest.sourceBinding,
    current: { version: "1.2.3" },
    previous: { version: "1.2.2" },
  }));
  rollbackReport.sha256 = digest(fs.readFileSync(path.join(value.root, rollbackReport.path)));
  rollbackReport.size = fs.statSync(path.join(value.root, rollbackReport.path)).size;
  assert.match(inspect(rollback).errors.join("\n"), /platform packages|rollbackInstructions/);
});

test("rejects foreign run, ref, commit, artifact id and digest bindings", (context) => {
  const value = createFixture();
  context.after(() => fs.rmSync(value.root, { recursive: true, force: true }));
  for (const [field, replacement] of [
    ["runId", 81235],
    ["releaseRef", "refs/tags/v9.9.9"],
    ["ref", "refs/tags/v9.9.9"],
    ["commitSha", "b".repeat(40)],
  ]) {
    const candidate = structuredClone(value);
    candidate.manifest[field] = replacement;
    assert.equal(inspect(candidate).valid, false, field);
  }
  const foreignArtifact = structuredClone(value);
  foreignArtifact.manifest.artifact = { ...foreignArtifact.manifest.artifact, id: foreignArtifact.manifest.artifact.id + 1 };
  assert.equal(inspect(foreignArtifact).valid, false);
  const foreignDigest = structuredClone(value);
  foreignDigest.manifest.artifact = { ...foreignDigest.manifest.artifact, digest: `sha256:${"2".repeat(64)}` };
  assert.equal(inspect(foreignDigest).valid, false);
});

test("enforces trusted workflow and rejects shell injection in names and paths", (context) => {
  const value = createFixture();
  context.after(() => fs.rmSync(value.root, { recursive: true, force: true }));
  const workflow = structuredClone(value);
  workflow.manifest.workflow = "desktop-release-evidence.yml; touch pwned";
  assert.match(inspect(workflow).errors.join("\n"), /trusted evidence producer/);
  const artifact = structuredClone(value);
  artifact.manifest.artifact.name = "evidence$(touch pwned)";
  assert.equal(inspect(artifact).valid, false);
  const traversal = structuredClone(value);
  traversal.manifest.evidence["signed-updater-inputs"].files[0].path = "../outside.json";
  assert.match(inspect(traversal).errors.join("\n"), /relative POSIX|parent path/);
});

test("accepts only repository source workflows for the source binding", (context) => {
  const value = createFixture();
  context.after(() => fs.rmSync(value.root, { recursive: true, force: true }));
  assert.ok(TRUSTED_SOURCE_WORKFLOWS.includes("desktop-release-evidence-intake.yml"));
  assert.ok(!TRUSTED_SOURCE_WORKFLOWS.includes("desktop-release-evidence-payload.yml"));
  const foreign = structuredClone(value);
  foreign.manifest.sourceBinding.workflow = "external-release-evidence.yml";
  for (const report of Object.values(foreign.manifest.evidence)) {
    const file = report.files[0];
    fs.writeFileSync(path.join(value.root, file.path), JSON.stringify({
      ...JSON.parse(fs.readFileSync(path.join(value.root, file.path), "utf8")),
      binding: foreign.manifest.sourceBinding,
    }));
    file.sha256 = digest(fs.readFileSync(path.join(value.root, file.path)));
    file.size = fs.statSync(path.join(value.root, file.path)).size;
  }
  assert.match(inspect(foreign).errors.join("\n"), /trusted evidence producer/);
});

test("compares the canonical nested expected source binding", (context) => {
  const value = createFixture();
  context.after(() => fs.rmSync(value.root, { recursive: true, force: true }));
  value.expected.sourceBinding = {
    ...value.expected.sourceBinding,
    runId: value.expected.sourceBinding.runId + 1,
  };
  const result = inspect(value);
  assert.equal(result.valid, false);
  assert.match(result.errors.join("\n"), /manifest\.sourceBinding does not match expected source binding/);
});

test("rejects symlink escapes, path or basename collisions, and schema extras", (context) => {
  const value = createFixture();
  context.after(() => fs.rmSync(value.root, { recursive: true, force: true }));
  const linkPath = path.join(value.root, "reports/link.json");
  try {
    fs.symlinkSync(path.join(value.root, "reports/signed-updater-inputs.json"), linkPath);
  } catch {
    return;
  }
  const symlink = structuredClone(value);
  symlink.manifest.evidence["signed-updater-inputs"].files[0].path = "reports/link.json";
  assert.match(inspect(symlink).errors.join("\n"), /symlink/);
  const collision = structuredClone(value);
  collision.manifest.evidence["sbom-provenance-inputs"].files[0].path = "reports/signed-updater-inputs.json";
  assert.match(inspect(collision).errors.join("\n"), /collides/);
  const extra = structuredClone(value);
  extra.manifest.unexpected = true;
  assert.match(inspect(extra).errors.join("\n"), /not allowed/);
  const extraFile = structuredClone(value);
  extraFile.manifest.evidence["signed-updater-inputs"].files[0].unexpected = true;
  assert.match(inspect(extraFile).errors.join("\n"), /not allowed/);
});
