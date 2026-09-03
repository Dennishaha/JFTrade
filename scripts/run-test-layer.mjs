#!/usr/bin/env node
import { spawn } from "node:child_process";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

import { spawnChecked } from "./lib/spawn.mjs";

const pnpmRun = (script, ...args) => ["pnpm", ["run", script, ...args]];
const sequentialStage = (...commands) => ({ mode: "sequential", commands });
const parallelStage = (...commands) => ({ mode: "parallel", commands });

export const policyChecks = Object.freeze([
  pnpmRun("check:zero-go"),
  pnpmRun("check:rust:architecture"),
  pnpmRun("check:rust:production-policy"),
  pnpmRun("check:product-gate-policy"),
  pnpmRun("check:test-names"),
  pnpmRun("check:ai-context"),
  pnpmRun("test:scripts", "--", "policy"),
  pnpmRun("check:actionlint"),
]);

export const contractChecks = Object.freeze([
  pnpmRun("check:generated"),
  pnpmRun("check:route-contracts"),
  pnpmRun("check:openapi-quality"),
  pnpmRun("check:web-api-boundary"),
  pnpmRun("check:web-contract-index"),
  pnpmRun("check:web-contract-audit"),
  pnpmRun("check:web-openapi-imports"),
]);

const coreProductChecks = Object.freeze([
  pnpmRun("check:rust:static"),
  pnpmRun("check:rust:workspace"),
  pnpmRun("check:web"),
  pnpmRun("check:pine"),
  pnpmRun("check:python"),
]);

const completeProductChecks = Object.freeze([
  pnpmRun("audit:dependencies"),
  pnpmRun("check:oss-license"),
  pnpmRun("check:desktop"),
  pnpmRun("test:scripts", "--", "release", "desktop"),
  pnpmRun("build:frontend-assets:generated"),
  ["node", ["scripts/report-web-bundle.mjs"]],
  pnpmRun("build:pineworker"),
  pnpmRun("test:pineworker-asset-build"),
  pnpmRun("test:marketdata-sidecar-asset-build"),
  pnpmRun("build:marketdata-sidecar"),
  pnpmRun("smoke:marketdata-sidecar"),
  pnpmRun("smoke:pinets-backtest"),
]);

const layerStages = Object.freeze({
  policy: Object.freeze([parallelStage(...policyChecks)]),
  contracts: Object.freeze([parallelStage(...contractChecks)]),
  preflight: Object.freeze([
    parallelStage(pnpmRun("check:policy"), pnpmRun("check:contracts")),
    parallelStage(...coreProductChecks),
  ]),
  "ci-local": Object.freeze([
    parallelStage(pnpmRun("check:policy"), pnpmRun("check:contracts")),
    parallelStage(...coreProductChecks),
    sequentialStage(...completeProductChecks.slice(0, 2)),
  ]),
  main: Object.freeze([
    parallelStage(pnpmRun("check:policy"), pnpmRun("check:contracts")),
    parallelStage(...coreProductChecks),
    sequentialStage(...completeProductChecks),
  ]),
});

export function executionStagesForLayer(layer) {
  if (!Object.hasOwn(layerStages, layer)) throw new Error(`unknown test layer: ${String(layer)}`);
  return layerStages[layer];
}

export function commandsForLayer(layer) {
  return executionStagesForLayer(layer).flatMap(({ commands }) => commands);
}

export async function runExecutionStages(stages, options = {}) {
  const stdout = options.stdout ?? process.stdout;
  const stderr = options.stderr ?? process.stderr;
  const runSequential = options.runSequential ?? runSequentialCommand;
  const runParallel = options.runParallel ?? runBufferedCommand;
  for (const stage of stages) {
    if (stage.mode === "parallel") {
      const status = await runParallelStage(stage.commands, runParallel, stdout, stderr);
      if (status !== 0) return status;
      continue;
    }
    for (const command of stage.commands) {
      writeCommandHeader(stdout, command);
      const status = await runSequential(command);
      if (status !== 0) return status;
    }
  }
  return 0;
}

async function runParallelStage(commands, runner, stdout, stderr) {
  stdout.write(`\n> running ${commands.length} independent checks in parallel\n`);
  for (const command of commands) stdout.write(`  - ${formatCommand(command)}\n`);
  const results = await Promise.all(commands.map(async (command) => {
    const startedAt = Date.now();
    const heartbeat = setInterval(() => {
      stdout.write(`  ... ${formatCommand(command)} still running (${formatDuration(Date.now() - startedAt)})\n`);
    }, 30_000);
    heartbeat.unref?.();
    try {
      return { ...normalizeParallelResult(await runner(command)), elapsedMs: Date.now() - startedAt };
    } catch (error) {
      return { status: 1, stdout: "", stderr: `${errorMessage(error)}\n`, elapsedMs: Date.now() - startedAt };
    } finally {
      clearInterval(heartbeat);
    }
  }));
  for (const [index, result] of results.entries()) {
    writeCommandHeader(stdout, commands[index]);
    stdout.write(`> completed in ${formatDuration(result.elapsedMs)}\n`);
    if (result.stdout) stdout.write(result.stdout);
    if (result.stderr) stderr.write(result.stderr);
  }
  const failures = results.flatMap((result, index) => result.status === 0 ? [] : [{ command: commands[index], status: result.status }]);
  if (failures.length === 0) return 0;
  stderr.write("\nParallel check failures:\n");
  for (const failure of failures) stderr.write(`- ${formatCommand(failure.command)} (exit ${failure.status})\n`);
  return failures[0].status;
}

function runSequentialCommand([command, args]) {
  return spawnChecked(command, args);
}

function runBufferedCommand([command, args]) {
  return new Promise((complete) => {
    const stdout = [];
    const stderr = [];
    let stdoutBytes = 0;
    let stderrBytes = 0;
    let completed = false;
    const finish = (status) => {
      if (completed) return;
      completed = true;
      complete({
        status: normalizeStatus(status),
        stdout: Buffer.concat(stdout).toString(),
        stderr: Buffer.concat(stderr).toString(),
      });
    };
    const child = spawn(command, args, { shell: process.platform === "win32", stdio: ["ignore", "pipe", "pipe"] });
    child.stdout.on("data", (chunk) => { stdoutBytes = appendBufferedOutput(stdout, stdoutBytes, chunk); });
    child.stderr.on("data", (chunk) => { stderrBytes = appendBufferedOutput(stderr, stderrBytes, chunk); });
    child.once("error", (error) => { stderr.push(Buffer.from(`${error.message}\n`)); finish(1); });
    child.once("close", (status, signal) => {
      if (signal) stderr.push(Buffer.from(`process terminated by ${signal}\n`));
      finish(status);
    });
  });
}

function normalizeParallelResult(result) {
  return { status: normalizeStatus(result?.status), stdout: String(result?.stdout ?? ""), stderr: String(result?.stderr ?? "") };
}
function normalizeStatus(status) { return Number.isInteger(status) && status >= 0 ? status : 1; }
function writeCommandHeader(output, command) { output.write(`\n> ${formatCommand(command)}\n`); }
function formatCommand([command, args]) { return `${command} ${args.join(" ")}`; }
function errorMessage(error) { return error instanceof Error ? error.message : String(error); }
function formatDuration(milliseconds) { return `${(milliseconds / 1_000).toFixed(1)}s`; }

function appendBufferedOutput(chunks, currentBytes, chunk, limit = 4 * 1024 * 1024) {
  if (currentBytes >= limit) return currentBytes;
  const value = Buffer.from(chunk);
  const remaining = limit - currentBytes;
  if (value.length <= remaining) { chunks.push(value); return currentBytes + value.length; }
  chunks.push(value.subarray(0, remaining));
  chunks.push(Buffer.from("\n[output truncated by test runner]\n"));
  return limit;
}

async function main() {
  const layer = process.argv[2];
  if (process.argv.length !== 3 || !Object.hasOwn(layerStages, layer)) {
    console.error("Usage: node scripts/run-test-layer.mjs <policy|contracts|preflight|ci-local|main>");
    process.exitCode = 2;
    return;
  }
  process.exitCode = await runExecutionStages(executionStagesForLayer(layer));
}

if (resolve(process.argv[1] || "") === fileURLToPath(import.meta.url)) await main();
