#!/usr/bin/env node
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

const repositoryRoot = fileURLToPath(new URL("..", import.meta.url));
const baseline = JSON.parse(
  fs.readFileSync(path.join(repositoryRoot, "scripts/go-retirement-baseline.json"), "utf8"),
);

const activePattern = /\bgo\s+(?:run|build|test|generate|vet)\b|actions\/setup-go|\.github\/actions\/setup-go|cmd\/jftrade-api|@wailsio\/runtime|\bwails(?:3)?\s+(?:build|dev|generate)\b/giu;

function git(args, options = {}) {
  return execFileSync("git", args, {
    cwd: repositoryRoot,
    encoding: options.encoding ?? "utf8",
    maxBuffer: 128 * 1024 * 1024,
    stdio: ["ignore", "pipe", "pipe"],
  });
}

function trackedFiles(tree = null) {
  const args = tree
    ? ["ls-tree", "-r", "--name-only", tree]
    : ["ls-files"];
  return git(args).split(/\r?\n/u).filter(Boolean);
}

function isGoArtifact(file) {
  return file.endsWith(".go") || /(^|\/)go\.(?:mod|sum|work|work\.sum)$/u.test(file);
}

function isUnderActiveRoot(file) {
  if (file === "scripts/check-go-retirement.test.mjs") return false;
  return baseline.activeConfigurationRoots.some((root) => file === root || file.startsWith(`${root}/`));
}

function readTreeFile(tree, file) {
  return git(["show", `${tree}:${file}`]);
}

function activeSignatures(files, readFile) {
  const signatures = new Map();
  for (const file of files.filter(isUnderActiveRoot)) {
    let source;
    try {
      source = readFile(file);
    } catch {
      continue;
    }
    for (const line of source.split(/\r?\n/u)) {
      activePattern.lastIndex = 0;
      if (!activePattern.test(line)) continue;
      const signature = `${file}\0${line.trim()}`;
      signatures.set(signature, (signatures.get(signature) ?? 0) + 1);
    }
  }
  return signatures;
}

export function validateGoRetirement({ baselineFiles, currentFiles, baselineSignatures, currentSignatures }) {
  const errors = [];
  const allowedArtifacts = new Set(baselineFiles.filter(isGoArtifact));
  for (const file of currentFiles.filter(isGoArtifact)) {
    if (!allowedArtifacts.has(file)) errors.push(`new or moved Go artifact: ${file}`);
  }
  for (const [signature, count] of currentSignatures) {
    const allowed = baselineSignatures.get(signature) ?? 0;
    if (count > allowed) {
      const [file, line] = signature.split("\0");
      errors.push(`new active Go/Wails configuration in ${file}: ${line}`);
    }
  }
  return errors;
}

export function checkGoRetirement() {
  assert.match(baseline.baselineCommit, /^[0-9a-f]{40}$/u, "baselineCommit must be an immutable commit");
  git(["cat-file", "-e", `${baseline.baselineCommit}^{commit}`]);
  const baselineFiles = trackedFiles(baseline.baselineCommit);
  const currentFiles = trackedFiles();
  const errors = validateGoRetirement({
    baselineFiles,
    currentFiles,
    baselineSignatures: activeSignatures(baselineFiles, (file) => readTreeFile(baseline.baselineCommit, file)),
    currentSignatures: activeSignatures(currentFiles, (file) => fs.readFileSync(path.join(repositoryRoot, file), "utf8")),
  });
  if (errors.length > 0) throw new Error(`Go retirement gate failed:\n${errors.map((error) => `- ${error}`).join("\n")}`);
  const currentGoFiles = currentFiles.filter((file) => file.endsWith(".go")).length;
  const baselineGoFiles = baselineFiles.filter((file) => file.endsWith(".go")).length;
  console.log(`Go retirement gate passed: ${currentGoFiles}/${baselineGoFiles} tracked Go files remain; no Go/Wails scope grew.`);
}

if (path.resolve(process.argv[1] ?? "") === path.resolve(import.meta.filename)) {
  try {
    checkGoRetirement();
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  }
}
