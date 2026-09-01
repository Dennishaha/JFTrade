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

function digestSources(target, kind) {
  const names = kind === "artifact"
    ? ["artifactSha256", "artifactSHA256", "artifactDigest", "sha256", "digest"]
    : [`${kind}Sha256`, `${kind}SHA256`, `${kind}Digest`, "sha256", "digest"];
  const nested = isRecord(target?.digests) ? target.digests : {};
  const sources = [];
  for (const name of names) {
    if (Object.prototype.hasOwnProperty.call(target ?? {}, name)) {
      sources.push({ name, value: target[name] });
    } else if (Object.prototype.hasOwnProperty.call(nested, name)) {
      sources.push({ name: `digests.${name}`, value: nested[name] });
    }
  }
  if (Object.prototype.hasOwnProperty.call(nested, kind)) {
    sources.push({ name: `digests.${kind}`, value: nested[kind] });
  }
  return sources;
}

function expectedDigest(target, kind) {
  for (const source of digestSources(target, kind)) {
    const digest = asDigest(source.value);
    if (digest) return digest;
  }
  return null;
}

function validateDigestSources(target, kind, errors) {
  const sources = digestSources(target, kind);
  const valid = [];
  for (const source of sources) {
    const digest = asDigest(source.value);
    if (!digest) {
      errors.push(`${kind} digest ${source.name} must be a valid SHA-256 digest`);
    } else {
      valid.push(digest);
    }
  }
  if (new Set(valid).size > 1) {
    errors.push(`${kind} digest fields disagree`);
  }
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

function resolveReference(reference, baseDirectory, kind, errors) {
  if (typeof reference !== "string" || reference.trim() === "") {
    errors.push(`${kind} file reference is required`);
    return null;
  }
  const value = reference.trim();
  if (path.isAbsolute(value) || /^[A-Za-z]:/.test(value) || value.startsWith("\\\\")) {
    errors.push(`${kind} file reference must be a relative POSIX path: ${reference}`);
    return null;
  }
  if (value.includes("\\") || value.includes("\0")) {
    errors.push(`${kind} file reference must be a relative POSIX path: ${reference}`);
    return null;
  }
  const segments = value.split("/");
  if (segments.some((segment) => segment === "" || segment === "." || segment === "..")) {
    errors.push(`${kind} file reference must not contain empty, dot or parent path segments: ${reference}`);
    return null;
  }

  let root;
  try {
    root = fs.realpathSync(path.resolve(baseDirectory));
  } catch (error) {
    errors.push(`${kind} evidence directory is missing: ${error.message}`);
    return null;
  }
  const resolved = path.resolve(root, ...segments);
  const outside = path.relative(root, resolved);
  if (outside === ".." || outside.startsWith(`..${path.sep}`)) {
    errors.push(`${kind} file reference escapes the evidence directory: ${reference}`);
    return null;
  }

  let current = root;
  try {
    for (const segment of segments) {
      current = path.join(current, segment);
      if (fs.lstatSync(current).isSymbolicLink()) {
        errors.push(`${kind} file reference must not traverse a symlink: ${reference}`);
        return null;
      }
    }
    const real = fs.realpathSync(resolved);
    const realOutside = path.relative(root, real);
    if (realOutside === ".." || realOutside.startsWith(`..${path.sep}`)) {
      errors.push(`${kind} file reference realpath escapes the evidence directory: ${reference}`);
      return null;
    }
    return { path: resolved, realpath: real };
  } catch (error) {
    errors.push(`${kind} file is missing: ${reference} (${error.message})`);
    return null;
  }
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
  const resolved = resolveReference(reference, baseDirectory, kind, errors);
  if (!resolved) return result;
  result.path = resolved.path;
  result.realpath = resolved.realpath;
  let stat;
  try {
    stat = fs.lstatSync(resolved.realpath);
  } catch (error) {
    errors.push(`${kind} file is missing: ${reference} (${error.message})`);
    return result;
  }
  if (stat.isSymbolicLink()) {
    errors.push(`${kind} file reference must not traverse a symlink: ${reference}`);
    return result;
  }
  if (!stat.isFile()) {
    errors.push(`${kind} reference is not a file: ${reference}`);
    return result;
  }
  result.exists = true;
  const nonEmpty = stat.size > 0;
  if (!nonEmpty) errors.push(`${kind} file is empty: ${reference}`);
  if (expected === null) {
    errors.push(`${kind} file is missing a valid SHA-256 digest: ${reference}`);
  }
  try {
    result.sha256 = hashFile(resolved.realpath);
  } catch (error) {
    errors.push(`${kind} file could not be hashed: ${reference} (${error.message})`);
    return result;
  }
  if (expected !== null && result.sha256 !== expected) {
    errors.push(`${kind} SHA-256 mismatch for ${reference}`);
  } else if (expected !== null && nonEmpty) {
    result.valid = true;
  }
  return { ...result, contentPath: resolved.realpath };
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

function validateSpdxPackages(packages, reference, errors) {
  if (!Array.isArray(packages) || packages.length === 0) {
    errors.push(`SPDX SBOM has no packages/components: ${reference}`);
    return false;
  }
  let valid = true;
  for (const [index, packageEntry] of packages.entries()) {
    if (!isRecord(packageEntry) || typeof packageEntry.name !== "string"
      || packageEntry.name.trim() === "") {
      errors.push(`SPDX package ${index + 1} must include a name: ${reference}`);
      valid = false;
      continue;
    }
    const checksums = packageEntry.checksums;
    if (checksums === undefined) continue;
    if (!Array.isArray(checksums)) {
      errors.push(`SPDX package ${index + 1} checksums must be an array: ${reference}`);
      valid = false;
      continue;
    }
    for (const [checksumIndex, checksum] of checksums.entries()) {
      const algorithm = typeof checksum?.algorithm === "string"
        ? checksum.algorithm.trim()
        : "";
      const value = checksum?.checksumValue;
      if (!isRecord(checksum) || algorithm === "" || typeof value !== "string"
        || value.trim() === "") {
        errors.push(`SPDX package ${index + 1} checksum ${checksumIndex + 1} is invalid: ${reference}`);
        valid = false;
      } else if (/^sha-?256$/i.test(algorithm) && !asDigest(value)) {
        errors.push(`SPDX package ${index + 1} checksum ${checksumIndex + 1} has an invalid SHA-256 digest: ${reference}`);
        valid = false;
      }
    }
  }
  return valid;
}

function validateCycloneDxComponents(components, reference, errors) {
  if (!Array.isArray(components) || components.length === 0) {
    errors.push(`CycloneDX SBOM has no components: ${reference}`);
    return false;
  }
  let valid = true;
  for (const [index, component] of components.entries()) {
    if (!isRecord(component) || typeof component.name !== "string" || component.name.trim() === "") {
      errors.push(`CycloneDX component ${index + 1} must include a name: ${reference}`);
      valid = false;
      continue;
    }
    const hashes = component.hashes;
    if (hashes === undefined) continue;
    if (!Array.isArray(hashes)) {
      errors.push(`CycloneDX component ${index + 1} hashes must be an array: ${reference}`);
      valid = false;
      continue;
    }
    for (const [hashIndex, hash] of hashes.entries()) {
      const algorithm = typeof hash?.alg === "string" ? hash.alg.trim() : "";
      const value = hash?.content;
      if (!isRecord(hash) || algorithm === "" || typeof value !== "string" || value.trim() === "") {
        errors.push(`CycloneDX component ${index + 1} hash ${hashIndex + 1} is invalid: ${reference}`);
        valid = false;
      } else if (/^sha-?256$/i.test(algorithm) && !asDigest(value)) {
        errors.push(`CycloneDX component ${index + 1} hash ${hashIndex + 1} has an invalid SHA-256 digest: ${reference}`);
        valid = false;
      }
    }
  }
  return valid;
}

function validateSbomDocument(document, reference, errors) {
  if (!isRecord(document)) {
    errors.push(`SBOM must be an SPDX or CycloneDX JSON object: ${reference}`);
    return { format: null, componentCount: 0, valid: false };
  }
  if (typeof document.spdxVersion === "string" && /^SPDX-\d+\.\d+$/i.test(document.spdxVersion.trim())) {
    const packages = Array.isArray(document.packages) ? document.packages : [];
    return {
      format: "spdx",
      componentCount: packages.length,
      valid: validateSpdxPackages(packages, reference, errors),
    };
  }
  if (typeof document.bomFormat === "string" && document.bomFormat.trim().toLowerCase() === "cyclonedx") {
    const components = Array.isArray(document.components) ? document.components : [];
    return {
      format: "cyclonedx",
      componentCount: components.length,
      valid: validateCycloneDxComponents(components, reference, errors),
    };
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

function validateProvenanceDocument(document, reference, expectedArtifactDigest, errors) {
  const subjects = provenanceSubjects(document);
  if (subjects.length === 0) {
    errors.push(`provenance JSON has no subject: ${reference}`);
    return { subjectCount: 0, digestCount: 0, valid: false };
  }
  let digestCount = 0;
  const subjectDigests = [];
  for (const [index, subject] of subjects.entries()) {
    if (!isRecord(subject) || typeof subject.name !== "string" || subject.name.trim() === "") {
      errors.push(`provenance subject ${index + 1} must include a name: ${reference}`);
      continue;
    }
    const digest = isRecord(subject.digest) ? subject.digest : null;
    const sha256 = [];
    for (const [name, value] of Object.entries(digest ?? {})) {
      if (name.toLowerCase() !== "sha256") continue;
      const normalized = asDigest(value);
      if (!normalized) {
        errors.push(`provenance subject ${index + 1} has an invalid SHA-256 digest: ${reference}`);
      } else {
        sha256.push(normalized);
      }
    }
    if (sha256.length === 0) {
      errors.push(`provenance subject ${index + 1} has no SHA-256 digest: ${reference}`);
    } else {
      digestCount += 1;
      subjectDigests.push(...sha256);
    }
  }
  const artifactDigestMatched = expectedArtifactDigest !== null
    && subjectDigests.includes(expectedArtifactDigest);
  if (expectedArtifactDigest !== null && !artifactDigestMatched) {
    errors.push(`provenance subjects do not include the artifact SHA-256 digest: ${reference}`);
  }
  return {
    subjectCount: subjects.length,
    digestCount,
    artifactDigestMatched,
    valid: digestCount === subjects.length && artifactDigestMatched,
  };
}

function inspectTarget(target, baseDirectory, errors) {
  const platform = typeof target?.platform === "string"
    ? target.platform.trim()
    : typeof target?.target === "string" ? target.target.trim() : "";
  const targetErrors = [];
  if (!platform) targetErrors.push("target is missing platform");
  validateDigestSources(target, "artifact", targetErrors);
  validateDigestSources(target, "sbom", targetErrors);
  validateDigestSources(target, "provenance", targetErrors);
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
        const validation = validateSbomDocument(sbomDocument, sbomReference, targetErrors);
        sbom.format = validation.format;
        sbom.componentCount = validation.componentCount;
        sbom.documentValid = validation.valid;
      }
    }
  }
  if (provenance.valid && provenance.contentPath) {
    const provenanceDocument = readJsonDocument(provenance.contentPath, "provenance", targetErrors);
    if (provenanceDocument !== null) {
      Object.assign(
        provenance,
        validateProvenanceDocument(
          provenanceDocument,
          provenanceReference,
          artifact.expectedSha256,
          targetErrors,
        ),
      );
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
  const settings = isRecord(options) ? options : {};
  const errors = [];
  const warnings = [];
  if (!isRecord(manifest)) {
    errors.push("SBOM/provenance manifest must be a JSON object");
  }
  const entries = isRecord(manifest) ? targetEntries(manifest) : [];
  if (entries.length === 0) errors.push("SBOM/provenance manifest has no target entries");
  if (isRecord(manifest)) {
    const collections = ["targets", "platforms", "artifacts"]
      .filter((field) => Object.prototype.hasOwnProperty.call(manifest, field));
    const directTargets = Object.keys(manifest)
      .filter((key) => REQUIRED_TARGETS.includes(key));
    if (collections.length > 1) {
      errors.push("SBOM/provenance manifest must contain only one target collection");
    }
    if (collections.length > 0 && directTargets.length > 0) {
      errors.push("SBOM/provenance manifest must not mix target collections and direct target entries");
    }
  }
  let baseDirectory;
  try {
    baseDirectory = path.resolve(settings.baseDirectory ?? repositoryRoot);
  } catch (error) {
    baseDirectory = null;
    errors.push(`SBOM/provenance evidence directory is invalid: ${error.message}`);
  }
  const seen = new Set();
  const seenFiles = new Map();
  const targets = [];
  for (const [index, entry] of entries.entries()) {
    const platform = typeof entry?.platform === "string" ? entry.platform.trim() : "";
    if (platform && seen.has(platform)) errors.push(`duplicate SBOM/provenance target: ${platform}`);
    if (platform) seen.add(platform);
    if (platform && !REQUIRED_TARGETS.includes(platform)) errors.push(`unknown SBOM/provenance target: ${platform}`);
    const target = inspectTarget(entry, baseDirectory, errors);
    const references = [
      ["artifact", target.artifact],
      ["SBOM", target.sbom],
      ["provenance", target.provenance],
    ];
    const localFiles = new Set();
    for (const [kind, file] of references) {
      const filePath = file.realpath;
      if (!filePath) continue;
      if (localFiles.has(filePath)) {
        errors.push(`${platform || `target ${index + 1}`}: duplicate evidence file reference: ${file.reference}`);
      }
      localFiles.add(filePath);
      const previous = seenFiles.get(filePath);
      if (previous) {
        errors.push(`duplicate SBOM/provenance target file: ${file.reference} reused by ${previous}`);
      } else {
        seenFiles.set(filePath, `${platform || `target ${index + 1}`} ${kind}`);
      }
    }
    targets.push(target);
  }
  const missingTargets = REQUIRED_TARGETS.filter((target) => !seen.has(target));
  const requireTargets = settings.requireTargets === true;
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
  const settings = isRecord(options) ? options : {};
  if (isRecord(manifestOrPath)) return inspectSbomProvenance(manifestOrPath, settings);
  const manifestPath = manifestOrPath || settings.manifestPath;
  if (!manifestPath) {
    return inspectSbomProvenance(null, settings);
  }
  let absolutePath;
  let manifest;
  try {
    absolutePath = path.resolve(manifestPath);
    manifest = JSON.parse(fs.readFileSync(absolutePath, "utf8"));
  } catch (error) {
    return {
      schemaVersion: SBOM_PROVENANCE_SCHEMA,
      status: "invalid_inputs",
      valid: false,
      requireTargets: settings.requireTargets === true,
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
    ...settings,
    baseDirectory: settings.baseDirectory ?? path.dirname(absolutePath),
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
