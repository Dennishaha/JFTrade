#!/usr/bin/env node
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

import { spawnChecked } from "./lib/spawn.mjs";

export const preflightChecks = [
  ["pnpm", ["run", "test:test-policy"]],
  ["pnpm", ["run", "check:test-names"]],
  ["pnpm", ["run", "check:test-quality"]],
  ["pnpm", ["run", "check:servercore-budget"]],
  ["pnpm", ["run", "check:openapi-quality"]],
  ["pnpm", ["run", "check:web-api-boundary"]],
  ["pnpm", ["run", "check:web-contract-index"]],
  ["pnpm", ["run", "check:web-contract-audit"]],
  ["pnpm", ["run", "check:web-openapi-imports"]],
  ["pnpm", ["run", "check:web-component-budget"]],
  ["pnpm", ["run", "test:pine-structure-corpus"]],
  ["pnpm", ["run", "lint:go"]],
  ["pnpm", ["run", "lint:go:errorlint"]],
  ["pnpm", ["run", "vet:go"]],
  ["pnpm", ["run", "test:coverage"]],
  ["pnpm", ["run", "typecheck"]],
  ["pnpm", ["run", "check:arch-deps"]],
];

const generateDocs = ["pnpm", ["run", "generate:docs"]];
const contractDriftCheck = [
  "git",
  [
    "diff",
    "--exit-code",
    "--",
    "docs/swagger",
    "apps/web/src/generated/openapi.ts",
    "tests/fixtures/openapi-baseline.json",
    "docs/reference/generated",
  ],
];

const layers = {
  preflight: [generateDocs, ...preflightChecks],
  "ci-local": [
    generateDocs,
    contractDriftCheck,
    ["pnpm", ["run", "audit:dependencies"]],
    ["pnpm", ["run", "check:oss-license"]],
    ...preflightChecks,
    ["go", ["build", "./..."]],
    ["go", ["test", "./cmd/...", "-count=1", "-timeout=300s"]],
    ["pnpm", ["run", "check:wails-bindings"]],
    ["pnpm", ["run", "test:desktop-release-metadata"]],
    ["pnpm", ["run", "test:desktop-release-artifacts"]],
    ["pnpm", ["run", "test:desktop-linux-artifacts"]],
    ["pnpm", ["run", "test:desktop-release-inputs"]],
    ["pnpm", ["run", "test:desktop-wails-tasks"]],
    ["pnpm", ["run", "test:desktop-signing"]],
    ["pnpm", ["run", "test:desktop-linux-package-config"]],
    ["node", ["scripts/dev-desktop.test.mjs"]],
    ["pnpm", ["run", "build:frontend-assets:generated"]],
    ["go", ["test", "-tags", "release_assets", "./internal/frontendassets", "-run", "TestFileSystem"]],
    ["pnpm", ["run", "build:pineworker"]],
    ["go", ["test", "-tags", "release_assets", "./internal/pineworkerassets", "-count=1"]],
    ["pnpm", ["run", "test:pinets-release-check"]],
    ["pnpm", ["run", "check:pinets-compliance"]],
    ["pnpm", ["run", "test:pinets-shadow-corpus"]],
    ["pnpm", ["run", "test:pineworker-asset-build"]],
  ],
  main: [
    ["pnpm", ["run", "test:ci-local"]],
    ["pnpm", ["run", "test:go"]],
    ["pnpm", ["run", "test:desktop"]],
    ["pnpm", ["run", "smoke:pinets-backtest"]],
  ],
};

export function commandsForLayer(layer) {
  if (!Object.hasOwn(layers, layer)) {
    throw new Error(`unknown test layer: ${String(layer)}`);
  }
  return layers[layer];
}

function main() {
  const layer = process.argv[2];
  if (process.argv.length !== 3 || !Object.hasOwn(layers, layer)) {
    console.error("Usage: node scripts/run-test-layer.mjs <preflight|ci-local|main>");
    process.exitCode = 2;
    return;
  }

  for (const [command, args] of commandsForLayer(layer)) {
    console.log(`\n> ${command} ${args.join(" ")}`);
    const status = spawnChecked(command, args);
    if (status !== 0) {
      process.exitCode = status;
      return;
    }
  }
}

if (resolve(process.argv[1] || "") === fileURLToPath(import.meta.url)) {
  main();
}
