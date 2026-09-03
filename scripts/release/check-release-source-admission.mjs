#!/usr/bin/env node
import { execFileSync, spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));
export const RELEASE_SOURCE_ADMISSION_SCHEMA = "jftrade.release-source-admission.v1";

function readJson(root, relativePath) {
  return JSON.parse(fs.readFileSync(path.join(root, relativePath), "utf8"));
}

function readToolchain(root) {
  const source = fs.readFileSync(path.join(root, "rust-toolchain.toml"), "utf8");
  return source.match(/channel\s*=\s*"([^"]+)"/)?.[1] ?? "";
}

export function validateReleaseConfiguration(root = repositoryRoot) {
  const errors = [];
  const packageManifest = readJson(root, "package.json");
  const tauriConfig = readJson(root, "apps/desktop/src-tauri/tauri.conf.json");
  if (packageManifest.packageManager !== "pnpm@11.21.0") errors.push("packageManager must be pnpm@11.21.0");
  if (readToolchain(root) !== "1.97.1") errors.push("Rust toolchain must be 1.97.1");
  if (tauriConfig.bundle?.active !== true) errors.push("Tauri bundle must remain active");
  if (tauriConfig.bundle?.createUpdaterArtifacts !== true) errors.push("Tauri updater artifacts must remain enabled");
  if (!fs.existsSync(path.join(root, "Cargo.lock"))) errors.push("Cargo.lock is required");
  return errors;
}

function runGate(root, script, runner = spawnSync) {
  const result = runner(process.execPath, [path.join(root, script)], {
    cwd: root,
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"],
  });
  if (result.error) return result.error.message;
  if (result.status !== 0) return (result.stderr || result.stdout || `${script} failed`).trim();
  return null;
}

export function evaluateReleaseSource({
  root = repositoryRoot,
  sourceRef,
  commitSha,
  plannedReleaseTag,
  ciStatus,
  ciUrl,
  runChecks = true,
  runner,
} = {}) {
  const errors = [];
  if (!/^refs\/heads\/[A-Za-z0-9._/-]+$/.test(sourceRef ?? "")) errors.push("sourceRef must be an exact branch ref");
  if (!/^[a-f0-9]{40}$/.test(commitSha ?? "")) errors.push("commitSha must be a 40-character lowercase SHA");
  if (!/^v(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)$/.test(plannedReleaseTag ?? "") || plannedReleaseTag === "v0.0.0") {
    errors.push("plannedReleaseTag must be a non-zero vX.Y.Z tag");
  }
  if (ciStatus !== "success") errors.push("Build & Test must be verified successful for the exact commit");
  if (ciUrl !== undefined && (typeof ciUrl !== "string" || !/^https:\/\/github\.com\//.test(ciUrl))) errors.push("ciUrl must be a GitHub Actions URL");
  errors.push(...validateReleaseConfiguration(root));
  if (runChecks) {
    for (const script of ["scripts/check-zero-go.mjs", "scripts/quality/check-contracts.mjs"]) {
      const failure = runGate(root, script, runner);
      if (failure) errors.push(`${script}: ${failure}`);
    }
  }
  return {
    $schema: "./release-source-admission.schema.json",
    schemaVersion: RELEASE_SOURCE_ADMISSION_SCHEMA,
    status: errors.length === 0 ? "admitted" : "blocked",
    releaseQualified: false,
    sourceRef: sourceRef ?? null,
    plannedReleaseTag: plannedReleaseTag ?? null,
    commitSha: commitSha ?? null,
    requiredCheck: { name: "Build & Test", status: ciStatus ?? "not_verified", url: ciUrl ?? null },
    checks: {
      zeroGo: errors.some((error) => error.startsWith("scripts/check-zero-go")) ? "failed" : "passed",
      contracts: errors.some((error) => error.startsWith("scripts/quality/check-contracts")) ? "failed" : "passed",
      versionConfiguration: errors.some((error) => /packageManager|toolchain|Tauri|Cargo\.lock/.test(error)) ? "failed" : "passed",
    },
    errors,
    limitations: [
      "Source admission does not authorize a tag or publication.",
      "Signing, notarization, updater, platform lifecycle, provenance, backup recovery and security approval are evaluated by candidate evidence.",
    ],
  };
}

function parseArgs(args) {
  const parsed = {};
  for (let index = 0; index < args.length; index += 1) {
    const argument = args[index];
    if (argument === "--source-ref") parsed.sourceRef = args[++index];
    else if (argument === "--commit-sha") parsed.commitSha = args[++index];
    else if (argument === "--planned-release-tag") parsed.plannedReleaseTag = args[++index];
    else if (argument === "--ci-status") parsed.ciStatus = args[++index];
    else if (argument === "--ci-url") parsed.ciUrl = args[++index];
    else if (argument === "--repository-only") parsed.repositoryOnly = true;
    else if (argument === "--candidate-static") parsed.legacyAlias = true;
    else throw new Error(`unknown argument: ${argument}`);
  }
  return parsed;
}

function gitValue(root, args) {
  return execFileSync("git", args, { cwd: root, encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] }).trim();
}

export function main(args = process.argv.slice(2), options = {}) {
  try {
    const parsed = parseArgs(args);
    const root = options.root ?? repositoryRoot;
    const sourceRef = parsed.sourceRef ?? process.env.JFTRADE_SOURCE_REF
      ?? `refs/heads/${gitValue(root, ["branch", "--show-current"])}`;
    const commitSha = parsed.commitSha ?? process.env.JFTRADE_SOURCE_COMMIT ?? gitValue(root, ["rev-parse", "HEAD"]);
    const plannedReleaseTag = parsed.plannedReleaseTag ?? process.env.JFTRADE_DESKTOP_RELEASE_TAG;
    const ciStatus = parsed.repositoryOnly ? "success" : (parsed.ciStatus ?? process.env.JFTRADE_SOURCE_CI_STATUS);
    const ciUrl = parsed.ciUrl ?? process.env.JFTRADE_SOURCE_CI_URL;
    const result = evaluateReleaseSource({
      root, sourceRef, commitSha, plannedReleaseTag, ciStatus, ciUrl,
      runChecks: options.runChecks ?? true, runner: options.runner,
    });
    if (parsed.repositoryOnly) {
      result.status = result.errors.length === 0 ? "repository_checks_passed" : "blocked";
      result.requiredCheck.status = "not_verified_repository_only";
      result.releaseQualified = false;
    }
    console.log(JSON.stringify(result, null, 2));
    return result.errors.length === 0 ? 0 : 1;
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    return 1;
  }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) process.exitCode = main();
