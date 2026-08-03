#!/usr/bin/env node

import { execFileSync } from "node:child_process";
import { existsSync, readFileSync, readdirSync } from "node:fs";
import { dirname, extname, posix, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";

import ts from "typescript";

const defaultAllowlist = "scripts/web-openapi-import-allowlist.json";
const sourceRoots = ["apps/web/src", "apps/web/tests"];
const sourceExtensions = new Set([".js", ".jsx", ".ts", ".tsx", ".vue"]);
const exactInfrastructureFiles = new Set([
  "apps/web/src/composables/shared/apiClient.ts",
  "apps/web/tests/contracts/contractsModularization.test.ts",
]);

export function directGeneratedOpenAPIImportFiles(sources) {
  const files = [];
  for (const [rawPath, source] of [...sources.entries()].sort(([a], [b]) =>
    a.localeCompare(b),
  )) {
    const file = normalizePath(rawPath);
    const importedFiles = ts.preProcessFile(source, true, true).importedFiles;
    if (importedFiles.some((entry) => targetsGeneratedOpenAPI(file, entry.fileName))) {
      files.push(file);
    }
  }
  return files;
}

export function isOpenAPIImportInfrastructure(rawPath) {
  const file = normalizePath(rawPath);
  return (
    exactInfrastructureFiles.has(file) ||
    /^apps\/web\/src\/contracts\/wire\/[^/]+\.ts$/.test(file)
  );
}

export function parseOpenAPIImportAllowlist(manifest) {
  const failures = [];
  if (manifest == null || typeof manifest !== "object" || Array.isArray(manifest)) {
    return { entries: new Map(), failures: ["allowlist must be a JSON object"] };
  }
  if (manifest.version !== 1) {
    failures.push("allowlist version must be 1");
  }
  const legacy = manifest.legacyDirectImports;
  if (legacy == null || typeof legacy !== "object" || Array.isArray(legacy)) {
    failures.push("legacyDirectImports must be an object keyed by repository path");
    return { entries: new Map(), failures };
  }
  const entries = new Map();
  for (const [rawPath, reason] of Object.entries(legacy).sort(([a], [b]) =>
    a.localeCompare(b),
  )) {
    const file = normalizePath(rawPath);
    if (file !== rawPath || !file.startsWith("apps/web/")) {
      failures.push(`${rawPath}: allowlist paths must be normalized apps/web paths`);
    }
    if (isOpenAPIImportInfrastructure(file)) {
      failures.push(`${file}: sanctioned infrastructure must not be allowlisted`);
    }
    if (typeof reason !== "string" || reason.trim().length < 12) {
      failures.push(`${file}: legacy import needs a concrete migration reason`);
    }
    entries.set(file, reason);
  }
  return { entries, failures };
}

export function compareOpenAPIImportPolicy({
  directImports,
  allowlistEntries,
  baseDirectImports,
}) {
  const currentDebt = new Set(
    directImports.filter((file) => !isOpenAPIImportInfrastructure(file)),
  );
  const baseDebt = new Set(
    baseDirectImports.filter((file) => !isOpenAPIImportInfrastructure(file)),
  );
  const allowlisted = new Set(allowlistEntries.keys());
  return {
    currentDebt: [...currentDebt].sort(),
    unallowlisted: [...currentDebt].filter((file) => !allowlisted.has(file)).sort(),
    stale: [...allowlisted].filter((file) => !currentDebt.has(file)).sort(),
    growth: [...allowlisted].filter((file) => !baseDebt.has(file)).sort(),
  };
}

function main() {
  const options = parseArgs(process.argv.slice(2));
  const repoRoot = resolve(options.repoRoot);
  const allowlistPath = resolve(repoRoot, options.allowlist);
  const base = options.base || process.env.JFTRADE_DIFF_BASE || defaultBase(repoRoot);
  if (!base || /^0+$/.test(base)) {
    throw new Error(
      "unable to determine an OpenAPI import baseline; pass --base <git-ref> or set JFTRADE_DIFF_BASE",
    );
  }
  if (!existsSync(allowlistPath)) {
    throw new Error(`OpenAPI import allowlist not found: ${relative(repoRoot, allowlistPath)}`);
  }

  const parsed = parseOpenAPIImportAllowlist(
    JSON.parse(readFileSync(allowlistPath, "utf8")),
  );
  const directImports = directGeneratedOpenAPIImportFiles(
    workingTreeSources(repoRoot),
  );
  const mergeBase = git(repoRoot, ["merge-base", base, "HEAD"]).trim();
  const baseDirectImports = directGeneratedOpenAPIImportFiles(
    treeCandidateSources(repoRoot, mergeBase),
  );
  const result = compareOpenAPIImportPolicy({
    directImports,
    allowlistEntries: parsed.entries,
    baseDirectImports,
  });

  const failures = [...parsed.failures];
  failures.push(
    ...result.unallowlisted.map(
      (file) => `${file}: import a wire alias from @/contracts instead`,
    ),
    ...result.stale.map(
      (file) => `${file}: remove this resolved legacy entry from the allowlist`,
    ),
    ...result.growth.map(
      (file) => `${file}: the legacy allowlist may only shrink relative to ${base}`,
    ),
  );
  if (failures.length > 0) {
    console.error(`Web OpenAPI import boundary failed with ${failures.length} violation(s):`);
    for (const failure of failures) console.error(`- ${failure}`);
    process.exitCode = 1;
    return;
  }

  const infrastructureCount = directImports.length - result.currentDebt.length;
  console.log(
    `Web OpenAPI import boundary passed: ${result.currentDebt.length} legacy consumer(s), ` +
      `${infrastructureCount} sanctioned infrastructure file(s); baseline ${base}.`,
  );
}

function targetsGeneratedOpenAPI(file, rawSpecifier) {
  const specifier = normalizePath(rawSpecifier).replace(/\.(?:[cm]?[jt]sx?)$/, "");
  if (specifier === "@/generated/openapi") return true;
  if (!specifier.startsWith(".")) return false;
  return posix.normalize(posix.join(posix.dirname(file), specifier)) ===
    "apps/web/src/generated/openapi";
}

function workingTreeSources(repoRoot) {
  const sources = new Map();
  for (const root of sourceRoots) {
    const absoluteRoot = resolve(repoRoot, root);
    if (!existsSync(absoluteRoot)) continue;
    for (const file of walkSourceFiles(absoluteRoot)) {
      sources.set(normalizePath(relative(repoRoot, file)), readFileSync(file, "utf8"));
    }
  }
  return sources;
}

function treeCandidateSources(repoRoot, tree) {
  let output = "";
  try {
    output = git(repoRoot, [
      "grep",
      "-l",
      "-z",
      "generated/openapi",
      tree,
      "--",
      ...sourceRoots,
    ]);
  } catch (error) {
    if (error && typeof error === "object" && "status" in error && error.status === 1) {
      return new Map();
    }
    throw error;
  }
  const sources = new Map();
  for (const entry of output.split("\0").filter(Boolean)) {
    const separator = entry.indexOf(":");
    const file = normalizePath(separator >= 0 ? entry.slice(separator + 1) : entry);
    if (!sourceExtensions.has(extname(file))) continue;
    sources.set(file, git(repoRoot, ["show", `${tree}:${file}`]));
  }
  return sources;
}

function walkSourceFiles(root) {
  const files = [];
  for (const entry of readdirSync(root, { withFileTypes: true })) {
    const file = resolve(root, entry.name);
    if (entry.isDirectory()) {
      files.push(...walkSourceFiles(file));
    } else if (sourceExtensions.has(extname(entry.name))) {
      files.push(file);
    }
  }
  return files;
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
      // Try the next local baseline.
    }
  }
  return "";
}

function git(cwd, args) {
  return execFileSync("git", args, {
    cwd,
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"],
  });
}

function normalizePath(path) {
  return path.replaceAll("\\", "/");
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
