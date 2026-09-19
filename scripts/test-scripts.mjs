#!/usr/bin/env node
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const repositoryRoot = fileURLToPath(new URL("..", import.meta.url));

function discoverTests(directory = path.join(repositoryRoot, "scripts")) {
  return fs.readdirSync(directory, { recursive: true, withFileTypes: true })
    .filter((entry) => entry.isFile() && entry.name.endsWith(".test.mjs"))
    .map((entry) => path.relative(repositoryRoot, path.join(entry.parentPath, entry.name)).split(path.sep).join("/"))
    .sort();
}

function discoverPythonTests(directory = path.join(repositoryRoot, "scripts", "compatibility")) {
  if (!fs.existsSync(directory)) return [];
  return fs.readdirSync(directory, { withFileTypes: true })
    .filter((entry) => entry.isFile() && /^test_.*\.py$/.test(entry.name))
    .map((entry) => path.relative(repositoryRoot, path.join(directory, entry.name)).split(path.sep).join("/"))
    .sort();
}

const allTests = discoverTests();
const allPythonTests = discoverPythonTests();
const rootPolicyNames = new Set([
  "check-ai-context.test.mjs",
  "check-diff.test.mjs",
  "check-openapi-quality.test.mjs",
  "check-rust-target-health.test.mjs",
  "check-rust-toolchain-bootstrap.test.mjs",
  "check-test-names.test.mjs",
  "check-web-api-boundary.test.mjs",
  "check-web-component-budget.test.mjs",
  "check-web-contract-audit.test.mjs",
  "check-web-contract-index.test.mjs",
  "check-web-diff-thresholds.test.mjs",
  "check-web-file-length-budget.test.mjs",
  "check-web-openapi-imports.test.mjs",
  "check-zero-go.test.mjs",
  "generate-api-types.test.mjs",
  "generate-contracts.test.mjs",
  "run-compatibility-checks.test.mjs",
  "run-rust-checks.test.mjs",
  "run-test-layer.test.mjs",
  "test-affected.test.mjs",
  "test-scripts.test.mjs",
]);

function matchingTests(predicate) {
  return Object.freeze(allTests.filter(predicate));
}

export const scriptTestSuites = Object.freeze({
  policy: matchingTests((file) => file.startsWith("scripts/quality/") || rootPolicyNames.has(path.basename(file))),
  compatibility: matchingTests((file) => file.startsWith("scripts/compatibility/")),
  release: matchingTests((file) => file.startsWith("scripts/release/") || [
    "scripts/check-desktop-release-policy.test.mjs",
    "scripts/check-desktop-release-workflow.test.mjs",
  ].includes(file)),
  desktop: matchingTests((file) => /(?:desktop|tauri|linux-package|marketdata-sidecar|materialize-directory-symlinks)/.test(file)),
  "api-release": Object.freeze(["scripts/api-release-scripts.test.mjs"]),
  "pinets-release": Object.freeze(["scripts/check-pinets-release.test.mjs"]),
  "pineworker-assets": Object.freeze(["scripts/build-pineworker-assets.test.mjs"]),
  "pineworker-dev": Object.freeze(["scripts/build-pineworker-dev.test.mjs"]),
  "marketdata-assets": Object.freeze([
    "scripts/build-marketdata-sidecar.test.mjs",
    "scripts/smoke-marketdata-sidecar.test.mjs",
  ]),
  "web-bundle": Object.freeze([
    "scripts/lib/monaco-layout.test.mjs",
    "scripts/report-web-bundle.test.mjs",
  ]),
});

export const pythonScriptTestSuites = Object.freeze({
  compatibility: Object.freeze(allPythonTests),
});

export function resolveScriptTestFiles(requestedSuites = []) {
  const suites = requestedSuites.length === 0 ? ["all"] : requestedSuites;
  const unknown = suites.filter((name) => name !== "all" && !(name in scriptTestSuites));
  if (unknown.length > 0) throw new Error(`unknown script test suite: ${unknown.join(", ")}`);
  if (suites.includes("all")) return [...allTests];
  return [...new Set(suites.flatMap((name) => scriptTestSuites[name]))];
}

export function resolvePythonScriptTestFiles(requestedSuites = []) {
  const suites = requestedSuites.length === 0 ? ["all"] : requestedSuites;
  if (suites.includes("all")) return [...allPythonTests];
  return [...new Set(suites.flatMap((name) => pythonScriptTestSuites[name] ?? []))];
}

function resolvePythonInterpreter() {
  for (const candidate of ["python3", "python"]) {
    const probe = spawnSync(candidate, ["--version"], { stdio: "ignore" });
    if (!probe.error && probe.status === 0) return candidate;
  }
  throw new Error("python3 is required to run the scripts/compatibility Python tests");
}

export function runPythonScriptTests(files, options = {}) {
  if (files.length === 0) return 0;
  const python = resolvePythonInterpreter();
  const directories = [...new Set(files.map((file) => path.dirname(file)))];
  for (const directory of directories) {
    const result = spawnSync(python, ["-m", "unittest", "discover", "-s", directory, "-p", "test_*.py"], {
      cwd: options.cwd ?? repositoryRoot,
      env: options.env ?? process.env,
      stdio: options.stdio ?? "inherit",
    });
    if (result.error) throw result.error;
    if ((result.status ?? 1) !== 0) return result.status ?? 1;
  }
  return 0;
}

export function runScriptTests(requestedSuites = [], options = {}) {
  const files = resolveScriptTestFiles(requestedSuites);
  const result = spawnSync(process.execPath, ["--test", ...files], {
    cwd: options.cwd ?? repositoryRoot,
    env: options.env ?? process.env,
    stdio: options.stdio ?? "inherit",
  });
  if (result.error) throw result.error;
  if ((result.status ?? 1) !== 0) return result.status ?? 1;
  return runPythonScriptTests(resolvePythonScriptTestFiles(requestedSuites), options);
}

export function scriptTestUsage() {
  return [
    "Usage: node scripts/test-scripts.mjs [suite ...]",
    "",
    `Suites: all, ${Object.keys(scriptTestSuites).join(", ")}`,
    "No suite is equivalent to 'all'.",
  ].join("\n");
}

function main(args) {
  const commandArgs = args.filter((argument) => argument !== "--");
  if (commandArgs.includes("--help") || commandArgs.includes("-h")) {
    console.log(scriptTestUsage());
    return 0;
  }
  if (commandArgs.includes("--list")) {
    console.log(["all", ...Object.keys(scriptTestSuites)].join("\n"));
    return 0;
  }
  try {
    return runScriptTests(commandArgs);
  } catch (error) {
    console.error(error.message);
    console.error(scriptTestUsage());
    return 1;
  }
}

const invokedPath = process.argv[1] ? pathToFileURL(process.argv[1]).href : "";
if (invokedPath === import.meta.url) process.exitCode = main(process.argv.slice(2));
