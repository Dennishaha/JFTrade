#!/usr/bin/env node
import { execFileSync, spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import process from "node:process";

const repoRoot = path.resolve(import.meta.dirname, "..");
const moduleMap = JSON.parse(fs.readFileSync(path.join(repoRoot, "scripts/module-map.json"), "utf8"));

export function resolveAffectedModules(files, map = moduleMap) {
  return map.modules.filter((module) => module.paths.some((prefix) => (
    files.some((file) => file === prefix || file.startsWith(`${prefix}/`))
  )));
}

export function resolveFallbackChecks(files) {
  const checks = new Set();
  if (files.some((file) => /(^|\/)(go\.mod|go\.sum)$|\.go$/.test(file))) {
    checks.add("go");
  }
  if (files.some((file) => /(^|\/)(Cargo\.toml|Cargo\.lock|rust-toolchain\.toml|deny\.toml)$|\.rs$/.test(file))) {
    checks.add("rust");
  }
  if (files.some((file) => /(^|\/)(package\.json|pnpm-lock\.yaml|tsconfig[^/]*\.json)$|\.(ts|tsx|vue|css)$/.test(file))) {
    checks.add("web");
  }
  if (files.some((file) => file.startsWith("scripts/") || file.startsWith(".github/"))) {
    checks.add("scripts");
  }
  if (files.some((file) => file.startsWith(".github/workflows/"))) {
    checks.add("workflows");
  }
  if (files.some((file) => file.startsWith("docs/swagger/") || file.startsWith("docs/reference/generated/") || file.startsWith("apps/web/src/generated/") || file === "tests/fixtures/openapi-baseline.json")) {
    checks.add("generated");
  }
  return checks;
}

export function changedFiles(root = repoRoot, base, { env = process.env, gitCommand = git } = {}) {
  const comparisonBase = base ?? resolveBase(root, { env, gitCommand });
  const names = gitCommand(root, ["diff", "--name-only", "--diff-filter=ACMRD", comparisonBase]);
  const untracked = gitCommand(root, ["ls-files", "--others", "--exclude-standard", "-z"])
    .split("\0")
    .filter(Boolean);
  return [...new Set([...names.split("\n").filter(Boolean), ...untracked])].sort();
}

export function resolveBase(root = repoRoot, { env = process.env, gitCommand = git } = {}) {
  for (const candidate of [env.JFTRADE_DIFF_BASE, "origin/main", "HEAD^"]) {
    if (!candidate) continue;
    try {
      gitCommand(root, ["rev-parse", "--verify", candidate]);
      return gitCommand(root, ["merge-base", "HEAD", candidate]);
    } catch {
      // Try the next available baseline.
    }
  }
  return "HEAD";
}

function git(root, args) {
  return execFileSync("git", args, { cwd: root, encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] }).trim();
}

function parseJsonObjects(source) {
  const values = [];
  let depth = 0;
  let start = -1;
  let inString = false;
  let escaped = false;
  for (let index = 0; index < source.length; index += 1) {
    const character = source[index];
    if (inString) {
      if (escaped) escaped = false;
      else if (character === "\\") escaped = true;
      else if (character === '"') inString = false;
      continue;
    }
    if (character === '"') {
      inString = true;
    } else if (character === "{") {
      if (depth === 0) start = index;
      depth += 1;
    } else if (character === "}") {
      depth -= 1;
      if (depth === 0 && start >= 0) {
        values.push(JSON.parse(source.slice(start, index + 1)));
        start = -1;
      }
    }
  }
  if (depth !== 0 || inString) throw new Error("go list returned incomplete JSON");
  return values;
}

export function listGoPackages(root = repoRoot) {
  const output = execFileSync("go", ["list", "-json", "-test", "./..."], {
    cwd: root,
    encoding: "utf8",
    maxBuffer: 128 * 1024 * 1024,
    stdio: ["ignore", "pipe", "pipe"],
  });
  return parseJsonObjects(output);
}

function normalizeGoImportPath(importPath = "") {
  return importPath.replace(/ \[[^\]]+\.test\]$/, "");
}

function ownerPackage(record) {
  if (record.ForTest) return record.ForTest;
  const importPath = normalizeGoImportPath(record.ImportPath);
  return importPath.endsWith(".test") ? importPath.slice(0, -5) : importPath;
}

function fullGoTestCommand() {
  return "go test ./... -count=1 -timeout 300s";
}

export function goAffectedTestCommands(files, {
  root = repoRoot,
  packages,
  fileExists = fs.existsSync,
  loadPackages = listGoPackages,
  maxAffectedPackages = 40,
} = {}) {
  const goFiles = files.filter((file) => file.endsWith(".go"));
  if (goFiles.length === 0 && !files.some((file) => /(^|\/)go\.(?:mod|sum)$/.test(file))) return [];
  if (files.some((file) => /(^|\/)go\.(?:mod|sum)$/.test(file))) return [fullGoTestCommand()];
  if (goFiles.some((file) => !fileExists(path.join(root, file)))) return [fullGoTestCommand()];

  let packageRecords;
  try {
    packageRecords = packages ?? loadPackages(root);
  } catch {
    return [fullGoTestCommand()];
  }
  const ordinaryPackages = packageRecords.filter((record) => (
    record.Module?.Main
    && !record.ForTest
    && !record.ImportPath.includes(" [")
    && !record.ImportPath.endsWith(".test")
  ));
  const ordinaryPaths = new Set(ordinaryPackages.map((record) => record.ImportPath));
  const changedPackages = new Set();
  const productionChanges = new Set();
  for (const file of goFiles) {
    const directory = path.dirname(path.join(root, file));
    const record = ordinaryPackages.find((candidate) => path.resolve(candidate.Dir) === path.resolve(directory));
    if (!record) return [fullGoTestCommand()];
    changedPackages.add(record.ImportPath);
    if (!file.endsWith("_test.go")) productionChanges.add(record.ImportPath);
  }

  const affectedPackages = new Set(changedPackages);
  if (productionChanges.size > 0) {
    for (const record of packageRecords) {
      const owner = ownerPackage(record);
      if (!ordinaryPaths.has(owner)) continue;
      const dependencies = [
        ...(record.Deps ?? []),
        ...(record.Imports ?? []),
        ...(record.TestImports ?? []),
        ...(record.XTestImports ?? []),
      ].map(normalizeGoImportPath);
      if (dependencies.some((dependency) => productionChanges.has(dependency))) {
        affectedPackages.add(owner);
      }
    }
  }
  if (affectedPackages.size === 0 || affectedPackages.size >= maxAffectedPackages) {
    return [fullGoTestCommand()];
  }
  return [`go test -p=4 ${[...affectedPackages].sort().join(" ")} -count=1 -timeout 300s`];
}

function run(command) {
  console.log(`\n> ${command}`);
  const result = spawnSync(command, { cwd: repoRoot, shell: true, stdio: "inherit" });
  return result.status ?? 1;
}

export function webAffectedTestCommands(files, { fileExists = fs.existsSync, webRoot = path.join(repoRoot, "apps/web") } = {}) {
  const webFiles = files
    .filter((file) => file.startsWith("apps/web/"))
    .map((file) => file.slice("apps/web/".length));
  const existingWebFiles = webFiles.filter((file) => fileExists(path.join(webRoot, file)));
  const directTests = existingWebFiles.filter((file) => (
    /(?:^|\/)(?:tests\/.*|src\/.*\.(?:test|spec)\.[cm]?[jt]sx?)$/.test(file)
  ));
  const sources = existingWebFiles.filter((file) => file.startsWith("src/") && !directTests.includes(file));
  const commands = [];
  if (directTests.length > 0) {
    commands.push(`pnpm --filter @jftrade/web exec vitest run ${directTests.map(shellQuote).join(" ")}`);
  }
  if (sources.length > 0) {
    commands.push(`pnpm --filter @jftrade/web exec vitest related --run ${sources.map(shellQuote).join(" ")}`);
  }
  return commands;
}

function shellQuote(value) {
  return `'${value.replaceAll("'", "'\\''")}'`;
}

function buildCommands(files, modules, withChecks, goOptions) {
  const commands = [];
  const moduleCommands = modules.flatMap((module) => module.affectedTests ?? []);
  commands.push(...new Set(moduleCommands));
  commands.push(...webAffectedTestCommands(files));
  const fallback = resolveFallbackChecks(files);
  if (fallback.has("go")) commands.push(...goAffectedTestCommands(files, goOptions));
  if (fallback.has("rust") && !moduleCommands.includes("pnpm run test:rust")) {
    commands.push("pnpm run test:rust");
  }
  if (withChecks) {
    commands.unshift("pnpm run check:ai-context");
    commands.unshift("pnpm run check:diff");
    if (fallback.has("go") || modules.some((module) => module.id === "apiserver" || module.id === "assistant")) {
      commands.push("pnpm run check:go-file-length");
      const vetTargets = [...new Set(modules.flatMap((module) => module.vetPackages ?? []))];
      commands.push(vetTargets.length > 0 ? `go vet ${vetTargets.join(" ")}` : "go vet ./...");
      commands.push("pnpm run check:arch-deps");
    }
    if (fallback.has("rust")) {
      commands.push("pnpm run format:rust:check");
      commands.push("pnpm run lint:rust");
    }
    if (fallback.has("web") || modules.some((module) => module.id === "web" || module.id === "strategy-backtest")) {
      commands.push("pnpm run check:web-file-length");
      commands.push("pnpm run typecheck:web");
    }
    if (modules.some((module) => module.id === "pineworker")) {
      commands.push("pnpm run typecheck:pineworker");
    }
    if (fallback.has("scripts")) {
      commands.push("pnpm run test:scripts -- policy");
    }
    if (fallback.has("workflows")) {
      commands.push("pnpm run check:actionlint");
    }
    if (fallback.has("generated") || modules.some((module) => module.id === "apiserver")) {
      commands.push("pnpm run check:generated");
    }
  }
  return [...new Set(commands)];
}

export function planAffected(files, { withChecks = false, map = moduleMap, goOptions } = {}) {
  const modules = resolveAffectedModules(files, map);
  return { files, modules, commands: buildCommands(files, modules, withChecks, goOptions) };
}

function main() {
  const withChecks = process.argv.includes("--with-checks");
  const printOnly = process.argv.includes("--print");
  const files = changedFiles();
  const plan = planAffected(files, { withChecks });
  if (files.length === 0) {
    console.log("No changed files found relative to the configured base.");
    return;
  }
  console.log(`Affected files: ${files.length}`);
  console.log(`Affected modules: ${plan.modules.map((module) => module.id).join(", ") || "none"}`);
  console.log(`Checks: ${plan.commands.join("; ") || "none"}`);
  if (printOnly) return;
  for (const command of plan.commands) {
    if (run(command) !== 0) {
      process.exitCode = 1;
      return;
    }
  }
}

if (path.resolve(process.argv[1] ?? "") === path.resolve(import.meta.filename)) {
  main();
}
