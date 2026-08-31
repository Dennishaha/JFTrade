#!/usr/bin/env node

/**
 * Verify the immutable raw evidence intake boundary.
 *
 * The source runner is intentionally outside this repository's release
 * producer chain.  This command checks that its report bytes are real,
 * semantically complete, and bound to the exact GitHub run/artifact supplied
 * by the intake workflow.  It never rewrites a report or manufactures a
 * passed status; it only writes a detached provenance sidecar.
 */

import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import { validateReleaseEvidencePayload, TRUSTED_EXTERNAL_SOURCE_WORKFLOWS } from "./check-release-evidence-inputs.mjs";

const REQUIRED_REPORTS = Object.freeze([
  "signed-updater-inputs",
  "sbom-provenance-inputs",
  "rollback-artifact-pair",
  "backup-restore-drill",
  "security-review-inputs",
]);
const BINDING_KEYS = Object.freeze([
  "repository", "releaseRef", "ref", "commitSha", "workflow", "runId", "attempt", "artifact",
]);
const ARTIFACT_KEYS = Object.freeze(["name", "id", "digest"]);
const SOURCE_BINDING_SCHEMA = "jftrade.release-evidence-source-binding.v1";

function isRecord(value) {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function required(value, label) {
  if (typeof value !== "string" || value.trim() === "") throw new Error(`${label} is required`);
  return value.trim();
}

function positive(value, label) {
  const text = required(value, label);
  if (!/^[1-9][0-9]*$/.test(text)) throw new Error(`${label} must be a positive integer`);
  return Number(text);
}

function rejectUnknown(value, allowed, label) {
  if (!isRecord(value)) throw new Error(`${label} must be an object`);
  const unknown = Object.keys(value).filter((key) => !allowed.includes(key));
  if (unknown.length > 0) throw new Error(`${label} contains unsupported field(s): ${unknown.join(", ")}`);
}

function parseArtifact(args) {
  const artifact = {
    name: required(args.source_artifact, "source_artifact"),
    id: positive(args.source_artifact_id, "source_artifact_id"),
    digest: required(args.source_artifact_digest, "source_artifact_digest"),
  };
  if (!/^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$/.test(artifact.name)) throw new Error("source_artifact has an invalid name");
  if (!/^sha256:[a-f0-9]{64}$/.test(artifact.digest)) throw new Error("source_artifact_digest must be sha256:<64 lowercase hex>");
  return artifact;
}

function parseArgs(argv) {
  const allowed = new Set([
    "root", "repository", "source_repository", "release_ref", "source_ref", "source_commit_sha", "source_workflow",
    "source_run_id", "source_run_attempt", "source_artifact", "source_artifact_id", "source_artifact_digest",
    "output",
  ]);
  const result = {};
  for (let index = 0; index < argv.length; index += 1) {
    const token = argv[index];
    if (!token.startsWith("--") || index + 1 >= argv.length || argv[index + 1].startsWith("--")) {
      throw new Error(`invalid argument: ${token}`);
    }
    const key = token.slice(2).replaceAll("-", "_");
    if (!allowed.has(key)) throw new Error(`unsupported argument: ${token}`);
    if (result[key] !== undefined) throw new Error(`duplicate argument: ${token}`);
    result[key] = argv[index + 1];
    index += 1;
  }
  return result;
}

function expectedBinding(args) {
  const workflow = required(args.source_workflow, "source_workflow");
  if (!TRUSTED_EXTERNAL_SOURCE_WORKFLOWS.includes(workflow)) {
    throw new Error(`source_workflow is not the fixed external producer: ${workflow}`);
  }
  const releaseRef = required(args.release_ref, "release_ref");
  if (!/^refs\/tags\/v\d+\.\d+\.\d+$/.test(releaseRef)) throw new Error("release_ref must be refs/tags/vX.Y.Z");
  const sourceRef = required(args.source_ref, "source_ref");
  if (!/^(?!\/)(?!.*\.\.)[A-Za-z0-9._/-]+$/.test(sourceRef)) throw new Error("source_ref is not a safe Git ref");
  const commitSha = required(args.source_commit_sha, "source_commit_sha");
  if (!/^(?:[a-f0-9]{40}|[a-f0-9]{64})$/.test(commitSha)) throw new Error("source_commit_sha must be lowercase SHA-1/SHA-256");
  const sourceRepository = required(args.source_repository, "source_repository");
  if (!/^[^/\s]+\/[^/\s]+$/.test(sourceRepository)) throw new Error("source_repository must be owner/name");
  return {
    repository: required(args.repository, "repository"),
    releaseRef,
    ref: sourceRef,
    commitSha,
    workflow,
    runId: positive(args.source_run_id, "source_run_id"),
    attempt: positive(args.source_run_attempt, "source_run_attempt"),
    artifact: parseArtifact(args),
  };
}

function assertRegularReports(root) {
  for (const id of REQUIRED_REPORTS) {
    const file = path.join(root, "reports", `${id}.json`);
    let stat;
    try {
      stat = fs.lstatSync(file);
    } catch (error) {
      throw new Error(`raw evidence report is missing: ${id} (${error.message})`);
    }
    if (!stat.isFile() || stat.isSymbolicLink() || stat.size === 0) {
      throw new Error(`raw evidence report must be a non-empty regular file: ${id}`);
    }
  }
}

function assertBackupFourPlatform(root) {
  const file = path.join(root, "reports", "backup-restore-drill.json");
  const report = JSON.parse(fs.readFileSync(file, "utf8"));
  if (!isRecord(report.nativeDrill) || !isRecord(report.nativeDrill.platforms)) {
    throw new Error("backup-restore evidence must include nativeDrill.platforms from the prior-version drill");
  }
  const expected = ["macos-arm64", "linux-x64", "windows-x64", "windows-arm64"];
  const actual = Object.keys(report.nativeDrill.platforms).sort();
  if (actual.length !== expected.length || expected.some((platform) => !actual.includes(platform))) {
    throw new Error("backup-restore evidence must cover exactly four native release platforms");
  }
  for (const platform of expected) {
    const status = report.nativeDrill.platforms[platform]?.status;
    if (!["passed", "verified"].includes(status)) {
      throw new Error(`backup-restore evidence platform ${platform} is not verified`);
    }
  }
}

function assertIndependentReview(root) {
  const file = path.join(root, "reports", "security-review-inputs.json");
  const report = JSON.parse(fs.readFileSync(file, "utf8"));
  const signoff = report.independentReview ?? report.signOff;
  if (!isRecord(signoff) || signoff.independent !== true
    || !["approved", "signed_off"].includes(signoff.status)
    || typeof signoff.reviewer !== "string" || signoff.reviewer.trim() === ""
    || typeof signoff.approvedAt !== "string" || signoff.approvedAt.trim() === "") {
    throw new Error("security evidence must contain an independent reviewer sign-off");
  }
  const attestation = signoff.attestation ?? signoff.reviewArtifact;
  if (!isRecord(attestation) || typeof attestation.uri !== "string" || attestation.uri.trim() === ""
    || typeof attestation.sha256 !== "string" || !/^[a-f0-9]{64}$/.test(attestation.sha256)) {
    throw new Error("security evidence must include an independently retained review attestation digest");
  }
}

function writeBinding(root, binding, sourceRepository, output) {
  const target = path.resolve(output ?? path.join(root, "source-binding.json"));
  if (fs.existsSync(target)) throw new Error(`source binding output already exists: ${target}`);
  const document = {
    $schema: "./release-evidence-source-binding.schema.json",
    schemaVersion: SOURCE_BINDING_SCHEMA,
    sourceRepository,
    binding,
  };
  rejectUnknown(document.binding, BINDING_KEYS, "source binding");
  rejectUnknown(document.binding.artifact, ARTIFACT_KEYS, "source binding artifact");
  fs.writeFileSync(target, `${JSON.stringify(document, null, 2)}\n`, { flag: "wx" });
  return target;
}

export function verifySourceEvidence({ args = process.argv.slice(2) } = {}) {
  const parsed = typeof args === "object" && !Array.isArray(args) ? args : parseArgs(args);
  const root = path.resolve(required(parsed.root, "root"));
  const binding = expectedBinding(parsed);
  assertRegularReports(root);
  assertBackupFourPlatform(root);
  assertIndependentReview(root);
  const result = validateReleaseEvidencePayload({ baseDirectory: root, expectedBinding: binding });
  if (!result.valid) throw new Error(result.errors.join("; "));
  const sourceRepository = required(parsed.source_repository, "source_repository");
  const output = writeBinding(root, binding, sourceRepository, parsed.output);
  return { ...result, sourceBindingPath: output, sourceBinding: binding, sourceRepository };
}

export function main(argv = process.argv.slice(2)) {
  try {
    const result = verifySourceEvidence({ args: parseArgs(argv) });
    process.stdout.write(`${JSON.stringify(result, null, 2)}\n`);
    return 0;
  } catch (error) {
    process.stderr.write(`${error instanceof Error ? error.message : String(error)}\n`);
    return 1;
  }
}

if (import.meta.url === `file://${process.argv[1]}`) process.exitCode = main();
