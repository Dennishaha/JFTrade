#!/usr/bin/env node
import { spawnSync } from "node:child_process";
import path from "node:path";

const rootDir = process.cwd();
const desktopRuntimeDir = path.join(rootDir, "var", "jftrade-api");

const defaults = runDevDesktop({
  FRONTEND_DEVSERVER_URL: "",
  JFTRADE_SETTINGS_PATH: "",
  JFTRADE_BACKTEST_DB: "",
  JFTRADE_API_BIND: "",
  DISABLE_MARKETS_CACHE: "",
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

const overrides = runDevDesktop({
  JFTRADE_SETTINGS_PATH: path.join(rootDir, "tmp", "settings.json"),
  JFTRADE_BACKTEST_DB: path.join(rootDir, "tmp", "backtest.db"),
  JFTRADE_API_BIND: "127.0.0.1:7788",
  DISABLE_MARKETS_CACHE: "0",
  VITE_API_BASE_URL: "http://127.0.0.1:8899",
  VITE_DEV_API_TARGET: "http://127.0.0.1:8899",
});
assert(
  overrides.status === 0,
  `desktop dev override dry run failed: ${overrides.stderr || overrides.stdout}`,
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
  overrides.stdout.includes("VITE_API_BASE_URL=http://127.0.0.1:8899"),
  "desktop dev did not preserve the frontend API base URL override",
);
assert(
  overrides.stdout.includes("VITE_DEV_API_TARGET=http://127.0.0.1:8899"),
  "desktop dev did not preserve the Vite proxy target override",
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
