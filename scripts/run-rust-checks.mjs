#!/usr/bin/env node
import { resolve } from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

import { runExecutionStages } from "./run-test-layer.mjs";

const pnpmRun = (script) => ["pnpm", ["run", script]];
const sequentialStage = (...commands) => ({ mode: "sequential", commands });
const parallelStage = (...commands) => ({ mode: "parallel", commands });
const healthStage = sequentialStage(pnpmRun("check:rust:target-health"));

const workspaceCoreStages = Object.freeze([
  parallelStage(
    pnpmRun("check:rust:layout"),
    pnpmRun("test:rust:stage9:route-coverage"),
    pnpmRun("format:rust:check"),
  ),
  sequentialStage(
    pnpmRun("lint:rust"),
    pnpmRun("test:rust"),
  ),
]);

const differentialCoreStages = Object.freeze([
  parallelStage(
    pnpmRun("test:rust:differential"),
    pnpmRun("test:rust:backtest:differential"),
  ),
  parallelStage(
    pnpmRun("test:rust:stage4:differential"),
    pnpmRun("test:rust:stage5:differential"),
  ),
  parallelStage(
    pnpmRun("test:rust:stage6:differential"),
    pnpmRun("test:rust:stage7:differential"),
  ),
  sequentialStage(pnpmRun("test:rust:stage8:differential")),
  sequentialStage(pnpmRun("test:rust:stage9:product-differential")),
]);

const rustGateStages = Object.freeze({
  workspace: Object.freeze([healthStage, ...workspaceCoreStages]),
  differential: Object.freeze([healthStage, ...differentialCoreStages]),
  full: Object.freeze([healthStage, ...workspaceCoreStages, ...differentialCoreStages]),
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
    console.error("Usage: node scripts/run-rust-checks.mjs <workspace|differential|full>");
    process.exitCode = 2;
    return;
  }
  process.exitCode = await runExecutionStages(executionStagesForRustGate(gate));
}

if (resolve(process.argv[1] || "") === fileURLToPath(import.meta.url)) {
  await main();
}
