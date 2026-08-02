#!/usr/bin/env node
import { spawnSync } from "node:child_process";
import { fileURLToPath, pathToFileURL } from "node:url";

const repositoryRoot = fileURLToPath(new URL("..", import.meta.url));

export const scriptTestSuites = Object.freeze({
  policy: Object.freeze([
    "scripts/check-arch-deps.test.mjs",
    "scripts/check-test-names.test.mjs",
    "scripts/report-go-test-quality.test.mjs",
    "scripts/check-servercore-budget.test.mjs",
    "scripts/check-openapi-quality.test.mjs",
    "scripts/generate-api-types.test.mjs",
    "scripts/check-web-api-boundary.test.mjs",
    "scripts/check-web-contract-index.test.mjs",
    "scripts/check-web-contract-audit.test.mjs",
    "scripts/check-web-openapi-imports.test.mjs",
    "scripts/check-web-component-budget.test.mjs",
    "scripts/check-web-diff-thresholds.test.mjs",
    "scripts/run-errorlint.test.mjs",
    "scripts/run-go-argument-forwarding.test.mjs",
    "scripts/run-test-layer.test.mjs",
    "scripts/test-scripts.test.mjs",
  ]),
  desktop: Object.freeze([
    "scripts/lib/desktop-release-metadata.test.mjs",
    "scripts/lib/desktop-release-artifacts.test.mjs",
    "scripts/manage-linux-release-artifacts.test.mjs",
    "scripts/lib/desktop-release-inputs.test.mjs",
    "scripts/desktop-wails-tasks.test.mjs",
    "scripts/sign-desktop-release.test.mjs",
    "scripts/prepare-linux-package-config.test.mjs",
    "scripts/dev-desktop.test.mjs",
    "scripts/build-yfinance-sidecar.test.mjs",
    "scripts/lib/materialize-directory-symlinks.test.mjs",
  ]),
  "api-release": Object.freeze(["scripts/api-release-scripts.test.mjs"]),
  "pinets-release": Object.freeze(["scripts/check-pinets-release.test.mjs"]),
  "pineworker-assets": Object.freeze([
    "scripts/build-pineworker-assets.test.mjs",
  ]),
  "pineworker-dev": Object.freeze(["scripts/build-pineworker-dev.test.mjs"]),
  "yfinance-assets": Object.freeze([
    "scripts/build-yfinance-sidecar.test.mjs",
  ]),
  "pine-benchmark": Object.freeze([
    "scripts/check-pine-benchmark-gates.test.mjs",
  ]),
  "web-bundle": Object.freeze(["scripts/report-web-bundle.test.mjs"]),
});

export function resolveScriptTestFiles(requestedSuites = []) {
  const suites = requestedSuites.length === 0 ? ["all"] : requestedSuites;
  const unknown = suites.filter(
    (name) => name !== "all" && !(name in scriptTestSuites),
  );
  if (unknown.length > 0) {
    throw new Error(`unknown script test suite: ${unknown.join(", ")}`);
  }
  const names = suites.includes("all")
    ? Object.keys(scriptTestSuites)
    : suites;
  return [...new Set(names.flatMap((name) => scriptTestSuites[name]))];
}

export function runScriptTests(requestedSuites = [], options = {}) {
  const files = resolveScriptTestFiles(requestedSuites);
  const result = spawnSync(process.execPath, ["--test", ...files], {
    cwd: options.cwd ?? repositoryRoot,
    env: options.env ?? process.env,
    stdio: options.stdio ?? "inherit",
  });
  if (result.error) {
    throw result.error;
  }
  return result.status ?? 1;
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

const invokedPath = process.argv[1]
  ? pathToFileURL(process.argv[1]).href
  : "";
if (invokedPath === import.meta.url) {
  process.exitCode = main(process.argv.slice(2));
}
