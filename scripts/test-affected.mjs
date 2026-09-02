#!/usr/bin/env node
import { execFileSync, spawn } from "node:child_process";
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
  const startedAt = Date.now();
  return new Promise((complete) => {
    const child = spawn(command, { cwd: repoRoot, shell: true, stdio: "inherit" });
    const heartbeat = setInterval(() => {
      console.log(`> still running (${((Date.now() - startedAt) / 1_000).toFixed(0)}s): ${command}`);
    }, 30_000);
    heartbeat.unref?.();
    child.once("error", () => {
      clearInterval(heartbeat);
      complete(1);
    });
    child.once("close", (status) => {
      clearInterval(heartbeat);
      console.log(`> completed in ${((Date.now() - startedAt) / 1_000).toFixed(1)}s: ${command}`);
      complete(status ?? 1);
    });
  });
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

const rustWorkspaceFiles = new Set([
  "Cargo.toml",
  "Cargo.lock",
  "deny.toml",
  "rust-toolchain.toml",
]);

function rustPackageManifest(file) {
  if (file.startsWith("apps/desktop/src-tauri/")) {
    return "apps/desktop/src-tauri/Cargo.toml";
  }
  const match = file.match(/^crates\/([^/]+)\//);
  return match ? `crates/${match[1]}/Cargo.toml` : null;
}

export function listRustWorkspace(root = repoRoot) {
  const metadata = JSON.parse(execFileSync("cargo", ["metadata", "--no-deps", "--format-version", "1"], {
    cwd: root,
    encoding: "utf8",
    maxBuffer: 32 * 1024 * 1024,
    stdio: ["ignore", "pipe", "pipe"],
  }));
  return metadata.packages.map((record) => ({
    dependencies: record.dependencies
      .filter((dependency) => dependency.path)
      .map((dependency) => dependency.name),
    manifest: path.relative(root, record.manifest_path).split(path.sep).join("/"),
    name: record.name,
  }));
}

function rustAffectedPackages(files, {
  root = repoRoot,
  fileExists = fs.existsSync,
  workspace,
  loadWorkspace = listRustWorkspace,
  maxAffectedPackages = 6,
} = {}) {
  const rustFiles = files.filter((file) => file.endsWith(".rs") || file.endsWith("Cargo.toml"));
  if (rustFiles.length === 0 && !files.some((file) => rustWorkspaceFiles.has(file))) return [];
  if (files.some((file) => rustWorkspaceFiles.has(file))) return null;

  let workspacePackages;
  try {
    workspacePackages = workspace ?? loadWorkspace(root);
  } catch {
    return null;
  }
  const packagesByManifest = new Map(workspacePackages.map((record) => [record.manifest, record]));
  const packages = new Set();
  const productionPackages = new Set();
  for (const file of rustFiles) {
    const manifest = rustPackageManifest(file);
    if (!manifest || !fileExists(path.join(root, manifest))) return null;
    const record = packagesByManifest.get(manifest);
    if (!record) return null;
    packages.add(record.name);
    if (!file.includes("/tests/") && !file.includes("/benches/")) productionPackages.add(record.name);
  }
  if (productionPackages.size > 0) {
    let added = true;
    while (added) {
      added = false;
      for (const record of workspacePackages) {
        if (packages.has(record.name)) continue;
        if (record.dependencies.some((dependency) => packages.has(dependency))) {
          packages.add(record.name);
          added = true;
        }
      }
    }
  }
  return packages.size > maxAffectedPackages ? null : [...packages].sort();
}

export function rustAffectedTestCommands(files, options = {}) {
  const packages = rustAffectedPackages(files, options);
  if (packages === null) return ["pnpm run test:rust"];
  if (packages.length === 0) return [];
  return [`cargo test ${packages.map((packageName) => `-p ${packageName}`).join(" ")} --all-targets`];
}

export function rustAffectedClippyCommands(files, options = {}) {
  const packages = rustAffectedPackages(files, options);
  if (packages === null) return ["pnpm run lint:rust"];
  if (packages.length === 0) return [];
  return [
    `cargo clippy ${packages.map((packageName) => `-p ${packageName}`).join(" ")} --all-targets --all-features -- -D warnings`,
  ];
}

function migrationAffectedCommands(files, profile) {
  const commands = [];
  const stageChecks = [
    [/(^|\/)stage2(?:[-_/])/, "pnpm run test:rust:differential"],
    [/(^|\/)stage3(?:[-_/])/, "pnpm run test:rust:backtest:differential"],
    [/(^|\/)stage4(?:[-_/])/, "pnpm run test:rust:stage4:differential"],
    [/(^|\/)stage5(?:[-_/])/, "pnpm run test:rust:stage5:differential"],
    [/(^|\/)stage6(?:[-_/])/, "pnpm run test:rust:stage6:differential"],
    [/(^|\/)stage7(?:[-_/])/, "pnpm run test:rust:stage7:differential"],
    [/(^|\/)stage8(?:[-_/])/, "pnpm run test:rust:stage8:differential"],
  ];
  for (const [pattern, command] of stageChecks) {
    if (files.some((file) => pattern.test(file))) commands.push(command);
  }
  const stage9Changed = files.some((file) => (
    file.startsWith("tests/fixtures/rust-migration/stage9/")
    || file.startsWith("scripts/rust-migration/stage9")
    || file.startsWith("scripts/rust-migration/check-stage9-")
    || file.startsWith("crates/jftrade-engine/src/product")
  ));
  if (stage9Changed) {
    commands.push(
      "node --test scripts/rust-migration/check-stage9-closeout.test.mjs scripts/rust-migration/stage9-route-ownership.test.mjs",
      "pnpm run test:rust:stage9:route-coverage",
    );
  }
  if (profile === "full") {
    const productDifferentialChanged = files.some((file) => (
      file === "scripts/rust-migration/check-stage9-product-differential.mjs"
      || file.startsWith("crates/jftrade-engine/src/product")
    ));
    const directStage9Checks = files.filter((file) => (
      /^scripts\/rust-migration\/check-stage9-[a-z0-9-]+\.mjs$/.test(file)
      && !file.endsWith(".test.mjs")
      && !file.endsWith("check-stage9-closeout.mjs")
      && !file.endsWith("check-stage9-route-coverage.mjs")
      && !file.endsWith("check-stage9-product-differential.mjs")
    ));
    if (productDifferentialChanged) {
      commands.push("pnpm run test:rust:stage9:product-differential");
    } else {
      commands.push(...directStage9Checks.map((file) => `node ${file}`));
    }
  }
  const tauriChanged = files.some((file) => (
    file.startsWith("apps/desktop/src-tauri/")
    || file === "scripts/prepare-tauri-release-runtime.mjs"
    || file === "scripts/smoke-tauri-release.mjs"
    || file === "scripts/lib/tauri-runtime.mjs"
  ));
  if (tauriChanged) commands.push("pnpm run test:tauri-release-runtime");
  const runsCargo = commands.some((command) => (
    command.includes(":differential")
    || command.includes("stage9:product-differential")
    || /^node scripts\/rust-migration\/check-stage9-/.test(command)
  ));
  if (runsCargo) commands.unshift("pnpm run check:rust:target-health");
  return commands;
}

function buildCommands(files, modules, withChecks, goOptions, profile) {
  const commands = [];
  const moduleCommands = modules.flatMap((module) => (
    profile === "quick" ? module.quickTests ?? module.affectedTests ?? [] : module.affectedTests ?? []
  ));
  commands.push(...new Set(moduleCommands));
  commands.push(...migrationAffectedCommands(files, profile));
  commands.push(...webAffectedTestCommands(files));
  const fallback = resolveFallbackChecks(files);
  if (fallback.has("go")) commands.push(...goAffectedTestCommands(files, goOptions));
  if (fallback.has("rust")) {
    commands.push("pnpm run check:rust:target-health");
    const rustCommands = rustAffectedTestCommands(files);
    if (!moduleCommands.includes("pnpm run test:rust")) commands.push(...rustCommands);
  }
  if (withChecks) {
    commands.unshift("pnpm run check:go-retirement");
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
      commands.push(...rustAffectedClippyCommands(files));
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

export function planAffected(files, {
  withChecks = false,
  profile = "full",
  map = moduleMap,
  goOptions,
} = {}) {
  if (!new Set(["full", "quick"]).has(profile)) {
    throw new Error(`unknown affected-test profile: ${profile}`);
  }
  const modules = resolveAffectedModules(files, map);
  const deferredCommands = modules.some((module) => module.id === "rust-foundation")
    ? ["pnpm run check:rust"]
    : [];
  return {
    files,
    modules,
    commands: buildCommands(files, modules, withChecks, goOptions, profile),
    deferredCommands,
  };
}

async function main() {
  const withChecks = process.argv.includes("--with-checks");
  const printOnly = process.argv.includes("--print");
  const worktreeOnly = process.argv.includes("--worktree");
  const profile = process.argv.find((argument) => argument.startsWith("--profile="))?.slice(10) ?? "full";
  const files = changedFiles(repoRoot, worktreeOnly ? "HEAD" : undefined);
  const plan = planAffected(files, { withChecks, profile });
  if (files.length === 0) {
    console.log("No changed files found relative to the configured base.");
    return;
  }
  console.log(`Affected scope: ${worktreeOnly ? "current worktree" : "merge base"} (${profile})`);
  console.log(`Affected files: ${files.length}`);
  console.log(`Affected modules: ${plan.modules.map((module) => module.id).join(", ") || "none"}`);
  console.log(`Checks: ${plan.commands.join("; ") || "none"}`);
  if (plan.deferredCommands.length > 0) {
    console.log(`Deferred integration checks: ${plan.deferredCommands.join("; ")}`);
  }
  if (printOnly) return;
  for (const command of plan.commands) {
    if (await run(command) !== 0) {
      process.exitCode = 1;
      return;
    }
  }
}

if (path.resolve(process.argv[1] ?? "") === path.resolve(import.meta.filename)) {
  await main();
}
