#!/usr/bin/env node
import { execFileSync, spawnSync } from "node:child_process";
import { lstatSync, readFileSync } from "node:fs";
import path from "node:path";
import process from "node:process";

import { resolveBase } from "./test-affected.mjs";

const repoRoot = path.resolve(import.meta.dirname, "..");
const binaryExtensions = new Set([
  ".7z", ".db", ".dmg", ".dll", ".exe", ".gif", ".gz", ".ico", ".jpg", ".jpeg",
  ".pdf", ".png", ".so", ".sqlite", ".tar", ".ttf", ".wasm", ".webp", ".woff", ".woff2", ".zip",
]);

export function diffCheckArgs(base) {
  // base is already the resolved merge-base. Compare it with the complete
  // index + working-tree projection so a later repair can fix whitespace
  // introduced by an earlier commit before the branch is pushed.
  return ["diff", "--check", base];
}

export function untrackedDiffArgs(file, platform = process.platform) {
  return ["diff", "--no-index", "--check", platform === "win32" ? "NUL" : "/dev/null", file];
}

export function isLikelyTextFile(file) {
  if (binaryExtensions.has(path.extname(file).toLowerCase())) {
    return false;
  }
  try {
    if (!lstatSync(file).isFile()) {
      return false;
    }
    return !readFileSync(file).subarray(0, 8192).includes(0);
  } catch {
    return false;
  }
}

export function listUntrackedTextFiles(root = repoRoot) {
  const output = execFileSync("git", ["ls-files", "--others", "--exclude-standard", "-z"], {
    cwd: root,
    encoding: "utf8",
  });
  return output
    .split("\0")
    .filter(Boolean)
    .map((file) => path.resolve(root, file))
    .filter(isLikelyTextFile);
}

export function checkUntrackedWhitespace(root, files) {
  for (const file of files) {
    const result = spawnSync("git", untrackedDiffArgs(file), {
      cwd: root,
      encoding: "utf8",
    });
    const diagnostics = `${result.stdout ?? ""}${result.stderr ?? ""}`;
    if (diagnostics.trim() !== "") {
      process.stderr.write(diagnostics);
      return false;
    }
  }
  return true;
}

export function runDiffCheck({ root = repoRoot, base = resolveBase(root) } = {}) {
  const result = spawnSync("git", diffCheckArgs(base), {
    cwd: root,
    stdio: "inherit",
  });
  if ((result.status ?? 1) !== 0) {
    return result.status ?? 1;
  }
  if (!checkUntrackedWhitespace(root, listUntrackedTextFiles(root))) {
    return 1;
  }
  return 0;
}

if (path.resolve(process.argv[1] ?? "") === path.resolve(import.meta.filename)) {
  process.exitCode = runDiffCheck();
}
