import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import { verifySourceEvidence } from "./verify-release-evidence-source.mjs";

const binding = {
  repository: "example/jftrade",
  releaseRef: "refs/tags/v1.2.3",
  ref: "refs/tags/v1.2.3",
  commitSha: "a".repeat(40),
  workflow: "desktop-release-evidence-source.yml",
  runId: 7001,
  attempt: 2,
  artifact: { name: "raw-release-evidence", id: 7002, digest: `sha256:${"b".repeat(64)}` },
};

function reportSet() {
  return {
    "signed-updater-inputs": {
      schemaVersion: "jftrade.release.signed-updater.v2",
      status: "verified",
      binding,
      feed: { entryCount: 4 },
      artifacts: [{ archive: "JFTrade-v1.2.3.zip", archiveSha256: "1".repeat(64), signatureSha256: "2".repeat(64) }],
      publicKeyConfigured: true,
      publicKeySha256: "3".repeat(64),
      endpoint: "https://updates.example.test/feed.json",
    },
    "sbom-provenance-inputs": {
      schemaVersion: "jftrade.release.sbom-provenance.v2",
      status: "verified",
      binding,
      subjects: [{ platform: "macos-arm64", sha256: "4".repeat(64) }],
    },
    "rollback-artifact-pair": {
      schemaVersion: "jftrade.release.rollback-artifact.v2",
      status: "verified",
      binding,
      current: { version: "1.2.3", platforms: { "macos-arm64": {} }, updaterMetadata: {} },
      previous: { version: "1.2.2", platforms: { "macos-arm64": {} }, updaterMetadata: {} },
      rollbackInstructions: "restore previous release",
    },
    "backup-restore-drill": {
      schemaVersion: "jftrade.release.backup-restore.v2",
      status: "verified",
      binding,
      priorVersion: "1.2.2",
      nativeDrill: {
        status: "verified",
        platforms: Object.fromEntries([
          "macos-arm64", "linux-x64", "windows-x64", "windows-arm64",
        ].map((platform) => [platform, { status: "verified" }])),
      },
    },
    "security-review-inputs": {
      schemaVersion: "jftrade.release.security-review.v2",
      status: "independent_review_signed_off",
      binding,
      independentReview: {
        independent: true,
        status: "signed_off",
        reviewer: "external-reviewer",
        approvedAt: "2026-08-31T00:00:00Z",
        attestation: { uri: "https://review.example.test/r/1", sha256: "5".repeat(64) },
      },
    },
  };
}

function fixture() {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-source-evidence-"));
  fs.mkdirSync(path.join(root, "reports"), { recursive: true });
  for (const [id, report] of Object.entries(reportSet())) {
    fs.writeFileSync(path.join(root, "reports", `${id}.json`), `${JSON.stringify(report)}\n`);
  }
  return root;
}

function args(root, extra = {}) {
  return {
    root,
    repository: binding.repository,
    source_repository: "external-org/release-evidence",
    release_ref: binding.releaseRef,
    source_ref: binding.ref,
    source_commit_sha: binding.commitSha,
    source_workflow: binding.workflow,
    source_run_id: String(binding.runId),
    source_run_attempt: String(binding.attempt),
    source_artifact: binding.artifact.name,
    source_artifact_id: String(binding.artifact.id),
    source_artifact_digest: binding.artifact.digest,
    ...extra,
  };
}

test("intake verifies real reports and only adds detached provenance", (context) => {
  const root = fixture();
  context.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const reportPath = path.join(root, "reports/security-review-inputs.json");
  const before = fs.readFileSync(reportPath);
  const result = verifySourceEvidence({ args: args(root) });
  assert.equal(result.valid, true, result.errors?.join("; "));
  assert.deepEqual(fs.readFileSync(reportPath), before);
  const sidecar = JSON.parse(fs.readFileSync(path.join(root, "source-binding.json"), "utf8"));
  assert.equal(sidecar.sourceRepository, "external-org/release-evidence");
  assert.deepEqual(sidecar.binding, binding);
});

test("intake rejects unsafe workflow and artifact identifiers", (context) => {
  const invalidValues = ["0", "-1", "1.5", "9007199254740992"];
  for (const field of ["source_run_id", "source_run_attempt", "source_artifact_id"]) {
    const root = fixture();
    context.after(() => fs.rmSync(root, { recursive: true, force: true }));
    for (const invalid of invalidValues) {
      assert.throws(
        () => verifySourceEvidence({ args: args(root, { [field]: invalid }) }),
        /must be a positive integer/,
        `${field}=${invalid}`,
      );
    }
  }
});

test("intake fails closed for missing four-platform and independent evidence", (context) => {
  const root = fixture();
  context.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const backupPath = path.join(root, "reports/backup-restore-drill.json");
  const backup = JSON.parse(fs.readFileSync(backupPath, "utf8"));
  delete backup.nativeDrill.platforms;
  fs.writeFileSync(backupPath, JSON.stringify(backup));
  assert.throws(
    () => verifySourceEvidence({ args: args(root) }),
    /nativeDrill\.platforms|four native release platforms/,
  );

  const second = fixture();
  context.after(() => fs.rmSync(second, { recursive: true, force: true }));
  const securityPath = path.join(second, "reports/security-review-inputs.json");
  const security = JSON.parse(fs.readFileSync(securityPath, "utf8"));
  delete security.independentReview.attestation;
  fs.writeFileSync(securityPath, JSON.stringify(security));
  assert.throws(() => verifySourceEvidence({ args: args(second) }), /review attestation digest/);
});

test("intake rejects non-fixed producer identities and output collisions", (context) => {
  const root = fixture();
  context.after(() => fs.rmSync(root, { recursive: true, force: true }));
  assert.throws(() => verifySourceEvidence({ args: args(root, { source_workflow: "arbitrary.yml" }) }), /fixed external producer/);
  const output = path.join(root, "already-there.json");
  fs.writeFileSync(output, "existing\n");
  assert.throws(() => verifySourceEvidence({ args: args(root, { output }) }), /already exists/);
});

test("report bytes remain hash-stable after source verification", (context) => {
  const root = fixture();
  context.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const hashes = Object.fromEntries(fs.readdirSync(path.join(root, "reports")).map((name) => {
    const file = path.join(root, "reports", name);
    return [name, createHash("sha256").update(fs.readFileSync(file)).digest("hex")];
  }));
  verifySourceEvidence({ args: args(root) });
  for (const [name, hash] of Object.entries(hashes)) {
    assert.equal(createHash("sha256").update(fs.readFileSync(path.join(root, "reports", name))).digest("hex"), hash);
  }
});
