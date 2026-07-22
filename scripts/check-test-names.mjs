#!/usr/bin/env node
import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { resolve } from "node:path";

const coverageNumberName = /(?:^|[._-])(?:coverage|c)[_-]?\d{2,3}(?=$|[._-])/i;

export function isTestFile(path) {
  return /(?:_test\.go|\.(?:test|spec)\.[cm]?[jt]sx?)$/i.test(path);
}

export function hasCoverageNumberName(path) {
  const basename = path.replace(/^.*[\\/]/, "");
  return coverageNumberName.test(basename);
}

export function parseAddedPaths(output) {
  const fields = output.split("\0");
  const paths = [];

  for (let index = 0; index < fields.length - 1;) {
    const status = fields[index++];
    if (!status) {
      continue;
    }

    const kind = status[0];
    if (kind === "R" || kind === "C") {
      index += 1; // old path
      const newPath = fields[index++];
      if (newPath) {
        paths.push(newPath);
      }
      continue;
    }

    const path = fields[index++];
    if (path) {
      paths.push(path);
    }
  }

  return paths;
}

function main() {
  const options = parseArgs(process.argv.slice(2));
  const repoRoot = resolve(options.repoRoot);
  const base = options.base || process.env.JFTRADE_DIFF_BASE || defaultBase(repoRoot);

  if (!base || /^0+$/.test(base)) {
    throw new Error("unable to determine a diff base; pass --base <git-ref> or set JFTRADE_DIFF_BASE");
  }

  const output = git(repoRoot, ["diff", "--name-status", "-z", "--find-renames", "--diff-filter=ACR", "--merge-base", base]);
  const untracked = git(repoRoot, ["ls-files", "--others", "--exclude-standard", "-z"]).split("\0").filter(Boolean);
  const candidates = [...new Set([...parseAddedPaths(output), ...untracked])];
  const violations = candidates.filter((path) => isTestFile(path) && hasCoverageNumberName(path));

  if (violations.length === 0) {
    console.log(`Test filename policy passed against ${base}.`);
    return;
  }

  console.error("New test filenames must describe business behavior, not a coverage percentage:");
  for (const path of violations) {
    console.error(`- ${path}`);
  }
  console.error("Rename files such as coverage_98_test.go or c95.spec.ts to the behavior they verify.");
  process.exitCode = 1;
}

function parseArgs(args) {
  const options = { base: "", repoRoot: process.cwd() };
  for (let index = 0; index < args.length; index += 1) {
    const arg = args[index];
    if (arg === "--base") {
      options.base = requireValue(args[++index], "--base");
    } else if (arg.startsWith("--base=")) {
      options.base = requireValue(arg.slice("--base=".length), "--base");
    } else if (arg === "--repo-root") {
      options.repoRoot = requireValue(args[++index], "--repo-root");
    } else if (arg.startsWith("--repo-root=")) {
      options.repoRoot = requireValue(arg.slice("--repo-root=".length), "--repo-root");
    } else if (arg === "--help" || arg === "-h") {
      console.log("Usage: node scripts/check-test-names.mjs [--base <git-ref>] [--repo-root <path>]");
      process.exit(0);
    } else {
      throw new Error(`unknown argument: ${arg}`);
    }
  }
  return options;
}

function defaultBase(repoRoot) {
  for (const candidate of ["origin/main", "HEAD^"]) {
    try {
      git(repoRoot, ["rev-parse", "--verify", candidate]);
      return candidate;
    } catch {
      // Try the next local fallback.
    }
  }
  return "";
}

function git(cwd, args) {
  return execFileSync("git", args, { cwd, encoding: "utf8" });
}

function requireValue(value, flag) {
  if (!value || value.startsWith("--")) {
    throw new Error(`${flag} requires a value`);
  }
  return value;
}

if (resolve(process.argv[1] || "") === fileURLToPath(import.meta.url)) {
  try {
    main();
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  }
}
