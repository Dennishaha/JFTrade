#!/usr/bin/env node
import { spawnChecked } from "./lib/spawn.mjs";

const layer = process.argv[2];
const layers = {
  pr: [
    ["pnpm", ["run", "lint:go"]],
    ["pnpm", ["run", "vet:go"]],
    ["pnpm", ["run", "test:coverage"]],
    ["pnpm", ["run", "typecheck"]],
    ["pnpm", ["run", "check:arch-deps"]],
    ["pnpm", ["run", "check:test-names"]],
  ],
  main: [
    ["pnpm", ["run", "test:pr"]],
    ["pnpm", ["run", "test:go"]],
    ["pnpm", ["run", "test:desktop"]],
    ["pnpm", ["run", "smoke:pinets-backtest"]],
  ],
  nightly: [
    ["go", ["test", "-race", "./internal/marketdata", "./internal/integration/futu", "./internal/trading", "./pkg/adk", "-count=1", "-timeout=300s"]],
    ["go", ["test", "./internal/marketdata", "./internal/integration/futu", "-run", "Concurrent", "-count=20", "-timeout=120s"]],
    ["pnpm", ["run", "smoke:pinets-backtest"]],
  ],
};

if (!Object.hasOwn(layers, layer) || process.argv.length !== 3) {
  console.error("Usage: node scripts/run-test-layer.mjs <pr|main|nightly>");
  process.exit(2);
}

for (const [command, args] of layers[layer]) {
  console.log(`\n> ${command} ${args.join(" ")}`);
  const status = spawnChecked(command, args);
  if (status !== 0) {
    process.exit(status);
  }
}
