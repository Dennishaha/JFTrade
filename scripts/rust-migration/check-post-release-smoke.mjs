#!/usr/bin/env node

import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));

export const POST_RELEASE_SMOKE_SCHEMA = "jftrade.post-release-smoke.v1";
export const TAURI_RUNTIME_SMOKE_SCHEMA = "jftrade.tauri-runtime-smoke.v1";
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

function validateRuntimeSmokeReport(report, label) {
  const errors = [];
  if (!isRecord(report)) {
    return { valid: false, errors: [`${label} must be an object`] };
  }
  if (report.schemaVersion !== TAURI_RUNTIME_SMOKE_SCHEMA) {
    errors.push(`${label}.schemaVersion must be ${TAURI_RUNTIME_SMOKE_SCHEMA}`);
  }
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
 * published release or changes the Stage 9 closeout manifest.
 */
export function inspectPostReleaseSmokeReports({ reports = [], reportPaths = [] } = {}) {
  const errors = [];
  const entries = [];
  const values = [...reports, ...reportPaths];
  if (values.length === 0) errors.push("at least one post-release smoke report is required");

  const seen = new Set();
  for (const [index, value] of values.entries()) {
    try {
      const entry = reportEntry(value, index);
      const validated = validateRuntimeSmokeReport(entry.report, `report[${index}]`);
      errors.push(...validated.errors);
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
      });
    } catch (error) {
      errors.push(error instanceof Error ? error.message : String(error));
    }
  }

  const missingPlatforms = REQUIRED_PLATFORMS.filter((platform) => !seen.has(platform));
  if (missingPlatforms.length > 0) {
    errors.push(`missing post-release smoke target(s): ${missingPlatforms.join(", ")}`);
  }
  const valid = errors.length === 0;
  return {
    schemaVersion: POST_RELEASE_SMOKE_SCHEMA,
    status: valid ? "inputs_verified" : "incomplete_inputs",
    valid,
    releaseQualified: false,
    releaseQualification: "external_post_release_observation_required",
    platforms: entries,
    missingPlatforms,
    errors,
    limitations: [
      "Only the structure, target mapping and completeness of repository-produced Tauri runtime smoke reports are validated.",
      "This report does not prove that smoke ran after publication or verify signed artifacts, native install, upgrade, uninstall or rollback.",
      "Matching release runners must provide independently retained post-release observations before the Stage 9 gate can be changed.",
      "This checker never changes the Stage 9 closeout manifest.",
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
    } else {
      throw new Error(`unknown argument: ${argument}`);
    }
  }
  return { reportPaths, outputPath };
}

export function main(args = process.argv.slice(2)) {
  try {
    const options = parseArgs(args);
    const result = options.outputPath
      ? writePostReleaseSmokeReport(options.outputPath, options)
      : inspectPostReleaseSmokeReports(options);
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
