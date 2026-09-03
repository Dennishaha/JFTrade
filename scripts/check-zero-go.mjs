#!/usr/bin/env node

import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import { fileURLToPath, pathToFileURL } from "node:url";

const repositoryRoot = fileURLToPath(new URL("..", import.meta.url));
const activeRoots = [
  ".github",
  "apps",
  "build-release.ps1",
  "build-release.sh",
  "crates",
  "package.json",
  "pnpm-workspace.yaml",
  "scripts",
  "workers",
];
const selfExclusions = new Set([
  "scripts/check-zero-go.mjs",
  "scripts/check-zero-go.test.mjs",
]);
const activeTextPattern = /\bgo\s+(?:run|build|test|generate|vet)\b|(?:spawnChecked|spawnSync|execFileSync|execFile|spawn)\(\s*["']go["']|actions\/setup-go|\.github\/actions\/setup-go|cmd\/jftrade-api|@wailsio\/runtime|\bwails(?:3)?\s+(?:build|dev|generate)\b|wails:\/\/|wails\.localhost|internal\/(?:frontendassets|marketdataassets|pineworkerassets)/giu;
const artifactTextPattern = /github\.com\/wailsapp|@wailsio\/runtime|pkg:golang|golang\.org\/|\bWails(?:Runtime|App)?\b/giu;
const goBuildInfoMagic = Buffer.from([0xff, 0x20, 0x47, 0x6f, 0x20, 0x62, 0x75, 0x69, 0x6c, 0x64, 0x69, 0x6e, 0x66, 0x3a]);

function normalize(file) {
  return file.split(path.sep).join("/").replace(/^\.\//u, "");
}

function isGoArtifact(file) {
  return file.endsWith(".go") || /(^|\/)go\.(?:mod|sum|work|work\.sum)$/u.test(file);
}

function isActive(file) {
  if (selfExclusions.has(file)) return false;
  return activeRoots.some((root) => file === root || file.startsWith(`${root}/`));
}

function looksTextual(file) {
  return /(?:^|\.)(?:json|jsonl|lock|md|mjs|js|cjs|ts|tsx|vue|yml|yaml|toml|xml|txt|spdx)$/iu.test(file)
    || /(?:sbom|bom|manifest|notice|license)/iu.test(path.basename(file));
}

export function validateSourceInventory(files, readText) {
  const errors = [];
  for (const rawFile of files) {
    const file = normalize(rawFile);
    if (isGoArtifact(file)) errors.push(`tracked Go artifact: ${file}`);
    if (/^cmd\/jftrade-api(?:\/|$)|(^|\/)(?:wails|wails3)(?:\/|$)/iu.test(file)) {
      errors.push(`retired production entrypoint: ${file}`);
    }
    if (!isActive(file) || isGoArtifact(file)) continue;
    let source;
    try {
      source = readText(file);
    } catch {
      continue;
    }
    for (const [index, line] of source.split(/\r?\n/u).entries()) {
      activeTextPattern.lastIndex = 0;
      if (activeTextPattern.test(line)) {
        errors.push(`active Go/Wails reference: ${file}:${index + 1}: ${line.trim()}`);
      }
    }
  }
  return errors;
}

function trackedFiles(root) {
  return execFileSync("git", ["ls-files", "-z"], {
    cwd: root,
    encoding: "utf8",
    maxBuffer: 128 * 1024 * 1024,
  }).split("\0").filter(Boolean);
}

function walk(root, current = root) {
  const entries = [];
  for (const entry of fs.readdirSync(current, { withFileTypes: true })) {
    const absolute = path.join(current, entry.name);
    const relative = normalize(path.relative(root, absolute));
    if (entry.isSymbolicLink()) {
      entries.push({ absolute, relative, kind: "symlink" });
    } else if (entry.isDirectory()) {
      entries.push(...walk(root, absolute));
    } else if (entry.isFile()) {
      entries.push({ absolute, relative, kind: "file" });
    }
  }
  return entries;
}

function containsMagic(file, magic) {
  const descriptor = fs.openSync(file, "r");
  const buffer = Buffer.alloc(1024 * 1024 + magic.length);
  let carry = 0;
  try {
    while (true) {
      const count = fs.readSync(descriptor, buffer, carry, buffer.length - carry, null);
      if (count === 0) return false;
      const length = carry + count;
      if (buffer.subarray(0, length).includes(magic)) return true;
      carry = Math.min(magic.length - 1, length);
      buffer.copy(buffer, 0, length - carry, length);
    }
  } finally {
    fs.closeSync(descriptor);
  }
}

export function inspectArtifact(root) {
  const absoluteRoot = path.resolve(root);
  if (!fs.existsSync(absoluteRoot)) return [`release artifact is missing: ${root}`];
  const stat = fs.lstatSync(absoluteRoot);
  const entries = stat.isDirectory()
    ? walk(absoluteRoot)
    : [{ absolute: absoluteRoot, relative: path.basename(absoluteRoot), kind: "file" }];
  const errors = [];
  for (const entry of entries) {
    const name = entry.relative;
    if (entry.kind === "symlink") {
      errors.push(`release artifact contains symlink: ${name}`);
      continue;
    }
    if (isGoArtifact(name) || /(?:^|[._-])wails(?:[._-]|$)/iu.test(name)) {
      errors.push(`release artifact contains retired file: ${name}`);
    }
    if (containsMagic(entry.absolute, goBuildInfoMagic)) {
      errors.push(`release artifact contains a Go executable: ${name}`);
    }
    if (looksTextual(name) && fs.statSync(entry.absolute).size <= 32 * 1024 * 1024) {
      const text = fs.readFileSync(entry.absolute, "utf8");
      artifactTextPattern.lastIndex = 0;
      if (artifactTextPattern.test(text)) {
        errors.push(`release artifact metadata contains Go/Wails component: ${name}`);
      }
    }
  }
  return errors;
}

function parseArgs(args) {
  const artifacts = [];
  for (let index = 0; index < args.length; index += 1) {
    if (args[index] !== "--artifact") throw new Error(`unknown argument: ${args[index]}`);
    const value = args[++index];
    if (!value) throw new Error("missing value for --artifact");
    artifacts.push(value);
  }
  return artifacts;
}

export function checkZeroGo({ root = repositoryRoot, artifacts = [] } = {}) {
  const files = trackedFiles(root);
  const errors = validateSourceInventory(files, (file) => fs.readFileSync(path.join(root, file), "utf8"));
  for (const artifact of artifacts) {
    errors.push(...inspectArtifact(path.resolve(root, artifact)));
  }
  return { errors, trackedFiles: files.length, artifacts: artifacts.length };
}

if (pathToFileURL(path.resolve(process.argv[1] ?? "")).href === import.meta.url) {
  try {
    const result = checkZeroGo({ artifacts: parseArgs(process.argv.slice(2)) });
    if (result.errors.length > 0) throw new Error(result.errors.map((error) => `- ${error}`).join("\n"));
    console.log(`Zero-Go gate passed: ${result.trackedFiles} tracked files and ${result.artifacts} release artifact(s) verified.`);
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  }
}
