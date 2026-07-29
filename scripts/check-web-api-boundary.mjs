#!/usr/bin/env node

import { readFileSync, readdirSync, statSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, relative, resolve } from "node:path";

const directFetch = /\bfetch\s*\(/g;
const manualEnvelope = /\bfetchEnvelope(?:WithInit)?\b/g;
const publicManualEnvelope = /\bexport\b[^\n]*\bfetchEnvelope(?:WithInit)?\b/g;
const sourceExtensions = new Set([".ts", ".tsx", ".vue", ".js", ".jsx"]);

export function directFetchViolations(
  sourceRoot,
  allowedRelativePaths = ["composables/shared/apiClient.ts"],
) {
  return identifierViolations(sourceRoot, directFetch, allowedRelativePaths);
}

export function manualEnvelopeViolations(
  sourceRoot,
  allowedRelativePaths = ["composables/shared/apiClient.ts"],
) {
  return identifierViolations(sourceRoot, manualEnvelope, allowedRelativePaths);
}

export function publicManualEnvelopeViolations(sourceRoot) {
  return identifierViolations(sourceRoot, publicManualEnvelope, []);
}

function identifierViolations(sourceRoot, pattern, allowedRelativePaths) {
  const allowed = new Set(allowedRelativePaths.map(normalizePath));
  const violations = [];
  for (const path of walkSourceFiles(sourceRoot)) {
    const relativePath = normalizePath(relative(sourceRoot, path));
    if (allowed.has(relativePath)) {
      continue;
    }
    const source = readFileSync(path, "utf8");
    pattern.lastIndex = 0;
    for (let match = pattern.exec(source); match != null; match = pattern.exec(source)) {
      violations.push({
        path: relativePath,
        line: source.slice(0, match.index).split("\n").length,
      });
    }
  }
  return violations;
}

function walkSourceFiles(root) {
  const files = [];
  for (const entry of readdirSync(root)) {
    const path = resolve(root, entry);
    const stat = statSync(path);
    if (stat.isDirectory()) {
      files.push(...walkSourceFiles(path));
      continue;
    }
    const extension = entry.slice(entry.lastIndexOf("."));
    if (sourceExtensions.has(extension)) {
      files.push(path);
    }
  }
  return files;
}

function normalizePath(path) {
  return path.replaceAll("\\", "/");
}

function main() {
  const scriptDir = dirname(fileURLToPath(import.meta.url));
  const repoRoot = resolve(scriptDir, "..");
  const sourceRoot = resolve(repoRoot, "apps/web/src");
  const directViolations = directFetchViolations(sourceRoot);
  const envelopeViolations = manualEnvelopeViolations(sourceRoot);
  const publicEnvelopeViolations = publicManualEnvelopeViolations(sourceRoot);
  if (
    directViolations.length === 0 &&
    envelopeViolations.length === 0 &&
    publicEnvelopeViolations.length === 0
  ) {
    console.log(
      "Web API boundary passed: fetch is isolated and business calls use generated operation types.",
    );
    return;
  }
  if (directViolations.length > 0) {
    console.error("Direct fetch calls must use the authenticated apiClient boundary:");
    for (const violation of directViolations) {
      console.error(`- apps/web/src/${violation.path}:${violation.line}`);
    }
  }
  if (envelopeViolations.length > 0) {
    console.error("Business calls must use typed apiGet/apiPost/... operation helpers:");
    for (const violation of envelopeViolations) {
      console.error(`- apps/web/src/${violation.path}:${violation.line}`);
    }
  }
  if (publicEnvelopeViolations.length > 0) {
    console.error("The shared client must not export caller-selected envelope generics:");
    for (const violation of publicEnvelopeViolations) {
      console.error(`- apps/web/src/${violation.path}:${violation.line}`);
    }
  }
  process.exitCode = 1;
}

if (resolve(process.argv[1] || "") === fileURLToPath(import.meta.url)) {
  main();
}
