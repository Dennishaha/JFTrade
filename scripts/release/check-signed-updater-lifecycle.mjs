#!/usr/bin/env node

import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));
const defaultPaths = Object.freeze({
  updater: path.join(repositoryRoot, "apps/desktop/src-tauri/src/native_notification_updater.rs"),
  lifecycle: path.join(repositoryRoot, "apps/desktop/src-tauri/src/native_lifecycle.rs"),
  supervisor: path.join(repositoryRoot, "crates/jftrade-engine/src/product_runtime_supervisor.rs"),
});

function readSource(filePath, label) {
  try {
    return fs.readFileSync(filePath, "utf8");
  } catch (error) {
    throw new Error(`cannot read ${label} ${filePath}: ${error.message}`);
  }
}

function requireInOrder(source, first, second, label) {
  const firstIndex = source.indexOf(first);
  const secondIndex = source.indexOf(second);
  if (firstIndex < 0 || secondIndex < 0 || firstIndex >= secondIndex) {
    throw new Error(`${label} must occur before ${second}`);
  }
}

/**
 * Check the production source-level contract around updater installation.
 * This deliberately complements (rather than replaces) a native release
 * smoke: it makes the pre-install ordering reviewable on every host.
 */
export function inspectUpdaterInstallLifecycle({
  updaterPath = defaultPaths.updater,
  lifecyclePath = defaultPaths.lifecycle,
  supervisorPath = defaultPaths.supervisor,
} = {}) {
  const updater = readSource(updaterPath, "updater source");
  const lifecycle = readSource(lifecyclePath, "native lifecycle source");
  const supervisor = readSource(supervisorPath, "product shutdown supervisor source");
  requireInOrder(updater, "update.download", "before_install();", "updater download/pre-install hook");
  requireInOrder(updater, "before_install();", "update.install(bytes)", "updater pre-install/install hook");
  if (!lifecycle.includes("stop_product(&install_product);")) {
    throw new Error("native updater install hook must stop the retained product runtime");
  }
  if (!lifecycle.includes("handle.shutdown()")) {
    throw new Error("native product stop hook must await ProductRuntimeHandle.shutdown");
  }
  const requiredShutdownEvents = ["http_join", "marketdata_helper", "pine_worker"];
  for (const event of requiredShutdownEvents) {
    if (!supervisor.includes(`record(\"${event}\")`)) {
      throw new Error(`product shutdown supervisor must record ${event}`);
    }
  }
  requireInOrder(supervisor, 'record("http_join")', 'record("marketdata_helper")', "Rust API/worker shutdown");
  requireInOrder(supervisor, 'record("marketdata_helper")', 'record("pine_worker")', "market-data/Pine shutdown");
  return {
    updaterPath,
    lifecyclePath,
    supervisorPath,
    preInstallStopsProduct: true,
    shutdownEvidence: requiredShutdownEvents,
  };
}

function waitForClose(child, timeoutMillis) {
  return new Promise((resolve, reject) => {
    let settled = false;
    const timer = setTimeout(() => {
      if (settled) return;
      settled = true;
      child.kill();
      reject(new Error(`managed updater harness child did not stop within ${timeoutMillis}ms`));
    }, timeoutMillis);
    child.once("error", (error) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      reject(error);
    });
    child.once("close", (code, signal) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      resolve({ code, signal });
    });
  });
}

/**
 * Spawn disposable stand-ins for the Rust API, PineTS worker and Python
 * helper, then stop all of them before returning an "install" boundary. The
 * harness uses stdin instead of OS-specific signals so it is reproducible on
 * macOS, Linux and Windows CI. It proves ordering/cleanup only; it does not
 * qualify a signed artifact or native installer.
 */
export async function runUpdaterPreInstallHarness({
  spawnProcess,
  roles = ["rust-api", "pine-worker", "python-helper"],
  timeoutMillis = 2_000,
} = {}) {
  const { spawn } = await import("node:child_process");
  const spawnImpl = spawnProcess ?? spawn;
  const childSource = [
    "process.stdin.setEncoding('utf8');",
    "process.stdin.on('data', () => process.exit(0));",
    "setInterval(() => {}, 1000);",
  ].join(" ");
  const children = [];
  const events = [];
  try {
    for (const role of roles) {
      const child = spawnImpl(process.execPath, ["-e", childSource], {
        stdio: ["pipe", "ignore", "ignore"],
        env: { ...process.env, JFTRADE_UPDATER_HARNESS_ROLE: role },
      });
      children.push({ child, role });
      await new Promise((resolve, reject) => {
        child.once("spawn", resolve);
        child.once("error", reject);
      });
      events.push({ action: "started", role });
    }
    for (const { child, role } of children) {
      child.stdin.end("stop\n");
      const result = await waitForClose(child, timeoutMillis);
      if (result.code !== 0) {
        throw new Error(`managed updater harness child ${role} exited unexpectedly`);
      }
      events.push({ action: "stopped", role });
    }
    events.push({ action: "install" });
    return {
      roles: [...roles],
      events,
      allStoppedBeforeInstall: events.findIndex((event) => event.action === "install") === events.length - 1,
    };
  } finally {
    for (const { child } of children) {
      if (!child.killed && child.exitCode === null) child.kill();
    }
  }
}

function parseArgs(args) {
  const values = {};
  for (let index = 0; index < args.length; index += 1) {
    const argument = args[index];
    if (!argument.startsWith("--")) throw new Error(`unknown argument: ${argument}`);
    const [key, inline] = argument.slice(2).split("=", 2);
    const value = inline ?? args[++index];
    if (!value) throw new Error(`missing value for --${key}`);
    values[{ updater: "updaterPath", lifecycle: "lifecyclePath", supervisor: "supervisorPath" }[key] ?? key] = value;
  }
  return values;
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const report = inspectUpdaterInstallLifecycle(parseArgs(process.argv.slice(2)));
    console.log(`Verified updater pre-install shutdown ordering for ${report.shutdownEvidence.join(", ")}.`);
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  }
}
