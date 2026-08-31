#!/usr/bin/env node

import { createHash } from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import process from "node:process";

import {
  TRUSTED_BINDING_WORKFLOWS,
  TRUSTED_PAYLOAD_WORKFLOWS,
  validateExternalEvidenceManifest,
} from "./check-release-evidence-inputs.mjs";

export const RELEASE_EVIDENCE_WORKFLOW = "desktop-release-evidence.yml";
export const PAYLOAD_BINDING_SCHEMA = "jftrade.release-evidence-payload-binding.v1";

const REQUIRED_REPORTS = Object.freeze({
  "signed-updater-inputs": "signed-updater",
  "sbom-provenance-inputs": "sbom-provenance",
  "rollback-artifact-pair": "rollback-artifact",
  "backup-restore-drill": "backup-restore",
  "security-review-inputs": "security-review",
});

const REPORT_PATHS = Object.freeze({
  "signed-updater-inputs": "reports/signed-updater-inputs.json",
  "sbom-provenance-inputs": "reports/sbom-provenance-inputs.json",
  "rollback-artifact-pair": "reports/rollback-artifact-pair.json",
  "backup-restore-drill": "reports/backup-restore-drill.json",
  "security-review-inputs": "reports/security-review-inputs.json",
});
const PAYLOAD_METADATA_KEYS = Object.freeze([
  "$schema", "schemaVersion", "repository", "sourceRepository", "releaseRef", "evidenceRef", "payloadRun", "artifact", "sourceBinding",
]);
const PAYLOAD_RUN_KEYS = Object.freeze(["id", "attempt", "workflow", "ref", "commitSha"]);
const PAYLOAD_ARTIFACT_KEYS = Object.freeze(["name", "id", "digest"]);

function isRecord(value) {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function requiredString(value, label) {
  if (typeof value !== "string" || value.trim() === "") throw new Error(`${label} must be a non-empty string`);
  return value.trim();
}

function rejectUnknownKeys(value, allowed, label) {
  if (!isRecord(value)) return;
  const unknown = Object.keys(value).filter((key) => !allowed.includes(key));
  if (unknown.length > 0) throw new Error(`${label} contains unsupported field(s): ${unknown.join(", ")}`);
}

function safeGitRef(value, label) {
  const result = requiredString(value, label);
  const parts = result.split("/");
  if (!/^[A-Za-z0-9._/-]+$/.test(result) || result.startsWith("/") || result.endsWith("/")
    || result.includes("..") || parts.some((part) => part === "." || part === "..")) {
    throw new Error(`${label} must be a safe Git ref`);
  }
  return result;
}

function positiveInteger(value, label) {
  if (Number.isInteger(value) && value > 0) return value;
  if (typeof value === "string" && /^[1-9][0-9]*$/.test(value.trim())) return Number(value);
  throw new Error(`${label} must be a positive integer`);
}

function commitSha(value, label) {
  const result = requiredString(value, label);
  if (!/^(?:[a-f0-9]{40}|[a-f0-9]{64})$/.test(result)) throw new Error(`${label} must be a lowercase commit SHA`);
  return result;
}

function releaseRef(value, label) {
  const result = requiredString(value, label);
  if (!/^refs\/tags\/v\d+\.\d+\.\d+$/.test(result)) throw new Error(`${label} must be refs/tags/vX.Y.Z`);
  return result;
}

function artifactMetadata(value, label, expectedName) {
  if (!isRecord(value)) throw new Error(`${label} must be an object`);
  const name = requiredString(value.name, `${label}.name`);
  if (expectedName && name !== expectedName) throw new Error(`${label}.name must be ${expectedName}`);
  const id = positiveInteger(value.id, `${label}.id`);
  const digest = requiredString(value.digest, `${label}.digest`);
  if (!/^sha256:[a-f0-9]{64}$/.test(digest)) throw new Error(`${label}.digest is invalid`);
  return { name, id, digest };
}

function safeRelativePath(value, label) {
  const result = requiredString(value, label);
  if (path.isAbsolute(result) || result.includes("\\") || result.includes("\0")
    || result.split("/").some((part) => part === "" || part === "." || part === "..")) {
    throw new Error(`${label} must be a safe relative POSIX path`);
  }
  return result;
}

function sameArtifact(left, right) {
  return isRecord(left) && isRecord(right)
    && String(left.name) === String(right.name)
    && Number(left.id) === Number(right.id)
    && left.digest === right.digest;
}

function sha256(filePath) {
  return createHash("sha256").update(fs.readFileSync(filePath)).digest("hex");
}

function readRegularJson(root, relative, label) {
  const safe = safeRelativePath(relative, `${label}.path`);
  const base = path.resolve(root);
  const resolved = path.resolve(base, safe);
  const outside = path.relative(base, resolved);
  if (outside === ".." || outside.startsWith(`..${path.sep}`)) throw new Error(`${label} escapes payload root`);
  let cursor = base;
  for (const part of safe.split("/")) {
    cursor = path.join(cursor, part);
    const stat = fs.lstatSync(cursor);
    if (stat.isSymbolicLink()) throw new Error(`${label} must not traverse a symlink`);
  }
  const stat = fs.lstatSync(resolved);
  if (!stat.isFile() || stat.size === 0) throw new Error(`${label} must be a non-empty regular file`);
  let document;
  try {
    document = JSON.parse(fs.readFileSync(resolved, "utf8"));
  } catch (error) {
    throw new Error(`${label} must be valid JSON: ${error.message}`);
  }
  if (!isRecord(document)) throw new Error(`${label} must contain a JSON object`);
  return { resolved, relative: safe, stat, document };
}

function validatePayloadBinding(document, expected, label) {
  if (!isRecord(document.binding)) throw new Error(`${label}.binding is required; unbound payload evidence is rejected`);
  const binding = document.binding;
  rejectUnknownKeys(binding, ["repository", "releaseRef", "ref", "commitSha", "workflow", "runId", "attempt", "artifact"], `${label}.binding`);
  if (isRecord(binding.artifact)) rejectUnknownKeys(binding.artifact, PAYLOAD_ARTIFACT_KEYS, `${label}.binding.artifact`);
  for (const key of ["repository", "releaseRef", "ref", "commitSha", "workflow"]) {
    if (binding[key] !== expected[key]) throw new Error(`${label}.binding.${key} does not match the payload run`);
  }
  if (positiveInteger(binding.runId, `${label}.binding.runId`) !== expected.runId
    || positiveInteger(binding.attempt, `${label}.binding.attempt`) !== expected.attempt) {
    throw new Error(`${label}.binding run does not match the payload run`);
  }
  if (!sameArtifact(binding.artifact, expected.artifact)) throw new Error(`${label}.binding.artifact does not match the payload artifact`);
}

function loadPayloadMetadata(filePath) {
  const metadataPath = path.resolve(filePath);
  const metadataStat = fs.lstatSync(metadataPath);
  if (!metadataStat.isFile() || metadataStat.isSymbolicLink() || metadataStat.size === 0) {
    throw new Error("payload artifact metadata must be a non-empty regular file");
  }
  const metadata = JSON.parse(fs.readFileSync(metadataPath, "utf8"));
  if (!isRecord(metadata) || metadata.$schema !== "./release-evidence-payload-binding.schema.json"
    || metadata.schemaVersion !== PAYLOAD_BINDING_SCHEMA) {
    throw new Error("payload artifact metadata has an unsupported schema");
  }
  rejectUnknownKeys(metadata, PAYLOAD_METADATA_KEYS, "payload metadata");
  const payloadRun = metadata.payloadRun;
  const artifact = metadata.artifact;
  const sourceBinding = metadata.sourceBinding;
  if (!isRecord(payloadRun) || !isRecord(artifact)) throw new Error("payload artifact metadata is incomplete");
  if (!isRecord(sourceBinding)) throw new Error("payload artifact metadata.sourceBinding is required");
  rejectUnknownKeys(payloadRun, PAYLOAD_RUN_KEYS, "payload metadata.payloadRun");
  rejectUnknownKeys(artifact, PAYLOAD_ARTIFACT_KEYS, "payload metadata.artifact");
  rejectUnknownKeys(sourceBinding, ["repository", "releaseRef", "ref", "commitSha", "workflow", "runId", "attempt", "artifact"], "payload metadata.sourceBinding");
  if (isRecord(sourceBinding.artifact)) rejectUnknownKeys(sourceBinding.artifact, PAYLOAD_ARTIFACT_KEYS, "payload metadata.sourceBinding.artifact");
  const result = {
    repository: requiredString(metadata.repository, "payload metadata.repository"),
    sourceRepository: requiredString(metadata.sourceRepository, "payload metadata.sourceRepository"),
    releaseRef: releaseRef(metadata.releaseRef, "payload metadata.releaseRef"),
    evidenceRef: safeGitRef(metadata.evidenceRef, "payload metadata.evidenceRef"),
    payloadRun: {
      id: positiveInteger(payloadRun.id, "payload metadata.payloadRun.id"),
      attempt: positiveInteger(payloadRun.attempt, "payload metadata.payloadRun.attempt"),
      workflow: safeGitRef(payloadRun.workflow, "payload metadata.payloadRun.workflow"),
      ref: safeGitRef(payloadRun.ref, "payload metadata.payloadRun.ref"),
      commitSha: commitSha(payloadRun.commitSha, "payload metadata.payloadRun.commitSha"),
    },
    artifact: {
      name: requiredString(artifact.name, "payload metadata.artifact.name"),
      id: positiveInteger(artifact.id, "payload metadata.artifact.id"),
      digest: requiredString(artifact.digest, "payload metadata.artifact.digest"),
    },
    sourceBinding: {
      repository: requiredString(sourceBinding.repository, "payload metadata.sourceBinding.repository"),
      releaseRef: releaseRef(sourceBinding.releaseRef, "payload metadata.sourceBinding.releaseRef"),
      ref: safeGitRef(sourceBinding.ref, "payload metadata.sourceBinding.ref"),
      commitSha: commitSha(sourceBinding.commitSha, "payload metadata.sourceBinding.commitSha"),
      workflow: safeGitRef(sourceBinding.workflow, "payload metadata.sourceBinding.workflow"),
      runId: positiveInteger(sourceBinding.runId, "payload metadata.sourceBinding.runId"),
      attempt: positiveInteger(sourceBinding.attempt, "payload metadata.sourceBinding.attempt"),
      artifact: artifactMetadata(sourceBinding.artifact, "payload metadata.sourceBinding.artifact"),
    },
  };
  if (!TRUSTED_PAYLOAD_WORKFLOWS.includes(result.payloadRun.workflow)) {
    throw new Error("payload metadata payload workflow is not trusted");
  }
  if (!TRUSTED_BINDING_WORKFLOWS.includes(result.sourceBinding.workflow)) {
    throw new Error("payload metadata source workflow is not trusted");
  }
  if (!/^sha256:[a-f0-9]{64}$/.test(result.artifact.digest)) throw new Error("payload metadata.artifact.digest is invalid");
  if (result.repository !== result.sourceBinding.repository
    || result.releaseRef !== result.sourceBinding.releaseRef
    || result.evidenceRef !== result.sourceBinding.ref) {
    throw new Error("payload metadata source binding does not match release/evidence ref");
  }
  return result;
}

export function bindReleaseEvidence({
  payloadRoot,
  outputRoot,
  payloadMetadataPath,
  repository,
  releaseRef: releaseReference,
  releaseCommit,
  producerRunId,
  producerAttempt,
  producerArtifact,
}) {
  const metadata = loadPayloadMetadata(payloadMetadataPath);
  const release = releaseRef(releaseReference, "releaseRef");
  const commit = commitSha(releaseCommit, "releaseCommit");
  const producerId = positiveInteger(producerRunId, "producerRunId");
  const producerTry = positiveInteger(producerAttempt, "producerAttempt");
  const repo = requiredString(repository, "repository");
  const boundArtifact = artifactMetadata(producerArtifact, "producerArtifact", "desktop-release-evidence-payload");
  if (metadata.repository !== repo || metadata.releaseRef !== release) throw new Error("payload metadata release binding does not match producer inputs");
  if (metadata.sourceBinding.repository !== repo || metadata.sourceBinding.releaseRef !== release) {
    throw new Error("payload source binding release does not match producer inputs");
  }
  if (metadata.sourceBinding.ref !== metadata.evidenceRef) throw new Error("source binding ref must equal evidence_ref");
  if (metadata.payloadRun.ref !== metadata.evidenceRef) throw new Error("payload run ref must equal evidence_ref");
  if (metadata.evidenceRef === release) throw new Error("evidence_ref must be distinct from release tag");
  if (metadata.sourceBinding.commitSha !== commit) throw new Error("source binding commit does not match release commit");
  if (metadata.payloadRun.commitSha !== commit) throw new Error("payload run commit does not match release commit");

  const payloadBinding = metadata.sourceBinding;
  const base = path.resolve(payloadRoot);
  const output = path.resolve(outputRoot);
  fs.mkdirSync(output, { recursive: true });
  const evidence = {};
  for (const [id, kind] of Object.entries(REQUIRED_REPORTS)) {
    const input = readRegularJson(base, REPORT_PATHS[id], `payload ${id}`);
    validatePayloadBinding(input.document, payloadBinding, `payload ${id}`);
    const destination = path.join(output, "evidence", id, `${id}.json`);
    fs.mkdirSync(path.dirname(destination), { recursive: true });
    fs.copyFileSync(input.resolved, destination, fs.constants.COPYFILE_EXCL);
    const text = fs.readFileSync(destination);
    evidence[id] = {
      kind,
      status: "passed",
      files: [{
        path: `evidence/${id}/${id}.json`,
        sha256: createHash("sha256").update(text).digest("hex"),
        size: text.length,
        kind,
        schemaVersion: input.document.schemaVersion,
      }],
    };
  }
  const manifest = {
    $schema: "./release-evidence-inputs.schema.json",
    schemaVersion: "jftrade.release-evidence-inputs.v2",
    repository: repo,
    releaseRef: release,
    ref: release,
    commitSha: commit,
    workflow: RELEASE_EVIDENCE_WORKFLOW,
    runId: producerId,
    attempt: producerTry,
    artifact: boundArtifact,
    sourceBinding: payloadBinding,
    evidence,
  };
  const manifestPath = path.join(output, "release-evidence-inputs.json");
  fs.writeFileSync(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`, { flag: "wx" });
  fs.copyFileSync(
    path.resolve(payloadMetadataPath),
    path.join(output, "payload-artifact-metadata.json"),
    fs.constants.COPYFILE_EXCL,
  );
  const validation = validateExternalEvidenceManifest(manifest, {
    baseDirectory: output,
    expected: {
      repository: repo,
      releaseRef: release,
      ref: release,
      commitSha: commit,
      workflow: RELEASE_EVIDENCE_WORKFLOW,
      runId: producerId,
      attempt: producerTry,
      artifact: boundArtifact,
      sourceBinding: payloadBinding,
    },
    expectedArtifactMetadata: boundArtifact,
  });
  if (!validation.valid) throw new Error(`bound release evidence is invalid: ${validation.errors.join("; ")}`);
  return { manifest, metadata, files: Object.keys(evidence).length + 2, outputRoot: output };
}

function parseArgs(argv) {
  const result = {};
  for (let index = 0; index < argv.length; index += 1) {
    const token = argv[index];
    if (!token.startsWith("--") || index + 1 >= argv.length) throw new Error(`invalid argument: ${token}`);
    result[token.slice(2).replaceAll("-", "_")] = argv[index + 1];
    index += 1;
  }
  return result;
}

if (import.meta.url === `file://${process.argv[1]}`) {
  try {
    const args = parseArgs(process.argv.slice(2));
    const result = bindReleaseEvidence({
      payloadRoot: args.payload_root,
      outputRoot: args.output_root,
      payloadMetadataPath: args.payload_metadata,
      repository: args.repository,
      releaseRef: args.release_ref,
      releaseCommit: args.release_commit,
      producerRunId: args.producer_run_id,
      producerAttempt: args.producer_attempt,
      producerArtifact: {
        name: args.producer_artifact_name,
        id: args.producer_artifact_id,
        digest: args.producer_artifact_digest,
      },
    });
    process.stdout.write(`${JSON.stringify({ status: "bound", manifest: result.manifest, files: result.files }, null, 2)}\n`);
  } catch (error) {
    process.stderr.write(`${error.message}\n`);
    process.exitCode = 1;
  }
}
