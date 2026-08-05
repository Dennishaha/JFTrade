#!/usr/bin/env node
import { spawnSync } from "node:child_process";
import { readFileSync } from "node:fs";
import path from "node:path";

import {
  nativeBundleCacheReusable,
  selectMarketDataDevelopmentRuntime,
} from "./lib/desktop-dev-fast-path.mjs";

const rootDir = process.cwd();
const desktopRuntimeDir = path.join(rootDir, "var", "jftrade-api");

const defaults = runDevDesktop({
  FRONTEND_DEVSERVER_URL: "",
  JFTRADE_SETTINGS_PATH: "",
  JFTRADE_BACKTEST_DB: "",
  JFTRADE_API_BIND: "",
  DISABLE_MARKETS_CACHE: "",
  JFTRADE_MARKETDATA_SIDECAR: "",
  JFTRADE_MARKETDATA_DEV_PYTHON: "",
  JFTRADE_MARKETDATA_DEV_PYTHONPATH: "",
  JFTRADE_YFINANCE_SIDECAR: "",
  JFTRADE_YFINANCE_DEV_PYTHON: "",
  JFTRADE_YFINANCE_DEV_PYTHONPATH: "",
  VITE_API_BASE_URL: "",
  VITE_DEV_API_TARGET: "",
});
assert(
  defaults.status === 0,
  `desktop dev dry run failed: ${defaults.stderr || defaults.stdout}`,
);
assert(
  defaults.stdout.includes("JFTRADE_DESKTOP_MODE=1"),
  "desktop dev did not identify the Vite runtime as desktop mode",
);
assert(
  defaults.stdout.includes("FRONTEND_DEVSERVER_URL=http://127.0.0.1:3003"),
  "desktop dev did not default to the Vite development port",
);
assert(
  defaults.stdout.includes(
    `JFTRADE_SETTINGS_PATH=${path.join(desktopRuntimeDir, "settings.json")}`,
  ),
  "desktop dev did not default to the desktop settings path",
);
assert(
  defaults.stdout.includes(
    `JFTRADE_BACKTEST_DB=${path.join(desktopRuntimeDir, "backtest.db")}`,
  ),
  "desktop dev did not default to the desktop backtest DB path",
);
assert(
  defaults.stdout.includes("JFTRADE_API_BIND=127.0.0.1:3008"),
  "desktop dev did not default to the desktop API bind",
);
assert(
  defaults.stdout.includes("DISABLE_MARKETS_CACHE=1"),
  "desktop dev did not disable the markets cache by default",
);
assert(
  defaults.stdout.includes("VITE_API_BASE_URL=http://127.0.0.1:3008"),
  "desktop dev did not inject the desktop API base URL into the frontend",
);
assert(
  defaults.stdout.includes("VITE_DEV_API_TARGET=http://127.0.0.1:3008"),
  "desktop dev did not point the Vite proxy at the desktop API",
);
assert(
  defaults.stdout.includes("JFTRADE_MARKETDATA_DEV_MODE="),
  "desktop dev did not report its market-data runtime selection",
);

const overrides = runDevDesktop({
  JFTRADE_SETTINGS_PATH: path.join(rootDir, "tmp", "settings.json"),
  JFTRADE_BACKTEST_DB: path.join(rootDir, "tmp", "backtest.db"),
  JFTRADE_API_BIND: "127.0.0.1:7788",
  DISABLE_MARKETS_CACHE: "0",
  JFTRADE_MARKETDATA_SIDECAR: path.join(rootDir, "tmp", "marketdata-sidecar"),
  VITE_API_BASE_URL: "http://127.0.0.1:8899",
  VITE_DEV_API_TARGET: "http://127.0.0.1:8899",
});
assert(
  overrides.status === 0,
  `desktop dev override dry run failed: ${overrides.stderr || overrides.stdout}`,
);

const legacyOverride = runDevDesktop({
  JFTRADE_MARKETDATA_SIDECAR: "",
  JFTRADE_YFINANCE_SIDECAR: path.join(rootDir, "tmp", "legacy-yfinance-sidecar"),
});
assert(
  legacyOverride.status === 0 &&
    legacyOverride.stdout.includes(
      `JFTRADE_MARKETDATA_SIDECAR=${path.join(rootDir, "tmp", "legacy-yfinance-sidecar")}`,
    ),
  "desktop dev did not translate the legacy market-data helper override",
);

const pythonSource = runDevDesktop({
  JFTRADE_MARKETDATA_SIDECAR: "",
  JFTRADE_MARKETDATA_DEV_PYTHON: process.execPath,
  JFTRADE_MARKETDATA_DEV_PYTHONPATH: rootDir,
});
assert(
  pythonSource.status === 0 &&
    pythonSource.stdout.includes(
      `JFTRADE_MARKETDATA_DEV_PYTHON=${process.execPath}`,
    ) &&
    pythonSource.stdout.includes(
      `JFTRADE_MARKETDATA_DEV_PYTHONPATH=${rootDir}`,
    ) &&
    pythonSource.stdout.includes("JFTRADE_MARKETDATA_DEV_MODE=python-source"),
  "desktop dev did not preserve an explicit Python source runtime",
);
assert(
  overrides.stdout.includes(
    `JFTRADE_SETTINGS_PATH=${path.join(rootDir, "tmp", "settings.json")}`,
  ),
  "desktop dev did not preserve the settings path override",
);
assert(
  overrides.stdout.includes(
    `JFTRADE_BACKTEST_DB=${path.join(rootDir, "tmp", "backtest.db")}`,
  ),
  "desktop dev did not preserve the backtest DB override",
);
assert(
  overrides.stdout.includes("JFTRADE_API_BIND=127.0.0.1:7788"),
  "desktop dev did not preserve the API bind override",
);
assert(
  overrides.stdout.includes("DISABLE_MARKETS_CACHE=0"),
  "desktop dev did not preserve the markets cache override",
);
assert(
  overrides.stdout.includes(
    `JFTRADE_MARKETDATA_SIDECAR=${path.join(rootDir, "tmp", "marketdata-sidecar")}`,
  ),
  "desktop dev did not preserve the market-data sidecar override",
);
assert(
  overrides.stdout.includes("VITE_API_BASE_URL=http://127.0.0.1:8899"),
  "desktop dev did not preserve the frontend API base URL override",
);
assert(
  overrides.stdout.includes("VITE_DEV_API_TARGET=http://127.0.0.1:8899"),
  "desktop dev did not preserve the Vite proxy target override",
);

const implementation = readFileSync(
  path.join(rootDir, "scripts", "dev-desktop.mjs"),
  "utf8",
);
const viteConfig = readFileSync(
  path.join(rootDir, "apps", "web", "vite.config.ts"),
  "utf8",
);
assert(
  implementation.indexOf("run\", \"dev:web") <
    implementation.indexOf("prepareDesktopExecutable()"),
  "desktop dev does not start Vite before native preparation",
);
assert(
  implementation.includes("prepareFrontendDependencies()") &&
    implementation.indexOf("prepareFrontendDependencies()") <
      implementation.indexOf("launchLongRunning(") &&
    implementation.includes('"vite",\n      "optimize"'),
  "desktop dev does not pre-optimize frontend dependencies before starting Vite",
);
assert(
  [
    "@tanstack/vue-query",
    "@wailsio/runtime",
    "vuetify/components/VAlert",
    "vuetify/components/VTextarea",
  ].every((dependency) => viteConfig.includes(`"${dependency}"`)),
  "Vite does not explicitly pre-optimize the desktop startup dependencies",
);
assert(
  implementation.includes("nativeDesktopFingerprint") &&
    implementation.includes("validCodeSignature") &&
    implementation.includes("JFTrade native development bundle cache hit"),
  "desktop dev native fingerprint/signature cache is missing",
);
assert(
  !implementation.includes("build-marketdata-sidecar.mjs"),
  "desktop dev still performs an implicit frozen helper build",
);
assert(
  !implementation.includes("runtimeDependencies") &&
    !implementation.includes("pythonBinaryPath"),
  "desktop dev still reads the removed persisted Python path",
);

assert(
  nativeBundleCacheReusable({
    executableAvailable: true,
    fingerprint: "current",
    storedFingerprint: "current",
    signatureValid: true,
  }),
  "native cache did not accept a matching signed bundle",
);
for (const invalid of [
  { executableAvailable: false, storedFingerprint: "current", signatureValid: true },
  { executableAvailable: true, storedFingerprint: "stale", signatureValid: true },
  { executableAvailable: true, storedFingerprint: "current", signatureValid: false },
]) {
  assert(
    !nativeBundleCacheReusable({ fingerprint: "current", ...invalid }),
    "native cache accepted a missing, stale, or unsigned bundle",
  );
}

const baseRuntime = {
  explicitHelper: "",
  explicitHelperUsable: false,
  explicitPython: "",
  explicitSource: "",
  explicitPythonUsable: false,
  defaultPython: "/venv/python",
  defaultSource: "/source",
  defaultPythonUsable: false,
  frozenAvailable: false,
  frozenHelper: "/frozen/helper",
  allowUnavailable: false,
};
assert(
  selectMarketDataDevelopmentRuntime({
    ...baseRuntime,
    defaultPythonUsable: true,
  }).kind === "python-source",
  "desktop dev did not prefer the usable venv source runtime",
);
assert(
  selectMarketDataDevelopmentRuntime({
    ...baseRuntime,
    frozenAvailable: true,
  }).kind === "frozen-helper",
  "desktop dev did not fall back to the frozen helper",
);
assertThrows(
  () => selectMarketDataDevelopmentRuntime(baseRuntime),
  "pip install --editable",
  "desktop dev did not fail quickly with an actionable install command",
);

function runDevDesktop(extraEnv) {
  const env = { ...process.env, JFTRADE_DESKTOP_DEV_DRY_RUN: "1" };
  for (const [key, value] of Object.entries(extraEnv)) {
    if (value === "") {
      delete env[key];
    } else {
      env[key] = value;
    }
  }
  return spawnSync(process.execPath, ["scripts/dev-desktop.mjs"], {
    cwd: rootDir,
    env,
    encoding: "utf8",
  });
}

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

function assertThrows(operation, expected, message) {
  try {
    operation();
  } catch (error) {
    assert(String(error).includes(expected), message);
    return;
  }
  throw new Error(message);
}
