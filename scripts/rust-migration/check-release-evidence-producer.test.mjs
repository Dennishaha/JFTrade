import assert from "node:assert/strict";
import fs from "node:fs";
import { spawnSync } from "node:child_process";
import test from "node:test";

const producer = fs.readFileSync(".github/workflows/desktop-release-evidence.yml", "utf8");
const payloadWorkflow = fs.readFileSync(".github/workflows/desktop-release-evidence-payload.yml", "utf8");
const intakeWorkflow = fs.readFileSync(".github/workflows/desktop-release-evidence-intake.yml", "utf8");
const qualification = fs.readFileSync(".github/workflows/desktop-release-qualification.yml", "utf8");
const binder = fs.readFileSync("scripts/rust-migration/bind-release-evidence.mjs", "utf8");
const schema = fs.readFileSync(
  "tests/fixtures/rust-migration/stage9/release-evidence-payload-binding.schema.json",
  "utf8",
);

test("trusted evidence producer binds an external payload artifact", () => {
  assert.match(producer, /name: Desktop Release Evidence/);
  assert.match(producer, /workflow_dispatch:/);
  assert.match(producer, /actions: write/);
  for (const input of [
    "release_ref",
    "evidence_ref",
    "payload_run_id",
    "payload_run_attempt",
    "payload_ref",
    "payload_commit_sha",
    "payload_artifact",
    "payload_artifact_id",
    "payload_artifact_digest",
  ]) assert.match(producer, new RegExp(`${input}:`));
  assert.doesNotMatch(producer, /payload_workflow:\s*\n/);
  assert.match(producer, /PAYLOAD_WORKFLOW:\s*desktop-release-evidence-payload\.yml/);
  assert.match(producer, /actions\/runs\/\$PAYLOAD_RUN_ID/);
  assert.match(producer, /actions\/runs\/\$PAYLOAD_RUN_ID\/artifacts/);
  assert.match(producer, /actions\/artifacts\/\$PAYLOAD_ARTIFACT_ID/);
  assert.match(producer, /ref: refs\/tags\/\$\{\{ inputs\.release_ref \}\}/);
  assert.match(producer, /path: evidence-ref/);
  assert.match(producer, /git -C evidence-ref rev-parse HEAD/);
  assert.match(producer, /GITHUB_SHA.*RELEASE_COMMIT/);
  assert.match(producer, /PAYLOAD_COMMIT.*RELEASE_COMMIT/);
  assert.match(producer, /payload evidence must use the release tag commit/);
  assert.match(producer, /payload_ref must equal evidence_ref exactly/);
  assert.match(producer, /actions\/artifacts\/\$PAYLOAD_ARTIFACT_ID\/zip/);
  assert.match(producer, /bind-release-evidence\.mjs/);
  assert.match(producer, /upload-artifact@v7/);
  assert.match(producer, /desktop-release-evidence-bound/);
  assert.match(producer, /desktop-release-evidence-payload/);
  assert.match(producer, /upload_payload/);
  assert.match(producer, /artifact-digest/);
  assert.match(producer, /actions\/artifacts\/\$PRODUCER_ARTIFACT_ID/);
  assert.match(producer, /payload-artifact-metadata\.json/);
  assert.doesNotMatch(producer, /external_release_runner_evidence_required/);
  assert.doesNotMatch(producer, /status:\s*passed/);
  assert.doesNotMatch(producer, /check-signed-updater-lifecycle\.mjs\s*>/);
});

test("payload workflow is present and only validates then republishes external reports", () => {
  assert.match(payloadWorkflow, /name: Desktop Release Evidence Payload/);
  assert.match(payloadWorkflow, /source_artifact_id:/);
  assert.match(payloadWorkflow, /actions\/artifacts\/\$SOURCE_ARTIFACT_ID/);
  assert.match(payloadWorkflow, /verify-release-evidence-payload\.mjs/);
  assert.match(payloadWorkflow, /--payload-commit-sha/);
  assert.doesNotMatch(payloadWorkflow, /--payload-commit\s/);
  assert.match(payloadWorkflow, /source_workflow:/);
  assert.doesNotMatch(payloadWorkflow, /external-release-evidence\.yml/);
  assert.match(payloadWorkflow, /upload-artifact@v7/);
  assert.match(payloadWorkflow, /name: desktop-release-evidence-payload/);
  assert.doesNotMatch(payloadWorkflow, /status:\s*passed/);
  assert.doesNotMatch(payloadWorkflow, /external_release_runner_evidence_required/);
  assert.match(payloadWorkflow, /desktop-release-evidence-intake\.yml/);
  assert.match(payloadWorkflow, /source-binding\.json/);
  assert.doesNotMatch(payloadWorkflow, /verify-release-evidence-source\.mjs/);
  assert.doesNotMatch(payloadWorkflow, /incoming\/rollback\/(?:current|previous|rollback\.md)/);
  assert.doesNotMatch(payloadWorkflow, /check-rollback-artifact\.mjs/);
});

test("the immutable source intake is a real non-cyclic provenance root", () => {
  assert.match(intakeWorkflow, /name: Desktop Release Evidence Intake/);
  assert.match(intakeWorkflow, /source_repository:/);
  assert.match(intakeWorkflow, /TRUSTED_EXTERNAL_SOURCE_WORKFLOW: desktop-release-evidence-source\.yml/);
  assert.match(intakeWorkflow, /actions\/runs\/\$SOURCE_RUN_ID/);
  assert.match(intakeWorkflow, /actions\/artifacts\/\$SOURCE_ARTIFACT_ID/);
  assert.match(intakeWorkflow, /actions\/artifacts\/\$SOURCE_ARTIFACT_ID\/zip/);
  assert.match(intakeWorkflow, /verify-release-evidence-source\.mjs/);
  assert.match(intakeWorkflow, /Publish immutable intake artifact without rewriting reports/);
  assert.doesNotMatch(intakeWorkflow, /desktop-release-evidence-payload\.yml/);
  assert.doesNotMatch(intakeWorkflow, /status:\s*passed/);
});

test("producer preserves source binding and rejects synthetic evidence", () => {
  assert.match(binder, /sourceBinding/);
  assert.match(binder, /COPYFILE_EXCL/);
  assert.match(binder, /producerArtifact/);
  assert.match(binder, /unbound payload evidence is rejected/);
  assert.match(binder, /validateExternalEvidenceManifest/);
  assert.match(binder, /metadata\.sourceBinding/);
  assert.match(binder, /PAYLOAD_BINDING_SCHEMA/);
  assert.match(schema, /jftrade\.release-evidence-payload-binding\.v1/);
  assert.match(schema, /additionalProperties": false/);
});

test("payload schema resolves the nested source artifact definition and rejects an unresolved ref", () => {
  const document = JSON.parse(schema);
  const artifact = document.properties?.sourceBinding?.properties?.artifact;
  assert.deepEqual(artifact, { $ref: "#/properties/artifact" });
  assert.ok(document.properties?.artifact, "sourceBinding artifact ref must resolve to a declared schema node");
  assert.doesNotMatch(schema, /#\/$defs\/artifact/);
  const unresolved = structuredClone(document);
  unresolved.properties.sourceBinding.properties.artifact = { $ref: "#/$defs/missing-artifact" };
  const ref = unresolved.properties.sourceBinding.properties.artifact.$ref;
  assert.throws(() => {
    const node = ref.slice(2).split("/").reduce((current, key) => current?.[key], unresolved);
    if (!node) throw new Error(`unresolved local schema ref: ${ref}`);
  }, /unresolved local schema ref/);
});

test("qualification verifies the payload artifact separately from producer output", () => {
  assert.match(qualification, /Verify producer binding and payload artifact provenance/);
  assert.match(qualification, /payload-artifact-metadata\.json/);
  assert.match(qualification, /payload-artifact-api\.json/);
  assert.match(qualification, /actions\/artifacts\/\$payload_id/);
  assert.match(qualification, /expectedArtifactMetadata: producerArtifact/);
  assert.match(qualification, /artifact: producerArtifact/);
  assert.match(qualification, /byte-identical to producer staging payload/);
  assert.match(qualification, /producer-payload-artifact-api\.json/);
  assert.match(qualification, /GITHUB_SHA.*commit_sha/);
  assert.match(qualification, /evidence-bound-artifact-api\.json/);
  assert.match(qualification, /actions\/artifacts\/\$producer_id/);
  assert.match(qualification, /incoming\/producer-payload/);
  assert.match(qualification, /Download producer staging payload by immutable id/);
});

test("payload verifier rejects the legacy commit argument spelling", () => {
  const result = spawnSync(process.execPath, [
    "scripts/rust-migration/verify-release-evidence-payload.mjs",
    "--payload-commit", "a".repeat(40),
  ], { encoding: "utf8" });
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /unsupported argument: --payload-commit/);
});

test("payload verifier rejects unsafe workflow and artifact identifiers", () => {
  const script = "scripts/rust-migration/verify-release-evidence-payload.mjs";
  const baseArgs = [
    "--root", "/tmp",
    "--repository", "example/jftrade",
    "--release-ref", "refs/tags/v1.2.3",
    "--payload-ref", "refs/heads/release-evidence/v1.2.3",
    "--payload-commit-sha", "a".repeat(40),
    "--payload-workflow", "desktop-release-evidence-payload.yml",
    "--payload-run-id", "1",
    "--payload-run-attempt", "1",
    "--payload-artifact", "payload-evidence",
    "--payload-artifact-id", "1",
    "--payload-artifact-digest", `sha256:${"b".repeat(64)}`,
  ];
  for (const field of ["--payload-run-id", "--payload-run-attempt", "--payload-artifact-id"]) {
    for (const invalid of ["0", "-1", "1.5", "9007199254740992"]) {
      const args = [...baseArgs];
      args[args.indexOf(field) + 1] = invalid;
      const result = spawnSync(process.execPath, [script, ...args], { encoding: "utf8" });
      assert.notEqual(result.status, 0, `${field}=${invalid} should fail`);
      assert.match(result.stderr, /must be a positive integer/);
    }
  }
});
