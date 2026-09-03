#!/usr/bin/env node

import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

import { inspectReleaseCandidateRehearsal } from "./check-release-candidate-rehearsal.mjs";
import { parseSafePositiveInteger } from "./check-release-evidence-inputs.mjs";

const SOURCE_KEYS = Object.freeze([
  "schemaVersion",
  "qualificationMode",
  "repository",
  "candidateRef",
  "plannedReleaseTag",
  "commitSha",
  "workflowRun",
  "sourceWorkflowRun",
  "platforms",
  "limitations",
]);

function isRecord(value) {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function exactSource(source) {
  if (!isRecord(source)) throw new Error("rehearsal source must be an object");
  const unknown = Object.keys(source).filter((key) => !SOURCE_KEYS.includes(key));
  const missing = SOURCE_KEYS.filter((key) => !(key in source));
  if (unknown.length) throw new Error(`rehearsal source has unsupported field(s): ${unknown.join(", ")}`);
  if (missing.length) throw new Error(`rehearsal source is missing field(s): ${missing.join(", ")}`);
  if (source.schemaVersion !== "jftrade.release-candidate-rehearsal-source.v1"
    || source.qualificationMode !== "rehearsal") {
    throw new Error("rehearsal source schema or qualification mode is invalid");
  }
}

function artifactBinding(value) {
  if (!isRecord(value)) throw new Error("source artifact binding is required");
  const keys = ["name", "id", "digest"];
  const unknown = Object.keys(value).filter((key) => !keys.includes(key));
  if (unknown.length) throw new Error(`source artifact has unsupported field(s): ${unknown.join(", ")}`);
  const id = parseSafePositiveInteger(value.id);
  if (typeof value.name !== "string" || !/^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$/.test(value.name)) {
    throw new Error("source artifact name is invalid");
  }
  if (id === null) throw new Error("source artifact id must be a safe positive integer");
  if (typeof value.digest !== "string" || !/^sha256:[a-f0-9]{64}$/.test(value.digest)) {
    throw new Error("source artifact digest is invalid");
  }
  return { name: value.name, id, digest: value.digest };
}

export function bindReleaseCandidateRehearsal(source, artifact, options = {}) {
  exactSource(source);
  const receipt = {
    $schema: "./release-candidate-rehearsal.schema.json",
    schemaVersion: "jftrade.release-candidate-rehearsal.v1",
    phase: "pre-release",
    status: "rehearsal_passed",
    qualificationLevel: "unsigned-rehearsal",
    releaseQualified: false,
    repository: source.repository,
    candidateRef: source.candidateRef,
    plannedReleaseTag: source.plannedReleaseTag,
    commitSha: source.commitSha,
    workflowRun: source.workflowRun,
    sourceWorkflowRun: source.sourceWorkflowRun,
    artifact: artifactBinding(artifact),
    platforms: source.platforms,
    limitations: source.limitations,
  };
  const result = inspectReleaseCandidateRehearsal(receipt, options);
  if (!result.valid) throw new Error(result.errors.join("; "));
  return receipt;
}

function argument(args, name) {
  const index = args.indexOf(name);
  if (index >= 0) {
    const value = args[index + 1];
    if (!value || value.startsWith("--")) throw new Error(`${name} requires a value`);
    return value;
  }
  return undefined;
}

export function main(args = process.argv.slice(2)) {
  try {
    const allowed = ["--source", "--output", "--artifact-name", "--artifact-id", "--artifact-digest"];
    const unknown = args.find((item) => item.startsWith("--") && !allowed.includes(item));
    if (unknown) throw new Error(`unsupported argument: ${unknown}`);
    const sourcePath = argument(args, "--source");
    const outputPath = argument(args, "--output");
    if (!sourcePath || !outputPath) throw new Error("--source and --output are required");
    const source = JSON.parse(fs.readFileSync(path.resolve(sourcePath), "utf8"));
    const receipt = bindReleaseCandidateRehearsal(source, {
      name: argument(args, "--artifact-name"),
      id: argument(args, "--artifact-id"),
      digest: argument(args, "--artifact-digest"),
    });
    const output = path.resolve(outputPath);
    fs.mkdirSync(path.dirname(output), { recursive: true });
    fs.writeFileSync(output, `${JSON.stringify(receipt, null, 2)}\n`, { flag: "wx" });
    console.log(JSON.stringify({ status: "rehearsal_passed", releaseQualified: false, output }, null, 2));
    return 0;
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    return 1;
  }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  process.exitCode = main();
}
