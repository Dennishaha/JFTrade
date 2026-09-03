#!/usr/bin/env node
import { execFileSync, spawn } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import process from "node:process";

const repoRoot = path.resolve(import.meta.dirname, "..");
const moduleMap = JSON.parse(fs.readFileSync(path.join(repoRoot, "scripts/module-map.json"), "utf8"));

export const gateLaneNames = Object.freeze([
  "contracts", "rust_static", "rust_tests", "compatibility",
  "web", "pine", "python", "desktop",
]);
export const compatibilityCapabilities = Object.freeze([
  "storage", "backtest", "provider-runtime", "trading-strategy",
  "assistant-runtime", "api-transport", "desktop-runtime",
]);

const forceFullFiles = new Set([
  "Cargo.toml", "Cargo.lock", "deny.toml", "rust-toolchain.toml",
  "package.json", "pnpm-lock.yaml", "pnpm-workspace.yaml",
  "scripts/module-map.json", "AGENTS.md",
]);
const forceFullPrefixes = [
  ".github/", "scripts/quality/", "scripts/compatibility/", "scripts/release/",
];

export function resolveAffectedModules(files, map = moduleMap) {
  return map.modules.filter((module) => module.paths.some((prefix) => (
    files.some((file) => file === prefix || file.startsWith(`${prefix}/`))
  )));
}

export function resolveFallbackChecks(files) {
  const checks = new Set();
  if (files.some((file) => /(^|\/)(Cargo\.toml|Cargo\.lock|rust-toolchain\.toml|deny\.toml)$|\.rs$/.test(file))) checks.add("rust");
  if (files.some((file) => /(^|\/)(package\.json|pnpm-lock\.yaml|tsconfig[^/]*\.json)$|\.(ts|tsx|vue|css)$/.test(file))) checks.add("web");
  if (files.some((file) => file.startsWith("scripts/") || file.startsWith(".github/"))) checks.add("scripts");
  if (files.some((file) => file.startsWith(".github/workflows/"))) checks.add("workflows");
  if (files.some(isGeneratedContractInput)) checks.add("generated");
  return checks;
}

function isGeneratedContractInput(file) {
  return file.startsWith("docs/swagger/")
    || file.startsWith("docs/reference/generated/")
    || file.startsWith("apps/web/src/generated/")
    || file === "contracts/openapi/openapi.json";
}

export function changedFiles(root = repoRoot, base, { env = process.env, gitCommand = git } = {}) {
  const comparisonBase = base ?? resolveBase(root, { env, gitCommand });
  const names = gitCommand(root, ["diff", "--name-only", "--diff-filter=ACMRD", comparisonBase]);
  const untracked = gitCommand(root, ["ls-files", "--others", "--exclude-standard", "-z"])
    .split("\0").filter(Boolean);
  return [...new Set([...names.split("\n").filter(Boolean), ...untracked])].sort();
}

export function resolveBase(root = repoRoot, { env = process.env, gitCommand = git } = {}) {
  for (const candidate of [env.JFTRADE_DIFF_BASE, "origin/main", "HEAD^"]) {
    if (!candidate) continue;
    try {
      gitCommand(root, ["rev-parse", "--verify", candidate]);
      return gitCommand(root, ["merge-base", "HEAD", candidate]);
    } catch {
      // Try the next available baseline. If none resolves, the caller fails closed.
    }
  }
  throw new Error("unable to resolve an affected-test merge base");
}

function git(root, args) {
  return execFileSync("git", args, {
    cwd: root, encoding: "utf8", stdio: ["ignore", "pipe", "pipe"],
  }).trim();
}

export function webAffectedTestCommands(files, {
  fileExists = fs.existsSync,
  webRoot = path.join(repoRoot, "apps/web"),
} = {}) {
  const webFiles = files.filter((file) => file.startsWith("apps/web/"))
    .map((file) => file.slice("apps/web/".length));
  const existing = webFiles.filter((file) => fileExists(path.join(webRoot, file)));
  const directTests = existing.filter((file) => /(?:^|\/)(?:tests\/.*|src\/.*\.(?:test|spec)\.[cm]?[jt]sx?)$/.test(file));
  const sources = existing.filter((file) => file.startsWith("src/") && !directTests.includes(file));
  const commands = [];
  if (directTests.length > 0) commands.push(`pnpm --filter @jftrade/web exec vitest run ${directTests.map(shellQuote).join(" ")}`);
  if (sources.length > 0) commands.push(`pnpm --filter @jftrade/web exec vitest related --run ${sources.map(shellQuote).join(" ")}`);
  return commands;
}

function shellQuote(value) {
  return `'${value.replaceAll("'", "'\\''")}'`;
}

const rustWorkspaceFiles = new Set(["Cargo.toml", "Cargo.lock", "deny.toml", "rust-toolchain.toml"]);

function rustPackageManifest(file) {
  if (file.startsWith("apps/desktop/src-tauri/")) return "apps/desktop/src-tauri/Cargo.toml";
  const match = file.match(/^crates\/([^/]+)\//);
  return match ? `crates/${match[1]}/Cargo.toml` : null;
}

export function listRustWorkspace(root = repoRoot) {
  const metadata = JSON.parse(execFileSync("cargo", ["metadata", "--no-deps", "--format-version", "1"], {
    cwd: root, encoding: "utf8", maxBuffer: 32 * 1024 * 1024,
    stdio: ["ignore", "pipe", "pipe"],
  }));
  return metadata.packages.map((record) => ({
    dependencies: record.dependencies.filter((dependency) => dependency.path).map((dependency) => dependency.name),
    manifest: path.relative(root, record.manifest_path).split(path.sep).join("/"),
    name: record.name,
  }));
}

export function rustAffectedPackages(files, {
  root = repoRoot, fileExists = fs.existsSync, workspace,
  loadWorkspace = listRustWorkspace, maxAffectedPackages = 6,
} = {}) {
  const rustFiles = files.filter((file) => file.endsWith(".rs") || file.endsWith("Cargo.toml"));
  if (rustFiles.length === 0 && !files.some((file) => rustWorkspaceFiles.has(file))) return [];
  if (files.some((file) => rustWorkspaceFiles.has(file))) return null;
  let records;
  try {
    records = workspace ?? loadWorkspace(root);
  } catch {
    return null;
  }
  const packagesByManifest = new Map(records.map((record) => [record.manifest, record]));
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
      for (const record of records) {
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
  return [`cargo test ${packages.map((name) => `-p ${name}`).join(" ")} --all-targets --locked`];
}

export function rustAffectedClippyCommands(files, options = {}) {
  const packages = rustAffectedPackages(files, options);
  if (packages === null) return ["pnpm run lint:rust"];
  if (packages.length === 0) return [];
  return [`cargo clippy ${packages.map((name) => `-p ${name}`).join(" ")} --all-targets --all-features -- -D warnings`];
}

function emptyLanes() {
  return Object.fromEntries(gateLaneNames.map((name) => [name, false]));
}

export function fullGatePlan(reason = "full product validation") {
  return {
    full: true,
    reason,
    lanes: Object.fromEntries(gateLaneNames.map((name) => [name, true])),
    compatibilityCapabilities: [...compatibilityCapabilities],
  };
}

export function planGateLanes(files, { main = false, map = moduleMap } = {}) {
  if (main) return fullGatePlan("main branch always runs the full product gate");
  if (files.some((file) => forceFullFiles.has(file) || forceFullPrefixes.some((prefix) => file.startsWith(prefix)))) {
    return fullGatePlan("shared toolchain, workflow, gate, or module-map input changed");
  }
  const lanes = emptyLanes();
  const capabilities = new Set();
  const known = new Set();
  const mark = (file, lane) => { lanes[lane] = true; known.add(file); };

  for (const file of files) {
    if (file.startsWith("docs/") || /(^|\/)AGENTS\.md$/.test(file) || file === "README.md") {
      known.add(file);
      continue;
    }
    if (file.startsWith("contracts/") || file.startsWith("proto/") || isGeneratedContractInput(file)) {
      mark(file, "contracts");
      lanes.web = true;
      capabilities.add("api-transport");
      continue;
    }
    if (file.startsWith("tests/fixtures/compatibility/")) {
      known.add(file);
      const capability = file.split("/")[3];
      if (!compatibilityCapabilities.includes(capability)) return fullGatePlan(`unknown compatibility capability: ${capability || "missing"}`);
      capabilities.add(capability);
      continue;
    }
    if (file.startsWith("crates/")) {
      mark(file, "rust_static");
      lanes.rust_tests = true;
      const crate = file.split("/")[1];
      for (const capability of capabilitiesForCrate(crate)) capabilities.add(capability);
      if (["jftrade-api", "jftrade-engine"].includes(crate)) lanes.contracts = true;
      if (["jftrade-strategy", "jftrade-backtest", "jftrade-engine"].includes(crate)) lanes.pine = true;
      if (!isTestOnlyProductFile(file)) lanes.desktop = true;
      continue;
    }
    if (file.startsWith("apps/desktop/src-tauri/")) {
      mark(file, "rust_static");
      lanes.rust_tests = true;
      lanes.desktop = true;
      capabilities.add("desktop-runtime");
      continue;
    }
    if (file.startsWith("apps/web/")) {
      mark(file, "web");
      if (!isTestOnlyProductFile(file)) lanes.desktop = true;
      if (file.includes("desktopFacade")) capabilities.add("desktop-runtime");
      continue;
    }
    if (file.startsWith("workers/pineworker/")) {
      mark(file, "pine");
      if (!isTestOnlyProductFile(file)) lanes.desktop = true;
      capabilities.add("trading-strategy");
      continue;
    }
    if (file.startsWith("workers/marketdata-sidecar/")) {
      mark(file, "python");
      if (!isTestOnlyProductFile(file)) lanes.desktop = true;
      capabilities.add("provider-runtime");
      continue;
    }
    if (file.startsWith("runtime-assets/") || isDesktopScript(file)) {
      mark(file, "desktop");
      capabilities.add("desktop-runtime");
      continue;
    }
    if (file.startsWith("tests/fixtures/release/")) { mark(file, "desktop"); continue; }
    if (file.startsWith("scripts/")) return fullGatePlan(`unclassified gate script changed: ${file}`);
    if (map.sourceRoots.some((root) => file === root || file.startsWith(`${root}/`))) return fullGatePlan(`unclassified product path changed: ${file}`);
  }
  if (known.size !== files.length) {
    const unknown = files.find((file) => !known.has(file));
    return fullGatePlan(`unknown path changed: ${unknown}`);
  }
  if (lanes.desktop) lanes.contracts = true;
  lanes.compatibility = capabilities.size > 0;
  return {
    full: false,
    reason: files.length === 0 ? "no product files changed" : "affected product lanes",
    lanes,
    compatibilityCapabilities: [...capabilities].sort(),
  };
}

function capabilitiesForCrate(crate) {
  const mapping = {
    "jftrade-store-sqlite": ["storage"],
    "jftrade-backtest": ["backtest"],
    "jftrade-marketdata": ["provider-runtime"],
    "jftrade-integration-futu": ["provider-runtime"],
    "jftrade-trading": ["trading-strategy"],
    "jftrade-strategy": ["trading-strategy"],
    "jftrade-assistant": ["assistant-runtime"],
    "jftrade-api": ["api-transport"],
    "jftrade-engine": [...compatibilityCapabilities],
  };
  return mapping[crate] ?? [];
}

function isDesktopScript(file) {
  return /^scripts\/(?:.*desktop|.*tauri|prepare-linux-package|manage-linux-release)/.test(file);
}

function isTestOnlyProductFile(file) {
  const name = path.posix.basename(file);
  return file.includes("/tests/")
    || file.includes("/benches/")
    || /(?:^|_)(?:test|tests)\.[^.]+$/.test(name)
    || /\.(?:test|spec)\.[^.]+$/.test(name)
    || /^test_.+\.py$/.test(name);
}

function buildCommands(files, withChecks, profile, lanePlan, rustOptions) {
  if (lanePlan.full) {
    return [profile === "quick" ? "pnpm run test:preflight" : "pnpm run check:all"];
  }
  const commands = [];
  if (withChecks) commands.push("pnpm run check:policy");
  if (lanePlan.lanes.contracts) commands.push("pnpm run check:contracts");
  if (lanePlan.lanes.rust_tests) {
    commands.push("pnpm run check:rust:target-health", ...rustAffectedTestCommands(files, rustOptions));
  }
  if (lanePlan.lanes.rust_static && withChecks) {
    commands.push("pnpm run format:rust:check", ...rustAffectedClippyCommands(files, rustOptions));
  }
  if (lanePlan.lanes.compatibility) {
    for (const capability of lanePlan.compatibilityCapabilities) commands.push(`pnpm run check:compatibility:${capability}`);
  }
  if (lanePlan.lanes.web) {
    commands.push(...webAffectedTestCommands(files));
    if (withChecks || profile === "full") commands.push("pnpm run typecheck:web");
  }
  if (lanePlan.lanes.pine) commands.push(profile === "quick" ? "pnpm run test:pineworker" : "pnpm run check:pine");
  if (lanePlan.lanes.python) commands.push("pnpm run check:python");
  if (lanePlan.lanes.desktop) commands.push("pnpm run check:desktop");
  return [...new Set(commands)];
}

export function planAffected(files, {
  withChecks = false, profile = "full", map = moduleMap, rustOptions, main = false,
} = {}) {
  if (!new Set(["full", "quick"]).has(profile)) throw new Error(`unknown affected-test profile: ${profile}`);
  const lanePlan = planGateLanes(files, { main, map });
  return {
    files,
    modules: resolveAffectedModules(files, map),
    ...lanePlan,
    commands: buildCommands(files, withChecks, profile, lanePlan, rustOptions),
    deferredCommands: profile === "quick"
      ? [lanePlan.full ? "pnpm run check:all" : lanePlan.lanes.rust_tests ? "pnpm run check:rust" : null].filter(Boolean)
      : [],
  };
}

function run(command) {
  console.log(`\n> ${command}`);
  return new Promise((complete) => {
    const child = spawn(command, { cwd: repoRoot, shell: true, stdio: "inherit" });
    child.once("error", () => complete(1));
    child.once("close", (status) => complete(status ?? 1));
  });
}

export function githubOutputs(plan) {
  return [
    `full=${String(plan.full)}`,
    `reason=${plan.reason}`,
    ...gateLaneNames.map((name) => `${name}=${String(plan.lanes[name])}`),
    `compatibility_capabilities=${JSON.stringify(plan.compatibilityCapabilities)}`,
  ].join("\n");
}

async function main() {
  const withChecks = process.argv.includes("--with-checks");
  const printOnly = process.argv.includes("--print");
  const jsonOnly = process.argv.includes("--json");
  const githubOutput = process.argv.includes("--github-output");
  const worktreeOnly = process.argv.includes("--worktree");
  const forceMain = process.argv.includes("--main")
    || (process.env.GITHUB_EVENT_NAME === "push" && process.env.GITHUB_REF === "refs/heads/main");
  const profile = process.argv.find((argument) => argument.startsWith("--profile="))?.slice(10) ?? "full";
  let files;
  let plan;
  try {
    files = changedFiles(repoRoot, worktreeOnly ? "HEAD" : undefined);
    plan = planAffected(files, { withChecks, profile, main: forceMain });
  } catch (error) {
    files = [];
    plan = {
      files, modules: [], commands: ["pnpm run check:all"], deferredCommands: [],
      ...fullGatePlan(`planner failed closed: ${error instanceof Error ? error.message : String(error)}`),
    };
  }
  if (jsonOnly) { console.log(JSON.stringify(plan, null, 2)); return; }
  if (githubOutput) {
    const output = `${githubOutputs(plan)}\n`;
    if (process.env.GITHUB_OUTPUT) fs.appendFileSync(process.env.GITHUB_OUTPUT, output);
    else process.stdout.write(output);
    return;
  }
  if (files.length === 0 && !plan.full) {
    console.log("No changed product files found relative to the configured base.");
    return;
  }
  console.log(`Affected scope: ${worktreeOnly ? "current worktree" : "merge base"} (${profile})`);
  console.log(`Planner: ${plan.full ? "full" : "affected"} - ${plan.reason}`);
  console.log(`Affected files: ${files.length}`);
  console.log(`Affected modules: ${plan.modules.map((module) => module.id).join(", ") || "none"}`);
  console.log(`Checks: ${plan.commands.join("; ") || "none"}`);
  if (plan.deferredCommands.length > 0) console.log(`Deferred integration checks: ${plan.deferredCommands.join("; ")}`);
  if (printOnly) return;
  for (const command of plan.commands) {
    if (await run(command) !== 0) { process.exitCode = 1; return; }
  }
}

if (path.resolve(process.argv[1] ?? "") === path.resolve(import.meta.filename)) await main();
