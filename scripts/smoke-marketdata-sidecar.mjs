#!/usr/bin/env node

import { spawn, spawnSync } from "node:child_process";
import { appendFileSync, readdirSync, statSync } from "node:fs";
import { createServer } from "node:net";
import { basename, dirname, resolve } from "node:path";
import process from "node:process";
import { fileURLToPath, pathToFileURL } from "node:url";

import { currentMarketDataSidecarAssetPath } from "./lib/desktop-release-inputs.mjs";

const repositoryRoot = resolve(fileURLToPath(new URL("..", import.meta.url)));
const loopbackHost = "127.0.0.1";
const defaultTimeoutMs = 90_000;
const requestTimeoutMs = 5_000;
const pollIntervalMs = 200;
const shutdownTimeoutMs = 5_000;
const outputLimit = 16_384;

class PermanentSmokeError extends Error {}

export function resolveMarketDataSidecarExecutable({
  rootDir = repositoryRoot,
  environment = process.env,
  platform = process.platform,
  architecture = process.arch,
} = {}) {
  const override =
    String(environment.JFTRADE_MARKETDATA_SMOKE_EXECUTABLE ?? "").trim() ||
    String(environment.JFTRADE_YFINANCE_SMOKE_EXECUTABLE ?? "").trim();
  if (override !== "") {
    return resolve(rootDir, override);
  }

  const relativeDirectory = currentMarketDataSidecarAssetPath({
    environment,
    platform,
    architecture,
  });
  const binaryBase = basename(relativeDirectory);
  const extension = binaryBase.includes("-windows-") ? ".exe" : "";
  return resolve(
    rootDir,
    relativeDirectory,
    `${binaryBase}${extension}`,
  );
}

export async function runMarketDataSidecarSmoke(options = {}) {
  const environment = options.environment ?? process.env;
  const executable =
    options.executable ??
    resolveMarketDataSidecarExecutable({
      rootDir: options.rootDir,
      environment,
      platform: options.platform,
      architecture: options.architecture,
    });
  const executableArgs = options.executableArgs ?? [];
  const bundleDirectory = options.bundleDirectory ?? dirname(executable);
  const timeoutMs = options.timeoutMs ?? smokeTimeout(environment);
  const log = options.log ?? console.log;

  requireNonEmptyFile(executable);
  const bundleBytes = regularFileBytes(bundleDirectory);
  if (bundleBytes <= 0) {
    throw new Error(`Market-data sidecar bundle is empty: ${bundleDirectory}`);
  }

  const version = verifyVersion(
    executable,
    executableArgs,
    environment,
    timeoutMs,
  );
  const port = await reserveLoopbackPort();
  const child = spawn(
    executable,
    [...executableArgs, "--host", loopbackHost, "--port", String(port)],
    {
      env: { ...environment, PYTHONUNBUFFERED: "1" },
      stdio: ["ignore", "pipe", "pipe"],
      windowsHide: true,
    },
  );
  const captured = captureChildOutput(child);
  const deadline = Date.now() + timeoutMs;

  try {
    await waitForJSON({
      child,
      captured,
      deadline,
      port,
      path: "/healthz",
      ready: (body) => body?.ok === true,
    });
    log("marketdata-sidecar smoke: /healthz ready");

    const yahoo = await waitForProvider({
      child,
      captured,
      deadline,
      port,
      provider: "yfinance",
      versionField: "yfinance_version",
    });
    log(
      `marketdata-sidecar smoke: yfinance ${yahoo.yfinance_version} ready`,
    );

    const akshare = await waitForProvider({
      child,
      captured,
      deadline,
      port,
      provider: "akshare",
      versionField: "provider_version",
    });
    log(
      `marketdata-sidecar smoke: akshare ${akshare.provider_version} ready`,
    );

    const result = {
      executable,
      version,
      bundleBytes,
      bundleMiB: bundleBytes / 1024 / 1024,
      yfinanceVersion: yahoo.yfinance_version,
      akshareVersion: akshare.provider_version,
    };
    log(
      `marketdata-sidecar smoke passed: ${version}; bundle=${result.bundleMiB.toFixed(2)} MiB (${bundleBytes} bytes)`,
    );
    reportGitHubSummary(result, environment);
    return result;
  } catch (error) {
    throw enrichError(error, child, captured);
  } finally {
    await stopChild(child);
  }
}

function verifyVersion(executable, executableArgs, environment, timeoutMs) {
  const result = spawnSync(executable, [...executableArgs, "--version"], {
    encoding: "utf8",
    env: environment,
    timeout: Math.min(timeoutMs, 30_000),
    windowsHide: true,
  });
  if (result.error) {
    throw new Error(
      `Could not run market-data sidecar --version: ${result.error.message}`,
    );
  }
  if (result.status !== 0) {
    throw new Error(
      `Market-data sidecar --version exited ${result.status}: ${String(result.stderr || result.stdout || "").trim()}`,
    );
  }
  const version = String(result.stdout || result.stderr || "").trim();
  if (!/^marketdata-sidecar\s+\d+\.\d+\.\d+(?:[-+][^\s]+)?$/u.test(version)) {
    throw new Error(
      `Unexpected market-data sidecar --version output: ${JSON.stringify(version)}`,
    );
  }
  return version;
}

async function waitForProvider({ provider, versionField, ...options }) {
  return waitForJSON({
    ...options,
    path: `/providers/${provider}/health`,
    ready(body) {
      if (body?.runtime_state === "failed") {
        throw new PermanentSmokeError(
          `${provider} frozen runtime failed: ${body.warmup_error || "unknown import error"}`,
        );
      }
      return (
        body?.ok === true &&
        body.runtime_state === "ready" &&
        typeof body[versionField] === "string" &&
        body[versionField].trim() !== ""
      );
    },
  });
}

async function waitForJSON({ child, captured, deadline, port, path, ready }) {
  let lastFailure = "not requested";
  while (Date.now() < deadline) {
    assertChildRunning(child, captured);
    try {
      const remaining = Math.max(1, deadline - Date.now());
      const response = await fetch(
        `http://${loopbackHost}:${port}${path}`,
        {
          signal: AbortSignal.timeout(Math.min(requestTimeoutMs, remaining)),
        },
      );
      const text = await response.text();
      let body;
      try {
        body = JSON.parse(text);
      } catch {
        lastFailure = `HTTP ${response.status} returned non-JSON ${JSON.stringify(text.slice(0, 300))}`;
        await delay(pollIntervalMs);
        continue;
      }
      const errorCode = String(body?.error?.code ?? "").trim();
      const errorMessage = String(body?.error?.message ?? "").trim();
      if (errorCode.endsWith("_RUNTIME_FAILED")) {
        throw new PermanentSmokeError(
          `${path} returned ${errorCode}: ${errorMessage || "runtime import failed"}`,
        );
      }
      if (response.ok && ready(body)) {
        return body;
      }
      lastFailure = `HTTP ${response.status}: ${JSON.stringify(body)}`;
    } catch (error) {
      if (error instanceof PermanentSmokeError) {
        throw error;
      }
      lastFailure = error instanceof Error ? error.message : String(error);
    }
    await delay(pollIntervalMs);
  }
  throw new Error(
    `Timed out waiting for market-data sidecar ${path}: ${lastFailure}`,
  );
}

function assertChildRunning(child, captured) {
  if (captured.spawnError) {
    throw captured.spawnError;
  }
  if (child.exitCode !== null || child.signalCode !== null) {
    throw new Error(
      `Market-data sidecar exited before smoke completed (code=${child.exitCode}, signal=${child.signalCode})`,
    );
  }
}

function captureChildOutput(child) {
  const captured = { stdout: "", stderr: "", spawnError: null };
  child.stdout?.on("data", (chunk) => {
    captured.stdout = appendBounded(captured.stdout, chunk);
  });
  child.stderr?.on("data", (chunk) => {
    captured.stderr = appendBounded(captured.stderr, chunk);
  });
  child.on("error", (error) => {
    captured.spawnError = error;
  });
  return captured;
}

function appendBounded(current, chunk) {
  const combined = current + String(chunk);
  return combined.length <= outputLimit
    ? combined
    : combined.slice(combined.length - outputLimit);
}

function enrichError(error, child, captured) {
  const detail = [
    captured.stdout.trim() ? `stdout:\n${captured.stdout.trim()}` : "",
    captured.stderr.trim() ? `stderr:\n${captured.stderr.trim()}` : "",
    `child code=${child.exitCode} signal=${child.signalCode}`,
  ]
    .filter(Boolean)
    .join("\n");
  const message = error instanceof Error ? error.message : String(error);
  return new Error(`${message}\n${detail}`, { cause: error });
}

async function reserveLoopbackPort() {
  const server = createServer();
  await new Promise((resolvePromise, reject) => {
    server.once("error", reject);
    server.listen(0, loopbackHost, resolvePromise);
  });
  const address = server.address();
  const port = typeof address === "object" && address ? address.port : 0;
  await new Promise((resolvePromise, reject) => {
    server.close((error) => (error ? reject(error) : resolvePromise()));
  });
  if (port <= 0) {
    throw new Error("Could not reserve a dynamic loopback port");
  }
  return port;
}

async function stopChild(child) {
  if (
    child.pid == null ||
    child.exitCode !== null ||
    child.signalCode !== null
  ) {
    return;
  }
  const graceful = waitForExit(child, shutdownTimeoutMs);
  child.kill("SIGTERM");
  if (await graceful) {
    return;
  }
  const forced = waitForExit(child, shutdownTimeoutMs);
  child.kill("SIGKILL");
  if (!(await forced)) {
    throw new Error(`Could not stop market-data sidecar child ${child.pid}`);
  }
}

function waitForExit(child, timeoutMs) {
  return new Promise((resolvePromise) => {
    if (child.exitCode !== null || child.signalCode !== null) {
      resolvePromise(true);
      return;
    }
    const timer = setTimeout(() => {
      child.off("exit", onExit);
      resolvePromise(false);
    }, timeoutMs);
    const onExit = () => {
      clearTimeout(timer);
      resolvePromise(true);
    };
    child.once("exit", onExit);
  });
}

function requireNonEmptyFile(path) {
  let info;
  try {
    info = statSync(path);
  } catch (error) {
    throw new Error(`Market-data sidecar executable is missing: ${path}`, {
      cause: error,
    });
  }
  if (!info.isFile() || info.size <= 0) {
    throw new Error(`Market-data sidecar executable is empty or invalid: ${path}`);
  }
}

function regularFileBytes(directory) {
  let total = 0;
  for (const entry of readdirSync(directory, { withFileTypes: true })) {
    const path = resolve(directory, entry.name);
    const info = statSync(path);
    if (info.isDirectory()) {
      total += regularFileBytes(path);
    } else if (info.isFile()) {
      total += info.size;
    }
  }
  return total;
}

function smokeTimeout(environment) {
  const value = String(
    environment.JFTRADE_MARKETDATA_SMOKE_TIMEOUT_MS ?? "",
  ).trim();
  if (value === "") {
    return defaultTimeoutMs;
  }
  const timeout = Number(value);
  if (!Number.isSafeInteger(timeout) || timeout < 1_000) {
    throw new Error(
      "JFTRADE_MARKETDATA_SMOKE_TIMEOUT_MS must be an integer >= 1000",
    );
  }
  return timeout;
}

function reportGitHubSummary(result, environment) {
  const summary = String(environment.GITHUB_STEP_SUMMARY ?? "").trim();
  if (summary === "") {
    return;
  }
  appendFileSync(
    summary,
    `- Market-data sidecar frozen smoke: ${result.version}; yfinance ${result.yfinanceVersion}; AKShare ${result.akshareVersion}; bundle ${result.bundleMiB.toFixed(2)} MiB (${result.bundleBytes} bytes)\n`,
  );
}

function delay(milliseconds) {
  return new Promise((resolvePromise) => setTimeout(resolvePromise, milliseconds));
}

const invokedPath = process.argv[1]
  ? pathToFileURL(resolve(process.argv[1])).href
  : "";
if (invokedPath === import.meta.url) {
  try {
    await runMarketDataSidecarSmoke();
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  }
}
