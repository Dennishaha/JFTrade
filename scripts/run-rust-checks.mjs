#!/usr/bin/env node
import { resolve } from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

import { runExecutionStages } from "./run-test-layer.mjs";

const pnpmRun = (script) => ["pnpm", ["run", script]];
const sequentialStage = (...commands) => ({ mode: "sequential", commands });
const parallelStage = (...commands) => ({ mode: "parallel", commands });
const staticStages = Object.freeze([
  sequentialStage(pnpmRun("check:rust:target-health")),
  parallelStage(
    pnpmRun("check:rust:architecture"),
    pnpmRun("check:rust:production-policy"),
    pnpmRun("format:rust:check"),
  ),
  sequentialStage(pnpmRun("lint:rust"), pnpmRun("check:rust:policy")),
]);

const workspaceStages = Object.freeze([
  sequentialStage(pnpmRun("check:rust:target-health"), pnpmRun("test:rust")),
  sequentialStage(pnpmRun("check:compatibility")),
]);

const rustGateStages = Object.freeze({
  static: staticStages,
  workspace: workspaceStages,
  full: Object.freeze([
    ...staticStages,
    sequentialStage(pnpmRun("test:rust"), pnpmRun("check:compatibility")),
  ]),
});

export function executionStagesForRustGate(gate) {
  if (!Object.hasOwn(rustGateStages, gate)) {
    throw new Error(`unknown Rust gate: ${String(gate)}`);
  }
  return rustGateStages[gate];
}

async function main() {
  const gate = process.argv[2];
  if (process.argv.length !== 3 || !Object.hasOwn(rustGateStages, gate)) {
    console.error("Usage: node scripts/run-rust-checks.mjs <static|workspace|full>");
    process.exitCode = 2;
    return;
  }
  process.exitCode = await runExecutionStages(executionStagesForRustGate(gate));
}

if (resolve(process.argv[1] || "") === fileURLToPath(import.meta.url)) {
  await main();
}
