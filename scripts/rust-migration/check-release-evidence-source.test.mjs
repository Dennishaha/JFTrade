import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import { verifySourceEvidence } from "./verify-release-evidence-source.mjs";
import { inspectRollbackArtifactPair } from "./check-rollback-artifact.mjs";

const platforms = [
  ["macos-arm64", "arm64", [["dmg", "dmg"]], "darwin-aarch64"],
  ["linux-x64", "amd64", [["appimage", "AppImage"], ["deb", "deb"], ["rpm", "rpm"]], "linux-x86_64"],
  ["windows-x64", "amd64", [["nsis", "exe"]], "windows-x86_64"],
  ["windows-arm64", "arm64", [["nsis", "exe"]], "windows-aarch64"],
];
const signature = "untrusted comment: external fixture\nRURqRkFLRV9TSUdOQVRVUkU=";

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

function sha256(value) {
  return createHash("sha256").update(value).digest("hex");
}

function write(root, relative, value) {
  const file = path.join(root, relative);
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(file, value);
  return { path: relative, sha256: sha256(value), size: Buffer.byteLength(value) };
}

function createRelease(root, version) {
  const feed = { version, platforms: {} };
  for (const [platform, architecture, kinds, target] of platforms) {
    const packages = kinds.map(([kind, extension]) => {
      const suffix = extension === "exe" ? "-setup.exe" : `.${extension}`;
      return { kind, ...write(root, `packages/JFTrade-${version}-${platform}-${kind}${suffix}`, `${platform}:${kind}`) };
    });
    const archive = write(root, `updater/JFTrade_${version}_${platform}.tar.gz`, `${platform}:updater`);
    const sidecar = write(root, `${archive.path}.sig`, `${signature}\n`);
    write(root, `tauri-release-${platform}.json`, `${JSON.stringify({
      schemaVersion: "jftrade.tauri-release-artifacts.v1",
      target: { architecture, platform },
      version,
      scope: "package-and-integrity",
      packages,
      appBundle: null,
      updaterSignatures: [sidecar],
      updaterArchives: [archive],
    })}\n`);
    feed.platforms[target] = {
      url: `https://updates.example.test/${path.basename(archive.path)}`,
      signature,
    };
  }
  write(root, "latest.json", `${JSON.stringify(feed)}\n`);
}

function createRollbackEvidence(root) {
  const currentRoot = path.join(root, "rollback", "current");
  const previousRoot = path.join(root, "rollback", "previous");
  createRelease(currentRoot, "1.2.3");
  createRelease(previousRoot, "1.2.2");
  const instructions = path.join(root, "rollback", "rollback.md");
  fs.writeFileSync(instructions, "Rollback 1.2.3 to 1.2.2 using the retained signed package and updater metadata.\n");
  return inspectRollbackArtifactPair({
    currentRoot,
    previousRoot,
    currentVersion: "1.2.3",
    previousVersion: "1.2.2",
    allowDowngrade: true,
    instructionsPath: instructions,
  });
}

function reportSet(rollbackCheck) {
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
      checkerResult: rollbackCheck,
      current: rollbackCheck.current,
      previous: rollbackCheck.previous,
      rollbackInstructions: rollbackCheck.rollbackInstructions,
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
  const rollbackCheck = createRollbackEvidence(root);
  for (const [id, report] of Object.entries(reportSet(rollbackCheck))) {
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

test("intake contract requires the raw rollback pair and instructions", (context) => {
  const root = fixture();
  context.after(() => fs.rmSync(root, { recursive: true, force: true }));
  fs.rmSync(path.join(root, "rollback", "previous"), { recursive: true });
  assert.throws(
    () => verifySourceEvidence({ args: args(root) }),
    /previous|release artifact directory|directory is required|unavailable/,
  );
  assert.equal(fs.existsSync(path.join(root, "source-binding.json")), false);
});

test("intake rejects symlinked rollback ancestors before writing binding", (context) => {
  const root = fixture();
  context.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const rollback = path.join(root, "rollback");
  const rollbackTarget = path.join(root, "rollback-target");
  fs.renameSync(rollback, rollbackTarget);
  try {
    fs.symlinkSync(rollbackTarget, rollback, "dir");
  } catch (error) {
    if (["EACCES", "EPERM", "ENOTSUP"].includes(error.code)) {
      context.skip(`directory symlinks are unavailable: ${error.code}`);
      return;
    }
    throw error;
  }
  assert.throws(
    () => verifySourceEvidence({ args: args(root) }),
    /symbolic link/,
  );
  assert.equal(fs.existsSync(path.join(root, "source-binding.json")), false);
});

test("intake rejects a symlinked evidence root before reading reports", (context) => {
  const root = fixture();
  const alias = `${root}-alias`;
  context.after(() => {
    fs.rmSync(alias, { recursive: true, force: true });
    fs.rmSync(root, { recursive: true, force: true });
  });
  try {
    fs.symlinkSync(root, alias, "dir");
  } catch (error) {
    if (["EACCES", "EPERM", "ENOTSUP"].includes(error.code)) {
      context.skip(`directory symlinks are unavailable: ${error.code}`);
      return;
    }
    throw error;
  }
  assert.throws(
    () => verifySourceEvidence({ args: args(alias) }),
    /symbolic link/,
  );
  assert.equal(fs.existsSync(path.join(root, "source-binding.json")), false);
});

test("intake rejects rollback reports not bound to the checker result", (context) => {
  const root = fixture();
  context.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const reportPath = path.join(root, "reports", "rollback-artifact-pair.json");
  const report = JSON.parse(fs.readFileSync(reportPath, "utf8"));
  report.checkerResult.current.platforms["macos-arm64"].packageCount += 1;
  fs.writeFileSync(reportPath, `${JSON.stringify(report)}\n`);
  assert.throws(
    () => verifySourceEvidence({ args: args(root) }),
    /checkerResult does not match the rollback checker result/,
  );
  assert.equal(fs.existsSync(path.join(root, "source-binding.json")), false);
});

test("intake rejects rollback artifact bytes that no longer match their declared hashes", (context) => {
  const root = fixture();
  context.after(() => fs.rmSync(root, { recursive: true, force: true }));
  fs.appendFileSync(
    path.join(root, "rollback", "current", "packages", "JFTrade-1.2.3-macos-arm64-dmg.dmg"),
    "tampered",
  );
  assert.throws(
    () => verifySourceEvidence({ args: args(root) }),
    /sha256 does not match|size does not match/,
  );
  assert.equal(fs.existsSync(path.join(root, "source-binding.json")), false);
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
