#!/usr/bin/env node

import { createHash } from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";
import { main as candidateMain } from "./check-release-candidate-cli.mjs";
import {
  buildReleaseCandidateEvidence,
  createReleaseCandidateEvidence,
  writeReleaseCandidateEvidence,
} from "./check-release-candidate-builder.mjs";
import { parseSafePositiveInteger } from "./check-release-evidence-inputs.mjs";

export { candidateMain as main };
export {
  buildReleaseCandidateEvidence,
  createReleaseCandidateEvidence,
  writeReleaseCandidateEvidence,
};

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));

export const RELEASE_CANDIDATE_EVIDENCE_SCHEMA =
  "jftrade.release-candidate-evidence.v1";
export const RELEASE_CANDIDATE_SCHEMA = RELEASE_CANDIDATE_EVIDENCE_SCHEMA;
export const REQUIRED_PLATFORMS = Object.freeze([
  "macos-arm64",
  "linux-x64",
  "windows-x64",
  "windows-arm64",
]);
export const REQUIRED_SOURCE_ARTIFACTS = Object.freeze([
  "desktop-release-linux",
  "desktop-release-macos",
  "desktop-release-windows",
  "desktop-release-windows-arm64",
]);

// These are repository-side prerequisites.  They are deliberately different
// from post-publication observations and from an independent security signoff.
export const REQUIRED_PREREQUISITES = Object.freeze([
  "candidate-admission",
  "signed-updater-inputs",
  "sbom-provenance-inputs",
  "rollback-artifact-pair",
  "backup-restore-drill",
  "security-review-inputs",
]);

// Every prerequisite must identify the type of release evidence it carries.
// Keeping this mapping here makes the checker the source of truth instead of
// allowing a workflow to relabel an arbitrary text file as a passed gate.
export const REQUIRED_PREREQUISITE_KINDS = Object.freeze({
  "candidate-admission": "release-source-admission",
  "signed-updater-inputs": "signed-updater",
  "sbom-provenance-inputs": "sbom-provenance",
  "rollback-artifact-pair": "rollback-artifact",
  "backup-restore-drill": "backup-restore",
  "security-review-inputs": "security-review",
});

export const RELEASE_CANDIDATE_LIMITATIONS = Object.freeze([
  "This pre-release evidence does not prove post-release smoke or native lifecycle observations.",
  "Repository security inputs are recorded, but an independent security sign-off remains required.",
  "Artifact and checksum digests are verified locally; publication, attestation and platform runner trust remain external.",
]);

const requiredRootKeys = Object.freeze([
  "$schema",
  "schemaVersion",
  "phase",
  "status",
  "releaseRef",
  "releaseTag",
  "commitSha",
  "workflowRun",
  "sourceWorkflowRun",
  "platforms",
  "sourceArtifacts",
  "sha256sums",
  "prerequisites",
]);

function isRecord(value) {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function nonEmptyString(value, label, errors) {
  if (typeof value !== "string" || value.trim() === "") {
    errors.push(`${label} must be a non-empty string`);
    return null;
  }
  return value.trim();
}

function fileSha256(filePath) {
  return createHash("sha256").update(fs.readFileSync(filePath)).digest("hex");
}

function validDigest(value) {
  return typeof value === "string" && /^[a-f0-9]{64}$/.test(value.trim().toLowerCase());
}

function digestOrError(value, label, errors) {
  if (!validDigest(value)) {
    errors.push(`${label} must be a lowercase SHA-256 digest`);
    return null;
  }
  return value.trim().toLowerCase();
}

function validCommit(value) {
  return typeof value === "string" && /^(?:[a-f0-9]{40}|[a-f0-9]{64})$/.test(value.trim());
}

function commitOrError(value, label, errors) {
  if (!validCommit(value)) {
    errors.push(`${label} must be a 40 or 64 character lowercase commit SHA`);
    return null;
  }
  return value.trim();
}

function semverTag(value, label, errors) {
  const tag = nonEmptyString(value, label, errors);
  if (tag && !/^v\d+\.\d+\.\d+$/.test(tag)) {
    errors.push(`${label} must be a vX.Y.Z release tag`);
  }
  return tag;
}

function runId(value, label, errors) {
  const parsed = parseSafePositiveInteger(value);
  if (parsed === null) {
    errors.push(`${label} must be a positive workflow run id`);
    return null;
  }
  return parsed;
}

function positiveAttempt(value, label, errors) {
  const parsed = parseSafePositiveInteger(value);
  if (parsed !== null) return parsed;
  errors.push(`${label} must be a positive integer`);
  return null;
}

function readJson(filePath, label) {
  let text;
  try {
    text = fs.readFileSync(filePath, "utf8");
  } catch (error) {
    throw new Error(`cannot read ${label} ${filePath}: ${error.message}`);
  }
  try {
    return JSON.parse(text);
  } catch (error) {
    throw new Error(`cannot parse ${label} ${filePath}: ${error.message}`);
  }
}

function resolveContained(reference, baseDirectory, label, errors) {
  const value = nonEmptyString(reference, `${label}.path`, errors);
  if (!value) return null;
  if (path.isAbsolute(value) || /^[A-Za-z]:/.test(value) || value.startsWith("\\\\")
    || value.includes("\\") || value.includes("\0")) {
    errors.push(`${label}.path must be a relative POSIX path`);
    return null;
  }
  const segments = value.split("/");
  if (segments.some((segment) => segment === "" || segment === "." || segment === "..")) {
    errors.push(`${label}.path must not contain empty, dot or parent path segments`);
    return null;
  }
  let root;
  try {
    root = fs.realpathSync(path.resolve(baseDirectory));
  } catch (error) {
    errors.push(`${label} evidence directory is missing: ${error.message}`);
    return null;
  }
  const resolved = path.resolve(root, ...segments);
  const relative = path.relative(root, resolved);
  if (relative === ".." || relative.startsWith(`..${path.sep}`)) {
    errors.push(`${label}.path escapes the evidence base directory`);
    return null;
  }
  let current = root;
  try {
    for (const segment of segments) {
      current = path.join(current, segment);
      if (fs.lstatSync(current).isSymbolicLink()) {
        errors.push(`${label}.path must not traverse a symlink`);
        return null;
      }
    }
    const real = fs.realpathSync(resolved);
    const realRelative = path.relative(root, real);
    if (realRelative === ".." || realRelative.startsWith(`..${path.sep}`)) {
      errors.push(`${label}.path realpath escapes the evidence directory`);
      return null;
    }
  } catch (error) {
    errors.push(`${label} file is missing: ${value} (${error.message})`);
    return null;
  }
  return resolved;
}

function portablePath(reference, baseDirectory, label) {
  if (typeof reference !== "string" || reference.trim() === "") {
    throw new Error(`${label}.path must be a non-empty string`);
  }
  const value = reference.trim();
  const absolute = path.resolve(baseDirectory, value);
  const root = path.resolve(baseDirectory);
  const relative = path.relative(root, absolute);
  if (relative === ".." || relative.startsWith(`..${path.sep}`)) {
    throw new Error(`${label}.path must be inside the evidence base directory`);
  }
  return relative.split(path.sep).join("/");
}

function inspectFile(reference, expected, baseDirectory, label, errors) {
  const result = { path: reference ?? null, sha256: expected ?? null, exists: false, valid: false };
  const filePath = resolveContained(reference, baseDirectory, label, errors);
  if (!filePath) return result;
  let stat;
  try {
    stat = fs.statSync(filePath);
  } catch (error) {
    errors.push(`${label} file is missing: ${reference} (${error.message})`);
    return result;
  }
  if (!stat.isFile() || stat.size === 0) {
    errors.push(`${label} file is missing or empty: ${reference}`);
    return result;
  }
  result.exists = true;
  const digest = digestOrError(expected, `${label}.sha256`, errors);
  if (!digest) return result;
  try {
    result.actualSha256 = fileSha256(filePath);
  } catch (error) {
    errors.push(`${label} file could not be hashed: ${reference} (${error.message})`);
    return result;
  }
  if (result.actualSha256 !== digest) {
    errors.push(`${label} SHA-256 mismatch for ${reference}`);
    return result;
  }
  result.valid = true;
  result.size = stat.size;
  return result;
}

function referenceObject(value, label, errors) {
  if (typeof value === "string") return { path: value };
  if (!isRecord(value)) {
    errors.push(`${label} must be an object with path and sha256`);
    return null;
  }
  return value;
}

function validateReference(value, baseDirectory, label, errors, { allowSize = true } = {}) {
  const entry = referenceObject(value, label, errors);
  if (!entry) return null;
  const allowed = allowSize
    ? ["path", "sha256", "size", "kind", "schemaVersion"]
    : ["path", "sha256", "kind", "schemaVersion"];
  for (const key of Object.keys(entry)) {
    if (!allowed.includes(key)) errors.push(`${label}.${key} is not allowed`);
  }
  const checked = inspectFile(entry.path, entry.sha256, baseDirectory, label, errors);
  if ("size" in entry && (!Number.isInteger(entry.size) || entry.size <= 0)) {
    errors.push(`${label}.size must be a positive integer`);
  } else if ("size" in entry && checked.exists && checked.size !== entry.size) {
    errors.push(`${label}.size does not match ${entry.path}`);
  }
  return { ...entry, ...checked };
}

function normalizeWorkflowRun(value, label, errors) {
  if (!isRecord(value)) {
    errors.push(`${label} must be an object`);
    return null;
  }
  const allowed = ["id", "runId", "attempt", "runAttempt", "workflow", "name", "ref", "commitSha", "url"];
  for (const key of Object.keys(value)) {
    if (!allowed.includes(key)) errors.push(`${label}.${key} is not allowed`);
  }
  const id = runId(value.id ?? value.runId, `${label}.id`, errors);
  const attempt = positiveAttempt(value.attempt ?? value.runAttempt, `${label}.attempt`, errors);
  const workflow = nonEmptyString(value.workflow ?? value.name, `${label}.workflow`, errors);
  const ref = nonEmptyString(value.ref, `${label}.ref`, errors);
  const commitSha = commitOrError(value.commitSha, `${label}.commitSha`, errors);
  if ("url" in value) {
    try {
      const url = new URL(nonEmptyString(value.url, `${label}.url`, errors) ?? "");
      if (url.protocol !== "https:" || !url.hostname || url.username || url.password) {
        errors.push(`${label}.url must be an HTTPS URL without credentials`);
      }
    } catch {
      errors.push(`${label}.url must be a valid HTTPS URL`);
    }
  }
  const url = typeof value.url === "string" && value.url.trim() !== ""
    ? value.url.trim()
    : undefined;
  return { id, attempt, workflow, ref, commitSha, ...(url ? { url } : {}) };
}

function sameRun(left, right) {
  return left && right
    && left.id === right.id
    && left.attempt === right.attempt
    && left.workflow === right.workflow
    && left.ref === right.ref
    && left.commitSha === right.commitSha;
}

function validateExpectedBinding(document, options, errors) {
  const expected = options.expected ?? options;
  for (const [field, label] of [
    ["releaseRef", "release ref"],
    ["releaseTag", "release tag"],
    ["commitSha", "commit SHA"],
  ]) {
    if (expected[field] !== undefined && String(document[field]) !== String(expected[field])) {
      errors.push(`manifest.${field} does not match expected ${label}`);
    }
  }
  if (expected.workflowRun !== undefined) {
    const expectedRun = normalizeWorkflowRun(expected.workflowRun, "expected.workflowRun", errors);
    const actualRun = normalizeWorkflowRun(document.workflowRun, "manifest.workflowRun", errors);
    if (expectedRun && actualRun && !sameRun(actualRun, expectedRun)) {
      errors.push("manifest.workflowRun does not match expected workflow run");
    }
  }
  if (expected.sourceWorkflowRun !== undefined) {
    const expectedRun = normalizeWorkflowRun(
      expected.sourceWorkflowRun,
      "expected.sourceWorkflowRun",
      errors,
    );
    const actualRun = normalizeWorkflowRun(
      document.sourceWorkflowRun,
      "manifest.sourceWorkflowRun",
      errors,
    );
    if (expectedRun && actualRun && !sameRun(actualRun, expectedRun)) {
      errors.push("manifest.sourceWorkflowRun does not match expected source workflow run");
    }
  }
}

function releaseRefErrors(releaseRef, releaseTag, errors, label = "manifest") {
  const ref = nonEmptyString(releaseRef, `${label}.releaseRef`, errors);
  const tag = semverTag(releaseTag, `${label}.releaseTag`, errors);
  if (ref && ref.startsWith("refs/tags/") && ref !== `refs/tags/${tag}`) {
    errors.push(`${label}.releaseRef must match releaseTag for a tag ref`);
  }
  if (ref && !/^(?:refs\/(?:heads|tags)\/|[A-Za-z0-9._/-]+$)/.test(ref)) {
    errors.push(`${label}.releaseRef must be a safe Git ref`);
  }
  return { ref, tag };
}

function normalizePlatformArtifacts(platformValue, label, errors) {
  if (!isRecord(platformValue)) {
    errors.push(`${label} must be an object`);
    return { manifest: null, artifacts: [] };
  }
  const manifest = platformValue.manifest ?? platformValue.packageManifest;
  const source = platformValue.artifacts ?? platformValue.artifact ?? platformValue.packages;
  const values = Array.isArray(source) ? source : source === undefined ? [] : [source];
  if (values.length === 0) errors.push(`${label}.artifacts must contain at least one artifact`);
  return { manifest, artifacts: values };
}

function checksumEntries(filePath, sumsPath, baseDirectory, errors) {
  let text;
  try {
    text = fs.readFileSync(filePath, "utf8");
  } catch (error) {
    errors.push(`SHA256SUMS could not be read: ${error.message}`);
    return [];
  }
  const entries = [];
  const seen = new Set();
  for (const [index, line] of text.split(/\r?\n/).entries()) {
    if (line.trim() === "") continue;
    const match = line.match(/^([a-f0-9]{64})\s+\*?(.+)$/i);
    if (!match) {
      errors.push(`SHA256SUMS line ${index + 1} is not a SHA-256 entry`);
      continue;
    }
    const digest = match[1].toLowerCase();
    const relative = match[2].trim();
    if (!relative || path.isAbsolute(relative)) {
      errors.push(`SHA256SUMS entry path must be relative: ${relative}`);
      continue;
    }
    if (seen.has(relative)) {
      errors.push(`SHA256SUMS contains duplicate entry: ${relative}`);
      continue;
    }
    seen.add(relative);
    const resolved = path.resolve(path.dirname(sumsPath), relative);
    const root = path.resolve(baseDirectory);
    const outside = path.relative(root, resolved);
    if (outside === ".." || outside.startsWith(`..${path.sep}`)) {
      errors.push(`SHA256SUMS entry escapes the evidence base directory: ${relative}`);
      continue;
    }
    let actual;
    try {
      if (!fs.statSync(resolved).isFile()) throw new Error("not a file");
      actual = fileSha256(resolved);
    } catch (error) {
      errors.push(`SHA256SUMS entry is missing: ${relative} (${error.message})`);
      continue;
    }
    if (actual !== digest) errors.push(`SHA256SUMS digest mismatch for ${relative}`);
    entries.push({ path: relative.split(path.sep).join("/"), sha256: digest });
  }
  if (entries.length === 0) errors.push("SHA256SUMS must contain at least one entry");
  return entries;
}

function validateChecksums(value, baseDirectory, declaredFiles, errors) {
  if (!isRecord(value)) {
    errors.push("manifest.sha256sums must be an object");
    return null;
  }
  const checked = validateReference(value, baseDirectory, "manifest.sha256sums", errors);
  if (!checked?.valid) return checked;
  const sumsPath = path.resolve(baseDirectory, checked.path);
  const entries = checksumEntries(sumsPath, sumsPath, baseDirectory, errors);
  const normalizedDeclared = new Set(declaredFiles.map((file) => path.resolve(baseDirectory, file)));
  const represented = new Set();
  for (const entry of entries) {
    represented.add(path.resolve(path.dirname(sumsPath), entry.path));
  }
  for (const file of normalizedDeclared) {
    if (!represented.has(file)) {
      errors.push(`SHA256SUMS is missing declared release file: ${path.relative(baseDirectory, file)}`);
    }
  }
  return { ...checked, entries };
}

function rejectEvidencePlaceholder(filePath, kind, label, errors) {
  let text;
  try {
    text = fs.readFileSync(filePath, "utf8");
  } catch (error) {
    errors.push(`${label} could not be read for content validation: ${error.message}`);
    return;
  }
  if (text.trim() === "") {
    errors.push(`${label} is empty or whitespace-only`);
    return;
  }
  // A generated report may mention that independent release evidence is still
  // required in an explanatory field.  It may not use that marker as its
  // status, which is the exact placeholder emitted by the old workflow.
  try {
    const parsed = JSON.parse(text);
    if (isRecord(parsed) && parsed.status === "external_release_runner_evidence_required") {
      errors.push(`${label} is a placeholder, not verified ${kind} evidence`);
    }
  } catch {
    // Binary packages and signed text are intentionally not JSON. Their
    // existence/hash is checked above; only the known rollback command output
    // is forbidden below.
  }
  if (kind === "rollback-artifact" && /check-signed-updater-lifecycle(?:\.mjs)?/.test(text)) {
    errors.push(`${label} contains a lifecycle-check command instead of rollback artifact evidence`);
  }
}

function validatePrerequisites(values, baseDirectory, release, workflow, sourceWorkflow, errors) {
  if (!Array.isArray(values) || values.length === 0) {
    errors.push("manifest.prerequisites must be a non-empty array");
    return [];
  }
  const seen = new Set();
  const validated = [];
  for (const [index, value] of values.entries()) {
    const label = `manifest.prerequisites[${index}]`;
    if (!isRecord(value)) {
      errors.push(`${label} must be an object`);
      continue;
    }
    const allowed = [
      "id",
      "kind",
      "status",
      "releaseRef",
      "commitSha",
      "workflowRun",
      "sourceWorkflowRun",
      "evidence",
      "summary",
    ];
    for (const key of Object.keys(value)) {
      if (!allowed.includes(key)) errors.push(`${label}.${key} is not allowed`);
    }
    const id = nonEmptyString(value.id, `${label}.id`, errors);
    if (id && seen.has(id)) errors.push(`duplicate prerequisite id: ${id}`);
    if (id) seen.add(id);
    const expectedKind = id ? REQUIRED_PREREQUISITE_KINDS[id] : undefined;
    const kind = nonEmptyString(value.kind, `${label}.kind`, errors);
    if (!expectedKind) {
      if (id) errors.push(`${label}.id is not a recognized prerequisite`);
    } else if (kind !== expectedKind) {
      errors.push(`${label}.kind must be ${expectedKind} for ${id}`);
    }
    if (value.status !== "passed") errors.push(`${label}.status must be passed`);
    const ref = nonEmptyString(value.releaseRef, `${label}.releaseRef`, errors);
    const commitSha = commitOrError(value.commitSha, `${label}.commitSha`, errors);
    if (ref && ref !== release.ref) errors.push(`${label}.releaseRef does not match manifest.releaseRef`);
    if (commitSha && commitSha !== release.commitSha) errors.push(`${label}.commitSha does not match manifest.commitSha`);
    const run = normalizeWorkflowRun(value.workflowRun, `${label}.workflowRun`, errors);
    if (run && !sameRun(run, workflow)) errors.push(`${label}.workflowRun does not match manifest.workflowRun`);
    const sourceRun = normalizeWorkflowRun(
      value.sourceWorkflowRun,
      `${label}.sourceWorkflowRun`,
      errors,
    );
    if (sourceRun && sourceWorkflow && (
      sourceRun.ref !== sourceWorkflow.ref || sourceRun.commitSha !== sourceWorkflow.commitSha
    )) {
      errors.push(`${label}.sourceWorkflowRun does not match manifest release ref/commit`);
    }
    if (!Array.isArray(value.evidence) || value.evidence.length === 0) {
      errors.push(`${label}.evidence must be a non-empty array`);
    }
    const evidence = [];
    for (const [evidenceIndex, entry] of (Array.isArray(value.evidence) ? value.evidence : []).entries()) {
      const evidenceLabel = `${label}.evidence[${evidenceIndex}]`;
      const evidenceObject = referenceObject(entry, evidenceLabel, errors);
      if (evidenceObject && evidenceObject.kind !== kind) {
        errors.push(`${evidenceLabel}.kind must match ${label}.kind`);
      }
      if (evidenceObject && (typeof evidenceObject.schemaVersion !== "string" || evidenceObject.schemaVersion.trim() === "")) {
        errors.push(`${evidenceLabel}.schemaVersion must be a non-empty string`);
      }
      const checked = validateReference(
        entry,
        baseDirectory,
        evidenceLabel,
        errors,
      );
      if (checked?.valid && kind) rejectEvidencePlaceholder(
        path.resolve(baseDirectory, checked.path),
        kind,
        evidenceLabel,
        errors,
      );
      evidence.push(checked);
    }
    validated.push({
      ...value,
      id,
      kind,
      releaseRef: ref,
      commitSha,
      workflowRun: run,
      sourceWorkflowRun: sourceRun,
      evidence,
    });
  }
  for (const required of REQUIRED_PREREQUISITES) {
    if (!seen.has(required)) errors.push(`missing prerequisite evidence: ${required}`);
  }
  return validated;
}

/**
 * Inspect a pre-release evidence document.  The checker binds every input to
 * one immutable release ref, commit and workflow run, and verifies local file
 * digests. It never mutates source readiness evidence or calls external services.
 */
export function inspectReleaseCandidateEvidence(document, options = {}) {
  const errors = [];
  let value = document;
  let baseDirectory = path.resolve(options.baseDirectory ?? repositoryRoot);
  if (typeof document === "string") {
    const absolutePath = path.resolve(document);
    baseDirectory = path.resolve(options.baseDirectory ?? path.dirname(absolutePath));
    try {
      value = readJson(absolutePath, "release-candidate evidence");
    } catch (error) {
      errors.push(error instanceof Error ? error.message : String(error));
      return resultReport(null, errors, baseDirectory);
    }
  }
  if (!isRecord(value)) {
    errors.push("release-candidate evidence must be a JSON object");
    return resultReport(document, errors, baseDirectory);
  }
  document = value;
  for (const key of Object.keys(document)) {
    if (![...requiredRootKeys, "limitations"].includes(key)) errors.push(`manifest.${key} is not allowed`);
  }
  for (const key of requiredRootKeys) {
    if (!(key in document)) errors.push(`manifest.${key} is required`);
  }
  if (document.$schema !== "./release-candidate-evidence.schema.json") {
    errors.push("manifest.$schema must be ./release-candidate-evidence.schema.json");
  }
  if (document.schemaVersion !== RELEASE_CANDIDATE_EVIDENCE_SCHEMA) {
    errors.push(`manifest.schemaVersion must be ${RELEASE_CANDIDATE_EVIDENCE_SCHEMA}`);
  }
  if (document.phase !== "pre-release") errors.push("manifest.phase must be pre-release");
  if (document.status !== "candidate_ready") errors.push("manifest.status must be candidate_ready");
  const releaseValues = releaseRefErrors(document.releaseRef, document.releaseTag, errors);
  const commitSha = commitOrError(document.commitSha, "manifest.commitSha", errors);
  const workflow = normalizeWorkflowRun(document.workflowRun, "manifest.workflowRun", errors);
  const sourceWorkflow = normalizeWorkflowRun(
    document.sourceWorkflowRun,
    "manifest.sourceWorkflowRun",
    errors,
  );
  validateExpectedBinding(document, options, errors);
  if (workflow && releaseValues.ref && workflow.ref !== releaseValues.ref) {
    errors.push("manifest.workflowRun.ref does not match manifest.releaseRef");
  }
  if (workflow && commitSha && workflow.commitSha !== commitSha) {
    errors.push("manifest.workflowRun.commitSha does not match manifest.commitSha");
  }
  if (sourceWorkflow && releaseValues.ref && sourceWorkflow.ref !== releaseValues.ref) {
    errors.push("manifest.sourceWorkflowRun.ref does not match manifest.releaseRef");
  }
  if (sourceWorkflow && commitSha && sourceWorkflow.commitSha !== commitSha) {
    errors.push("manifest.sourceWorkflowRun.commitSha does not match manifest.commitSha");
  }

  const platforms = isRecord(document.platforms) ? document.platforms : null;
  if (!platforms) {
    errors.push("manifest.platforms must be an object");
  }
  const declaredFiles = [];
  const validatedPlatforms = {};
  for (const platform of REQUIRED_PLATFORMS) {
    if (!platforms || !(platform in platforms)) {
      errors.push(`missing release platform evidence: ${platform}`);
      continue;
    }
    const normalized = normalizePlatformArtifacts(platforms[platform], `manifest.platforms.${platform}`, errors);
    const manifest = validateReference(
      normalized.manifest,
      baseDirectory,
      `manifest.platforms.${platform}.manifest`,
      errors,
    );
    if (manifest?.path) declaredFiles.push(manifest.path);
    const artifacts = normalized.artifacts.map((entry, index) => {
      const checked = validateReference(
        entry,
        baseDirectory,
        `manifest.platforms.${platform}.artifacts[${index}]`,
        errors,
      );
      if (checked?.path) declaredFiles.push(checked.path);
      return checked;
    });
    validatedPlatforms[platform] = { manifest, artifacts };
  }
  if (platforms) {
    for (const platform of Object.keys(platforms)) {
      if (!REQUIRED_PLATFORMS.includes(platform)) errors.push(`unknown release platform: ${platform}`);
    }
  }
  if ("sourceArtifacts" in document) {
    if (!Array.isArray(document.sourceArtifacts) || document.sourceArtifacts.length !== REQUIRED_SOURCE_ARTIFACTS.length) {
      errors.push(`manifest.sourceArtifacts must contain exactly ${REQUIRED_SOURCE_ARTIFACTS.length} artifacts`);
    } else {
      const seenSourceArtifacts = new Set();
      for (const [index, sourceArtifact] of document.sourceArtifacts.entries()) {
        const label = `manifest.sourceArtifacts[${index}]`;
        if (!isRecord(sourceArtifact)) {
          errors.push(`${label} must be an object`);
          continue;
        }
        for (const key of Object.keys(sourceArtifact)) {
          if (!["name", "id", "digest", "expired", "runId", "runAttempt", "workflow", "ref", "commitSha"].includes(key)) {
            errors.push(`${label}.${key} is not allowed`);
          }
        }
        const name = nonEmptyString(sourceArtifact.name, `${label}.name`, errors);
        if (name && !REQUIRED_SOURCE_ARTIFACTS.includes(name)) errors.push(`${label}.name is not a required source artifact`);
        if (name && seenSourceArtifacts.has(name)) errors.push(`${label}.name is duplicated`);
        if (name) seenSourceArtifacts.add(name);
        runId(sourceArtifact.id, `${label}.id`, errors);
        if (!validDigest(String(sourceArtifact.digest ?? "").replace(/^sha256:/, ""))) {
          errors.push(`${label}.digest must be a SHA-256 artifact digest`);
        }
        if (sourceArtifact.expired !== false) errors.push(`${label}.expired must be false`);
        const sourceArtifactRunId = runId(sourceArtifact.runId, `${label}.runId`, errors);
        const sourceArtifactAttempt = positiveAttempt(sourceArtifact.runAttempt, `${label}.runAttempt`, errors);
        for (const key of ["workflow", "ref", "commitSha"]) {
          if (typeof sourceArtifact[key] !== "string" || sourceArtifact[key].trim() === "") errors.push(`${label}.${key} must be non-empty`);
        }
        if (sourceArtifact.workflow !== "desktop-release.yml") errors.push(`${label}.workflow must be desktop-release.yml`);
        if (typeof sourceArtifact.ref === "string" && !/^refs\/tags\/v\d+\.\d+\.\d+$/.test(sourceArtifact.ref)) errors.push(`${label}.ref must be a release tag ref`);
        if (typeof sourceArtifact.commitSha === "string" && !validCommit(sourceArtifact.commitSha)) errors.push(`${label}.commitSha must be a commit SHA`);
        if (sourceWorkflow && sourceArtifactRunId !== sourceWorkflow.id) {
          errors.push(`${label}.runId does not match manifest.sourceWorkflowRun.id`);
        }
        if (sourceWorkflow && sourceArtifactAttempt !== sourceWorkflow.attempt) {
          errors.push(`${label}.runAttempt does not match manifest.sourceWorkflowRun.attempt`);
        }
        if (sourceWorkflow && sourceArtifact.workflow !== sourceWorkflow.workflow) errors.push(`${label}.workflow does not match manifest.sourceWorkflowRun.workflow`);
        if (sourceWorkflow && sourceArtifact.ref !== sourceWorkflow.ref) errors.push(`${label}.ref does not match manifest.sourceWorkflowRun.ref`);
        if (sourceWorkflow && sourceArtifact.commitSha !== sourceWorkflow.commitSha) errors.push(`${label}.commitSha does not match manifest.sourceWorkflowRun.commitSha`);
      }
      for (const required of REQUIRED_SOURCE_ARTIFACTS) if (!seenSourceArtifacts.has(required)) errors.push(`manifest.sourceArtifacts is missing required artifact: ${required}`);
    }
  }
  const shaSums = validateChecksums(document.sha256sums, baseDirectory, declaredFiles, errors);
  const prerequisites = validatePrerequisites(
    document.prerequisites,
    baseDirectory,
    { ref: releaseValues.ref, commitSha },
    workflow,
    sourceWorkflow,
    errors,
  );
  if ("limitations" in document) {
    if (!Array.isArray(document.limitations)) errors.push("manifest.limitations must be a string array");
    else document.limitations.forEach((item, index) => nonEmptyString(item, `manifest.limitations[${index}]`, errors));
  }
  return {
    schemaVersion: RELEASE_CANDIDATE_EVIDENCE_SCHEMA,
    phase: "pre-release",
    status: errors.length === 0 ? "candidate_ready" : "invalid_inputs",
    valid: errors.length === 0,
    releaseQualified: false,
    releaseQualification: "pre_release_inputs_verified_only",
    releaseRef: releaseValues.ref,
    releaseTag: releaseValues.tag,
    commitSha,
    workflowRun: workflow,
    sourceWorkflowRun: sourceWorkflow,
    platforms: validatedPlatforms,
    sha256sums: shaSums,
    prerequisites,
    errors,
    limitations: [...RELEASE_CANDIDATE_LIMITATIONS],
    externalRequirements: [
      "Post-release smoke and native install/upgrade/uninstall/rollback observations are still required.",
      "An independent security review/sign-off remains required.",
    ],
    baseDirectory,
  };
}

function resultReport(document, errors, baseDirectory) {
  return {
    schemaVersion: RELEASE_CANDIDATE_EVIDENCE_SCHEMA,
    phase: "pre-release",
    status: "invalid_inputs",
    valid: false,
    releaseQualified: false,
    releaseQualification: "pre_release_inputs_verified_only",
    releaseRef: null,
    releaseTag: null,
    commitSha: null,
    workflowRun: null,
    sourceWorkflowRun: null,
    platforms: {},
    prerequisites: [],
    sha256sums: null,
    errors,
    limitations: [...RELEASE_CANDIDATE_LIMITATIONS],
    externalRequirements: [
      "Post-release smoke and native install/upgrade/uninstall/rollback observations are still required.",
      "An independent security review/sign-off remains required.",
    ],
    baseDirectory,
  };
}

export const validateReleaseCandidateEvidence = inspectReleaseCandidateEvidence;
export const checkReleaseCandidateEvidence = inspectReleaseCandidateEvidence;
export const evaluateReleaseCandidate = inspectReleaseCandidateEvidence;

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  process.exitCode = candidateMain();
}
