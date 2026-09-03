#!/usr/bin/env node

import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

import { parseSafePositiveInteger } from "./check-release-evidence-inputs.mjs";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));

export const POST_RELEASE_VALIDATION_SCHEMA = "jftrade.post-release-validation.v1";
export const TAURI_RUNTIME_SMOKE_SCHEMA = "jftrade.tauri-runtime-smoke.v1";
export const POST_RELEASE_BINDING_SCHEMA = "jftrade.post-release-binding.v1";
export const REQUIRED_PLATFORMS = Object.freeze([
  "macos-arm64",
  "linux-x64",
  "windows-x64",
  "windows-arm64",
]);

const canonicalTargets = Object.freeze({
  "macos-arm64": Object.freeze([
    Object.freeze({ platform: "macos-arm64", architecture: "arm64" }),
    Object.freeze({ platform: "darwin", architecture: "arm64" }),
  ]),
  "linux-x64": Object.freeze([
    Object.freeze({ platform: "linux-x64", architecture: "amd64" }),
    Object.freeze({ platform: "linux", architecture: "amd64" }),
    Object.freeze({ platform: "linux", architecture: "x64" }),
  ]),
  "windows-x64": Object.freeze([
    Object.freeze({ platform: "windows-x64", architecture: "amd64" }),
    Object.freeze({ platform: "windows", architecture: "amd64" }),
    Object.freeze({ platform: "windows", architecture: "x64" }),
  ]),
  "windows-arm64": Object.freeze([
    Object.freeze({ platform: "windows-arm64", architecture: "arm64" }),
    Object.freeze({ platform: "windows", architecture: "arm64" }),
  ]),
});

const requiredScope = Object.freeze([
  "packaged runtime resource presence and startup integrity validation",
  "unauthenticated API fail-closed response",
  "startup and graceful shutdown with retained child cleanup",
]);

const bindingKeys = Object.freeze([
  "schemaVersion",
  "releaseTag",
  "releaseRef",
  "commitSha",
  "releaseRun",
  "artifacts",
]);

function isRecord(value) {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function nonEmptyString(value, label, errors) {
  if (typeof value !== "string" || value.trim() === "") {
    errors.push(`${label} must be a non-empty string`);
    return null;
  }
  return value.trim();
}

function nonNegativeNumber(value, label, errors) {
  if (typeof value !== "number" || !Number.isFinite(value) || value < 0) {
    errors.push(`${label} must be a finite non-negative number`);
    return null;
  }
  return value;
}

function validCommit(value) {
  return typeof value === "string" && /^(?:[a-f0-9]{40}|[a-f0-9]{64})$/.test(value.trim());
}

function validDigest(value) {
  return typeof value === "string" && /^[a-f0-9]{64}$/.test(value.trim().toLowerCase());
}

function positiveRunId(value, label, errors) {
  const parsed = parseSafePositiveInteger(value);
  if (parsed !== null) return parsed;
  errors.push(`${label} must be a positive workflow run id`);
  return null;
}

function positiveAttempt(value, label, errors) {
  const parsed = parseSafePositiveInteger(value);
  if (parsed !== null) return parsed;
  errors.push(`${label} must be a positive workflow run attempt`);
  return null;
}

function normalizeDigestEntries(value, label, errors) {
  if (!Array.isArray(value) || value.length === 0) {
    errors.push(`${label} must be a non-empty artifact digest array`);
    return [];
  }
  const seen = new Set();
  const entries = [];
  for (const [index, item] of value.entries()) {
    const itemLabel = `${label}[${index}]`;
    if (!isRecord(item)) {
      errors.push(`${itemLabel} must be an object`);
      continue;
    }
    for (const key of Object.keys(item)) {
      if (!["path", "sha256"].includes(key)) errors.push(`${itemLabel}.${key} is not allowed`);
    }
    const artifactPath = nonEmptyString(item.path, `${itemLabel}.path`, errors);
    const digest = item.sha256;
    if (!artifactPath || path.isAbsolute(artifactPath) || artifactPath.split(/[\\/]/).includes("..")) {
      errors.push(`${itemLabel}.path must be a relative artifact path without traversal`);
    }
    if (!validDigest(digest)) errors.push(`${itemLabel}.sha256 must be a lowercase SHA-256 digest`);
    const normalizedPath = artifactPath?.replaceAll("\\", "/");
    if (normalizedPath && seen.has(normalizedPath)) errors.push(`${label} contains duplicate artifact path: ${normalizedPath}`);
    if (normalizedPath) seen.add(normalizedPath);
    entries.push({ path: normalizedPath ?? null, sha256: validDigest(digest) ? digest.toLowerCase() : null });
  }
  return entries;
}

function normalizeReleaseRun(value, label, errors) {
  if (!isRecord(value)) {
    errors.push(`${label} must be an object`);
    return null;
  }
  const allowed = ["id", "attempt", "workflow", "ref", "commitSha", "url"];
  for (const key of Object.keys(value)) {
    if (!allowed.includes(key)) errors.push(`${label}.${key} is not allowed`);
  }
  const id = positiveRunId(value.id, `${label}.id`, errors);
  const attempt = positiveAttempt(value.attempt, `${label}.attempt`, errors);
  const workflow = nonEmptyString(value.workflow, `${label}.workflow`, errors);
  const ref = nonEmptyString(value.ref, `${label}.ref`, errors);
  const commitSha = validCommit(value.commitSha)
    ? value.commitSha.trim()
    : (errors.push(`${label}.commitSha must be a 40 or 64 character lowercase commit SHA`), null);
  if (value.url !== undefined) {
    try {
      const url = new URL(nonEmptyString(value.url, `${label}.url`, errors) ?? "");
      if (url.protocol !== "https:" || !url.hostname || url.username || url.password) {
        errors.push(`${label}.url must be an HTTPS URL without credentials`);
      }
    } catch {
      errors.push(`${label}.url must be a valid HTTPS URL`);
    }
  }
  return {
    id,
    attempt,
    workflow,
    ref,
    commitSha,
    ...(typeof value.url === "string" && value.url.trim() ? { url: value.url.trim() } : {}),
  };
}

function normalizeReleaseBinding(value, label, errors) {
  if (!isRecord(value)) {
    errors.push(`${label} must be an object`);
    return null;
  }
  for (const key of Object.keys(value)) {
    if (!bindingKeys.includes(key)) errors.push(`${label}.${key} is not allowed`);
  }
  const schemaVersion = value.schemaVersion;
  if (schemaVersion !== POST_RELEASE_BINDING_SCHEMA) {
    errors.push(`${label}.schemaVersion must be ${POST_RELEASE_BINDING_SCHEMA}`);
  }
  const releaseTag = nonEmptyString(value.releaseTag, `${label}.releaseTag`, errors);
  if (releaseTag && !/^v\d+\.\d+\.\d+$/.test(releaseTag)) errors.push(`${label}.releaseTag must be a vX.Y.Z release tag`);
  const releaseRef = nonEmptyString(value.releaseRef, `${label}.releaseRef`, errors);
  if (releaseRef && releaseTag && releaseRef !== `refs/tags/${releaseTag}`) {
    errors.push(`${label}.releaseRef must match releaseTag for a tag ref`);
  }
  const commitSha = validCommit(value.commitSha)
    ? value.commitSha.trim()
    : (errors.push(`${label}.commitSha must be a 40 or 64 character lowercase commit SHA`), null);
  const releaseRun = normalizeReleaseRun(value.releaseRun, `${label}.releaseRun`, errors);
  if (releaseRun && releaseRef && releaseRun.ref !== releaseRef) errors.push(`${label}.releaseRun.ref does not match releaseRef`);
  if (releaseRun && commitSha && releaseRun.commitSha !== commitSha) errors.push(`${label}.releaseRun.commitSha does not match commitSha`);
  const artifacts = normalizeDigestEntries(value.artifacts, `${label}.artifacts`, errors);
  return { schemaVersion, releaseTag, releaseRef, commitSha, releaseRun, artifacts };
}

function artifactSetsEqual(left, right) {
  if (!left || !right || left.length !== right.length) return false;
  const normalize = (entries) => entries
    .map((entry) => `${entry.path}\u0000${entry.sha256}`)
    .sort();
  return normalize(left).every((entry, index) => entry === normalize(right)[index]);
}

function bindingsEqual(left, right) {
  return left && right && left.releaseRun && right.releaseRun
    && left.releaseTag === right.releaseTag
    && left.releaseRef === right.releaseRef
    && left.commitSha === right.commitSha
    && left.releaseRun.id === right.releaseRun.id
    && left.releaseRun.attempt === right.releaseRun.attempt
    && left.releaseRun.workflow === right.releaseRun.workflow
    && left.releaseRun.ref === right.releaseRun.ref
    && left.releaseRun.commitSha === right.releaseRun.commitSha
    && artifactSetsEqual(left.artifacts, right.artifacts);
}

function readJson(filePath, label) {
  let content;
  try {
    content = fs.readFileSync(filePath, "utf8");
  } catch (error) {
    throw new Error(`cannot read ${label} ${filePath}: ${error.message}`);
  }
  try {
    return JSON.parse(content);
  } catch (error) {
    throw new Error(`cannot parse ${label} ${filePath}: ${error.message}`);
  }
}

function canonicalPlatform(target, label, errors) {
  if (!isRecord(target)) {
    errors.push(`${label} must be an object`);
    return null;
  }
  const platform = nonEmptyString(target.platform, `${label}.platform`, errors);
  const architecture = nonEmptyString(target.architecture, `${label}.architecture`, errors);
  if (!platform || !architecture) return null;
  for (const [canonical, aliases] of Object.entries(canonicalTargets)) {
    if (aliases.some((alias) => alias.platform === platform && alias.architecture === architecture)) {
      return canonical;
    }
  }
  errors.push(`${label} is not a supported Tauri smoke target: ${platform}/${architecture}`);
  return null;
}

function expectedBinding(options = {}, errors) {
  const source = options.expectedBinding ?? options.expected ?? options.binding;
  if (source === undefined || source === null) return null;
  if (!isRecord(source)) {
    errors.push("expected release binding must be an object");
    return null;
  }
  const artifacts = source.artifacts ?? source.artifactDigests;
  const { artifactDigests: _artifactDigests, ...bindingSource } = source;
  const releaseRun = source.releaseRun
    ? {
      ...source.releaseRun,
      ref: source.releaseRun.ref ?? source.releaseRef,
      commitSha: source.releaseRun.commitSha ?? source.commitSha,
    }
    : source.releaseRun;
  return normalizeReleaseBinding(
    {
      ...bindingSource,
      schemaVersion: source.schemaVersion ?? POST_RELEASE_BINDING_SCHEMA,
      releaseRun,
      artifacts,
    },
    "expected.releaseBinding",
    errors,
  );
}

function readArtifactDigestBinding(filePath) {
  const value = readJson(filePath, "expected release artifact digest manifest");
  const source = isRecord(value) && isRecord(value.releaseBinding) ? value.releaseBinding : value;
  if (!isRecord(source)) throw new Error("expected release artifact digest manifest must be an object");
  const hasReleaseMetadata = ["releaseTag", "releaseRef", "commitSha", "releaseRun"]
    .some((key) => key in source);
  if (!hasReleaseMetadata) {
    const errors = [];
    const artifacts = normalizeDigestEntries(
      source.artifacts ?? source.artifactDigests,
      "expected.releaseBinding.artifacts",
      errors,
    );
    if (errors.length > 0) throw new Error(errors.join("; "));
    return { schemaVersion: POST_RELEASE_BINDING_SCHEMA, artifacts };
  }
  const errors = [];
  const binding = normalizeReleaseBinding(
    { ...source, schemaVersion: source.schemaVersion ?? POST_RELEASE_BINDING_SCHEMA },
    "expected.releaseBinding",
    errors,
  );
  if (errors.length > 0) throw new Error(errors.join("; "));
  return binding;
}

function validateRuntimeSmokeReport(report, label) {
  const errors = [];
  if (!isRecord(report)) {
    return { valid: false, errors: [`${label} must be an object`] };
  }
  if (report.schemaVersion !== TAURI_RUNTIME_SMOKE_SCHEMA) {
    errors.push(`${label}.schemaVersion must be ${TAURI_RUNTIME_SMOKE_SCHEMA}`);
  }
  const releaseBinding = normalizeReleaseBinding(
    report.releaseBinding ?? report.binding,
    `${label}.releaseBinding`,
    errors,
  );
  const platform = canonicalPlatform(report.target, `${label}.target`, errors);
  const executable = nonEmptyString(report.executable, `${label}.executable`, errors);
  if (executable && !path.basename(executable).startsWith("jftrade-desktop")) {
    errors.push(`${label}.executable must identify the jftrade-desktop binary`);
  }

  if (!Array.isArray(report.scope) || report.scope.length === 0) {
    errors.push(`${label}.scope must be a non-empty string array`);
  } else {
    for (const item of requiredScope) {
      if (!report.scope.includes(item)) errors.push(`${label}.scope is missing ${JSON.stringify(item)}`);
    }
  }

  const readiness = report.readiness;
  if (!isRecord(readiness)) {
    errors.push(`${label}.readiness must be an object`);
  } else {
    if (readiness.status !== 401) errors.push(`${label}.readiness.status must be 401`);
    if (readiness.errorCode !== "WEB_AUTH_REQUIRED") {
      errors.push(`${label}.readiness.errorCode must be WEB_AUTH_REQUIRED`);
    }
    nonNegativeNumber(readiness.readyMs, `${label}.readiness.readyMs`, errors);
  }

  const shutdown = report.shutdown;
  if (!isRecord(shutdown)) {
    errors.push(`${label}.shutdown must be an object`);
  } else {
    if (shutdown.code !== 0) errors.push(`${label}.shutdown.code must be 0`);
    nonNegativeNumber(shutdown.shutdownMs, `${label}.shutdown.shutdownMs`, errors);
  }

  if (report.orphanCheck !== "passed" && report.orphanCheck !== "not-applicable-on-windows") {
    errors.push(`${label}.orphanCheck must be passed or not-applicable-on-windows`);
  } else if (platform && platform.startsWith("windows-") && report.orphanCheck !== "not-applicable-on-windows") {
    errors.push(`${label}.orphanCheck must be not-applicable-on-windows for Windows`);
  } else if (platform && !platform.startsWith("windows-") && report.orphanCheck !== "passed") {
    errors.push(`${label}.orphanCheck must be passed for non-Windows targets`);
  }

  if (!Array.isArray(report.externalRequired) || report.externalRequired.length === 0) {
    errors.push(`${label}.externalRequired must be a non-empty string array`);
  } else {
    const externalText = report.externalRequired.join(" ").toLowerCase();
    if (!/(install|upgrade|uninstall|rollback)/i.test(externalText)) {
      errors.push(`${label}.externalRequired must retain native lifecycle limitations`);
    }
    if (!/(sign|notari)/i.test(externalText)) {
      errors.push(`${label}.externalRequired must retain signing limitations`);
    }
  }

  return {
    valid: errors.length === 0,
    errors,
    platform,
    target: report.target,
    executable,
    readiness: readiness && { status: readiness.status, errorCode: readiness.errorCode, readyMs: readiness.readyMs },
    shutdown: shutdown && { code: shutdown.code, signal: shutdown.signal ?? null, shutdownMs: shutdown.shutdownMs },
    orphanCheck: report.orphanCheck,
    releaseBinding,
  };
}

function reportPath(pathValue) {
  if (!pathValue) return null;
  const absolute = path.resolve(pathValue);
  const relative = path.relative(repositoryRoot, absolute);
  return relative && !relative.startsWith(`..${path.sep}`) && relative !== ".."
    ? relative.split(path.sep).join("/")
    : absolute;
}

function reportEntry(value, index) {
  if (typeof value === "string") {
    const absolute = path.resolve(value);
    return { path: reportPath(absolute), report: readJson(absolute, `post-release smoke report[${index}]`) };
  }
  if (isRecord(value) && "schemaVersion" in value) {
    return { path: null, report: value };
  }
  if (!isRecord(value) || !isRecord(value.report)) {
    throw new Error(`post-release smoke report[${index}] must be a path or { report, path } object`);
  }
  return { path: reportPath(value.path), report: value.report };
}

/**
 * Validate locally produced Tauri runtime smoke reports for all four targets.
 * This proves input structure and completeness only; it never qualifies a
 * published release or changes source readiness evidence.
 */
export function inspectPostReleaseSmokeReports(options = {}) {
  const { reports = [], reportPaths = [] } = options;
  const errors = [];
  const entries = [];
  const values = [...reports, ...reportPaths];
  if (values.length === 0) errors.push("at least one post-release smoke report is required");
  const expected = expectedBinding(options, errors);

  const seen = new Set();
  let commonBinding = null;
  for (const [index, value] of values.entries()) {
    try {
      const entry = reportEntry(value, index);
      const validated = validateRuntimeSmokeReport(entry.report, `report[${index}]`);
      errors.push(...validated.errors);
      if (validated.releaseBinding) {
        if (!commonBinding) commonBinding = validated.releaseBinding;
        else if (!bindingsEqual(commonBinding, validated.releaseBinding)) {
          errors.push(`report[${index}].releaseBinding does not match the other post-release reports`);
        }
        if (expected && !bindingsEqual(expected, validated.releaseBinding)) {
          errors.push(`report[${index}].releaseBinding does not match expected release binding`);
        }
      }
      if (validated.platform && seen.has(validated.platform)) {
        errors.push(`duplicate post-release smoke target: ${validated.platform}`);
      }
      if (validated.platform) seen.add(validated.platform);
      entries.push({
        path: entry.path,
        platform: validated.platform,
        target: validated.target,
        valid: validated.valid,
        readiness: validated.readiness,
        shutdown: validated.shutdown,
        orphanCheck: validated.orphanCheck,
        releaseBinding: validated.releaseBinding,
      });
    } catch (error) {
      errors.push(error instanceof Error ? error.message : String(error));
    }
  }

  const missingPlatforms = REQUIRED_PLATFORMS.filter((platform) => !seen.has(platform));
  if (missingPlatforms.length > 0) {
    errors.push(`missing post-release smoke target(s): ${missingPlatforms.join(", ")}`);
  }
  if (expected && commonBinding && !bindingsEqual(expected, commonBinding)) {
    errors.push("post-release smoke reports do not match expected release binding");
  }
  const valid = errors.length === 0;
  return {
    schemaVersion: POST_RELEASE_VALIDATION_SCHEMA,
    status: valid ? "inputs_verified" : "incomplete_inputs",
    valid,
    releaseQualified: false,
    releaseQualification: "external_post_release_observation_required",
    releaseBinding: commonBinding,
    artifactDigests: commonBinding?.artifacts ?? [],
    platforms: entries,
    missingPlatforms,
    errors,
    limitations: [
      "Only the structure, target mapping and completeness of repository-produced Tauri runtime smoke reports are validated.",
      "This report does not prove that smoke ran after publication or verify signed artifacts, native install, upgrade, uninstall or rollback.",
      "Matching release runners must provide independently retained post-release observations.",
      "This checker writes an independent post-release validation receipt and never changes source readiness evidence.",
    ],
  };
}

export function writePostReleaseSmokeReport(outputPath, options = {}) {
  if (!outputPath) throw new Error("outputPath is required");
  const result = inspectPostReleaseSmokeReports(options);
  const absolute = path.resolve(outputPath);
  fs.mkdirSync(path.dirname(absolute), { recursive: true });
  fs.writeFileSync(absolute, `${JSON.stringify(result, null, 2)}\n`, "utf8");
  return result;
}

function parseArgs(args) {
  const reportPaths = [];
  let outputPath;
  const expected = {};
  let expectedArtifactDigests;
  for (let index = 0; index < args.length; index += 1) {
    const argument = args[index];
    if (argument === "--report") {
      const value = args[++index];
      if (!value || value.startsWith("--")) throw new Error("--report requires a path");
      reportPaths.push(value);
    } else if (argument.startsWith("--report=")) {
      const value = argument.slice("--report=".length);
      if (!value) throw new Error("--report requires a path");
      reportPaths.push(value);
    } else if (argument === "--output") {
      outputPath = args[++index];
      if (!outputPath || outputPath.startsWith("--")) throw new Error("--output requires a path");
    } else if (argument.startsWith("--output=")) {
      outputPath = argument.slice("--output=".length);
      if (!outputPath) throw new Error("--output requires a path");
    } else if (argument === "--expected-tag") {
      expected.releaseTag = args[++index];
      if (!expected.releaseTag || expected.releaseTag.startsWith("--")) throw new Error("--expected-tag requires a value");
    } else if (argument.startsWith("--expected-tag=")) {
      expected.releaseTag = argument.slice("--expected-tag=".length);
    } else if (argument === "--expected-ref") {
      expected.releaseRef = args[++index];
      if (!expected.releaseRef || expected.releaseRef.startsWith("--")) throw new Error("--expected-ref requires a value");
    } else if (argument.startsWith("--expected-ref=")) {
      expected.releaseRef = argument.slice("--expected-ref=".length);
    } else if (argument === "--expected-commit") {
      expected.commitSha = args[++index];
      if (!expected.commitSha || expected.commitSha.startsWith("--")) throw new Error("--expected-commit requires a value");
    } else if (argument.startsWith("--expected-commit=")) {
      expected.commitSha = argument.slice("--expected-commit=".length);
    } else if (argument === "--expected-run-id") {
      expected.releaseRun = { ...(expected.releaseRun ?? {}), id: args[++index] };
      if (!expected.releaseRun.id || expected.releaseRun.id.startsWith("--")) throw new Error("--expected-run-id requires a value");
    } else if (argument.startsWith("--expected-run-id=")) {
      expected.releaseRun = { ...(expected.releaseRun ?? {}), id: argument.slice("--expected-run-id=".length) };
    } else if (argument === "--expected-run-attempt" || argument === "--expected-attempt") {
      expected.releaseRun = { ...(expected.releaseRun ?? {}), attempt: args[++index] };
      if (!expected.releaseRun.attempt || expected.releaseRun.attempt.startsWith("--")) throw new Error(`${argument} requires a value`);
    } else if (argument.startsWith("--expected-run-attempt=") || argument.startsWith("--expected-attempt=")) {
      const prefix = argument.startsWith("--expected-run-attempt=")
        ? "--expected-run-attempt="
        : "--expected-attempt=";
      expected.releaseRun = { ...(expected.releaseRun ?? {}), attempt: argument.slice(prefix.length) };
    } else if (argument === "--expected-workflow") {
      expected.releaseRun = { ...(expected.releaseRun ?? {}), workflow: args[++index] };
      if (!expected.releaseRun.workflow || expected.releaseRun.workflow.startsWith("--")) throw new Error("--expected-workflow requires a value");
    } else if (argument.startsWith("--expected-workflow=")) {
      expected.releaseRun = { ...(expected.releaseRun ?? {}), workflow: argument.slice("--expected-workflow=".length) };
    } else if (argument === "--expected-artifact-digests") {
      expectedArtifactDigests = args[++index];
      if (!expectedArtifactDigests || expectedArtifactDigests.startsWith("--")) throw new Error("--expected-artifact-digests requires a path");
    } else if (argument.startsWith("--expected-artifact-digests=")) {
      expectedArtifactDigests = argument.slice("--expected-artifact-digests=".length);
      if (!expectedArtifactDigests) throw new Error("--expected-artifact-digests requires a path");
    } else {
      throw new Error(`unknown argument: ${argument}`);
    }
  }
  return {
    reportPaths,
    outputPath,
    expected: Object.keys(expected).length > 0 ? expected : undefined,
    expectedArtifactDigests,
  };
}

export function main(args = process.argv.slice(2)) {
  try {
    const options = parseArgs(args);
    let expected = options.expected;
    if (options.expectedArtifactDigests) {
      const manifestBinding = readArtifactDigestBinding(path.resolve(options.expectedArtifactDigests));
      if (expected) {
        const expectedWithRun = {
          ...manifestBinding,
          ...expected,
          releaseRun: {
            ...manifestBinding.releaseRun,
            ...expected.releaseRun,
            ref: expected.releaseRun?.ref ?? expected.releaseRef ?? manifestBinding.releaseRun?.ref,
            commitSha: expected.releaseRun?.commitSha ?? expected.commitSha ?? manifestBinding.releaseRun?.commitSha,
          },
          artifacts: manifestBinding.artifacts,
        };
        const errors = [];
        const normalized = normalizeReleaseBinding(
          expectedWithRun,
          "expected.releaseBinding",
          errors,
        );
        if (errors.length > 0) throw new Error(errors.join("; "));
        if (manifestBinding.releaseTag !== undefined && !bindingsEqual(manifestBinding, normalized)) {
          throw new Error("expected artifact digest manifest does not match expected release binding");
        }
        expected = expectedWithRun;
      } else {
        expected = manifestBinding;
      }
    }
    const result = options.outputPath
      ? writePostReleaseSmokeReport(options.outputPath, { ...options, expected })
      : inspectPostReleaseSmokeReports({ ...options, expected });
    console.log(JSON.stringify(result, null, 2));
    return result.valid ? 0 : 1;
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    return 1;
  }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  process.exitCode = main();
}
