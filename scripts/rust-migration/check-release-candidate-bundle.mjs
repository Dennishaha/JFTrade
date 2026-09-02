#!/usr/bin/env node

import { createHash } from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";
import { REQUIRED_PLATFORMS } from "./check-release-candidate.mjs";
import { parseSafePositiveInteger } from "./check-release-evidence-inputs.mjs";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));
const SEALED_BUNDLE_SCHEMA = "jftrade.sealed-release-bundle.v1";
const REQUIRED_SOURCE_ARTIFACTS = Object.freeze([
  "desktop-release-linux",
  "desktop-release-macos",
  "desktop-release-windows",
  "desktop-release-windows-arm64",
]);
const REQUIRED_UPDATER_ARTIFACTS = Object.freeze([
  "desktop-release-updater-linux",
  "desktop-release-updater-macos",
  "desktop-release-updater-windows",
  "desktop-release-updater-windows-arm64",
]);

function isRecord(value) {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function sha256(filePath) {
  return createHash("sha256").update(fs.readFileSync(filePath)).digest("hex");
}

function requiredFile(root, relative, label) {
  if (typeof relative !== "string" || relative.trim() === "" || path.isAbsolute(relative)
    || relative.includes("\\") || relative.split("/").some((part) => !part || part === "." || part === "..")) {
    throw new Error(`${label} must be a safe relative POSIX path`);
  }
  const resolved = path.resolve(root, relative);
  const base = path.resolve(root);
  const outside = path.relative(base, resolved);
  if (outside === ".." || outside.startsWith(`..${path.sep}`)) throw new Error(`${label} escapes its bundle root`);
  let current = base;
  for (const part of path.relative(base, resolved).split(path.sep).filter(Boolean)) {
    current = path.join(current, part);
    let entry;
    try {
      entry = fs.lstatSync(current);
    } catch (error) {
      throw new Error(`${label} is missing: ${relative} (${error.message})`);
    }
    if (entry.isSymbolicLink()) throw new Error(`${label} must not traverse a symbolic link: ${relative}`);
  }
  const stat = fs.lstatSync(resolved);
  if (!stat.isFile() || stat.size === 0) throw new Error(`${label} is missing or empty: ${relative}`);
  return { path: resolved, relative, size: stat.size, sha256: sha256(resolved) };
}

function compareFile(candidateRoot, releaseRoot, reference, label, seenReleaseNames) {
  const candidate = requiredFile(candidateRoot, reference.path, `${label} candidate file`);
  const releaseName = path.posix.basename(reference.path);
  if (releaseName !== reference.path) {
    throw new Error(`${label} must be a top-level release file: ${reference.path}`);
  }
  if (seenReleaseNames.has(releaseName)) {
    throw new Error(`${label} duplicates top-level release basename: ${releaseName}`);
  }
  seenReleaseNames.add(releaseName);
  const release = requiredFile(releaseRoot, releaseName, `${label} published file`);
  if (reference.sha256 !== candidate.sha256) throw new Error(`${label} candidate evidence digest is stale`);
  if (reference.size !== undefined && reference.size !== candidate.size) throw new Error(`${label} candidate evidence size is stale`);
  if (release.sha256 !== candidate.sha256 || release.size !== candidate.size) {
    throw new Error(`${label} published file differs from candidate bundle: ${releaseName}`);
  }
  return { path: releaseName, sha256: release.sha256, size: release.size };
}

function readJson(filePath, label) {
  let text;
  try {
    text = fs.readFileSync(filePath, "utf8");
  } catch (error) {
    throw new Error(`cannot read ${label}: ${error.message}`);
  }
  try {
    return JSON.parse(text);
  } catch (error) {
    throw new Error(`cannot parse ${label}: ${error.message}`);
  }
}

function positiveInteger(value, label) {
  const parsed = parseSafePositiveInteger(value);
  if (parsed !== null) return parsed;
  throw new Error(`${label} must be a positive integer`);
}

function digest(value, label, prefixed = false) {
  const pattern = prefixed ? /^sha256:[a-f0-9]{64}$/ : /^[a-f0-9]{64}$/;
  if (typeof value !== "string" || !pattern.test(value)) throw new Error(`${label} must be a SHA-256 digest`);
  return value;
}

function assertWorkflowBinding(value, label, expected) {
  if (!isRecord(value)) throw new Error(`${label} must be an object`);
  const id = positiveInteger(value.id ?? value.runId, `${label}.id`);
  const attempt = positiveInteger(value.attempt ?? value.runAttempt, `${label}.attempt`);
  for (const key of ["workflow", "ref", "commitSha"]) {
    if (typeof value[key] !== "string" || value[key].trim() === "") throw new Error(`${label}.${key} must be non-empty`);
    if (expected?.[key] !== undefined && value[key] !== expected[key]) throw new Error(`${label}.${key} does not match expected binding`);
  }
  const expectedId = expected?.id ?? expected?.runId;
  const expectedAttempt = expected?.attempt ?? expected?.runAttempt;
  if (expectedId !== undefined && id !== parseSafePositiveInteger(expectedId)) throw new Error(`${label}.id does not match expected binding`);
  if (expectedAttempt !== undefined && attempt !== parseSafePositiveInteger(expectedAttempt)) throw new Error(`${label}.attempt does not match expected binding`);
  return { ...value, id, attempt };
}

function validateArtifactMetadata(values, requiredNames, label, expected) {
  if (!Array.isArray(values) || values.length !== requiredNames.length) {
    throw new Error(`${label} must contain exactly ${requiredNames.length} artifacts`);
  }
  const byName = new Map();
  for (const [index, value] of values.entries()) {
    if (!isRecord(value)) throw new Error(`${label}[${index}] must be an object`);
    const name = value.name;
    if (!requiredNames.includes(name)) throw new Error(`${label}[${index}].name is not a required artifact: ${name}`);
    if (byName.has(name)) throw new Error(`${label} contains duplicate artifact: ${name}`);
    const id = positiveInteger(value.id, `${label}[${index}].id`);
    digest(value.digest, `${label}[${index}].digest`, true);
    if (value.expired !== false) throw new Error(`${label}[${index}].expired must be false`);
    const runId = positiveInteger(value.runId, `${label}[${index}].runId`);
    const runAttempt = positiveInteger(value.runAttempt, `${label}[${index}].runAttempt`);
    for (const key of ["workflow", "ref", "commitSha"]) {
      if (typeof value[key] !== "string" || value[key].trim() === "") throw new Error(`${label}[${index}].${key} must be non-empty`);
      if (expected?.[key] !== undefined && value[key] !== expected[key]) throw new Error(`${label}[${index}].${key} does not match expected binding`);
    }
    if (expected?.runId !== undefined && runId !== parseSafePositiveInteger(expected.runId)) throw new Error(`${label}[${index}].runId does not match expected binding`);
    if (expected?.runAttempt !== undefined && runAttempt !== parseSafePositiveInteger(expected.runAttempt)) throw new Error(`${label}[${index}].runAttempt does not match expected binding`);
    byName.set(name, { ...value, id, runId, runAttempt });
  }
  for (const name of requiredNames) if (!byName.has(name)) throw new Error(`${label} is missing required artifact: ${name}`);
  return [...byName.values()].sort((left, right) => left.name.localeCompare(right.name));
}

function validateSourceArtifactIdentifiers(values) {
  for (const [index, value] of values.entries()) {
    if (!isRecord(value)) throw new Error(`canonical sourceArtifacts[${index}] must be an object`);
    const label = `canonical sourceArtifacts[${index}]`;
    positiveInteger(value.id, `${label}.id`);
    positiveInteger(value.runId, `${label}.runId`);
    positiveInteger(value.runAttempt, `${label}.runAttempt`);
  }
}

function safeTopLevelPath(value, label) {
  if (typeof value !== "string" || value.trim() === "" || path.isAbsolute(value)
    || value.includes("\\") || value.includes("\0") || path.posix.basename(value) !== value
    || value === "." || value === "..") {
    throw new Error(`${label} must be a safe top-level release path`);
  }
  return value;
}

function listTopLevelFiles(root, label) {
  const base = path.resolve(root);
  const entries = fs.readdirSync(base, { withFileTypes: true });
  const files = [];
  for (const entry of entries) {
    const absolute = path.join(base, entry.name);
    const stat = fs.lstatSync(absolute);
    if (stat.isSymbolicLink()) throw new Error(`${label} must not contain symlink: ${entry.name}`);
    if (!stat.isFile() || stat.size === 0) throw new Error(`${label} contains a non-file or empty entry: ${entry.name}`);
    files.push(entry.name);
  }
  return files.sort();
}

function requireBundleCategory(files, predicate, label) {
  if (!files.some(predicate)) throw new Error(`sealed bundle is missing ${label}`);
}

function validateSealedFiles(files, candidateRoot, releaseRoot) {
  if (!Array.isArray(files) || files.length === 0) throw new Error("sealed bundle files must be a non-empty array");
  const seen = new Set();
  const checked = [];
  for (const [index, entry] of files.entries()) {
    if (!isRecord(entry)) throw new Error(`sealed bundle files[${index}] must be an object`);
    const relative = safeTopLevelPath(entry.path, `sealed bundle files[${index}].path`);
    if (seen.has(relative)) throw new Error(`sealed bundle files contains duplicate path: ${relative}`);
    seen.add(relative);
    const size = positiveInteger(entry.size, `sealed bundle files[${index}].size`);
    const expectedSha = digest(entry.sha256, `sealed bundle files[${index}].sha256`);
    const candidate = requiredFile(candidateRoot, relative, `sealed bundle ${relative} candidate`);
    const release = requiredFile(releaseRoot, relative, `sealed bundle ${relative} published`);
    if (candidate.size !== size || candidate.sha256 !== expectedSha) throw new Error(`sealed bundle candidate digest/size mismatch: ${relative}`);
    if (release.size !== size || release.sha256 !== expectedSha) throw new Error(`sealed bundle published digest/size mismatch: ${relative}`);
    checked.push({ path: relative, size, sha256: expectedSha, ...(entry.kind ? { kind: entry.kind } : {}) });
  }
  const expected = [...seen].sort();
  const actual = listTopLevelFiles(releaseRoot, "published release");
  if (expected.length !== actual.length || expected.some((name, index) => name !== actual[index])) {
    throw new Error(`published release files do not exactly match sealed bundle (expected: ${expected.join(",")}; actual: ${actual.join(",")})`);
  }
  for (const platform of REQUIRED_PLATFORMS) {
    requireBundleCategory(expected, (name) => name === `tauri-release-${platform}.json`, `${platform} package manifest`);
    requireBundleCategory(expected, (name) => name === `tauri-runtime-smoke-${platform}.json`, `${platform} runtime smoke report`);
    requireBundleCategory(expected, (name) => name === `JFTrade-${platform}.spdx.json`, `${platform} SBOM`);
  }
  requireBundleCategory(expected, (name) => name.includes("macos-arm64") && name.endsWith(".dmg"), "macOS package");
  for (const extension of [".AppImage", ".deb", ".rpm"]) requireBundleCategory(expected, (name) => name.endsWith(extension), `Linux ${extension} package`);
  requireBundleCategory(expected, (name) => /windows-x64.*(?:\.msi|-setup\.exe)$/.test(name), "Windows x64 package");
  requireBundleCategory(expected, (name) => /windows-arm64.*(?:\.msi|-setup\.exe)$/.test(name), "Windows ARM64 package");
  requireBundleCategory(expected, (name) => name.endsWith(".sig"), "updater signature");
  requireBundleCategory(expected, (name) => name.endsWith(".tar.gz") || name.endsWith(".zip"), "updater archive");
  requireBundleCategory(expected, (name) => name === "latest.json" || name === "updater.json" || name.startsWith("latest-") && name.endsWith(".json"), "updater feed");
  requireBundleCategory(expected, (name) => name === "LICENSE", "LICENSE");
  requireBundleCategory(expected, (name) => name === "THIRD-PARTY-NOTICES.md", "third-party notices");
  requireBundleCategory(expected, (name) => name === "SHA256SUMS", "SHA256SUMS");
  const sumsPath = path.join(candidateRoot, "SHA256SUMS");
  const sumsText = fs.readFileSync(sumsPath, "utf8");
  const sums = new Map();
  for (const [index, line] of sumsText.split(/\r?\n/).entries()) {
    if (line.trim() === "") continue;
    const match = line.match(/^([a-f0-9]{64})\s+\*?([^\s].*)$/i);
    if (!match) throw new Error(`SHA256SUMS line ${index + 1} is invalid`);
    const name = safeTopLevelPath(match[2].trim(), `SHA256SUMS line ${index + 1}`);
    if (sums.has(name)) throw new Error(`SHA256SUMS contains duplicate entry: ${name}`);
    sums.set(name, match[1].toLowerCase());
  }
  const expectedSums = expected.filter((name) => name !== "SHA256SUMS");
  if (sums.size !== expectedSums.length || expectedSums.some((name) => !sums.has(name))) {
    throw new Error("SHA256SUMS does not exactly represent sealed release files");
  }
  for (const name of expectedSums) {
    const actualDigest = sha256(path.join(candidateRoot, name));
    if (sums.get(name) !== actualDigest) throw new Error(`SHA256SUMS digest mismatch for ${name}`);
  }
  return checked;
}

/** Verify the sealed candidate bundle, including its exact source artifact provenance and all publishable files. */
export function verifySealedReleaseBundle({ manifestPath, evidencePath, candidateRoot, releaseRoot, expectedQualificationRun, expectedSourceWorkflowRun } = {}) {
  const manifest = readJson(path.resolve(manifestPath ?? ""), "sealed release bundle manifest");
  if (!isRecord(manifest) || manifest.$schema !== "./sealed-release-bundle.schema.json" || manifest.schemaVersion !== SEALED_BUNDLE_SCHEMA) throw new Error(`sealed release bundle must use ${SEALED_BUNDLE_SCHEMA}`);
  if (typeof manifest.repository !== "string" || !/^[^/\s]+\/[^/\s]+$/.test(manifest.repository)) throw new Error("sealed bundle repository is invalid");
  if (typeof manifest.releaseTag !== "string" || !/^v\d+\.\d+\.\d+$/.test(manifest.releaseTag)
    || typeof manifest.releaseRef !== "string"
    || !/^refs\/(?:heads|tags)\/(?!.*\.\.)[A-Za-z0-9._/-]+$/.test(manifest.releaseRef)
    || (manifest.releaseRef.startsWith("refs/tags/")
      && manifest.releaseRef !== `refs/tags/${manifest.releaseTag}`)) {
    throw new Error("sealed bundle candidate ref or planned tag binding is invalid");
  }
  const sourceRun = assertWorkflowBinding(manifest.sourceWorkflowRun, "sealed bundle sourceWorkflowRun", expectedSourceWorkflowRun);
  const qualificationRun = assertWorkflowBinding(manifest.qualificationRun, "sealed bundle qualificationRun", expectedQualificationRun);
  const expectedSource = {
    runId: sourceRun.id ?? sourceRun.runId,
    runAttempt: sourceRun.attempt ?? sourceRun.runAttempt,
    workflow: sourceRun.workflow,
    ref: sourceRun.ref,
    commitSha: sourceRun.commitSha,
  };
  const sourceArtifacts = validateArtifactMetadata(manifest.sourceArtifacts, REQUIRED_SOURCE_ARTIFACTS, "sealed bundle sourceArtifacts", expectedSource);
  const updaterArtifacts = validateArtifactMetadata(manifest.sourceUpdaterArtifacts, REQUIRED_UPDATER_ARTIFACTS, "sealed bundle sourceUpdaterArtifacts", expectedSource);
  if (manifest.releaseRef !== sourceRun.ref || manifest.commitSha !== sourceRun.commitSha) throw new Error("sealed bundle release binding does not match source workflow");
  if (manifest.qualificationRun.workflow !== "desktop-release-qualification.yml") throw new Error("sealed bundle qualification workflow is not trusted");
  if (manifest.qualificationRun.ref !== manifest.releaseRef || manifest.qualificationRun.commitSha !== manifest.commitSha) throw new Error("sealed bundle qualification binding does not match release");
  const candidateBase = path.resolve(candidateRoot ?? "");
  const releaseBase = path.resolve(releaseRoot ?? "");
  const files = validateSealedFiles(manifest.files, candidateBase, releaseBase);
  const canonical = manifest.canonicalEvidence;
  if (!isRecord(canonical)) throw new Error("sealed bundle canonicalEvidence is required");
  const canonicalPath = safeTopLevelPath(canonical.path, "sealed bundle canonicalEvidence.path");
  const canonicalFile = requiredFile(candidateBase, canonicalPath, "sealed bundle canonical evidence");
  if (canonicalFile.sha256 !== digest(canonical.sha256, "sealed bundle canonicalEvidence.sha256")
    || canonicalFile.size !== positiveInteger(canonical.size, "sealed bundle canonicalEvidence.size")) {
    throw new Error("sealed bundle canonical evidence digest/size mismatch");
  }
  if (evidencePath) {
    const requested = path.resolve(evidencePath);
    if (requested !== canonicalFile.path) throw new Error("sealed bundle canonical evidence path does not match requested evidence");
  }
  const evidence = readJson(canonicalFile.path, "canonical candidate evidence");
  const evidenceArtifacts = validateArtifactMetadata(evidence.sourceArtifacts, REQUIRED_SOURCE_ARTIFACTS, "canonical sourceArtifacts", expectedSource);
  for (const [index, artifact] of sourceArtifacts.entries()) {
    const candidate = evidenceArtifacts[index];
    if (artifact.name !== candidate.name || artifact.id !== candidate.id || artifact.digest !== candidate.digest) throw new Error(`canonical source artifact metadata differs: ${artifact.name}`);
  }
  const metadataPath = path.join(candidateBase, "source-artifact-metadata.json");
  const sourceMetadata = readJson(metadataPath, "source artifact metadata");
  const metadataSource = validateArtifactMetadata(sourceMetadata.releaseArtifacts, REQUIRED_SOURCE_ARTIFACTS, "downloaded sourceArtifacts", expectedSource);
  const metadataUpdater = validateArtifactMetadata(sourceMetadata.updaterArtifacts, REQUIRED_UPDATER_ARTIFACTS, "downloaded sourceUpdaterArtifacts", expectedSource);
  for (const [index, artifact] of sourceArtifacts.entries()) {
    const candidate = metadataSource[index];
    if (artifact.name !== candidate.name || artifact.id !== candidate.id || artifact.digest !== candidate.digest) throw new Error(`downloaded source artifact metadata differs: ${artifact.name}`);
  }
  for (const [index, artifact] of updaterArtifacts.entries()) {
    const candidate = metadataUpdater[index];
    if (artifact.name !== candidate.name || artifact.id !== candidate.id || artifact.digest !== candidate.digest) throw new Error(`downloaded updater artifact metadata differs: ${artifact.name}`);
  }
  return {
    status: "verified",
    manifestPath: path.resolve(manifestPath),
    evidencePath: canonicalFile.path,
    candidateRoot: candidateBase,
    releaseRoot: releaseBase,
    sourceArtifacts,
    sourceUpdaterArtifacts: updaterArtifacts,
    qualificationRun,
    files,
  };
}

/** Verify that the published files are byte-identical to one downloaded candidate bundle. */
export function verifyReleaseCandidateBundle({ evidencePath, candidateRoot, releaseRoot } = {}) {
  const evidence = readJson(path.resolve(evidencePath ?? ""), "canonical candidate evidence");
  if (!isRecord(evidence) || !isRecord(evidence.platforms)) throw new Error("canonical candidate evidence has no platforms");
  if (!Array.isArray(evidence.sourceArtifacts) || evidence.sourceArtifacts.length === 0) {
    throw new Error("canonical candidate evidence must include source artifact metadata");
  }
  validateSourceArtifactIdentifiers(evidence.sourceArtifacts);
  const platforms = Object.keys(evidence.platforms);
  const missingPlatforms = REQUIRED_PLATFORMS.filter((platform) => !platforms.includes(platform));
  const unknownPlatforms = platforms.filter((platform) => !REQUIRED_PLATFORMS.includes(platform));
  if (missingPlatforms.length > 0 || unknownPlatforms.length > 0 || platforms.length !== REQUIRED_PLATFORMS.length) {
    throw new Error(`canonical candidate evidence must contain exactly the required platforms (missing: ${missingPlatforms.join(",") || "none"}; unknown: ${unknownPlatforms.join(",") || "none"})`);
  }
  const candidateBase = path.resolve(candidateRoot ?? "");
  const releaseBase = path.resolve(releaseRoot ?? "");
  const files = [];
  const seenReleaseNames = new Set();
  for (const platform of REQUIRED_PLATFORMS) {
    const value = evidence.platforms[platform];
    if (!isRecord(value) || !isRecord(value.manifest) || !Array.isArray(value.artifacts)) {
      throw new Error(`canonical candidate evidence platform is incomplete: ${platform}`);
    }
    files.push(compareFile(candidateBase, releaseBase, value.manifest, `${platform}.manifest`, seenReleaseNames));
    for (const [index, artifact] of value.artifacts.entries()) {
      files.push(compareFile(candidateBase, releaseBase, artifact, `${platform}.artifacts[${index}]`, seenReleaseNames));
    }
  }
  return {
    status: "verified",
    evidencePath: path.resolve(evidencePath),
    candidateRoot: candidateBase,
    releaseRoot: releaseBase,
    sourceArtifacts: evidence.sourceArtifacts,
    files,
  };
}

function parseArgs(args) {
  const values = {};
  for (let index = 0; index < args.length; index += 1) {
    const argument = args[index];
    if (!argument.startsWith("--")) throw new Error(`unknown argument: ${argument}`);
    const [key, inline] = argument.slice(2).split("=", 2);
    const value = inline ?? args[++index];
    if (!value) throw new Error(`--${key} requires a value`);
    values[key] = value;
  }
  for (const key of ["evidence", "candidate-root", "release-root"]) {
    if (!values[key]) throw new Error(`--${key} is required`);
  }
  const knownFlags = new Set(["evidence", "candidate-root", "release-root", "sealed-manifest", "qualification-run-id", "qualification-attempt", "source-run-id", "source-attempt", "release-ref", "expected-commit"]);
  const unknown = Object.keys(values).find((key) => !knownFlags.has(key));
  if (unknown) throw new Error(`unknown argument: --${unknown}`);
  if (values["sealed-manifest"] && (!values["qualification-run-id"] || !values["qualification-attempt"] || !values["source-run-id"] || !values["source-attempt"] || !values["release-ref"] || !values["expected-commit"])) {
    throw new Error("--sealed-manifest requires qualification/source run bindings, --release-ref and --commit");
  }
  return values;
}

export function main(args = process.argv.slice(2)) {
  try {
    const values = parseArgs(args);
    const report = values["sealed-manifest"]
      ? verifySealedReleaseBundle({
        manifestPath: values["sealed-manifest"],
        evidencePath: values.evidence,
        candidateRoot: values["candidate-root"],
        releaseRoot: values["release-root"],
        expectedQualificationRun: {
          id: values["qualification-run-id"],
          attempt: values["qualification-attempt"],
          workflow: "desktop-release-qualification.yml",
          ref: values["release-ref"],
          commitSha: values["expected-commit"],
        },
        expectedSourceWorkflowRun: {
          id: values["source-run-id"],
          attempt: values["source-attempt"],
          workflow: "desktop-release.yml",
          ref: values["release-ref"],
          commitSha: values["expected-commit"],
        },
      })
      : verifyReleaseCandidateBundle({
        evidencePath: values.evidence,
        candidateRoot: values["candidate-root"],
        releaseRoot: values["release-root"],
      });
    console.log(JSON.stringify(report, null, 2));
    return 0;
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    return 1;
  }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  process.exitCode = main();
}
