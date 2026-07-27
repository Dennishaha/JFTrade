#!/usr/bin/env node
import { spawnChecked } from "./lib/spawn.mjs";

const layer = process.argv[2];
const layers = {
  preflight: [
    ["pnpm", ["run", "test:test-policy"]],
    ["pnpm", ["run", "check:test-names"]],
    ["pnpm", ["run", "check:test-quality"]],
    ["pnpm", ["run", "check:servercore-budget"]],
    ["pnpm", ["run", "check:openapi-quality"]],
    ["pnpm", ["run", "check:web-api-boundary"]],
    ["pnpm", ["run", "check:web-contract-index"]],
    ["pnpm", ["run", "check:web-contract-audit"]],
    ["pnpm", ["run", "lint:go"]],
    ["pnpm", ["run", "vet:go"]],
    ["pnpm", ["run", "test:coverage"]],
    ["pnpm", ["run", "typecheck"]],
    ["pnpm", ["run", "check:arch-deps"]],
  ],
  "ci-local": [
    ["pnpm", ["run", "generate:docs"]],
    [
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
    ],
    ["pnpm", ["run", "audit:dependencies"]],
    ["pnpm", ["run", "check:oss-license"]],
    ["pnpm", ["run", "test:preflight"]],
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

if (!Object.hasOwn(layers, layer) || process.argv.length !== 3) {
  console.error("Usage: node scripts/run-test-layer.mjs <preflight|ci-local|main>");
  process.exit(2);
}

for (const [command, args] of layers[layer]) {
  console.log(`\n> ${command} ${args.join(" ")}`);
  const status = spawnChecked(command, args);
  if (status !== 0) {
    process.exit(status);
  }
}
