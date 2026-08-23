#!/usr/bin/env node
import { spawn, spawnSync } from "node:child_process";
import { mkdirSync, mkdtempSync, readdirSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));
const commandTimeoutMs = 300_000;

export function stage9ReferenceEnvironment(temporaryRoot) {
  const dataManagementRoot = path.join(temporaryRoot, "data-management");
  const dataManagementCleanupRoot = path.join(temporaryRoot, "data-management-cleanup");
  mkdirSync(dataManagementRoot, { recursive: true });
  mkdirSync(dataManagementCleanupRoot, { recursive: true });
  return {
    JFTRADE_STAGE9_REAL_TRADE_REFERENCE: path.join(temporaryRoot, "go-real-trade-reference.json"),
    JFTRADE_STAGE9_BROKER_SETTINGS_REFERENCE: path.join(temporaryRoot, "go-broker-settings-reference.json"),
    JFTRADE_STAGE9_BROKER_SETTINGS_WRITE_REFERENCE: path.join(temporaryRoot, "go-broker-settings-write-reference.json"),
    JFTRADE_STAGE9_ONBOARDING_SETTINGS_WRITE_REFERENCE: path.join(temporaryRoot, "go-onboarding-settings-write-reference.json"),
    JFTRADE_STAGE9_PROVIDER_SETTINGS_WRITE_REFERENCE: path.join(temporaryRoot, "go-provider-settings-write-reference.json"),
    JFTRADE_STAGE9_MCP_SETTINGS_WRITE_REFERENCE: path.join(temporaryRoot, "go-mcp-settings-write-reference.json"),
    JFTRADE_STAGE9_SECURITY_SETTINGS_WRITE_REFERENCE: path.join(temporaryRoot, "go-security-settings-write-reference.json"),
    JFTRADE_STAGE9_ASSISTANT_AGENT_TEMPLATES_REFERENCE: path.join(temporaryRoot, "go-assistant-agent-templates-reference.json"),
    JFTRADE_STAGE9_DATA_MANAGEMENT_ROOT: dataManagementRoot,
    JFTRADE_STAGE9_DATA_MANAGEMENT_REFERENCE: path.join(dataManagementRoot, "go-reference.json"),
    JFTRADE_STAGE9_DATA_MANAGEMENT_CLEANUP_ROOT: dataManagementCleanupRoot,
    JFTRADE_STAGE9_DATA_MANAGEMENT_CLEANUP_REFERENCE: path.join(dataManagementCleanupRoot, "go-reference.json"),
  };
}

export function listEngineIntegrationTargets(root = repositoryRoot) {
  return readdirSync(path.join(root, "crates/jftrade-engine/tests"), { withFileTypes: true })
    .filter((entry) => entry.isFile() && entry.name.endsWith(".rs"))
    .map((entry) => entry.name.slice(0, -3))
    .sort();
}

export function stage9ProductCommands(extraEnv, integrationTargets = listEngineIntegrationTargets()) {
  const command = (label, executable, args) => ({ label, executable, args, extraEnv });
  return [
    command("Go Stage 9 fixture and reference corpus", "go", [
      "test", "./scripts/rust-migration", "-count=1", "-timeout=300s",
    ]),
    command("Go authenticated product rehearsals", "go", [
      "test", "./internal/app/apiserver/servercoretest", "-count=1", "-timeout=300s",
    ]),
    command("Go browser-session reference", "go", [
      "test", "./internal/app/apiserver/webaccess", "-count=1", "-timeout=300s",
    ]),
    command("Rust product library replay", "cargo", [
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
    temporaryRoot = mkdtempSync(path.join(tmpdir(), "jftrade-stage9-product-")),
  } = options;
  const ownsTemporaryRoot = options.temporaryRoot === undefined;
  try {
    const extraEnv = stage9ReferenceEnvironment(temporaryRoot);
    for (const specification of stage9ProductCommands(extraEnv, listEngineIntegrationTargets(root))) {
      await runner(specification, { root, timeoutMs: commandTimeoutMs });
    }
  } finally {
    if (ownsTemporaryRoot) rmSync(temporaryRoot, { recursive: true, force: true });
  }
}

function runCommand(specification, { root, timeoutMs }) {
  const { label, executable, args, extraEnv } = specification;
  console.log(`\n> ${label}\n> ${executable} ${args.join(" ")}`);
  const startedAt = Date.now();
  return new Promise((resolve, reject) => {
    const child = spawn(executable, args, {
      cwd: root,
      detached: process.platform !== "win32",
      env: { ...process.env, ...extraEnv },
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
      "Go/Rust Stage 9 product differential passed: batched Go fixtures, authenticated rehearsals, Rust product routes, integration fixtures, and supporting package contracts.",
    );
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  }
}

if (path.resolve(process.argv[1] || "") === fileURLToPath(import.meta.url)) {
  await main();
}
