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
  if (files.some((file) => /(^|\/)(package\.json|pnpm-lock\.yaml|tsconfig[^/]*\.json)$|\.(ts|tsx|vue|css)$/.test(file))) {
    checks.add("web");
  }
  if (files.some((file) => file.startsWith("scripts/") || file.startsWith(".github/"))) {
    checks.add("scripts");
  }
  if (files.some((file) => file.startsWith("docs/swagger/") || file.startsWith("docs/reference/generated/") || file.startsWith("apps/web/src/generated/") || file === "tests/fixtures/openapi-baseline.json")) {
    checks.add("generated");
  }
  return checks;
}

export function changedFiles(root = repoRoot, base = resolveBase(root)) {
  const names = git(root, ["diff", "--name-only", "--diff-filter=ACMRD", base]);
  const untracked = git(root, ["ls-files", "--others", "--exclude-standard", "-z"])
    .split("\0")
    .filter(Boolean);
  return [...new Set([...names.split("\n").filter(Boolean), ...untracked])].sort();
}

function resolveBase(root) {
  for (const candidate of [process.env.JFTRADE_DIFF_BASE, "origin/main", "HEAD^"]) {
    if (!candidate) continue;
    try {
      git(root, ["rev-parse", "--verify", candidate]);
      return candidate;
    } catch {
      // Try the next available baseline.
    }
  }
  return "HEAD";
}

function git(root, args) {
  return execFileSync("git", args, { cwd: root, encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] }).trim();
}

function run(command) {
  console.log(`\n> ${command}`);
  const result = spawnSync(command, { cwd: repoRoot, shell: true, stdio: "inherit" });
  return result.status ?? 1;
}

export function webAffectedTestCommands(files) {
  const webFiles = files
    .filter((file) => file.startsWith("apps/web/"))
    .map((file) => file.slice("apps/web/".length));
  const directTests = webFiles.filter((file) => /(?:^|\/)(?:tests\/.*|src\/.*\.(?:test|spec)\.[cm]?[jt]sx?)$/.test(file));
  const sources = webFiles.filter((file) => file.startsWith("src/") && !directTests.includes(file));
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

function buildCommands(files, modules, withChecks) {
  const commands = [];
  const moduleCommands = modules.flatMap((module) => module.affectedTests ?? []);
  commands.push(...new Set(moduleCommands));
  commands.push(...webAffectedTestCommands(files));
  const fallback = resolveFallbackChecks(files);
  if (withChecks) {
    commands.unshift("pnpm run check:ai-context");
    if (fallback.has("go") || modules.some((module) => module.id === "apiserver" || module.id === "assistant")) {
      commands.push("pnpm run check:go-file-length");
      const vetTargets = [...new Set(modules.flatMap((module) => module.vetPackages ?? []))];
      commands.push(vetTargets.length > 0 ? `go vet ${vetTargets.join(" ")}` : "go vet ./...");
      commands.push("pnpm run check:arch-deps");
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
    if (fallback.has("generated") || modules.some((module) => module.id === "apiserver")) {
      commands.push("pnpm run check:generated");
    }
  }
  return [...new Set(commands)];
}

export function planAffected(files, { withChecks = false, map = moduleMap } = {}) {
  const modules = resolveAffectedModules(files, map);
  return { files, modules, commands: buildCommands(files, modules, withChecks) };
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
