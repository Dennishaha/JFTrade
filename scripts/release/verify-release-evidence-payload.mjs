#!/usr/bin/env node

import process from "node:process";
import {
  parseSafePositiveInteger,
  validateReleaseEvidencePayload,
} from "./check-release-evidence-inputs.mjs";

function parseArgs(argv) {
  const result = {};
  const allowed = new Set([
    "root", "repository", "release_ref", "payload_ref", "payload_commit_sha",
    "payload_workflow", "payload_run_id", "payload_run_attempt", "payload_artifact",
    "payload_artifact_id", "payload_artifact_digest",
  ]);
  for (let index = 0; index < argv.length; index += 1) {
    const token = argv[index];
    if (!token.startsWith("--") || index + 1 >= argv.length || argv[index + 1].startsWith("--")) {
      throw new Error(`invalid argument: ${token}`);
    }
    const key = token.slice(2).replaceAll("-", "_");
    if (!allowed.has(key)) throw new Error(`unsupported argument: ${token}`);
    if (result[key] !== undefined) throw new Error(`duplicate argument: ${token}`);
    result[key] = argv[index + 1];
    index += 1;
  }
  return result;
}

function required(value, label) {
  if (typeof value !== "string" || value.trim() === "") throw new Error(`${label} is required`);
  return value.trim();
}

function positive(value, label) {
  const parsed = parseSafePositiveInteger(value);
  if (parsed === null) throw new Error(`${label} must be a positive integer`);
  return parsed;
}

function main(argv = process.argv.slice(2)) {
  const args = parseArgs(argv);
  const expectedBinding = {
    repository: required(args.repository, "repository"),
    releaseRef: required(args.release_ref, "release_ref"),
    ref: required(args.payload_ref, "payload_ref"),
    commitSha: required(args.payload_commit_sha, "payload_commit_sha"),
    workflow: required(args.payload_workflow, "payload_workflow"),
    runId: positive(args.payload_run_id, "payload_run_id"),
    attempt: positive(args.payload_run_attempt, "payload_run_attempt"),
    artifact: {
      name: required(args.payload_artifact, "payload_artifact"),
      id: positive(args.payload_artifact_id, "payload_artifact_id"),
      digest: required(args.payload_artifact_digest, "payload_artifact_digest"),
    },
  };
  const result = validateReleaseEvidencePayload({
    baseDirectory: required(args.root, "root"),
    expectedBinding,
  });
  process.stdout.write(`${JSON.stringify(result, null, 2)}\n`);
  return result.valid ? 0 : 1;
}

if (import.meta.url === `file://${process.argv[1]}`) {
  try {
    process.exitCode = main();
  } catch (error) {
    process.stderr.write(`${error.message}\n`);
    process.exitCode = 1;
  }
}
