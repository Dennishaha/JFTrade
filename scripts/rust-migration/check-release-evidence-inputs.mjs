#!/usr/bin/env node

import { createHash } from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import process from "node:process";

export const RELEASE_EVIDENCE_INPUTS_SCHEMA = "jftrade.release-evidence-inputs.v2";
export const TRUSTED_EVIDENCE_WORKFLOWS = Object.freeze([
  "desktop-release-evidence.yml",
]);
export const TRUSTED_PAYLOAD_WORKFLOWS = Object.freeze([
  "desktop-release-evidence-payload.yml",
]);
// A source binding is rooted in the dedicated intake workflow.  The intake
// accepts one fixed external producer identity below and republishes the raw
// reports without rewriting them.  The old producer/payload workflows are
// deliberately absent: allowing either one here would create a provenance
// cycle in which a payload could be its own source.
export const TRUSTED_SOURCE_WORKFLOWS = Object.freeze([
  "desktop-release-evidence-intake.yml",
]);
// The source workflow is an immutable identity for the external runner that
// produced the raw evidence artifact.  It is not an input-controlled path.
export const TRUSTED_EXTERNAL_SOURCE_WORKFLOWS = Object.freeze([
  "desktop-release-evidence-source.yml",
]);
export const TRUSTED_BINDING_WORKFLOWS = Object.freeze([
  ...TRUSTED_SOURCE_WORKFLOWS,
  ...TRUSTED_EXTERNAL_SOURCE_WORKFLOWS,
]);
export const TRUSTED_PRODUCER_WORKFLOWS = TRUSTED_EVIDENCE_WORKFLOWS;
export const REQUIRED_EVIDENCE = Object.freeze({
  "signed-updater-inputs": "signed-updater",
  "sbom-provenance-inputs": "sbom-provenance",
  "rollback-artifact-pair": "rollback-artifact",
  "backup-restore-drill": "backup-restore",
  "security-review-inputs": "security-review",
});

const REPORT_CONTRACTS = Object.freeze({
  "signed-updater": {
    schemaVersions: ["jftrade.release.signed-updater.v2", "jftrade.tauri-signed-updater.v1"],
    statuses: ["verified"],
  },
  "sbom-provenance": {
    schemaVersions: ["jftrade.release.sbom-provenance.v2", "jftrade.sbom-provenance-check.v1"],
    statuses: ["verified"],
  },
  "rollback-artifact": {
    schemaVersions: ["jftrade.release.rollback-artifact.v2", "jftrade.rollback-artifact.v1"],
    statuses: ["verified"],
  },
  "backup-restore": {
    schemaVersions: ["jftrade.release.backup-restore.v2", "jftrade.backup-restore-drill.v1"],
    statuses: ["verified"],
  },
  "security-review": {
    schemaVersions: ["jftrade.release.security-review.v2", "jftrade.security-review-signoff.v1"],
    statuses: ["independent_review_signed_off"],
  },
});

const ROOT_KEYS = Object.freeze([
  "$schema", "schemaVersion", "repository", "releaseRef", "ref", "commitSha",
  "workflow", "runId", "attempt", "artifact", "sourceBinding", "evidence",
]);
const ARTIFACT_KEYS = Object.freeze(["name", "id", "digest"]);
const EVIDENCE_KEYS = Object.freeze(["kind", "status", "files"]);
const FILE_KEYS = Object.freeze(["path", "sha256", "size", "kind", "schemaVersion"]);
const BINDING_KEYS = Object.freeze([
  "repository", "releaseRef", "ref", "commitSha", "workflow", "runId", "attempt", "artifact",
]);

function isRecord(value) {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function addUnknownKeys(value, allowed, label, errors) {
  if (!isRecord(value)) return;
  for (const key of Object.keys(value)) {
    if (!allowed.includes(key)) errors.push(`${label}.${key} is not allowed`);
  }
}

function requiredString(value, label, errors, pattern) {
  if (typeof value !== "string" || value.trim() === "") {
    errors.push(`${label} must be a non-empty string`);
    return null;
  }
  const result = value.trim();
  if (pattern && !pattern.test(result)) errors.push(`${label} has an invalid format`);
  return result;
}

const MAX_SAFE_INTEGER_TEXT = String(Number.MAX_SAFE_INTEGER);

/**
 * Parse a decimal positive integer without allowing precision loss.  GitHub
 * workflow and artifact identifiers are identity fields, so accepting an
 * unsafe Number would allow distinct strings to compare equal after rounding.
 */
export function parseSafePositiveInteger(value) {
  if (typeof value === "number") {
    return Number.isFinite(value) && Number.isSafeInteger(value) && value > 0 ? value : null;
  }
  if (typeof value !== "string") return null;
  const text = value.trim();
  if (!/^[1-9][0-9]*$/.test(text)
    || text.length > MAX_SAFE_INTEGER_TEXT.length
    || (text.length === MAX_SAFE_INTEGER_TEXT.length && text > MAX_SAFE_INTEGER_TEXT)) {
    return null;
  }
  const parsed = Number(text);
  return Number.isFinite(parsed) && Number.isSafeInteger(parsed) && parsed > 0 ? parsed : null;
}

function positiveInteger(value, label, errors) {
  const parsed = parseSafePositiveInteger(value);
  if (parsed !== null) return parsed;
  errors.push(`${label} must be a positive integer`);
  return null;
}

function digest(value, label, errors, prefixed = false) {
  const pattern = prefixed ? /^sha256:[a-f0-9]{64}$/ : /^[a-f0-9]{64}$/;
  return requiredString(value, label, errors, pattern);
}

function normalizeWorkflow(value, label, errors, trustedWorkflows = TRUSTED_EVIDENCE_WORKFLOWS) {
  const workflow = requiredString(value, label, errors, /^[A-Za-z0-9._-]+\.yml$/);
  if (workflow && !trustedWorkflows.includes(workflow)) {
    errors.push(`${label} is not a trusted evidence producer workflow`);
  }
  return workflow;
}

function safeGitRef(value, label, errors) {
  const ref = requiredString(value, label, errors);
  const parts = ref?.split("/") ?? [];
  if (ref && (!/^[A-Za-z0-9._/-]+$/.test(ref) || ref.startsWith("/") || ref.endsWith("/")
    || ref.includes("..") || parts.some((part) => part === "." || part === ".."))) {
    errors.push(`${label} must be a safe Git ref`);
  }
  return ref;
}

function safeRelativePath(value, label, errors) {
  const reference = requiredString(value, label, errors);
  if (!reference) return null;
  if (path.isAbsolute(reference) || reference.includes("\\") || reference.includes("\0")) {
    errors.push(`${label} must be a relative POSIX path`);
    return null;
  }
  const parts = reference.split("/");
  if (parts.some((part) => part === "" || part === "." || part === "..")) {
    errors.push(`${label} must not contain empty, dot or parent path segments`);
    return null;
  }
  return reference;
}

function resolveEvidenceFile(reference, baseDirectory, label, errors) {
  const relative = safeRelativePath(reference, `${label}.path`, errors);
  if (!relative) return null;
  let root;
  try {
    root = fs.realpathSync(path.resolve(baseDirectory));
  } catch (error) {
    errors.push(`${label} evidence directory is missing: ${error.message}`);
    return null;
  }
  const resolved = path.resolve(root, relative);
  const outside = path.relative(root, resolved);
  if (outside === ".." || outside.startsWith(`..${path.sep}`)) {
    errors.push(`${label}.path escapes the evidence directory`);
    return null;
  }
  const segments = relative.split("/");
  let current = root;
  try {
    for (const segment of segments) {
      current = path.join(current, segment);
      const stat = fs.lstatSync(current);
      if (stat.isSymbolicLink()) {
        errors.push(`${label}.path must not traverse a symlink`);
        return null;
      }
    }
    const real = fs.realpathSync(resolved);
    const realOutside = path.relative(root, real);
    if (realOutside === ".." || realOutside.startsWith(`..${path.sep}`)) {
      errors.push(`${label}.path realpath escapes the evidence directory`);
      return null;
    }
  } catch (error) {
    errors.push(`${label} file is missing: ${relative} (${error.message})`);
    return null;
  }
  return resolved;
}

function bindingError(binding, expected, label, errors) {
  if (!isRecord(binding)) {
    errors.push(`${label}.binding must be an object`);
    return;
  }
  addUnknownKeys(binding, BINDING_KEYS, `${label}.binding`, errors);
  for (const key of ["repository", "releaseRef", "ref", "commitSha", "workflow"]) {
    if (binding[key] !== expected[key]) errors.push(`${label}.binding.${key} does not match manifest`);
  }
  const runId = positiveInteger(binding.runId, `${label}.binding.runId`, errors);
  const attempt = positiveInteger(binding.attempt, `${label}.binding.attempt`, errors);
  if (runId === null || runId !== expected.runId) {
    errors.push(`${label}.binding.runId does not match manifest`);
  }
  if (attempt === null || attempt !== expected.attempt) {
    errors.push(`${label}.binding.attempt does not match manifest`);
  }
  if (!isRecord(binding.artifact)) {
    errors.push(`${label}.binding.artifact must be an object`);
  } else {
    addUnknownKeys(binding.artifact, ARTIFACT_KEYS, `${label}.binding.artifact`, errors);
    const artifactId = positiveInteger(binding.artifact.id, `${label}.binding.artifact.id`, errors);
    const expectedArtifact = expected?.artifact;
    const expectedArtifactId = parseSafePositiveInteger(expectedArtifact?.id);
    if (!expectedArtifact
      || binding.artifact.name !== expectedArtifact.name
      || artifactId === null
      || expectedArtifactId === null
      || artifactId !== expectedArtifactId
      || binding.artifact.digest !== expectedArtifact.digest) {
      errors.push(`${label}.binding.artifact does not match manifest`);
    }
  }
}

function validateBindingObject(binding, label, errors, trustedWorkflows = TRUSTED_PAYLOAD_WORKFLOWS) {
  if (!isRecord(binding)) {
    errors.push(`${label} must be an object`);
    return null;
  }
  addUnknownKeys(binding, BINDING_KEYS, label, errors);
  const artifactId = isRecord(binding.artifact)
    ? positiveInteger(binding.artifact.id, `${label}.artifact.id`, errors)
    : null;
  const normalized = {
    repository: requiredString(binding.repository, `${label}.repository`, errors, /^[^/\s]+\/[^/\s]+$/),
    releaseRef: requiredString(binding.releaseRef, `${label}.releaseRef`, errors, /^refs\/tags\/v\d+\.\d+\.\d+$/),
    ref: safeGitRef(binding.ref, `${label}.ref`, errors),
    commitSha: requiredString(binding.commitSha, `${label}.commitSha`, errors, /^(?:[a-f0-9]{40}|[a-f0-9]{64})$/),
    workflow: normalizeWorkflow(binding.workflow, `${label}.workflow`, errors, trustedWorkflows),
    runId: positiveInteger(binding.runId, `${label}.runId`, errors),
    attempt: positiveInteger(binding.attempt, `${label}.attempt`, errors),
    artifact: isRecord(binding.artifact)
      ? { ...binding.artifact, id: artifactId }
      : binding.artifact,
  };
  if (!isRecord(binding.artifact)) errors.push(`${label}.artifact must be an object`);
  else {
    addUnknownKeys(binding.artifact, ARTIFACT_KEYS, `${label}.artifact`, errors);
    requiredString(binding.artifact.name, `${label}.artifact.name`, errors, /^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$/);
    digest(binding.artifact.digest, `${label}.artifact.digest`, errors, true);
  }
  return normalized;
}

function compareSemver(left, right) {
  const parse = (value) => String(value).split(".").map((part) => Number(part));
  const a = parse(left);
  const b = parse(right);
  for (let index = 0; index < 3; index += 1) {
    if (a[index] !== b[index]) return a[index] > b[index] ? 1 : -1;
  }
  return 0;
}

function normalizedDigest(value) {
  if (typeof value !== "string") return null;
  const normalized = value.trim().toLowerCase().replace(/^sha256:/, "");
  return /^[a-f0-9]{64}$/.test(normalized) ? normalized : null;
}

function nonEmptyArray(value) {
  return Array.isArray(value) && value.length > 0;
}

function rejectMarkers(text, label, errors) {
  if (/external_release_runner_evidence_required|pre_release_inputs_verified_only|repository_inputs_verified|check-signed-updater-lifecycle/i.test(text)) {
    errors.push(`${label} contains a local checker marker instead of external evidence`);
  }
}

function validateReport(document, kind, file, expected, options, label, errors, rawText) {
  if (!isRecord(document)) {
    errors.push(`${label} must contain a JSON report object`);
    return;
  }
  rejectMarkers(rawText, label, errors);
  const contract = REPORT_CONTRACTS[kind];
  if (!contract.schemaVersions.includes(file.schemaVersion)) errors.push(`${label}.schemaVersion is not a trusted ${kind} report schema`);
  if (document.schemaVersion !== file.schemaVersion) errors.push(`${label}.schemaVersion does not match file metadata`);
  if (!contract.statuses.includes(document.status)) errors.push(`${label}.status is not a verified ${kind} status`);
  bindingError(document.binding, expected, label, errors);
  if (kind === "signed-updater") {
    if (!isRecord(document.feed) || !Number.isInteger(document.feed.entryCount) || document.feed.entryCount < 1) {
      errors.push(`${label}.feed must contain a positive entryCount`);
    }
    if (!nonEmptyArray(document.artifacts)) {
      errors.push(`${label}.artifacts must contain signed updater archives`);
    } else {
      for (const [index, artifact] of document.artifacts.entries()) {
        if (!isRecord(artifact) || typeof artifact.archive !== "string"
          || !normalizedDigest(artifact.archiveSha256) || !normalizedDigest(artifact.signatureSha256)) {
          errors.push(`${label}.artifacts[${index}] must include archive and archive/signature digests`);
        }
      }
    }
    let endpointValid = false;
    if (typeof document.endpoint === "string") {
      try {
        const endpoint = new URL(document.endpoint);
        endpointValid = endpoint.protocol === "https:" && endpoint.hostname !== ""
          && endpoint.username === "" && endpoint.password === "";
      } catch {
        endpointValid = false;
      }
    }
    if (document.publicKeyConfigured !== true || !normalizedDigest(document.publicKeySha256) || !endpointValid) {
      errors.push(`${label} must include verified public-key and HTTPS endpoint fields`);
    }
  } else if (kind === "sbom-provenance") {
    const subjects = document.subjects;
    if (!nonEmptyArray(subjects)) {
      errors.push(`${label}.subjects must contain platform artifact digests`);
    } else if (subjects.some((subject) => typeof subject?.platform !== "string"
      || !normalizedDigest(subject?.sha256 ?? subject?.digest))) {
      errors.push(`${label}.subjects must include valid platform and SHA-256 fields`);
    } else if (options.releaseArtifactDigests) {
      const actual = new Set(subjects.map((subject) => `${subject?.platform}:${normalizedDigest(subject?.sha256 ?? subject?.digest)}`));
      const required = new Set(Object.entries(options.releaseArtifactDigests).flatMap(([platform, values]) =>
        (Array.isArray(values) ? values : [values]).map((value) => `${platform}:${normalizedDigest(value)}`)));
      if (actual.size !== required.size || [...required].some((value) => !actual.has(value))) {
        errors.push(`${label}.subjects do not exactly match platform artifact digests`);
      }
    }
  } else if (kind === "rollback-artifact") {
    if (!isRecord(document.current) || !isRecord(document.previous)) {
      errors.push(`${label} must contain current and previous artifact reports`);
    } else if (typeof document.current.version !== "string" || typeof document.previous.version !== "string"
      || compareSemver(document.current.version, document.previous.version) <= 0) {
      errors.push(`${label} current/previous must be a valid downgrade pair`);
    }
    if (isRecord(document.current) && (!isRecord(document.current.platforms)
      || Object.keys(document.current.platforms).length === 0 || !isRecord(document.current.updaterMetadata))) {
      errors.push(`${label}.current must include platform packages and updater metadata`);
    }
    if (isRecord(document.previous) && (!isRecord(document.previous.platforms)
      || Object.keys(document.previous.platforms).length === 0 || !isRecord(document.previous.updaterMetadata))) {
      errors.push(`${label}.previous must include platform packages and updater metadata`);
    }
    if (!document.rollbackInstructions) errors.push(`${label}.rollbackInstructions are required`);
  } else if (kind === "backup-restore") {
    if (typeof document.priorVersion !== "string" || !isRecord(document.nativeDrill)
      || !["passed", "verified"].includes(document.nativeDrill.status)) {
      errors.push(`${label} must include a prior-version native backup/restore drill`);
    }
  } else if (kind === "security-review") {
    const signoff = document.independentReview ?? document.signOff;
    if (!isRecord(signoff) || signoff.independent !== true
      || !["approved", "signed_off"].includes(signoff.status)
      || typeof signoff.reviewer !== "string" || signoff.reviewer.trim() === ""
      || typeof signoff.approvedAt !== "string" || signoff.approvedAt.trim() === "") {
      errors.push(`${label} must contain an independent security review sign-off`);
    }
  }
}

/**
 * Validate the five externally supplied payload reports without manufacturing
 * a release manifest or changing any report bytes.  The payload workflow uses
 * this helper as a semantic gate before publishing its pass-through artifact.
 */
export function validateReleaseEvidencePayload({ baseDirectory, expectedBinding, releaseArtifactDigests } = {}) {
  const errors = [];
  const root = path.resolve(baseDirectory ?? process.cwd());
  const binding = validateBindingObject(expectedBinding, "payload.binding", errors, TRUSTED_BINDING_WORKFLOWS);
  const reports = {};
  for (const [id, kind] of Object.entries(REQUIRED_EVIDENCE)) {
    const relative = `reports/${id}.json`;
    const label = `payload.${id}`;
    const resolved = resolveEvidenceFile(relative, root, label, errors);
    if (!resolved) continue;
    let text;
    let parsed;
    try {
      text = fs.readFileSync(resolved, "utf8");
      parsed = JSON.parse(text);
    } catch (error) {
      errors.push(`${label} must be a JSON report: ${error.message}`);
      continue;
    }
    const file = { schemaVersion: parsed?.schemaVersion };
    validateReport(parsed, kind, file, binding ?? expectedBinding, { releaseArtifactDigests }, label, errors, text);
    reports[id] = { path: relative, schemaVersion: file.schemaVersion };
  }
  return {
    valid: errors.length === 0,
    status: errors.length === 0 ? "payload_reports_validated" : "payload_reports_invalid",
    reports,
    errors,
  };
}

function validateExpected(document, options, expected, errors) {
  const configured = options.expected ?? options;
  for (const key of ["repository", "releaseRef", "ref", "commitSha", "workflow"]) {
    if (configured[key] !== undefined && document[key] !== configured[key]) errors.push(`manifest.${key} does not match expected value`);
  }
  for (const key of ["runId", "attempt"]) {
    if (configured[key] !== undefined) {
      const actual = parseSafePositiveInteger(document[key]);
      const expectedValue = parseSafePositiveInteger(configured[key]);
      if (actual === null || expectedValue === null || actual !== expectedValue) {
        errors.push(`manifest.${key} does not match expected value`);
      }
    }
  }
  if (configured.artifact) {
    for (const key of ARTIFACT_KEYS) {
      const actual = key === "id"
        ? parseSafePositiveInteger(document.artifact?.[key])
        : document.artifact?.[key];
      const expectedValue = key === "id"
        ? parseSafePositiveInteger(configured.artifact[key])
        : configured.artifact[key];
      if (actual === null || expectedValue === null || actual !== expectedValue) {
        errors.push(`manifest.artifact.${key} does not match expected value`);
      }
    }
  }
  const runId = parseSafePositiveInteger(document.runId);
  const attempt = parseSafePositiveInteger(document.attempt);
  Object.assign(expected, {
    repository: document.repository,
    releaseRef: document.releaseRef,
    ref: document.ref,
    commitSha: document.commitSha,
    workflow: document.workflow,
    runId,
    attempt,
    artifact: document.artifact,
  });
}

/** Strictly validate a v2 manifest and all referenced external report files. */
export function validateExternalEvidenceManifest(document, options = {}) {
  const errors = [];
  const baseDirectory = path.resolve(options.baseDirectory ?? process.cwd());
  if (!isRecord(document)) return { valid: false, schemaVersion: RELEASE_EVIDENCE_INPUTS_SCHEMA, errors: ["manifest must be a JSON object"] };
  addUnknownKeys(document, ROOT_KEYS, "manifest", errors);
  for (const key of ROOT_KEYS) if (!(key in document)) errors.push(`manifest.${key} is required`);
  if (document.$schema !== "./release-evidence-inputs.schema.json") errors.push("manifest.$schema must reference release-evidence-inputs.schema.json");
  if (document.schemaVersion !== RELEASE_EVIDENCE_INPUTS_SCHEMA) errors.push(`manifest.schemaVersion must be ${RELEASE_EVIDENCE_INPUTS_SCHEMA}`);
  const repository = requiredString(document.repository, "manifest.repository", errors, /^[^/\s]+\/[^/\s]+$/);
  const releaseRef = requiredString(document.releaseRef, "manifest.releaseRef", errors, /^refs\/tags\/v\d+\.\d+\.\d+$/);
  const ref = requiredString(document.ref, "manifest.ref", errors, /^refs\/tags\/v\d+\.\d+\.\d+$/);
  if (releaseRef && ref && releaseRef !== ref) errors.push("manifest.ref must equal manifest.releaseRef");
  const commitSha = requiredString(document.commitSha, "manifest.commitSha", errors, /^(?:[a-f0-9]{40}|[a-f0-9]{64})$/);
  const workflow = normalizeWorkflow(document.workflow, "manifest.workflow", errors);
  const runId = positiveInteger(document.runId, "manifest.runId", errors);
  const attempt = positiveInteger(document.attempt, "manifest.attempt", errors);
  const artifact = document.artifact;
  if (!isRecord(artifact)) errors.push("manifest.artifact must be an object");
  else {
    addUnknownKeys(artifact, ARTIFACT_KEYS, "manifest.artifact", errors);
    requiredString(artifact.name, "manifest.artifact.name", errors, /^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$/);
    positiveInteger(artifact.id, "manifest.artifact.id", errors);
    digest(artifact.digest, "manifest.artifact.digest", errors, true);
  }
  const expected = {};
  validateExpected(document, options, expected, errors);
  const sourceBinding = validateBindingObject(document.sourceBinding, "manifest.sourceBinding", errors, TRUSTED_BINDING_WORKFLOWS);
  if (sourceBinding && releaseRef && sourceBinding.releaseRef !== releaseRef) {
    errors.push("manifest.sourceBinding.releaseRef must match manifest.releaseRef");
  }
  // Callers pass all expected bindings through options.expected.  Keep the
  // legacy top-level alias only for compatibility, but always prefer the
  // canonical nested value so a qualification caller cannot silently skip
  // source-binding comparison.
  const expectedSourceBinding = options.expected?.sourceBinding ?? options.sourceBinding;
  if (expectedSourceBinding) {
    const normalizedExpected = validateBindingObject(expectedSourceBinding, "expected.sourceBinding", errors, TRUSTED_BINDING_WORKFLOWS);
    if (sourceBinding && normalizedExpected && JSON.stringify(sourceBinding) !== JSON.stringify(normalizedExpected)) {
      errors.push("manifest.sourceBinding does not match expected source binding");
    }
  }
  if (options.expectedArtifactMetadata) {
    for (const key of ARTIFACT_KEYS) {
      if (key === "id") {
        const actualId = parseSafePositiveInteger(artifact?.id);
        const expectedId = parseSafePositiveInteger(options.expectedArtifactMetadata.id);
        if (expectedId === null) {
          errors.push("manifest.artifact.id expected GitHub artifact metadata must be a positive integer");
        }
        if (actualId === null || expectedId === null || actualId !== expectedId) {
          errors.push(`manifest.artifact.${key} does not match GitHub artifact metadata`);
        }
        continue;
      }
      if (artifact?.[key] !== options.expectedArtifactMetadata[key]) {
        errors.push(`manifest.artifact.${key} does not match GitHub artifact metadata`);
      }
    }
  }
  const evidence = document.evidence;
  if (!isRecord(evidence)) errors.push("manifest.evidence must be an object");
  else {
    const seenPaths = new Set();
    const seenBasenames = new Set();
    for (const id of Object.keys(evidence)) if (!(id in REQUIRED_EVIDENCE)) errors.push(`manifest.evidence.${id} is not allowed`);
    for (const [id, kind] of Object.entries(REQUIRED_EVIDENCE)) {
      const entry = evidence[id];
      const label = `manifest.evidence.${id}`;
      if (!isRecord(entry)) {
        errors.push(`${label} is required and must be an object`);
        continue;
      }
      addUnknownKeys(entry, EVIDENCE_KEYS, label, errors);
      if (entry.kind !== kind) errors.push(`${label}.kind must be ${kind}`);
      if (entry.status !== "passed") errors.push(`${label}.status must be passed`);
      if (!Array.isArray(entry.files) || entry.files.length === 0) {
        errors.push(`${label}.files must be a non-empty array`);
        continue;
      }
      const contract = REPORT_CONTRACTS[kind];
      for (const [index, file] of entry.files.entries()) {
        const fileLabel = `${label}.files[${index}]`;
        if (!isRecord(file)) {
          errors.push(`${fileLabel} must be an object`);
          continue;
        }
        addUnknownKeys(file, FILE_KEYS, fileLabel, errors);
        const resolved = resolveEvidenceFile(file.path, baseDirectory, fileLabel, errors);
        if (typeof file.path === "string") {
          const normalizedPath = file.path.trim();
          const basename = path.posix.basename(normalizedPath);
          if (seenPaths.has(normalizedPath)) errors.push(`${fileLabel}.path collides with another evidence file`);
          if (seenBasenames.has(basename)) errors.push(`${fileLabel}.path basename collides with another evidence file`);
          seenPaths.add(normalizedPath);
          seenBasenames.add(basename);
        }
        const fileKind = requiredString(file.kind, `${fileLabel}.kind`, errors);
        if (fileKind !== kind) errors.push(`${fileLabel}.kind must match ${label}.kind`);
        const schemaVersion = requiredString(file.schemaVersion, `${fileLabel}.schemaVersion`, errors);
        if (!contract.schemaVersions.includes(schemaVersion)) errors.push(`${fileLabel}.schemaVersion is not a trusted ${kind} report schema`);
        const expectedDigest = digest(file.sha256, `${fileLabel}.sha256`, errors);
        if (!Number.isInteger(file.size) || file.size <= 0) errors.push(`${fileLabel}.size must be a positive integer`);
        if (!resolved) continue;
        const stat = fs.statSync(resolved);
        if (stat.size !== file.size) errors.push(`${fileLabel}.size does not match file`);
        const actualDigest = createHash("sha256").update(fs.readFileSync(resolved)).digest("hex");
        if (expectedDigest && actualDigest !== expectedDigest) errors.push(`${fileLabel}.sha256 does not match file`);
        let parsed;
        let text;
        try {
          text = fs.readFileSync(resolved, "utf8");
          parsed = JSON.parse(text);
        } catch (error) {
          errors.push(`${fileLabel} must be a JSON report, not arbitrary text (${error.message})`);
          continue;
        }
        validateReport(parsed, kind, file, sourceBinding ?? expected, options, fileLabel, errors, text);
      }
    }
  }
  return {
    schemaVersion: RELEASE_EVIDENCE_INPUTS_SCHEMA,
    status: errors.length === 0 ? "inputs_verified" : "invalid_inputs",
    valid: errors.length === 0,
    releaseRef,
    ref,
    commitSha,
    workflow,
    runId,
    attempt,
    artifact,
    errors,
  };
}

export const inspectExternalEvidenceManifest = validateExternalEvidenceManifest;
