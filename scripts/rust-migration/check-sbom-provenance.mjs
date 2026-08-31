#!/usr/bin/env node

import { createHash } from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));

export const SBOM_PROVENANCE_SCHEMA = "jftrade.sbom-provenance-check.v1";
export const REQUIRED_TARGETS = Object.freeze([
  "macos-arm64",
  "linux-x64",
  "windows-x64",
  "windows-arm64",
]);
export const REQUIRED_PLATFORMS = REQUIRED_TARGETS;

const FIELD_ALIASES = Object.freeze({
  artifact: ["artifact", "artifactPath", "artifactFile", "file", "path"],
  sbom: ["sbom", "sbomPath", "sbomFile"],
  provenance: ["provenance", "provenancePath", "provenanceFile", "attestation"],
});

function isRecord(value) {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function asDigest(value) {
  if (typeof value !== "string") return null;
  const digest = value.trim().replace(/^sha256:/i, "").toLowerCase();
  return /^[0-9a-f]{64}$/.test(digest) ? digest : null;
}

function expectedDigest(target, kind) {
  const names = kind === "artifact"
    ? ["artifactSha256", "artifactSHA256", "artifactDigest", "sha256", "digest"]
    : [`${kind}Sha256`, `${kind}SHA256`, `${kind}Digest`, "sha256", "digest"];
  const nested = isRecord(target.digests) ? target.digests : {};
  for (const name of [...names, kind]) {
    const digest = asDigest(target[name] ?? nested[name]);
    if (digest) return digest;
  }
  return null;
}

function pickField(target, kind) {
  for (const name of FIELD_ALIASES[kind]) {
    if (typeof target[name] === "string" && target[name].trim() !== "") return target[name].trim();
  }
  return null;
}

function targetEntries(manifest) {
  for (const field of ["targets", "platforms", "artifacts"]) {
    const value = manifest?.[field];
    if (Array.isArray(value)) return value.map((entry) => ({ ...entry }));
    if (isRecord(value)) {
      return Object.entries(value).map(([platform, entry]) => ({
        ...(isRecord(entry) ? entry : {}),
        platform: isRecord(entry) && entry.platform ? entry.platform : platform,
      }));
    }
  }
  if (isRecord(manifest)) {
    const entries = Object.entries(manifest)
      .filter(([key, value]) => REQUIRED_TARGETS.includes(key) && isRecord(value))
      .map(([platform, entry]) => ({ ...entry, platform: entry.platform ?? platform }));
    if (entries.length > 0) return entries;
  }
  return [];
}

function resolveReference(reference, baseDirectory) {
  return path.isAbsolute(reference) ? reference : path.resolve(baseDirectory, reference);
}

function hashFile(filePath) {
  return createHash("sha256").update(fs.readFileSync(filePath)).digest("hex");
}

function inspectFile(reference, expected, kind, baseDirectory, errors) {
  const result = {
    reference,
    expectedSha256: expected,
    exists: false,
    sha256: null,
    valid: false,
  };
  if (!reference) {
    errors.push(`${kind} file reference is required`);
    return result;
  }
  const filePath = resolveReference(reference, baseDirectory);
  result.path = filePath;
  let stat;
  try {
    stat = fs.statSync(filePath);
  } catch (error) {
    errors.push(`${kind} file is missing: ${reference} (${error.message})`);
    return result;
  }
  if (!stat.isFile()) {
    errors.push(`${kind} reference is not a file: ${reference}`);
    return result;
  }
  result.exists = true;
  if (expected === null) {
    errors.push(`${kind} file is missing a valid SHA-256 digest: ${reference}`);
  }
  try {
    result.sha256 = hashFile(filePath);
  } catch (error) {
    errors.push(`${kind} file could not be hashed: ${reference} (${error.message})`);
    return result;
  }
  if (expected !== null && result.sha256 !== expected) {
    errors.push(`${kind} SHA-256 mismatch for ${reference}`);
  } else if (expected !== null) {
    result.valid = true;
  }
  return { ...result, contentPath: filePath };
}

function readJsonDocument(filePath, kind, errors) {
  let text;
  try {
    text = fs.readFileSync(filePath, "utf8");
  } catch (error) {
    errors.push(`${kind} file could not be read: ${error.message}`);
    return null;
  }
  if (text.trim() === "") {
    errors.push(`${kind} file is empty`);
    return null;
  }
  try {
    return JSON.parse(text);
  } catch (jsonError) {
    // GitHub attestation downloads may be JSON Lines.  Accept that encoding
    // while still parsing every line and rejecting an incomplete document.
    const values = [];
    for (const [index, line] of text.split(/\r?\n/).entries()) {
      if (line.trim() === "") continue;
      try {
        values.push(JSON.parse(line));
      } catch (lineError) {
        errors.push(`${kind} is not valid JSON (line ${index + 1}): ${lineError.message}`);
        return null;
      }
    }
    if (values.length > 0) return values;
    errors.push(`${kind} is not valid JSON: ${jsonError.message}`);
    return null;
  }
}

function validateSbomDocument(document, reference, errors) {
  if (!isRecord(document)) {
    errors.push(`SBOM must be an SPDX or CycloneDX JSON object: ${reference}`);
    return { format: null, componentCount: 0, valid: false };
  }
  if (typeof document.spdxVersion === "string" && /^SPDX-/i.test(document.spdxVersion)) {
    const packages = Array.isArray(document.packages) ? document.packages : [];
    if (packages.length === 0) errors.push(`SPDX SBOM has no packages/components: ${reference}`);
    return { format: "spdx", componentCount: packages.length, valid: packages.length > 0 };
  }
  if (typeof document.bomFormat === "string" && document.bomFormat.toLowerCase() === "cyclonedx") {
    const components = Array.isArray(document.components) ? document.components : [];
    if (components.length === 0) errors.push(`CycloneDX SBOM has no components: ${reference}`);
    return { format: "cyclonedx", componentCount: components.length, valid: components.length > 0 };
  }
  errors.push(`SBOM is neither SPDX nor CycloneDX JSON: ${reference}`);
  return { format: null, componentCount: 0, valid: false };
}

function provenanceSubjects(document) {
  if (Array.isArray(document)) return document.flatMap(provenanceSubjects);
  if (!isRecord(document)) return [];
  const subject = document.subject;
  if (Array.isArray(subject)) return subject;
  if (isRecord(subject)) return [subject];
  return [];
}

function validateProvenanceDocument(document, reference, errors) {
  const subjects = provenanceSubjects(document);
  if (subjects.length === 0) {
    errors.push(`provenance JSON has no subject: ${reference}`);
    return { subjectCount: 0, digestCount: 0, valid: false };
  }
  let digestCount = 0;
  for (const [index, subject] of subjects.entries()) {
    const digest = isRecord(subject) && isRecord(subject.digest) ? subject.digest : null;
    const sha256 = digest && Object.entries(digest)
      .find(([name, value]) => name.toLowerCase() === "sha256" && asDigest(String(value)));
    if (!sha256) {
      errors.push(`provenance subject ${index + 1} has no SHA-256 digest: ${reference}`);
    } else {
      digestCount += 1;
    }
  }
  return { subjectCount: subjects.length, digestCount, valid: digestCount > 0 };
}

function inspectTarget(target, baseDirectory, errors) {
  const platform = typeof target?.platform === "string"
    ? target.platform.trim()
    : typeof target?.target === "string" ? target.target.trim() : "";
  const targetErrors = [];
  if (!platform) targetErrors.push("target is missing platform");
  const artifactReference = pickField(target, "artifact");
  const sbomReference = pickField(target, "sbom");
  const provenanceReference = pickField(target, "provenance");
  const artifact = inspectFile(
    artifactReference,
    expectedDigest(target, "artifact"),
    "artifact",
    baseDirectory,
    targetErrors,
  );
  const sbom = inspectFile(
    sbomReference,
    expectedDigest(target, "sbom"),
    "SBOM",
    baseDirectory,
    targetErrors,
  );
  const provenance = inspectFile(
    provenanceReference,
    expectedDigest(target, "provenance"),
    "provenance",
    baseDirectory,
    targetErrors,
  );
  let sbomDocument;
  if (sbom.valid && sbom.contentPath) {
    sbomDocument = readJsonDocument(sbom.contentPath, "SBOM", targetErrors);
    if (sbomDocument !== null) {
      // SPDX and CycloneDX are single JSON documents.  JSON Lines are
      // intentionally rejected for SBOMs because that would hide a partial
      // package inventory.
      if (Array.isArray(sbomDocument)) {
        targetErrors.push(`SBOM must be one SPDX or CycloneDX document: ${sbomReference}`);
      } else {
        sbom.format = validateSbomDocument(sbomDocument, sbomReference, targetErrors).format;
        sbom.componentCount = validateSbomDocument(sbomDocument, sbomReference, []).componentCount;
      }
    }
  }
  if (provenance.valid && provenance.contentPath) {
    const provenanceDocument = readJsonDocument(provenance.contentPath, "provenance", targetErrors);
    if (provenanceDocument !== null) {
      Object.assign(provenance, validateProvenanceDocument(provenanceDocument, provenanceReference, targetErrors));
    }
  }
  errors.push(...targetErrors.map((error) => `${platform || "unknown target"}: ${error}`));
  return {
    platform,
    valid: targetErrors.length === 0,
    errors: targetErrors,
    artifact,
    sbom,
    provenance,
  };
}

/**
 * Validate externally generated SBOM/provenance inputs.  This checker does
 * not generate an SBOM, contact Anchore or GitHub, verify an attestation
 * online, or qualify a release runner.  Those remain external release gates.
 */
export function inspectSbomProvenance(manifest, options = {}) {
  const errors = [];
  const warnings = [];
  if (!isRecord(manifest)) {
    errors.push("SBOM/provenance manifest must be a JSON object");
  }
  const entries = isRecord(manifest) ? targetEntries(manifest) : [];
  if (entries.length === 0) errors.push("SBOM/provenance manifest has no target entries");
  const baseDirectory = path.resolve(options.baseDirectory ?? repositoryRoot);
  const seen = new Set();
  const targets = [];
  for (const entry of entries) {
    const platform = typeof entry?.platform === "string" ? entry.platform.trim() : "";
    if (platform && seen.has(platform)) errors.push(`duplicate SBOM/provenance target: ${platform}`);
    if (platform) seen.add(platform);
    if (platform && !REQUIRED_TARGETS.includes(platform)) errors.push(`unknown SBOM/provenance target: ${platform}`);
    targets.push(inspectTarget(entry, baseDirectory, errors));
  }
  const missingTargets = REQUIRED_TARGETS.filter((target) => !seen.has(target));
  const requireTargets = options.requireTargets === true;
  if (missingTargets.length > 0) {
    const message = `missing SBOM/provenance targets: ${missingTargets.join(", ")}`;
    if (requireTargets) errors.push(message);
    else warnings.push(message);
  }
  const valid = errors.length === 0;
  return {
    schemaVersion: SBOM_PROVENANCE_SCHEMA,
    status: !valid ? "invalid_inputs" : missingTargets.length > 0 ? "partial_inputs_verified" : "inputs_verified",
    valid,
    requireTargets,
    targets,
    missingTargets,
    errors,
    warnings,
    releaseQualified: false,
    releaseQualification: "external_release_runner_evidence_required",
    externalRequirements: [
      "Anchore SBOM action output must be produced by the release runner.",
      "GitHub artifact attestation must be issued and independently verified.",
      "Real four-platform release-runner evidence remains required; this local check is not release success.",
    ],
  };
}

export const validateSbomProvenance = inspectSbomProvenance;
export const validateSbomProvenanceManifest = inspectSbomProvenance;

export function checkSbomProvenance(manifestOrPath, options = {}) {
  if (isRecord(manifestOrPath)) return inspectSbomProvenance(manifestOrPath, options);
  const manifestPath = manifestOrPath || options.manifestPath;
  if (!manifestPath) {
    return inspectSbomProvenance(null, options);
  }
  const absolutePath = path.resolve(manifestPath);
  let manifest;
  try {
    manifest = JSON.parse(fs.readFileSync(absolutePath, "utf8"));
  } catch (error) {
    return {
      schemaVersion: SBOM_PROVENANCE_SCHEMA,
      status: "invalid_inputs",
      valid: false,
      requireTargets: options.requireTargets === true,
      targets: [],
      missingTargets: [...REQUIRED_TARGETS],
      errors: [`cannot read SBOM/provenance manifest ${manifestPath}: ${error.message}`],
      warnings: [],
      releaseQualified: false,
      releaseQualification: "external_release_runner_evidence_required",
      externalRequirements: [
        "Anchore SBOM action output must be produced by the release runner.",
        "GitHub artifact attestation must be issued and independently verified.",
        "Real four-platform release-runner evidence remains required; this local check is not release success.",
      ],
    };
  }
  return inspectSbomProvenance(manifest, {
    ...options,
    baseDirectory: options.baseDirectory ?? path.dirname(absolutePath),
  });
}

function argumentValue(args, flag) {
  const index = args.indexOf(flag);
  if (index === -1) return null;
  const value = args[index + 1];
  if (!value || value.startsWith("--")) throw new Error(`${flag} requires a value`);
  return value;
}

export function main(args = process.argv.slice(2)) {
  let result;
  try {
    const positional = args.filter((argument) => !argument.startsWith("--"));
    const manifestPath = argumentValue(args, "--manifest") ?? positional[0];
    result = checkSbomProvenance(manifestPath, {
      baseDirectory: argumentValue(args, "--base-dir") ?? undefined,
      requireTargets: args.includes("--require-targets"),
    });
  } catch (error) {
    result = inspectSbomProvenance(null, { requireTargets: args.includes("--require-targets") });
    result.errors.push(error.message);
    result.status = "invalid_inputs";
    result.valid = false;
  }
  console.log(JSON.stringify(result, null, 2));
  return result.valid ? 0 : 1;
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  process.exitCode = main();
}
