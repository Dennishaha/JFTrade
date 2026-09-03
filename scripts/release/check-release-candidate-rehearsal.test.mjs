import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import { validateReleaseCandidateEvidence } from "./check-release-candidate.mjs";
import {
  inspectReleaseCandidateRehearsal,
  main,
  REHEARSAL_CHECKS,
  REHEARSAL_LIMITATIONS,
  REHEARSAL_PLATFORMS,
} from "./check-release-candidate-rehearsal.mjs";

const commitSha = "a".repeat(40);
const candidateRef = "refs/heads/release/0.29.0-candidate";
const digest = (character) => `sha256:${character.repeat(64)}`;

function fixture() {
  const checks = Object.fromEntries(REHEARSAL_CHECKS.map((name) => [name, "passed"]));
  const workflowRun = {
    id: 2001,
    attempt: 1,
    workflow: "desktop-release-qualification.yml",
    ref: candidateRef,
    commitSha,
  };
  const sourceWorkflowRun = {
    id: 1001,
    attempt: 2,
    workflow: "desktop-release-evidence-source.yml",
    ref: candidateRef,
    commitSha,
  };
  return {
    $schema: "./release-candidate-rehearsal.schema.json",
    schemaVersion: "jftrade.release-candidate-rehearsal.v1",
    phase: "pre-release",
    status: "rehearsal_passed",
    qualificationLevel: "unsigned-rehearsal",
    releaseQualified: false,
    repository: "Dennishaha/jftrade",
    candidateRef,
    plannedReleaseTag: "v0.29.0",
    commitSha,
    workflowRun,
    sourceWorkflowRun,
    artifact: { name: "desktop-release-rehearsal-source", id: 3001, digest: digest("e") },
    platforms: Object.fromEntries(REHEARSAL_PLATFORMS.map((platform, index) => [
      platform,
      {
        status: "passed",
        artifact: {
          name: `desktop-release-rehearsal-${platform}`,
          id: 4001 + index,
          digest: digest(String(index + 1)),
        },
        checks: { ...checks },
      },
    ])),
    limitations: { ...REHEARSAL_LIMITATIONS },
  };
}

test("accepts an unsigned four-platform rehearsal without granting release qualification", () => {
  const document = fixture();
  const result = inspectReleaseCandidateRehearsal(document, {
    expected: {
      candidateRef,
      plannedReleaseTag: "v0.29.0",
      commitSha,
      runId: 2001,
      runAttempt: 1,
      workflow: "desktop-release-qualification.yml",
      artifactName: "desktop-release-rehearsal-source",
      artifactId: 3001,
      artifactDigest: digest("e"),
    },
  });
  assert.equal(result.valid, true, result.errors.join("; "));
  assert.equal(result.status, "rehearsal_passed");
  assert.equal(result.qualificationLevel, "unsigned-rehearsal");
  assert.equal(result.releaseQualified, false);
});

test("rejects candidate branch, planned version, SHA, and artifact digest drift", () => {
  const cases = [
    ["candidateRef", "refs/heads/release/0.29.1-candidate", /candidateRef/],
    ["plannedReleaseTag", "v0.29.1", /plannedReleaseTag/],
    ["commitSha", "b".repeat(40), /commitSha/],
  ];
  for (const [field, value, pattern] of cases) {
    const document = fixture();
    document[field] = value;
    const result = inspectReleaseCandidateRehearsal(document);
    assert.equal(result.valid, false);
    assert.match(result.errors.join("; "), pattern);
  }
  const document = fixture();
  const result = inspectReleaseCandidateRehearsal(document, {
    expected: { artifactDigest: digest("f") },
  });
  assert.equal(result.valid, false);
  assert.match(result.errors.join("; "), /artifact\.digest does not match/);
});

test("rejects a missing platform, incomplete drill, or reused platform artifact", () => {
  const missing = fixture();
  delete missing.platforms["windows-arm64"];
  assert.match(inspectReleaseCandidateRehearsal(missing).errors.join("; "), /windows-arm64 is required/);

  const incomplete = fixture();
  incomplete.platforms["linux-x64"].checks.rollback = "open";
  assert.match(inspectReleaseCandidateRehearsal(incomplete).errors.join("; "), /rollback must be "passed"/);

  const reused = fixture();
  reused.platforms["windows-arm64"].artifact.id = reused.platforms["windows-x64"].artifact.id;
  assert.match(inspectReleaseCandidateRehearsal(reused).errors.join("; "), /artifact id is reused/);
});

test("rejects signed, notarized, updater, or security claims in rehearsal mode", () => {
  for (const [field, value] of [
    ["packageSigning", "passed"],
    ["notarization", "passed"],
    ["updaterSignature", "passed"],
    ["independentSecuritySignOff", "passed"],
  ]) {
    const document = fixture();
    document.limitations[field] = value;
    const result = inspectReleaseCandidateRehearsal(document);
    assert.equal(result.valid, false);
    assert.match(result.errors.join("; "), new RegExp(field));
  }
});

test("formal candidate and rehearsal contracts cannot be used interchangeably", () => {
  const rehearsal = fixture();
  assert.equal(validateReleaseCandidateEvidence(rehearsal).valid, false);

  const formalCandidate = {
    $schema: "./release-candidate-evidence.schema.json",
    schemaVersion: "jftrade.release-candidate-evidence.v1",
    phase: "pre-release",
    status: "candidate_ready",
    releaseRef: candidateRef,
    releaseTag: "v0.29.0",
  };
  const result = inspectReleaseCandidateRehearsal(formalCandidate);
  assert.equal(result.valid, false);
  assert.match(result.errors.join("; "), /schemaVersion|releaseQualified|qualificationLevel/);
});

test("CLI builds and checks a receipt without changing the source document", (context) => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-rehearsal-"));
  context.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const input = path.join(root, "config.json");
  const output = path.join(root, "receipt.json");
  fs.writeFileSync(input, `${JSON.stringify(fixture(), null, 2)}\n`);
  assert.equal(main(["--build", "--config", input, "--output", output]), 0);
  assert.equal(main(["--check", "--input", output]), 0);
  assert.deepEqual(JSON.parse(fs.readFileSync(output, "utf8")), fixture());
});
