import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";
import test from "node:test";
import { fileURLToPath } from "node:url";

import {
  buildReleaseCandidateEvidence,
  inspectReleaseCandidateEvidence,
  main,
  REQUIRED_PLATFORMS,
  REQUIRED_PREREQUISITE_KINDS,
  REQUIRED_PREREQUISITES,
  RELEASE_CANDIDATE_LIMITATIONS,
} from "./check-release-candidate.mjs";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));
const closeoutPath = path.join(
  repositoryRoot,
  "tests/fixtures/rust-migration/stage9/closeout-evidence.json",
);

function digest(value) {
  return createHash("sha256").update(value).digest("hex");
}

function write(root, relativePath, value) {
  const filePath = path.join(root, relativePath);
  fs.mkdirSync(path.dirname(filePath), { recursive: true });
  fs.writeFileSync(filePath, value);
  return filePath;
}

function fixture() {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-release-candidate-"));
  const commitSha = "a".repeat(40);
  const workflowRun = {
    id: 81234,
    attempt: 2,
    workflow: "Desktop Release",
    ref: "refs/tags/v1.2.3",
    commitSha,
    url: "https://github.com/example/jftrade/actions/runs/81234",
  };
  const sourceWorkflowRun = {
    id: 81233,
    attempt: 1,
    workflow: "Desktop Release",
    ref: workflowRun.ref,
    commitSha,
    url: "https://github.com/example/jftrade/actions/runs/81233",
  };
  const platforms = {};
  const releaseFiles = [];
  for (const platform of REQUIRED_PLATFORMS) {
    const manifestPath = `release/tauri-release-${platform}.json`;
    const artifactPath = `release/JFTrade-v1.2.3-${platform}.pkg`;
    write(root, manifestPath, JSON.stringify({ platform, version: "1.2.3" }));
    write(root, artifactPath, `artifact:${platform}`);
    platforms[platform] = {
      manifest: manifestPath,
      artifacts: [{ path: artifactPath, kind: "package" }],
    };
    releaseFiles.push(manifestPath, artifactPath);
  }
  const sumsText = `${releaseFiles
    .map((relativePath) => `${digest(fs.readFileSync(path.join(root, relativePath)))}  ${path.basename(relativePath)}`)
    .join("\n")}\n`;
  write(root, "release/SHA256SUMS", sumsText);

  const prerequisites = REQUIRED_PREREQUISITES.map((id, index) => {
    const evidencePath = `evidence/${id}.json`;
    write(root, evidencePath, JSON.stringify({ id, run: workflowRun.id }));
    return {
      id,
      kind: REQUIRED_PREREQUISITE_KINDS[id],
      releaseRef: workflowRun.ref,
      commitSha,
      workflowRun,
      sourceWorkflowRun,
      evidence: [{ path: evidencePath, schemaVersion: "jftrade.test.evidence.v1" }],
      summary: `${id} passed for this run`,
      status: "passed",
    };
  });
  return {
    root,
    commitSha,
    workflowRun,
    sourceWorkflowRun,
    platforms,
    prerequisites,
    config: {
      releaseRef: workflowRun.ref,
      releaseTag: "v1.2.3",
      commitSha,
      workflowRun,
      sourceWorkflowRun,
      platforms,
      sha256sums: "release/SHA256SUMS",
      prerequisites,
    },
    cleanup: () => fs.rmSync(root, { recursive: true, force: true }),
  };
}

function builtFixture() {
  const value = fixture();
  const evidence = buildReleaseCandidateEvidence({
    ...value.config,
    baseDirectory: value.root,
  });
  return { ...value, evidence };
}

test("builds and inspects a four-platform candidate bound to one workflow run", (context) => {
  const value = builtFixture();
  context.after(value.cleanup);
  const result = inspectReleaseCandidateEvidence(value.evidence, { baseDirectory: value.root });
  assert.equal(result.valid, true);
  assert.equal(result.releaseQualified, false);
  assert.equal(result.releaseRef, "refs/tags/v1.2.3");
  assert.equal(result.commitSha, value.commitSha);
  assert.equal(result.workflowRun.id, value.workflowRun.id);
  assert.equal(result.sourceWorkflowRun.id, value.sourceWorkflowRun.id);
  assert.deepEqual(Object.keys(result.platforms), REQUIRED_PLATFORMS);
  assert.deepEqual(
    result.prerequisites.map((entry) => entry.id),
    REQUIRED_PREREQUISITES,
  );
  assert.ok(RELEASE_CANDIDATE_LIMITATIONS.some((item) => /post-release smoke/i.test(item)));
  assert.ok(RELEASE_CANDIDATE_LIMITATIONS.some((item) => /hard-cut/i.test(item)));
  assert.ok(RELEASE_CANDIDATE_LIMITATIONS.some((item) => /independent security/i.test(item)));
});

test("rejects a tag/ref mismatch and a workflow commit mismatch", (context) => {
  const value = builtFixture();
  context.after(value.cleanup);
  const wrongTag = structuredClone(value.evidence);
  wrongTag.releaseTag = "v1.2.4";
  const tagResult = inspectReleaseCandidateEvidence(wrongTag, { baseDirectory: value.root });
  assert.equal(tagResult.valid, false);
  assert.match(tagResult.errors.join("\n"), /releaseRef must match releaseTag/);

  const wrongCommit = structuredClone(value.evidence);
  wrongCommit.workflowRun.commitSha = "b".repeat(40);
  const commitResult = inspectReleaseCandidateEvidence(wrongCommit, { baseDirectory: value.root });
  assert.equal(commitResult.valid, false);
  assert.match(commitResult.errors.join("\n"), /workflowRun\.commitSha does not match/);
});

test("fails closed on a tampered artifact digest and missing platform", (context) => {
  const value = builtFixture();
  context.after(value.cleanup);
  fs.appendFileSync(path.join(value.root, value.evidence.platforms["linux-x64"].artifacts[0].path), "tamper");
  const tampered = inspectReleaseCandidateEvidence(value.evidence, { baseDirectory: value.root });
  assert.equal(tampered.valid, false);
  assert.match(tampered.errors.join("\n"), /artifacts\[0\] SHA-256 mismatch/);

  const missing = structuredClone(value.evidence);
  delete missing.platforms["windows-arm64"];
  const missingResult = inspectReleaseCandidateEvidence(missing, { baseDirectory: value.root });
  assert.equal(missingResult.valid, false);
  assert.match(missingResult.errors.join("\n"), /missing release platform evidence: windows-arm64/);
});

test("rejects prerequisite evidence from another run or ref", (context) => {
  const value = builtFixture();
  context.after(value.cleanup);
  const crossRun = structuredClone(value.evidence);
  crossRun.prerequisites[0].workflowRun.id += 1;
  const runResult = inspectReleaseCandidateEvidence(crossRun, { baseDirectory: value.root });
  assert.equal(runResult.valid, false);
  assert.match(runResult.errors.join("\n"), /workflowRun does not match manifest/);

  const crossRef = structuredClone(value.evidence);
  crossRef.prerequisites[1].releaseRef = "refs/heads/main";
  const refResult = inspectReleaseCandidateEvidence(crossRef, { baseDirectory: value.root });
  assert.equal(refResult.valid, false);
  assert.match(refResult.errors.join("\n"), /releaseRef does not match manifest/);
});

test("rejects prerequisite placeholders, wrong kinds, and foreign source runs", (context) => {
  const value = builtFixture();
  context.after(value.cleanup);

  const placeholder = structuredClone(value.evidence);
  const placeholderPath = path.join(value.root, placeholder.prerequisites[0].evidence[0].path);
  fs.writeFileSync(placeholderPath, JSON.stringify({ status: "external_release_runner_evidence_required" }));
  placeholder.prerequisites[0].evidence[0].sha256 = digest(
    fs.readFileSync(placeholderPath),
  );
  const placeholderResult = inspectReleaseCandidateEvidence(placeholder, { baseDirectory: value.root });
  assert.equal(placeholderResult.valid, false);
  assert.match(placeholderResult.errors.join("\n"), /placeholder/);

  const wrongKind = structuredClone(value.evidence);
  wrongKind.prerequisites[1].kind = "rollback-artifact";
  const wrongKindResult = inspectReleaseCandidateEvidence(wrongKind, { baseDirectory: value.root });
  assert.equal(wrongKindResult.valid, false);
  assert.match(wrongKindResult.errors.join("\n"), /kind must be signed-updater/);

  const foreignSource = structuredClone(value.evidence);
  foreignSource.prerequisites[2].sourceWorkflowRun.commitSha = "c".repeat(40);
  const foreignResult = inspectReleaseCandidateEvidence(foreignSource, { baseDirectory: value.root });
  assert.equal(foreignResult.valid, false);
  assert.match(foreignResult.errors.join("\n"), /sourceWorkflowRun does not match manifest release ref\/commit/);
});

test("rejects the old lifecycle command masquerading as rollback evidence", (context) => {
  const value = builtFixture();
  context.after(value.cleanup);
  const rollback = value.evidence.prerequisites.find((entry) => entry.id === "rollback-artifact-pair");
  const filePath = path.join(value.root, rollback.evidence[0].path);
  fs.writeFileSync(filePath, "check-signed-updater-lifecycle.mjs\n");
  rollback.evidence[0].sha256 = digest(fs.readFileSync(filePath));
  const result = inspectReleaseCandidateEvidence(value.evidence, { baseDirectory: value.root });
  assert.equal(result.valid, false);
  assert.match(result.errors.join("\n"), /lifecycle-check command/);
});

test("rejects candidate metadata that is not bound to the current workflow context", (context) => {
  const value = builtFixture();
  context.after(value.cleanup);
  const result = inspectReleaseCandidateEvidence(value.evidence, {
    baseDirectory: value.root,
    expected: {
      releaseRef: value.workflowRun.ref,
      releaseTag: "v1.2.3",
      commitSha: value.commitSha,
      workflowRun: value.workflowRun,
      sourceWorkflowRun: value.sourceWorkflowRun,
    },
  });
  assert.equal(result.valid, true);

  const wrongRun = structuredClone(value.workflowRun);
  wrongRun.id += 1;
  const blocked = inspectReleaseCandidateEvidence(value.evidence, {
    baseDirectory: value.root,
    expected: { workflowRun: wrongRun },
  });
  assert.equal(blocked.valid, false);
  assert.match(blocked.errors.join("\n"), /does not match expected workflow run/);
});

test("requires every named prerequisite and real evidence file", (context) => {
  const value = builtFixture();
  context.after(value.cleanup);
  const missing = structuredClone(value.evidence);
  missing.prerequisites = missing.prerequisites.filter((entry) => entry.id !== "security-review-inputs");
  const missingResult = inspectReleaseCandidateEvidence(missing, { baseDirectory: value.root });
  assert.equal(missingResult.valid, false);
  assert.match(missingResult.errors.join("\n"), /missing prerequisite evidence: security-review-inputs/);

  const absent = structuredClone(value.evidence);
  absent.prerequisites[0].evidence[0].path = "evidence/not-found.json";
  const absentResult = inspectReleaseCandidateEvidence(absent, { baseDirectory: value.root });
  assert.equal(absentResult.valid, false);
  assert.match(absentResult.errors.join("\n"), /evidence\[0\] file is missing/);
});

test("CLI builds and validates without changing the Stage 9 closeout manifest", (context) => {
  const value = fixture();
  context.after(value.cleanup);
  const configPath = write(value.root, "candidate-config.json", JSON.stringify(value.config));
  const outputPath = path.join(value.root, "candidate-evidence.json");
  const before = fs.readFileSync(closeoutPath);
  const scriptPath = fileURLToPath(new URL("./check-release-candidate.mjs", import.meta.url));
  const built = spawnSync(process.execPath, [
    scriptPath,
    "--build",
    "--config",
    configPath,
    "--base-dir",
    value.root,
    "--output",
    outputPath,
    "--expected-ref",
    value.workflowRun.ref,
    "--expected-tag",
    "v1.2.3",
    "--expected-commit",
    value.commitSha,
    "--expected-run-id",
    String(value.workflowRun.id),
    "--expected-attempt",
    String(value.workflowRun.attempt),
    "--expected-workflow",
    value.workflowRun.workflow,
  ], { cwd: repositoryRoot, encoding: "utf8" });
  assert.equal(built.status, 0, built.stderr);
  const checked = spawnSync(process.execPath, [
    scriptPath,
    "--check",
    "--input",
    outputPath,
    "--base-dir",
    value.root,
    "--expected-ref",
    value.workflowRun.ref,
    "--expected-tag",
    "v1.2.3",
    "--expected-commit",
    value.commitSha,
    "--expected-run-id",
    String(value.workflowRun.id),
    "--expected-attempt",
    String(value.workflowRun.attempt),
    "--expected-workflow",
    value.workflowRun.workflow,
  ], { cwd: repositoryRoot, encoding: "utf8" });
  assert.equal(checked.status, 0, checked.stderr);
  assert.match(checked.stdout, /pre_release_inputs_verified_only/);
  assert.deepEqual(fs.readFileSync(closeoutPath), before);
});

test("CLI reports missing input as a failed check", () => {
  assert.equal(main(["--check", "--input", "/tmp/jftrade-release-candidate-missing.json"]), 1);
});
