#!/usr/bin/env node
import { execFileSync } from "node:child_process";
import { existsSync, readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { relative, resolve } from "node:path";

const coverageName = /coverage/i;
const coverageNumberShorthand = /(?:^|[._-])c[_-]?\d{2,3}(?=$|[._-])/i;
const defaultAllowlist = "scripts/test-name-allowlist.txt";

export function isTestFile(path) {
  return /(?:_test\.go|\.(?:test|spec)\.[cm]?[jt]sx?)$/i.test(path);
}

export function hasManagedCoverageName(path) {
  const basename = path.replace(/^.*[\\/]/, "");
  return coverageName.test(basename) || coverageNumberShorthand.test(basename);
}

export function parseAllowlist(contents) {
  const entries = contents
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter((line) => line && !line.startsWith("#"));
  return new Set(entries);
}

export function comparePolicyState(violations, allowlist, baseAllowlist) {
  const violationSet = new Set(violations);
  return {
    unallowlisted: violations.filter((path) => !allowlist.has(path)),
    stale: [...allowlist].filter((path) => !violationSet.has(path)).sort(),
    growth: baseAllowlist ? [...allowlist].filter((path) => !baseAllowlist.has(path)).sort() : [],
  };
}

function main() {
  const options = parseArgs(process.argv.slice(2));
  const repoRoot = resolve(options.repoRoot);
  const allowlistPath = resolve(repoRoot, options.allowlist);
  const base = options.base || process.env.JFTRADE_DIFF_BASE || defaultBase(repoRoot);

  if (!base || /^0+$/.test(base)) {
    throw new Error("unable to determine a diff base; pass --base <git-ref> or set JFTRADE_DIFF_BASE");
  }
  if (!existsSync(allowlistPath)) {
    throw new Error(`test filename allowlist not found: ${relative(repoRoot, allowlistPath)}`);
  }

  const candidates = trackedAndUntrackedFiles(repoRoot);
  const violations = candidates.filter((path) => isTestFile(path) && hasManagedCoverageName(path)).sort();
  const allowlist = parseAllowlist(readFileSync(allowlistPath, "utf8"));
  const baseViolations = readBaseViolations(repoRoot, base);
  const result = comparePolicyState(violations, allowlist, baseViolations);

  if (result.unallowlisted.length > 0) {
    printPaths("Test filenames must describe business behavior, not a coverage target:", result.unallowlisted);
  }
  if (result.stale.length > 0) {
    printPaths("Remove resolved entries from the test filename allowlist:", result.stale);
  }
  if (result.growth.length > 0) {
    printPaths(`The test filename allowlist may only shrink relative to ${base}:`, result.growth);
  }
  if (result.unallowlisted.length > 0 || result.stale.length > 0 || result.growth.length > 0) {
    process.exitCode = 1;
    return;
  }

  console.log(
    `Test filename policy passed across the repository `
    + `(${violations.length} allowlisted legacy files; baseline derived from the ${base} tree).`,
  );
}

function trackedAndUntrackedFiles(repoRoot) {
  const output = git(repoRoot, ["ls-files", "--cached", "--others", "--exclude-standard", "-z"]);
  return [...new Set(output.split("\0").filter(Boolean))]
    .filter((path) => existsSync(resolve(repoRoot, path)));
}

function readBaseViolations(repoRoot, base) {
  let mergeBase;
  try {
    mergeBase = git(repoRoot, ["merge-base", base, "HEAD"]).trim();
  } catch (error) {
    throw new Error(`unable to resolve merge base for ${base}: ${gitErrorMessage(error)}`);
  }
  return violationsFromTree(repoRoot, mergeBase);
}

function violationsFromTree(repoRoot, mergeBase) {
  let output;
  try {
    output = git(repoRoot, ["ls-tree", "-r", "--name-only", "-z", mergeBase]);
  } catch (error) {
    throw new Error(`unable to derive the test filename baseline from ${mergeBase}: ${gitErrorMessage(error)}`);
  }
  return new Set(
    output
      .split("\0")
      .filter((path) => path && isTestFile(path) && hasManagedCoverageName(path)),
  );
}

function printPaths(message, paths) {
  console.error(message);
  for (const path of paths) {
    console.error(`- ${path}`);
  }
}

function parseArgs(args) {
  const options = { allowlist: defaultAllowlist, base: "", repoRoot: process.cwd() };
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
    } else if (arg === "--allowlist") {
      options.allowlist = requireValue(args[++index], "--allowlist");
    } else if (arg.startsWith("--allowlist=")) {
      options.allowlist = requireValue(arg.slice("--allowlist=".length), "--allowlist");
    } else if (arg === "--help" || arg === "-h") {
      console.log("Usage: node scripts/check-test-names.mjs [--base <git-ref>] [--repo-root <path>] [--allowlist <path>]");
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
  return execFileSync("git", args, { cwd, encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] });
}

function gitErrorMessage(error) {
  const stderr = error && typeof error === "object" && "stderr" in error
    ? String(error.stderr).trim()
    : "";
  return stderr || (error instanceof Error ? error.message : String(error));
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
