import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import {
  PAYLOAD_BINDING_SCHEMA,
  RELEASE_EVIDENCE_WORKFLOW,
  bindReleaseEvidence,
} from "./bind-release-evidence.mjs";
import { validateExternalEvidenceManifest } from "./check-release-evidence-inputs.mjs";

const releaseRef = "refs/tags/v1.2.3";
const releaseCommit = "a".repeat(40);
const payloadArtifact = {
  name: "platform-evidence-payload",
  id: 7711,
  digest: `sha256:${"b".repeat(64)}`,
};
const payloadRun = {
  id: 6601,
  attempt: 3,
  workflow: "platform-evidence.yml",
  ref: "refs/heads/release-evidence/v1.2.3",
  commitSha: releaseCommit,
};

const reports = {
  "signed-updater-inputs": {
    schemaVersion: "jftrade.release.signed-updater.v2",
    status: "verified",
    feed: { entryCount: 4 },
    artifacts: [{ archive: "JFTrade-v1.2.3.zip", archiveSha256: "1".repeat(64), signatureSha256: "2".repeat(64) }],
    publicKeyConfigured: true,
    publicKeySha256: "3".repeat(64),
    endpoint: "https://updates.example.test/feed.json",
  },
  "sbom-provenance-inputs": {
    schemaVersion: "jftrade.release.sbom-provenance.v2",
    status: "verified",
    subjects: [
      { platform: "macos-arm64", sha256: "a".repeat(64) },
      { platform: "linux-x64", sha256: "b".repeat(64) },
      { platform: "linux-x64", sha256: "c".repeat(64) },
      { platform: "linux-x64", sha256: "d".repeat(64) },
      { platform: "windows-x64", sha256: "e".repeat(64) },
      { platform: "windows-arm64", sha256: "f".repeat(64) },
    ],
  },
  "rollback-artifact-pair": {
    schemaVersion: "jftrade.release.rollback-artifact.v2",
    status: "verified",
    current: { version: "1.2.3", platforms: { "macos-arm64": {} }, updaterMetadata: {} },
    previous: { version: "1.2.2", platforms: { "macos-arm64": {} }, updaterMetadata: {} },
    rollbackInstructions: "rollback from 1.2.3 to 1.2.2",
  },
  "backup-restore-drill": {
    schemaVersion: "jftrade.release.backup-restore.v2",
    status: "verified",
    priorVersion: "1.2.2",
    nativeDrill: { status: "verified" },
  },
  "security-review-inputs": {
    schemaVersion: "jftrade.release.security-review.v2",
    status: "independent_review_signed_off",
    independentReview: {
      independent: true,
      status: "signed_off",
      reviewer: "independent-reviewer",
      approvedAt: "2026-08-31T00:00:00Z",
    },
  },
};

function sha256(filePath) {
  return createHash("sha256").update(fs.readFileSync(filePath)).digest("hex");
}

function fixture() {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-bind-evidence-"));
  fs.mkdirSync(path.join(root, "reports"), { recursive: true });
  const binding = {
    repository: "example/jftrade",
    releaseRef,
    ref: releaseRef,
    commitSha: releaseCommit,
    workflow: payloadRun.workflow,
    runId: payloadRun.id,
    attempt: payloadRun.attempt,
    artifact: payloadArtifact,
  };
  for (const [id, report] of Object.entries(reports)) {
    fs.writeFileSync(
      path.join(root, "reports", `${id}.json`),
      `${JSON.stringify({ ...report, binding }, null, 2)}\n`,
    );
  }
  const metadata = {
    $schema: "./release-evidence-payload-binding.schema.json",
    schemaVersion: PAYLOAD_BINDING_SCHEMA,
    repository: "example/jftrade",
    releaseRef,
    evidenceRef: payloadRun.ref,
    payloadRun,
    artifact: payloadArtifact,
  };
  const metadataPath = path.join(root, "payload-artifact-metadata.json");
  fs.writeFileSync(metadataPath, `${JSON.stringify(metadata, null, 2)}\n`);
  return { root, metadataPath, binding };
}

function bind(value) {
  return bindReleaseEvidence({
    payloadRoot: value.root,
    outputRoot: path.join(value.root, "bound"),
    payloadMetadataPath: value.metadataPath,
    repository: "example/jftrade",
    releaseRef,
    releaseCommit,
    producerRunId: 8801,
    producerAttempt: 1,
  });
}

test("binds real payload reports without self-referencing output artifact", (context) => {
  const value = fixture();
  context.after(() => fs.rmSync(value.root, { recursive: true, force: true }));
  const result = bind(value);
  assert.equal(result.manifest.workflow, RELEASE_EVIDENCE_WORKFLOW);
  assert.deepEqual(result.manifest.artifact, payloadArtifact);
  assert.equal(result.manifest.runId, 8801);
  assert.equal(result.manifest.evidence["signed-updater-inputs"].status, "passed");
  const outputReport = JSON.parse(fs.readFileSync(path.join(value.root, "bound/evidence/signed-updater-inputs/signed-updater-inputs.json"), "utf8"));
  assert.deepEqual(outputReport.sourceBinding, value.binding);
  assert.equal(outputReport.binding.workflow, RELEASE_EVIDENCE_WORKFLOW);
  const checked = validateExternalEvidenceManifest(result.manifest, {
    baseDirectory: result.outputRoot,
    expected: {
      repository: "example/jftrade",
      releaseRef,
      ref: releaseRef,
      commitSha: releaseCommit,
      workflow: RELEASE_EVIDENCE_WORKFLOW,
      runId: 8801,
      attempt: 1,
      artifact: payloadArtifact,
    },
    expectedArtifactMetadata: payloadArtifact,
  });
  assert.equal(checked.valid, true, checked.errors.join("; "));
});

test("rejects an unbound or placeholder payload report", (context) => {
  const value = fixture();
  context.after(() => fs.rmSync(value.root, { recursive: true, force: true }));
  const reportPath = path.join(value.root, "reports/security-review-inputs.json");
  const report = JSON.parse(fs.readFileSync(reportPath, "utf8"));
  delete report.binding;
  report.status = "external_release_runner_evidence_required";
  fs.writeFileSync(reportPath, `${JSON.stringify(report)}\n`);
  assert.throws(() => bind(value), /binding is required/);
});

test("rejects payload metadata whose ref is not trusted", (context) => {
  const value = fixture();
  context.after(() => fs.rmSync(value.root, { recursive: true, force: true }));
  const metadata = JSON.parse(fs.readFileSync(value.metadataPath, "utf8"));
  metadata.payloadRun.ref = "refs/heads/foreign-evidence";
  fs.writeFileSync(value.metadataPath, `${JSON.stringify(metadata)}\n`);
  assert.throws(() => bind(value), /payload run ref must equal evidence_ref/);
});

test("rejects payload metadata from a different commit", (context) => {
  const value = fixture();
  context.after(() => fs.rmSync(value.root, { recursive: true, force: true }));
  const metadata = JSON.parse(fs.readFileSync(value.metadataPath, "utf8"));
  metadata.payloadRun.commitSha = "c".repeat(40);
  fs.writeFileSync(value.metadataPath, `${JSON.stringify(metadata)}\n`);
  assert.throws(() => bind(value), /payload run commit does not match release commit/);
});

test("rejects symlink traversal in downloaded payload", (context) => {
  const value = fixture();
  context.after(() => fs.rmSync(value.root, { recursive: true, force: true }));
  const target = path.join(value.root, "reports/signed-updater-inputs.json");
  const copy = path.join(value.root, "reports/signed-updater-copy.json");
  fs.renameSync(target, copy);
  fs.symlinkSync(copy, target);
  assert.throws(() => bind(value), /must not traverse a symlink/);
});
