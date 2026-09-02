#!/usr/bin/env node
import { spawn, spawnSync } from "node:child_process";
import { readdirSync } from "node:fs";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));
const defaultCommandTimeoutMs = 300_000;
const maximumCommandTimeoutMs = 1_800_000;

export function resolveCommandTimeoutMs(environment = process.env) {
  const value = environment.JFTRADE_STAGE9_PRODUCT_TIMEOUT_MS;
  if (value === undefined || value === "") return defaultCommandTimeoutMs;
  if (!/^[1-9][0-9]*$/.test(value)) {
    throw new Error("JFTRADE_STAGE9_PRODUCT_TIMEOUT_MS must be a positive integer");
  }
  const timeoutMs = Number(value);
  if (!Number.isSafeInteger(timeoutMs) || timeoutMs > maximumCommandTimeoutMs) {
    throw new Error(
      `JFTRADE_STAGE9_PRODUCT_TIMEOUT_MS must not exceed ${maximumCommandTimeoutMs}`,
    );
  }
  return timeoutMs;
}

export function listEngineIntegrationTargets(root = repositoryRoot) {
  return readdirSync(path.join(root, "crates/jftrade-engine/tests"), { withFileTypes: true })
    .filter((entry) => entry.isFile() && entry.name.endsWith(".rs"))
    .map((entry) => entry.name.slice(0, -3))
    .sort();
}

export function stage9ProductCommands(integrationTargets = listEngineIntegrationTargets()) {
  const command = (label, executable, args) => ({ label, executable, args });
  return [
    command("Rust product compatibility replay", "cargo", [
      "test", "-p", "jftrade-engine", "--lib",
    ]),
    command("Rust Stage 9 integration replay", "cargo", [
      "test", "-p", "jftrade-engine",
      ...integrationTargets.flatMap((target) => ["--test", target]),
    ]),
    command("Rust supporting package replay", "cargo", [
      "test",
      "-p", "jftrade-store-settings-file",
      "-p", "jftrade-integration-futu",
      "-p", "jftrade-calendar",
    ]),
  ];
}

export async function runStage9ProductDifferential(options = {}) {
  const {
    root = repositoryRoot,
    runner = runCommand,
    timeoutMs = resolveCommandTimeoutMs(),
  } = options;
  for (const specification of stage9ProductCommands(listEngineIntegrationTargets(root))) {
    await runner(specification, { root, timeoutMs });
  }
}

function runCommand(specification, { root, timeoutMs }) {
  const { label, executable, args } = specification;
  console.log(`\n> ${label}\n> ${executable} ${args.join(" ")}`);
  const startedAt = Date.now();
  return new Promise((resolve, reject) => {
    const child = spawn(executable, args, {
      cwd: root,
      detached: process.platform !== "win32",
      env: process.env,
      stdio: "inherit",
    });
    let timedOut = false;
    const heartbeat = setInterval(() => {
      console.log(`> ${label} still running (${formatDuration(Date.now() - startedAt)})`);
    }, 30_000);
    heartbeat.unref?.();
    const timeout = setTimeout(() => {
      timedOut = true;
      terminateProcessTree(child);
    }, timeoutMs);
    timeout.unref?.();
    const forwardSignal = () => terminateProcessTree(child);
    process.once("SIGINT", forwardSignal);
    process.once("SIGTERM", forwardSignal);

    const finish = () => {
      clearInterval(heartbeat);
      clearTimeout(timeout);
      process.off("SIGINT", forwardSignal);
      process.off("SIGTERM", forwardSignal);
    };
    child.once("error", (error) => {
      finish();
      reject(error);
    });
    child.once("close", (status, signal) => {
      finish();
      const elapsed = formatDuration(Date.now() - startedAt);
      if (status === 0) {
        console.log(`> ${label} completed in ${elapsed}`);
        resolve();
        return;
      }
      const reason = timedOut
        ? `timed out after ${timeoutMs}ms`
        : `failed with ${signal ? `signal ${signal}` : `exit ${status}`}`;
      reject(new Error(`${label} ${reason} (${elapsed})`));
    });
  });
}

function terminateProcessTree(child) {
  if (!child.pid || child.killed) return;
  if (process.platform === "win32") {
    spawnSync("taskkill", ["/pid", String(child.pid), "/t", "/f"], { stdio: "ignore" });
    return;
  }
  try {
    process.kill(-child.pid, "SIGTERM");
  } catch {
    child.kill("SIGTERM");
  }
}

function formatDuration(milliseconds) {
  return `${(milliseconds / 1_000).toFixed(1)}s`;
}

async function main() {
  try {
    await runStage9ProductDifferential();
    console.log(
      "Rust Stage 9 compatibility replay passed: product routes, integration fixtures, and supporting package contracts.",
    );
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  }
}

if (path.resolve(process.argv[1] || "") === fileURLToPath(import.meta.url)) {
  await main();
}
