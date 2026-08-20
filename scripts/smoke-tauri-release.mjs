#!/usr/bin/env node

import { execFileSync, spawn } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

const repositoryRoot = fileURLToPath(new URL("..", import.meta.url));

export function releaseBundlePaths({
  root = repositoryRoot,
  platform = process.platform,
  executableOverride = process.env.JFTRADE_TAURI_BINARY,
} = {}) {
  if (executableOverride) {
    const executable = path.resolve(executableOverride);
    return { executable, resourceRoot: path.resolve(path.dirname(executable), "../Resources") };
  }
  if (platform === "darwin") {
    const bundle = path.join(root, "target/release/bundle/macos/JFTrade.app");
    return {
      executable: path.join(bundle, "Contents/MacOS/jftrade-desktop"),
      resourceRoot: path.join(bundle, "Contents/Resources"),
    };
  }
  if (platform === "win32") {
    return {
      executable: path.join(root, "target/release/jftrade-desktop.exe"),
      resourceRoot: path.join(root, "target/release"),
    };
  }
  return {
    executable: path.join(root, "target/release/jftrade-desktop"),
    resourceRoot: path.join(root, "target/release"),
  };
}

function requireFile(filePath, label) {
  const stat = fs.statSync(filePath);
  if (!stat.isFile() || stat.size === 0) throw new Error(`${label} is invalid: ${filePath}`);
}

async function waitFor(predicate, timeoutMs, label) {
  const deadline = Date.now() + timeoutMs;
  let lastError;
  while (Date.now() < deadline) {
    try {
      const value = await predicate();
      if (value) return value;
    } catch (error) {
      lastError = error;
    }
    await new Promise((resolve) => setTimeout(resolve, 100));
  }
  throw new Error(`${label} timed out after ${timeoutMs}ms${lastError ? `: ${lastError}` : ""}`);
}

async function requestStatus() {
  const response = await fetch("http://127.0.0.1:6699/api/v1/system/status");
  return { body: await response.json(), status: response.status };
}

function findFile(root, predicate, label) {
  const pending = [root];
  while (pending.length > 0) {
    const current = pending.pop();
    for (const entry of fs.readdirSync(current, { withFileTypes: true })) {
      const child = path.join(current, entry.name);
      if (entry.isDirectory()) pending.push(child);
      else if (predicate(entry.name)) return child;
    }
  }
  throw new Error(`${label} was not created below ${root}`);
}

function desktopLog(root) {
  return findFile(
    root,
    (name) => /^desktop-\d{4}-\d{2}-\d{2}\.log$/.test(name),
    "desktop log",
  );
}

function assertNoPackagedOrphans(resourceRoot) {
  if (process.platform === "win32") return;
  const processes = execFileSync("ps", ["-axo", "command"], { encoding: "utf8" });
  const orphan = processes
    .split("\n")
    .find((line) => line.includes(resourceRoot) && /runtime\/(node|marketdata)/.test(line));
  if (orphan) throw new Error(`packaged child process survived desktop exit: ${orphan.trim()}`);
}

export async function smokeTauriRelease(options = {}) {
  const bundle = releaseBundlePaths(options);
  requireFile(bundle.executable, "Tauri release executable");
  for (const relative of [
    "runtime/node/manifest.json",
    process.platform === "win32" ? "runtime/node/node.exe" : "runtime/node/node",
    "runtime/pineworker/worker.mjs",
    "runtime/pineworker/proto/pineworker.proto",
  ]) {
    requireFile(path.join(bundle.resourceRoot, relative), `bundled ${relative}`);
  }
  try {
    const existing = await requestStatus();
    throw new Error(`port 6699 is already occupied (HTTP ${existing.status})`);
  } catch (error) {
    if (String(error).includes("already occupied")) throw error;
  }

  const isolatedHome = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-tauri-smoke-"));
  const startedAt = Date.now();
  const child = spawn(bundle.executable, [], {
    env: {
      ...process.env,
      APPDATA: path.join(isolatedHome, "AppData/Roaming"),
      HOME: isolatedHome,
      LOCALAPPDATA: path.join(isolatedHome, "AppData/Local"),
      XDG_DATA_HOME: path.join(isolatedHome, ".local/share"),
    },
    stdio: "ignore",
  });
  let exited = false;
  const exit = new Promise((resolve) => {
    child.once("exit", (code, signal) => {
      exited = true;
      resolve({ code, signal });
    });
  });
  try {
    const response = await waitFor(requestStatus, 20_000, "Tauri product readiness");
    if (response.status !== 401 || response.body?.error?.code !== "WEB_AUTH_REQUIRED") {
      throw new Error(`unauthenticated packaged API did not fail closed: ${JSON.stringify(response)}`);
    }
    const readyMs = Date.now() - startedAt;
    child.kill(process.platform === "win32" ? "SIGTERM" : "SIGINT");
    const outcome = await Promise.race([
      exit,
      new Promise((_, reject) =>
        setTimeout(() => reject(new Error("Tauri shutdown exceeded 5 seconds")), 5_000),
      ),
    ]);
    if (outcome.code !== 0) {
      throw new Error(`Tauri release exited with code=${outcome.code} signal=${outcome.signal}`);
    }
    const shutdownMs = Date.now() - startedAt - readyMs;
    await waitFor(async () => {
      try {
        await requestStatus();
        return false;
      } catch {
        return true;
      }
    }, 5_000, "Tauri API shutdown");
    const log = fs.readFileSync(desktopLog(isolatedHome), "utf8");
    for (const message of [
      "Rust API, PineTS worker, and market-data helper are ready",
      "Rust native desktop shutdown started",
      "Rust retained runtime stopped",
    ]) {
      if (!log.includes(message)) throw new Error(`desktop log is missing ${JSON.stringify(message)}`);
    }
    const state = JSON.parse(
      fs.readFileSync(
        findFile(isolatedHome, (name) => name === "desktop-state.json", "desktop window state"),
        "utf8",
      ),
    );
    if (state.version !== 1 || state.width < 1024 || state.height < 700) {
      throw new Error(`desktop window state is invalid: ${JSON.stringify(state)}`);
    }
    assertNoPackagedOrphans(bundle.resourceRoot);
    return { readyMs, shutdownMs };
  } finally {
    if (!exited) child.kill("SIGKILL");
    fs.rmSync(isolatedHome, { recursive: true, force: true });
  }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  smokeTauriRelease()
    .then((result) => {
      console.log(`Tauri release smoke passed: ready=${result.readyMs}ms shutdown=${result.shutdownMs}ms, no packaged child orphans.`);
    })
    .catch((error) => {
      console.error(error);
      process.exitCode = 1;
    });
}
