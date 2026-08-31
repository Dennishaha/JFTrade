#!/usr/bin/env node

import { createHash } from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

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

export const RELEASE_CANDIDATE_LIMITATIONS = Object.freeze([
  "This pre-release evidence does not prove post-release smoke or native lifecycle observations.",
  "This pre-release evidence does not prove hard-cut readiness or owner-deletion closeout.",
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
  "platforms",
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
  if (!(Number.isInteger(value) && value > 0) && !(typeof value === "string" && /^\d+$/.test(value.trim()))) {
    errors.push(`${label} must be a positive workflow run id`);
    return null;
  }
  return typeof value === "number" ? value : value.trim();
}

function positiveAttempt(value, label, errors) {
  if (Number.isInteger(value) && value >= 1) return value;
  if (typeof value === "string" && /^\d+$/.test(value.trim()) && Number(value) >= 1) {
    return Number(value);
  }
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
  if (path.isAbsolute(value)) {
    errors.push(`${label}.path must be relative to the evidence base directory`);
    return null;
  }
  const root = path.resolve(baseDirectory);
  const resolved = path.resolve(root, value);
  const relative = path.relative(root, resolved);
  if (relative === ".." || relative.startsWith(`..${path.sep}`)) {
    errors.push(`${label}.path escapes the evidence base directory`);
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
  const allowed = allowSize ? ["path", "sha256", "size", "kind"] : ["path", "sha256", "kind"];
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
    && String(left.id) === String(right.id)
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

function validatePrerequisites(values, baseDirectory, release, workflow, errors) {
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
    const allowed = ["id", "status", "releaseRef", "commitSha", "workflowRun", "evidence", "summary"];
    for (const key of Object.keys(value)) {
      if (!allowed.includes(key)) errors.push(`${label}.${key} is not allowed`);
    }
    const id = nonEmptyString(value.id, `${label}.id`, errors);
    if (id && seen.has(id)) errors.push(`duplicate prerequisite id: ${id}`);
    if (id) seen.add(id);
    if (value.status !== "passed") errors.push(`${label}.status must be passed`);
    const ref = nonEmptyString(value.releaseRef, `${label}.releaseRef`, errors);
    const commitSha = commitOrError(value.commitSha, `${label}.commitSha`, errors);
    if (ref && ref !== release.ref) errors.push(`${label}.releaseRef does not match manifest.releaseRef`);
    if (commitSha && commitSha !== release.commitSha) errors.push(`${label}.commitSha does not match manifest.commitSha`);
    const run = normalizeWorkflowRun(value.workflowRun, `${label}.workflowRun`, errors);
    if (run && !sameRun(run, workflow)) errors.push(`${label}.workflowRun does not match manifest.workflowRun`);
    if (!Array.isArray(value.evidence) || value.evidence.length === 0) {
      errors.push(`${label}.evidence must be a non-empty array`);
    }
    const evidence = [];
    for (const [evidenceIndex, entry] of (Array.isArray(value.evidence) ? value.evidence : []).entries()) {
      evidence.push(validateReference(
        entry,
        baseDirectory,
        `${label}.evidence[${evidenceIndex}]`,
        errors,
      ));
    }
    validated.push({ ...value, id, releaseRef: ref, commitSha, workflowRun: run, evidence });
  }
  for (const required of REQUIRED_PREREQUISITES) {
    if (!seen.has(required)) errors.push(`missing prerequisite evidence: ${required}`);
  }
  return validated;
}

/**
 * Inspect a pre-release evidence document.  The checker binds every input to
 * one immutable release ref, commit and workflow run, and verifies local file
 * digests.  It never mutates the closeout manifest or calls external services.
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
  validateExpectedBinding(document, options, errors);
  if (workflow && releaseValues.ref && workflow.ref !== releaseValues.ref) {
    errors.push("manifest.workflowRun.ref does not match manifest.releaseRef");
  }
  if (workflow && commitSha && workflow.commitSha !== commitSha) {
    errors.push("manifest.workflowRun.commitSha does not match manifest.commitSha");
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
  const shaSums = validateChecksums(document.sha256sums, baseDirectory, declaredFiles, errors);
  const prerequisites = validatePrerequisites(
    document.prerequisites,
    baseDirectory,
    { ref: releaseValues.ref, commitSha },
    workflow,
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
    platforms: validatedPlatforms,
    sha256sums: shaSums,
    prerequisites,
    errors,
    limitations: [...RELEASE_CANDIDATE_LIMITATIONS],
    externalRequirements: [
      "Post-release smoke and native install/upgrade/uninstall/rollback observations are still required.",
      "Hard-cut readiness and owner-deletion closeout remain separate gates.",
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
    platforms: {},
    prerequisites: [],
    sha256sums: null,
    errors,
    limitations: [...RELEASE_CANDIDATE_LIMITATIONS],
    externalRequirements: [
      "Post-release smoke and native install/upgrade/uninstall/rollback observations are still required.",
      "Hard-cut readiness and owner-deletion closeout remain separate gates.",
      "An independent security review/sign-off remains required.",
    ],
    baseDirectory,
  };
}

export const validateReleaseCandidateEvidence = inspectReleaseCandidateEvidence;
export const checkReleaseCandidateEvidence = inspectReleaseCandidateEvidence;
export const evaluateReleaseCandidate = inspectReleaseCandidateEvidence;

function sourceReference(value, baseDirectory, label) {
  if (typeof value !== "string" && !isRecord(value)) throw new Error(`${label} must be a path or object`);
  const output = typeof value === "string"
    ? { path: portablePath(value, baseDirectory, label) }
    : { ...value, path: portablePath(value.path, baseDirectory, label) };
  const absolute = path.resolve(baseDirectory, output.path);
  if (!fs.existsSync(absolute) || !fs.statSync(absolute).isFile() || fs.statSync(absolute).size === 0) {
    throw new Error(`${label} file is missing or empty: ${output.path}`);
  }
  const actual = fileSha256(absolute);
  if (output.sha256 !== undefined
    && (typeof output.sha256 !== "string" || output.sha256.toLowerCase() !== actual)) {
    throw new Error(`${label}.sha256 does not match ${output.path}`);
  }
  output.sha256 = actual;
  output.size = fs.statSync(absolute).size;
  return output;
}

function normalizeBuildWorkflow(value) {
  if (!isRecord(value)) throw new Error("workflowRun must be an object");
  const id = value.id ?? value.runId;
  const attempt = value.attempt ?? value.runAttempt;
  const workflow = value.workflow ?? value.name;
  return {
    id,
    attempt,
    workflow,
    ref: value.ref,
    commitSha: value.commitSha,
    ...(value.url ? { url: value.url } : {}),
  };
}

/** Build canonical evidence from local files, calculating all SHA-256 values. */
export function buildReleaseCandidateEvidence(options = {}) {
  const baseDirectory = path.resolve(options.baseDirectory ?? repositoryRoot);
  const releaseRef = options.releaseRef;
  const releaseTag = options.releaseTag;
  const commitSha = options.commitSha;
  const workflowRun = normalizeBuildWorkflow(options.workflowRun);
  const platformInput = options.platforms;
  if (!isRecord(platformInput)) throw new Error("platforms must be an object");
  const platforms = {};
  for (const platform of REQUIRED_PLATFORMS) {
    if (!(platform in platformInput)) throw new Error(`missing release platform evidence: ${platform}`);
    const value = platformInput[platform];
    if (!isRecord(value)) throw new Error(`platforms.${platform} must be an object`);
    const manifest = sourceReference(value.manifest ?? value.packageManifest, baseDirectory, `platforms.${platform}.manifest`);
    const source = value.artifacts ?? value.artifact ?? value.packages;
    const entries = Array.isArray(source) ? source : source === undefined ? [] : [source];
    if (entries.length === 0) throw new Error(`platforms.${platform}.artifacts must contain at least one artifact`);
    platforms[platform] = {
      manifest,
      artifacts: entries.map((entry, index) => sourceReference(entry, baseDirectory, `platforms.${platform}.artifacts[${index}]`)),
    };
  }
  const sums = sourceReference(
    options.sha256sums ?? options.sha256Sums ?? options.checksums,
    baseDirectory,
    "sha256sums",
  );
  const prerequisites = options.prerequisites;
  if (!Array.isArray(prerequisites)) throw new Error("prerequisites must be an array");
  const builtPrerequisites = prerequisites.map((entry, index) => {
    if (!isRecord(entry)) throw new Error(`prerequisites[${index}] must be an object`);
    const evidence = Array.isArray(entry.evidence) ? entry.evidence : [];
    return {
      id: entry.id,
      status: entry.status ?? "passed",
      releaseRef: entry.releaseRef ?? releaseRef,
      commitSha: entry.commitSha ?? commitSha,
      workflowRun: entry.workflowRun ?? workflowRun,
      ...(entry.summary ? { summary: entry.summary } : {}),
      evidence: evidence.map((item, evidenceIndex) => sourceReference(item, baseDirectory, `prerequisites[${index}].evidence[${evidenceIndex}]`)),
    };
  });
  const evidence = {
    $schema: "./release-candidate-evidence.schema.json",
    schemaVersion: RELEASE_CANDIDATE_EVIDENCE_SCHEMA,
    phase: "pre-release",
    status: "candidate_ready",
    releaseRef,
    releaseTag,
    commitSha,
    workflowRun,
    platforms,
    sha256sums: sums,
    prerequisites: builtPrerequisites,
    limitations: [...RELEASE_CANDIDATE_LIMITATIONS],
  };
  const report = inspectReleaseCandidateEvidence(evidence, {
    baseDirectory,
    expected: options.expected,
  });
  if (!report.valid) {
    throw new Error(`built release-candidate evidence is invalid: ${report.errors.join("; ")}`);
  }
  return evidence;
}

export const createReleaseCandidateEvidence = buildReleaseCandidateEvidence;

export function writeReleaseCandidateEvidence(outputPath, options = {}) {
  if (!outputPath) throw new Error("outputPath is required");
  const evidence = buildReleaseCandidateEvidence(options);
  const absolute = path.resolve(outputPath);
  fs.mkdirSync(path.dirname(absolute), { recursive: true });
  fs.writeFileSync(absolute, `${JSON.stringify(evidence, null, 2)}\n`, "utf8");
  return evidence;
}

function argumentValue(args, names) {
  for (const name of names) {
    const index = args.indexOf(name);
    if (index !== -1) {
      const value = args[index + 1];
      if (!value || value.startsWith("--")) throw new Error(`${name} requires a value`);
      return value;
    }
    const inline = args.find((argument) => argument.startsWith(`${name}=`));
    if (inline) return inline.slice(name.length + 1);
  }
  return null;
}

function parseArgs(args) {
  const build = args.includes("--build");
  const configPath = argumentValue(args, ["--config"]);
  const inputPath = argumentValue(args, ["--input", "--manifest"]);
  const outputPath = argumentValue(args, ["--output"]);
  const baseDirectory = argumentValue(args, ["--base-dir"]);
  const expectedRef = argumentValue(args, ["--expected-ref"]);
  const expectedTag = argumentValue(args, ["--expected-tag"]);
  const expectedCommit = argumentValue(args, ["--expected-commit"]);
  const expectedRunId = argumentValue(args, ["--expected-run-id"]);
  const expectedAttempt = argumentValue(args, ["--expected-attempt"]);
  const expectedWorkflow = argumentValue(args, ["--expected-workflow"]);
  const expectedRunValues = [expectedRunId, expectedAttempt, expectedWorkflow, expectedRef, expectedCommit];
  const expectedRunCount = expectedRunValues.filter((value) => value !== null).length;
  if (expectedRunCount > 0 && expectedRunCount < expectedRunValues.length) {
    throw new Error(
      "expected workflow binding requires --expected-ref, --expected-commit, "
        + "--expected-run-id, --expected-attempt and --expected-workflow",
    );
  }
  const hasExpectedRun = expectedRunCount === expectedRunValues.length;
  const expected = expectedRef || expectedTag || expectedCommit || expectedRunId || expectedAttempt || expectedWorkflow
    ? {
      releaseRef: expectedRef ?? undefined,
      releaseTag: expectedTag ?? undefined,
      commitSha: expectedCommit ?? undefined,
      ...(hasExpectedRun ? {
        workflowRun: {
          id: expectedRunId,
          attempt: Number(expectedAttempt),
          workflow: expectedWorkflow,
          ref: expectedRef,
          commitSha: expectedCommit,
        },
      } : {}),
    }
    : undefined;
  const knownFlags = [
    "--build", "--config", "--input", "--manifest", "--output", "--base-dir", "--check",
    "--expected-ref", "--expected-tag", "--expected-commit", "--expected-run-id",
    "--expected-attempt", "--expected-workflow",
  ];
  const unknown = args.find((argument) => argument.startsWith("--")
    && !knownFlags.some((flag) => argument === flag || argument.startsWith(`${flag}=`)));
  if (unknown) {
    throw new Error(`unknown argument: ${unknown}`);
  }
  if (build && !configPath) throw new Error("--build requires --config");
  if (!build && !inputPath) throw new Error("--check requires --input or --manifest");
  return { build, configPath, inputPath, outputPath, baseDirectory, expected };
}

export function main(args = process.argv.slice(2)) {
  try {
    const parsed = parseArgs(args);
    if (parsed.build) {
      const config = readJson(path.resolve(parsed.configPath), "release-candidate build config");
      const evidence = writeReleaseCandidateEvidence(parsed.outputPath ?? "release-candidate-evidence.json", {
        ...config,
        baseDirectory: parsed.baseDirectory ?? config.baseDirectory ?? path.dirname(path.resolve(parsed.configPath)),
        expected: parsed.expected,
      });
      console.log(JSON.stringify(evidence, null, 2));
      return 0;
    }
    const inputAbsolute = path.resolve(parsed.inputPath);
    const document = readJson(inputAbsolute, "release-candidate evidence");
    const report = inspectReleaseCandidateEvidence(document, {
      baseDirectory: parsed.baseDirectory ?? path.dirname(inputAbsolute),
      expected: parsed.expected,
    });
    console.log(JSON.stringify(report, null, 2));
    return report.valid ? 0 : 1;
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    return 1;
  }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  process.exitCode = main();
}
