#!/usr/bin/env node

import { createHash } from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import process from "node:process";

import { validateExternalEvidenceManifest } from "./check-release-evidence-inputs.mjs";

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

function isRecord(value) {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function requiredString(value, label) {
  if (typeof value !== "string" || value.trim() === "") throw new Error(`${label} must be a non-empty string`);
  return value.trim();
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
  const metadata = JSON.parse(fs.readFileSync(metadataPath, "utf8"));
  if (!isRecord(metadata) || metadata.$schema !== "./release-evidence-payload-binding.schema.json"
    || metadata.schemaVersion !== PAYLOAD_BINDING_SCHEMA) {
    throw new Error("payload artifact metadata has an unsupported schema");
  }
  const payloadRun = metadata.payloadRun;
  const artifact = metadata.artifact;
  if (!isRecord(payloadRun) || !isRecord(artifact)) throw new Error("payload artifact metadata is incomplete");
  const result = {
    repository: requiredString(metadata.repository, "payload metadata.repository"),
    releaseRef: releaseRef(metadata.releaseRef, "payload metadata.releaseRef"),
    evidenceRef: requiredString(metadata.evidenceRef, "payload metadata.evidenceRef"),
    payloadRun: {
      id: positiveInteger(payloadRun.id, "payload metadata.payloadRun.id"),
      attempt: positiveInteger(payloadRun.attempt, "payload metadata.payloadRun.attempt"),
      workflow: requiredString(payloadRun.workflow, "payload metadata.payloadRun.workflow"),
      ref: requiredString(payloadRun.ref, "payload metadata.payloadRun.ref"),
      commitSha: commitSha(payloadRun.commitSha, "payload metadata.payloadRun.commitSha"),
    },
    artifact: {
      name: requiredString(artifact.name, "payload metadata.artifact.name"),
      id: positiveInteger(artifact.id, "payload metadata.artifact.id"),
      digest: requiredString(artifact.digest, "payload metadata.artifact.digest"),
    },
  };
  if (!/^sha256:[a-f0-9]{64}$/.test(result.artifact.digest)) throw new Error("payload metadata.artifact.digest is invalid");
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
}) {
  const metadata = loadPayloadMetadata(payloadMetadataPath);
  const release = releaseRef(releaseReference, "releaseRef");
  const commit = commitSha(releaseCommit, "releaseCommit");
  const producerId = positiveInteger(producerRunId, "producerRunId");
  const producerTry = positiveInteger(producerAttempt, "producerAttempt");
  const repo = requiredString(repository, "repository");
  if (metadata.repository !== repo || metadata.releaseRef !== release) throw new Error("payload metadata release binding does not match producer inputs");
  if (metadata.payloadRun.ref !== metadata.evidenceRef) throw new Error("payload run ref must equal evidence_ref");
  if (metadata.payloadRun.commitSha !== commit) throw new Error("payload run commit does not match release commit");

  const payloadBinding = {
    repository: repo,
    releaseRef: release,
    ref: release,
    commitSha: commit,
    workflow: metadata.payloadRun.workflow,
    runId: metadata.payloadRun.id,
    attempt: metadata.payloadRun.attempt,
    artifact: metadata.artifact,
  };
  const producerBinding = {
    repository: repo,
    releaseRef: release,
    ref: release,
    commitSha: commit,
    workflow: RELEASE_EVIDENCE_WORKFLOW,
    runId: producerId,
    attempt: producerTry,
    artifact: metadata.artifact,
  };
  const base = path.resolve(payloadRoot);
  const output = path.resolve(outputRoot);
  fs.mkdirSync(output, { recursive: true });
  const evidence = {};
  for (const [id, kind] of Object.entries(REQUIRED_REPORTS)) {
    const input = readRegularJson(base, REPORT_PATHS[id], `payload ${id}`);
    validatePayloadBinding(input.document, payloadBinding, `payload ${id}`);
    const boundDocument = {
      ...input.document,
      sourceBinding: input.document.binding,
      binding: producerBinding,
    };
    const destination = path.join(output, "evidence", id, `${id}.json`);
    fs.mkdirSync(path.dirname(destination), { recursive: true });
    fs.writeFileSync(destination, `${JSON.stringify(boundDocument, null, 2)}\n`, { flag: "wx" });
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
    artifact: metadata.artifact,
    evidence,
  };
  const manifestPath = path.join(output, "release-evidence-inputs.json");
  fs.writeFileSync(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`, { flag: "wx" });
  fs.copyFileSync(path.resolve(payloadMetadataPath), path.join(output, "payload-artifact-metadata.json"));
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
      artifact: metadata.artifact,
    },
    expectedArtifactMetadata: metadata.artifact,
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
    });
    process.stdout.write(`${JSON.stringify({ status: "bound", manifest: result.manifest, files: result.files }, null, 2)}\n`);
  } catch (error) {
    process.stderr.write(`${error.message}\n`);
    process.exitCode = 1;
  }
}
