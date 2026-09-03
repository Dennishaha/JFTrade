#!/usr/bin/env node

import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

import { parseSafePositiveInteger } from "./check-release-evidence-inputs.mjs";

export const RELEASE_CANDIDATE_REHEARSAL_SCHEMA =
  "jftrade.release-candidate-rehearsal.v1";
export const REHEARSAL_CANDIDATE_REF =
  "refs/heads/release/0.29.0-candidate";
export const REHEARSAL_RELEASE_TAG = "v0.29.0";
export const REHEARSAL_PLATFORMS = Object.freeze([
  "macos-arm64",
  "linux-x64",
  "windows-x64",
  "windows-arm64",
]);
export const REHEARSAL_CHECKS = Object.freeze([
  "package",
  "install",
  "firstStart",
  "upgrade",
  "databaseUpgrade",
  "runtimeSmoke",
  "uninstall",
  "backupRestore",
  "rollback",
  "zeroGo",
  "sbomZeroGo",
]);
export const REHEARSAL_LIMITATIONS = Object.freeze({
  packageSigning: "not_run",
  notarization: "not_run",
  updaterSignature: "not_run",
  independentSecuritySignOff: "open",
});

const ROOT_KEYS = Object.freeze([
  "$schema",
  "schemaVersion",
  "phase",
  "status",
  "qualificationLevel",
  "releaseQualified",
  "repository",
  "candidateRef",
  "plannedReleaseTag",
  "commitSha",
  "workflowRun",
  "sourceWorkflowRun",
  "artifact",
  "platforms",
  "limitations",
]);

function isRecord(value) {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function exactKeys(value, allowed, required, label, errors) {
  if (!isRecord(value)) {
    errors.push(`${label} must be an object`);
    return false;
  }
  for (const key of Object.keys(value)) {
    if (!allowed.includes(key)) errors.push(`${label}.${key} is not allowed`);
  }
  for (const key of required) {
    if (!(key in value)) errors.push(`${label}.${key} is required`);
  }
  return true;
}

function constant(value, expected, label, errors) {
  if (value !== expected) errors.push(`${label} must be ${JSON.stringify(expected)}`);
}

function validCommit(value, label, errors) {
  if (typeof value !== "string" || !/^(?:[a-f0-9]{40}|[a-f0-9]{64})$/.test(value)) {
    errors.push(`${label} must be a lowercase commit SHA`);
    return null;
  }
  return value;
}

function safeInteger(value, label, errors) {
  const parsed = parseSafePositiveInteger(value);
  if (parsed === null) {
    errors.push(`${label} must be a safe positive integer`);
    return null;
  }
  return parsed;
}

function workflowRun(value, label, commitSha, errors) {
  const keys = ["id", "attempt", "workflow", "ref", "commitSha"];
  if (!exactKeys(value, keys, keys, label, errors)) return null;
  const result = {
    id: safeInteger(value.id, `${label}.id`, errors),
    attempt: safeInteger(value.attempt, `${label}.attempt`, errors),
    workflow: value.workflow,
    ref: value.ref,
    commitSha: validCommit(value.commitSha, `${label}.commitSha`, errors),
  };
  if (typeof result.workflow !== "string"
    || !/^[A-Za-z0-9._/-]+\.yml$/.test(result.workflow)
    || result.workflow.includes("..")) {
    errors.push(`${label}.workflow must be a safe workflow filename`);
  }
  constant(result.ref, REHEARSAL_CANDIDATE_REF, `${label}.ref`, errors);
  if (result.commitSha && commitSha && result.commitSha !== commitSha) {
    errors.push(`${label}.commitSha does not match manifest.commitSha`);
  }
  return result;
}

function artifact(value, label, errors) {
  const keys = ["name", "id", "digest"];
  if (!exactKeys(value, keys, keys, label, errors)) return null;
  const result = {
    name: value.name,
    id: safeInteger(value.id, `${label}.id`, errors),
    digest: value.digest,
  };
  if (typeof result.name !== "string" || !/^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$/.test(result.name)) {
    errors.push(`${label}.name must be a simple artifact name`);
  }
  if (typeof result.digest !== "string" || !/^sha256:[a-f0-9]{64}$/.test(result.digest)) {
    errors.push(`${label}.digest must be sha256:<64 lowercase hex>`);
  }
  return result;
}

function platform(value, name, errors) {
  const label = `manifest.platforms.${name}`;
  const keys = ["status", "artifact", "checks"];
  if (!exactKeys(value, keys, keys, label, errors)) return null;
  constant(value.status, "passed", `${label}.status`, errors);
  const sourceArtifact = artifact(value.artifact, `${label}.artifact`, errors);
  if (exactKeys(value.checks, REHEARSAL_CHECKS, REHEARSAL_CHECKS, `${label}.checks`, errors)) {
    for (const check of REHEARSAL_CHECKS) {
      constant(value.checks[check], "passed", `${label}.checks.${check}`, errors);
    }
  }
  return { status: value.status, artifact: sourceArtifact, checks: value.checks };
}

function expectedValue(actual, expected, label, errors) {
  if (expected !== undefined && String(actual) !== String(expected)) {
    errors.push(`${label} does not match expected value`);
  }
}

export function inspectReleaseCandidateRehearsal(document, options = {}) {
  const errors = [];
  if (!exactKeys(document, ROOT_KEYS, ROOT_KEYS, "manifest", errors)) {
    return { valid: false, status: "invalid_inputs", releaseQualified: false, errors };
  }
  constant(document.$schema, "./release-candidate-rehearsal.schema.json", "manifest.$schema", errors);
  constant(document.schemaVersion, RELEASE_CANDIDATE_REHEARSAL_SCHEMA, "manifest.schemaVersion", errors);
  constant(document.phase, "pre-release", "manifest.phase", errors);
  constant(document.status, "rehearsal_passed", "manifest.status", errors);
  constant(document.qualificationLevel, "unsigned-rehearsal", "manifest.qualificationLevel", errors);
  constant(document.releaseQualified, false, "manifest.releaseQualified", errors);
  constant(document.repository, "Dennishaha/jftrade", "manifest.repository", errors);
  constant(document.candidateRef, REHEARSAL_CANDIDATE_REF, "manifest.candidateRef", errors);
  constant(document.plannedReleaseTag, REHEARSAL_RELEASE_TAG, "manifest.plannedReleaseTag", errors);
  const commitSha = validCommit(document.commitSha, "manifest.commitSha", errors);
  const run = workflowRun(document.workflowRun, "manifest.workflowRun", commitSha, errors);
  const sourceRun = workflowRun(document.sourceWorkflowRun, "manifest.sourceWorkflowRun", commitSha, errors);
  if (run && sourceRun && run.workflow === sourceRun.workflow && run.id === sourceRun.id) {
    errors.push("manifest.workflowRun must be distinct from sourceWorkflowRun");
  }
  const evidenceArtifact = artifact(document.artifact, "manifest.artifact", errors);
  const platforms = {};
  if (exactKeys(document.platforms, REHEARSAL_PLATFORMS, REHEARSAL_PLATFORMS, "manifest.platforms", errors)) {
    const names = new Set();
    const ids = new Set();
    for (const name of REHEARSAL_PLATFORMS) {
      platforms[name] = platform(document.platforms[name], name, errors);
      const platformArtifact = platforms[name]?.artifact;
      if (platformArtifact) {
        if (names.has(platformArtifact.name)) errors.push(`platform artifact name is reused: ${platformArtifact.name}`);
        if (ids.has(platformArtifact.id)) errors.push(`platform artifact id is reused: ${platformArtifact.id}`);
        names.add(platformArtifact.name);
        ids.add(platformArtifact.id);
      }
    }
  }
  const limitationKeys = Object.keys(REHEARSAL_LIMITATIONS);
  if (exactKeys(document.limitations, limitationKeys, limitationKeys, "manifest.limitations", errors)) {
    for (const [key, value] of Object.entries(REHEARSAL_LIMITATIONS)) {
      constant(document.limitations[key], value, `manifest.limitations.${key}`, errors);
    }
  }
  const expected = options.expected ?? options;
  expectedValue(document.candidateRef, expected.candidateRef, "manifest.candidateRef", errors);
  expectedValue(document.plannedReleaseTag, expected.plannedReleaseTag, "manifest.plannedReleaseTag", errors);
  expectedValue(document.commitSha, expected.commitSha, "manifest.commitSha", errors);
  expectedValue(run?.id, expected.runId, "manifest.workflowRun.id", errors);
  expectedValue(run?.attempt, expected.runAttempt, "manifest.workflowRun.attempt", errors);
  expectedValue(run?.workflow, expected.workflow, "manifest.workflowRun.workflow", errors);
  expectedValue(evidenceArtifact?.name, expected.artifactName, "manifest.artifact.name", errors);
  expectedValue(evidenceArtifact?.id, expected.artifactId, "manifest.artifact.id", errors);
  expectedValue(evidenceArtifact?.digest, expected.artifactDigest, "manifest.artifact.digest", errors);
  return {
    valid: errors.length === 0,
    schemaVersion: RELEASE_CANDIDATE_REHEARSAL_SCHEMA,
    phase: "pre-release",
    status: errors.length === 0 ? "rehearsal_passed" : "invalid_inputs",
    qualificationLevel: "unsigned-rehearsal",
    releaseQualified: false,
    candidateRef: document.candidateRef,
    plannedReleaseTag: document.plannedReleaseTag,
    commitSha,
    workflowRun: run,
    sourceWorkflowRun: sourceRun,
    artifact: evidenceArtifact,
    platforms,
    limitations: { ...REHEARSAL_LIMITATIONS },
    errors,
  };
}

function argument(args, name) {
  const index = args.indexOf(name);
  if (index >= 0) {
    const value = args[index + 1];
    if (!value || value.startsWith("--")) throw new Error(`${name} requires a value`);
    return value;
  }
  const inline = args.find((item) => item.startsWith(`${name}=`));
  return inline ? inline.slice(name.length + 1) : undefined;
}

function parseArgs(args) {
  const allowed = [
    "--check", "--build", "--input", "--config", "--output",
    "--expected-ref", "--expected-tag", "--expected-commit",
    "--expected-run-id", "--expected-attempt", "--expected-workflow",
    "--expected-artifact-name", "--expected-artifact-id", "--expected-artifact-digest",
  ];
  const unknown = args.find((item) => item.startsWith("--")
    && !allowed.some((flag) => item === flag || item.startsWith(`${flag}=`)));
  if (unknown) throw new Error(`unsupported argument: ${unknown}`);
  const build = args.includes("--build");
  const input = argument(args, build ? "--config" : "--input");
  if (!input) throw new Error(build ? "--build requires --config" : "--check requires --input");
  return {
    build,
    input,
    output: argument(args, "--output"),
    expected: {
      candidateRef: argument(args, "--expected-ref"),
      plannedReleaseTag: argument(args, "--expected-tag"),
      commitSha: argument(args, "--expected-commit"),
      runId: argument(args, "--expected-run-id"),
      runAttempt: argument(args, "--expected-attempt"),
      workflow: argument(args, "--expected-workflow"),
      artifactName: argument(args, "--expected-artifact-name"),
      artifactId: argument(args, "--expected-artifact-id"),
      artifactDigest: argument(args, "--expected-artifact-digest"),
    },
  };
}

export function main(args = process.argv.slice(2)) {
  try {
    const parsed = parseArgs(args);
    const inputPath = path.resolve(parsed.input);
    const document = JSON.parse(fs.readFileSync(inputPath, "utf8"));
    const report = inspectReleaseCandidateRehearsal(document, { expected: parsed.expected });
    if (parsed.build) {
      if (!report.valid) throw new Error(report.errors.join("; "));
      const output = path.resolve(parsed.output ?? "release-candidate-rehearsal.json");
      fs.mkdirSync(path.dirname(output), { recursive: true });
      fs.writeFileSync(output, `${JSON.stringify(document, null, 2)}\n`, { flag: "wx" });
    }
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
