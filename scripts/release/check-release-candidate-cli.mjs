#!/usr/bin/env node

import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

import {
  inspectReleaseCandidateEvidence,
  writeReleaseCandidateEvidence,
} from "./check-release-candidate.mjs";

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
  const expectedSourceRunId = argumentValue(args, ["--expected-source-run-id"]);
  const expectedSourceAttempt = argumentValue(args, ["--expected-source-attempt"]);
  const expectedSourceWorkflow = argumentValue(args, ["--expected-source-workflow"]);
  const expectedRunValues = [expectedRunId, expectedAttempt, expectedWorkflow, expectedRef, expectedCommit];
  const expectedRunCount = expectedRunValues.filter((value) => value !== null).length;
  if (expectedRunCount > 0 && expectedRunCount < expectedRunValues.length) {
    throw new Error(
      "expected workflow binding requires --expected-ref, --expected-commit, "
        + "--expected-run-id, --expected-attempt and --expected-workflow",
    );
  }
  const expectedSourceValues = [expectedSourceRunId, expectedSourceAttempt, expectedSourceWorkflow];
  const expectedSourceCount = expectedSourceValues.filter((value) => value !== null).length;
  if (expectedSourceCount > 0 && expectedSourceCount < expectedSourceValues.length) {
    throw new Error(
      "expected source workflow binding requires --expected-source-run-id, "
        + "--expected-source-attempt and --expected-source-workflow",
    );
  }
  if (expectedSourceCount > 0 && (!expectedRef || !expectedCommit)) {
    throw new Error(
      "expected source workflow binding also requires --expected-ref and --expected-commit",
    );
  }
  const hasExpectedRun = expectedRunCount === expectedRunValues.length;
  const hasExpectedSourceRun = expectedSourceCount === expectedSourceValues.length;
  const expected = expectedRef || expectedTag || expectedCommit || expectedRunId || expectedAttempt || expectedWorkflow
    || expectedSourceRunId || expectedSourceAttempt || expectedSourceWorkflow
    ? {
      releaseRef: expectedRef ?? undefined,
      releaseTag: expectedTag ?? undefined,
      commitSha: expectedCommit ?? undefined,
      ...(hasExpectedRun ? {
        workflowRun: {
          id: expectedRunId,
          attempt: expectedAttempt,
          workflow: expectedWorkflow,
          ref: expectedRef,
          commitSha: expectedCommit,
        },
      } : {}),
      ...(hasExpectedSourceRun ? {
        sourceWorkflowRun: {
          id: expectedSourceRunId,
          attempt: expectedSourceAttempt,
          workflow: expectedSourceWorkflow,
          ref: expectedRef,
          commitSha: expectedCommit,
        },
      } : {}),
    }
    : undefined;
  const knownFlags = [
    "--build", "--config", "--input", "--manifest", "--output", "--base-dir", "--check",
    "--expected-ref", "--expected-tag", "--expected-commit", "--expected-run-id",
    "--expected-attempt", "--expected-workflow", "--expected-source-run-id",
    "--expected-source-attempt", "--expected-source-workflow",
  ];
  const unknown = args.find((argument) => argument.startsWith("--")
    && !knownFlags.some((flag) => argument === flag || argument.startsWith(`${flag}=`)));
  if (unknown) throw new Error(`unknown argument: ${unknown}`);
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
