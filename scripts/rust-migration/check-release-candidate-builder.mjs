#!/usr/bin/env node

import { createHash } from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

import {
  inspectReleaseCandidateEvidence,
  RELEASE_CANDIDATE_EVIDENCE_SCHEMA,
  RELEASE_CANDIDATE_LIMITATIONS,
  REQUIRED_PLATFORMS,
} from "./check-release-candidate.mjs";

function isRecord(value) {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function fileSha256(filePath) {
  return createHash("sha256").update(fs.readFileSync(filePath)).digest("hex");
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

function sourceReference(value, baseDirectory, label) {
  if (typeof value !== "string" && !isRecord(value)) throw new Error(`${label} must be a path or object`);
  const output = typeof value === "string"
    ? { path: portablePath(value, baseDirectory, label) }
    : { ...value, path: portablePath(value.path, baseDirectory, label) };
  const absolute = path.resolve(baseDirectory, output.path);
  let stat;
  try {
    stat = fs.statSync(absolute);
  } catch {
    throw new Error(`${label} file is missing or empty: ${output.path}`);
  }
  if (!stat.isFile() || stat.size === 0) throw new Error(`${label} file is missing or empty: ${output.path}`);
  const actual = fileSha256(absolute);
  if (output.sha256 !== undefined
    && (typeof output.sha256 !== "string" || output.sha256.toLowerCase() !== actual)) {
    throw new Error(`${label}.sha256 does not match ${output.path}`);
  }
  return { ...output, sha256: actual, size: stat.size };
}

function normalizeBuildWorkflow(value) {
  if (!isRecord(value)) throw new Error("workflowRun must be an object");
  return {
    id: value.id ?? value.runId,
    attempt: value.attempt ?? value.runAttempt,
    workflow: value.workflow ?? value.name,
    ref: value.ref,
    commitSha: value.commitSha,
    ...(value.url ? { url: value.url } : {}),
  };
}

/** Build canonical evidence from local files, calculating all SHA-256 values. */
export function buildReleaseCandidateEvidence(options = {}) {
  const baseDirectory = path.resolve(
    options.baseDirectory ?? fileURLToPath(new URL("../..", import.meta.url)),
  );
  const releaseRef = options.releaseRef;
  const releaseTag = options.releaseTag;
  const commitSha = options.commitSha;
  const workflowRun = normalizeBuildWorkflow(options.workflowRun);
  if (!options.sourceWorkflowRun) {
    throw new Error("sourceWorkflowRun is required and must come from a verified source release run");
  }
  const sourceWorkflowRun = normalizeBuildWorkflow(options.sourceWorkflowRun);
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
      kind: entry.kind,
      releaseRef: entry.releaseRef ?? releaseRef,
      commitSha: entry.commitSha ?? commitSha,
      workflowRun: entry.workflowRun ?? workflowRun,
      sourceWorkflowRun: entry.sourceWorkflowRun ?? sourceWorkflowRun,
      ...(entry.summary ? { summary: entry.summary } : {}),
      evidence: evidence.map((item, evidenceIndex) => {
        const reference = typeof item === "string"
          ? { path: item, kind: entry.kind }
          : { ...item, kind: item.kind ?? entry.kind };
        return sourceReference(reference, baseDirectory, `prerequisites[${index}].evidence[${evidenceIndex}]`);
      }),
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
    sourceWorkflowRun,
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
