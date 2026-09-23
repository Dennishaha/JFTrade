#!/usr/bin/env node
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));
export const MIGRATION_MANIFEST_SCHEMA = "jftrade.migration-manifest.v1";

function readJson(root, relativePath) {
  return JSON.parse(fs.readFileSync(path.join(root, relativePath), "utf8"));
}

function isSha(value, length) {
  return typeof value === "string" && new RegExp(`^[0-9a-f]{${length}}$`).test(value);
}

function isReceiptDigest(value) {
  return typeof value === "string" && /^sha256:[0-9a-f]{64}$/.test(value);
}

function isTimestamp(value) {
  return typeof value === "string" && Number.isFinite(Date.parse(value));
}

export function validateMigrationManifest(root = repositoryRoot) {
  const errors = [];
  const relative = "tests/fixtures/release/migration-manifest.json";
  let manifest;
  try {
    manifest = readJson(root, relative);
  } catch (error) {
    return [`${relative}: ${error.message}`];
  }

  if (manifest.schemaVersion !== MIGRATION_MANIFEST_SCHEMA) errors.push("schemaVersion is invalid");
  if (!new RegExp("^v\\d+\\.\\d+\\.\\d+$").test(manifest.source?.release ?? "")) errors.push("source.release is invalid");
  if (!isSha(manifest.source?.commit, 40)) errors.push("source.commit must be a 40-character SHA");
  if (!isSha(manifest.source?.checksumManifestSha256, 64)) errors.push("source.checksumManifestSha256 must be a SHA-256");
  if (manifest.source?.upgradeBaselines !== "./upgrade-baselines.json") errors.push("source.upgradeBaselines must point to upgrade-baselines.json");
  if (!Array.isArray(manifest.source?.packageDigests) || manifest.source.packageDigests.length === 0) {
    errors.push("source.packageDigests must be non-empty");
  }
  if (manifest.target?.release !== "v0.29.0") errors.push("target.release must be v0.29.0");
  if (!Array.isArray(manifest.components) || manifest.components.length === 0) errors.push("components must be non-empty");

  let baseline;
  try {
    baseline = readJson(root, "tests/fixtures/release/upgrade-baselines.json");
  } catch (error) {
    errors.push(`upgrade-baselines.json: ${error.message}`);
  }
  if (baseline) {
    if (manifest.source.release !== `v${baseline.version}`) errors.push("source.release does not match upgrade-baselines.version");
    if (manifest.source.commit !== baseline.commit) errors.push("source.commit does not match upgrade-baselines.commit");
    if (manifest.source.checksumManifestSha256 !== baseline.checksumManifest?.sha256) errors.push("checksum manifest digest does not match upgrade-baselines");
    if (`v${baseline.policy?.upgradeTarget}` !== manifest.target.release) errors.push("target.release does not match upgrade-baselines.policy.upgradeTarget");
    const expectedPackages = new Map((baseline.packages ?? []).map((entry) => [entry.name, entry]));
    const actualPackages = manifest.source?.packageDigests ?? [];
    if (actualPackages.length !== expectedPackages.size) errors.push("source.packageDigests must include every published baseline package exactly once");
    const seenPackages = new Set();
    for (const [index, entry] of actualPackages.entries()) {
      if (!entry || typeof entry.name !== "string" || !expectedPackages.has(entry.name)) {
        errors.push(`source.packageDigests[${index}].name is not a published baseline package`);
        continue;
      }
      if (seenPackages.has(entry.name)) errors.push(`source.packageDigests[${index}].name is duplicated`);
      seenPackages.add(entry.name);
      const expected = expectedPackages.get(entry.name);
      if (entry.platform !== expected.platform || entry.kind !== expected.kind || entry.sha256 !== expected.sha256) {
        errors.push(`source.packageDigests[${index}] does not match upgrade-baselines package ${entry.name}`);
      }
    }
  }

  const componentNames = new Set();
  for (const [index, component] of (manifest.components ?? []).entries()) {
    const label = `components[${index}]`;
    if (componentNames.has(component.component)) errors.push(`${label}.component is duplicated`);
    componentNames.add(component.component);
    if (!Number.isInteger(component.fromVersion) || !Number.isInteger(component.toVersion) || component.fromVersion >= component.toVersion) {
      errors.push(`${label} must upgrade from a lower integer version`);
    }
    for (const side of ["precondition", "postcondition"]) {
      if (!Number.isInteger(component[side]?.schemaVersion)) errors.push(`${label}.${side}.schemaVersion is required`);
      if (component[side]?.sha256 !== null && !isSha(component[side]?.sha256, 64)) errors.push(`${label}.${side}.sha256 is invalid`);
    }
    if (!Array.isArray(component.invariants) || component.invariants.length === 0) errors.push(`${label}.invariants must be non-empty`);
    for (const field of ["backup", "restore", "rollback", "idempotency"]) {
      if (typeof component[field]?.required !== "boolean") errors.push(`${label}.${field}.required is required`);
      if (!["passed", "not_verified"].includes(component[field]?.status)) errors.push(`${label}.${field}.status is invalid`);
      if (!Array.isArray(component[field]?.evidence)) errors.push(`${label}.${field}.evidence must be an array`);
    }
    if (!["passed", "not_verified"].includes(component.failureRecovery?.status)) errors.push(`${label}.failureRecovery.status is invalid`);
    if (!Array.isArray(component.failureRecovery?.evidence)) errors.push(`${label}.failureRecovery.evidence must be an array`);
    const verification = component.verification;
    if (!["synthetic", "release"].includes(verification?.scope)) errors.push(`${label}.verification.scope is invalid`);
    if (!["passed", "not_verified"].includes(verification?.status)) errors.push(`${label}.verification.status is invalid`);
    if (typeof verification?.test !== "string" || verification.test.length === 0) errors.push(`${label}.verification.test is required`);
    if (!Array.isArray(verification?.platforms) || verification.platforms.length === 0) errors.push(`${label}.verification.platforms must be non-empty`);
    if (typeof verification?.runner !== "string" || verification.runner.length === 0) errors.push(`${label}.verification.runner is required`);
    if (verification?.status === "passed" && verification?.scope === "release" && (typeof verification.receipt !== "string" || !isReceiptDigest(verification.receiptDigest) || !isTimestamp(verification.verifiedAt))) {
      errors.push(`${label}.release verification requires receipt, receiptDigest and verifiedAt`);
    }
  }

  const releaseVerification = manifest.releaseVerification;
  if (!releaseVerification || !Array.isArray(releaseVerification.platforms)) errors.push("releaseVerification.platforms must be an array");
  if (releaseVerification?.status === "passed" && (
    releaseVerification.platforms.length === 0
    || typeof releaseVerification.runner !== "string"
    || typeof releaseVerification.receipt !== "string"
    || !isReceiptDigest(releaseVerification.receiptDigest)
    || !isTimestamp(releaseVerification.verifiedAt)
  )) {
    errors.push("passed release verification requires platforms, runner, receipt, receiptDigest and verifiedAt");
  }

  if (manifest.status === "release_qualified") {
    if (manifest.releaseVerification?.status !== "passed") errors.push("release_qualified requires releaseVerification.status=passed");
    if ((manifest.components ?? []).some((component) => component.verification?.scope !== "release" || component.verification?.status !== "passed")) {
      errors.push("release_qualified requires every component to have passed release verification");
    }
  } else if (manifest.releaseVerification?.status !== "not_verified") {
    errors.push("synthetic manifest must remain releaseVerification.status=not_verified");
  }
  return errors;
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const errors = validateMigrationManifest();
  if (errors.length > 0) {
    console.error(errors.map((error) => `- ${error}`).join("\n"));
    process.exitCode = 1;
  } else {
    console.log("Migration manifest structure and baseline binding passed; release verification remains explicit.");
  }
}
