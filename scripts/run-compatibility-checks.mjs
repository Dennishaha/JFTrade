#!/usr/bin/env node
import { resolve } from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

import { runExecutionStages } from "./run-test-layer.mjs";

const capabilityScripts = Object.freeze({
  storage: "check:compatibility:storage",
  backtest: "check:compatibility:backtest",
  "provider-runtime": "check:compatibility:provider-runtime",
  "trading-strategy": "check:compatibility:trading-strategy",
  "assistant-runtime": "check:compatibility:assistant-runtime",
  "api-transport": "check:compatibility:api-transport",
  "desktop-runtime": "check:compatibility:desktop-runtime",
});

const pnpmRun = (script) => ["pnpm", ["run", script]];

export function compatibilityStages(capability = "all") {
  if (capability === "all") {
    return [
      { mode: "sequential", commands: [pnpmRun("check:compatibility:manifests")] },
      { mode: "parallel", commands: Object.values(capabilityScripts).map(pnpmRun) },
    ];
  }
  if (!Object.hasOwn(capabilityScripts, capability)) {
    throw new Error(`unknown compatibility capability: ${String(capability)}`);
  }
  return [{ mode: "sequential", commands: [pnpmRun(capabilityScripts[capability])] }];
}

async function main() {
  const capability = process.argv[2] ?? "all";
  if (process.argv.length > 3) {
    console.error("Usage: node scripts/run-compatibility-checks.mjs [all|capability]");
    process.exitCode = 2;
    return;
  }
  try {
    process.exitCode = await runExecutionStages(compatibilityStages(capability));
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 2;
  }
}

if (resolve(process.argv[1] || "") === fileURLToPath(import.meta.url)) {
  await main();
}
