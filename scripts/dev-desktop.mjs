import { spawn, spawnSync } from "node:child_process";
import { existsSync } from "node:fs";
import path from "node:path";
import process from "node:process";

const packageManagerCommand = process.env.npm_execpath
  ? [process.execPath, [process.env.npm_execpath]]
  : [process.platform === "win32" ? "pnpm.cmd" : "pnpm", []];

const rootDir = path.resolve(import.meta.dirname, "..");
const desktopRuntimeDir = path.join(rootDir, "var", "jftrade-api");
const yfinanceSidecarPath = resolveYFinanceSidecar();
const apiBind = process.env.JFTRADE_API_BIND || "127.0.0.1:3008";
const apiBaseUrl = apiBaseURLForBind(apiBind);
const devEnv = {
  JFTRADE_DESKTOP_MODE: "1",
  FRONTEND_DEVSERVER_URL:
    process.env.FRONTEND_DEVSERVER_URL || "http://127.0.0.1:3003",
  JFTRADE_SETTINGS_PATH:
    process.env.JFTRADE_SETTINGS_PATH ||
    path.join(desktopRuntimeDir, "settings.json"),
  JFTRADE_BACKTEST_DB:
    process.env.JFTRADE_BACKTEST_DB ||
    path.join(desktopRuntimeDir, "backtest.db"),
  JFTRADE_API_BIND: apiBind,
  DISABLE_MARKETS_CACHE: process.env.DISABLE_MARKETS_CACHE || "1",
  VITE_API_BASE_URL: process.env.VITE_API_BASE_URL || apiBaseUrl,
  VITE_DEV_API_TARGET: process.env.VITE_DEV_API_TARGET || apiBaseUrl,
};
if (yfinanceSidecarPath) {
  devEnv.JFTRADE_YFINANCE_SIDECAR = yfinanceSidecarPath;
}

let desktopCommand = "go";
let desktopArgs = ["run", "./cmd/jftrade-desktop"];

if (process.env.JFTRADE_DESKTOP_DEV_DRY_RUN === "1") {
  console.log(`FRONTEND_DEVSERVER_URL=${devEnv.FRONTEND_DEVSERVER_URL}`);
  console.log(`JFTRADE_DESKTOP_MODE=${devEnv.JFTRADE_DESKTOP_MODE}`);
  console.log(`JFTRADE_SETTINGS_PATH=${devEnv.JFTRADE_SETTINGS_PATH}`);
  console.log(`JFTRADE_BACKTEST_DB=${devEnv.JFTRADE_BACKTEST_DB}`);
  console.log(`JFTRADE_API_BIND=${devEnv.JFTRADE_API_BIND}`);
  console.log(`DISABLE_MARKETS_CACHE=${devEnv.DISABLE_MARKETS_CACHE}`);
  if (yfinanceSidecarPath) {
    console.log(`JFTRADE_YFINANCE_SIDECAR=${yfinanceSidecarPath}`);
  }
  console.log(`VITE_API_BASE_URL=${devEnv.VITE_API_BASE_URL}`);
  console.log(`VITE_DEV_API_TARGET=${devEnv.VITE_DEV_API_TARGET}`);
  process.exit(0);
}

if (process.platform === "darwin") {
  const build = spawn(
    process.execPath,
    ["scripts/wails3.mjs", "task", "darwin:build:dev"],
    {
      cwd: rootDir,
      stdio: "inherit",
    },
  );
  const code = await new Promise((resolve) => build.on("exit", resolve));
  if (code !== 0) process.exit(code ?? 1);
  desktopCommand = path.join(
    rootDir,
    "dist",
    "dev",
    "JFTrade Dev.app",
    "Contents",
    "MacOS",
    "JFTrade Dev",
  );
  desktopArgs = [];
}

const commands = [
  [packageManagerCommand[0], [...packageManagerCommand[1], "run", "dev:web"], devEnv],
  [desktopCommand, desktopArgs, devEnv],
];

const children = commands.map(([command, args, extraEnv]) =>
  spawn(command, args, {
    stdio: "inherit",
    // Native executables (including process.execPath) must not go through
    // cmd.exe: unquoted paths such as C:\Program Files\... are split at the
    // space. Only the Windows package-manager fallback needs a command shell.
    shell:
      process.platform === "win32" && /\.(?:cmd|bat)$/i.test(command),
    env: { ...process.env, ...extraEnv },
    cwd: rootDir,
  }),
);

let shuttingDown = false;

for (const child of children) {
  child.on("exit", (code, signal) => {
    if (shuttingDown) return;
    shuttingDown = true;
    for (const other of children) {
      if (other !== child && !other.killed) other.kill();
    }
    if (signal) {
      process.kill(process.pid, signal);
    } else {
      process.exit(code ?? 0);
    }
  });
}

for (const signal of ["SIGINT", "SIGTERM"]) {
  process.on(signal, () => {
    shuttingDown = true;
    for (const child of children) {
      if (!child.killed) child.kill(signal);
    }
    process.exit(0);
  });
}

function apiBaseURLForBind(bind) {
  const value = bind.trim();
  const match = value.match(/^(.*):(\d+)$/);
  if (!match) return "";
  let host = match[1].trim();
  const port = match[2];
  if (host === "" || host === "0.0.0.0" || host === "::" || host === "[::]") {
    host = "127.0.0.1";
  }
  host = host.replace(/^\[(.*)\]$/, "$1");
  return `http://${host}:${port}`;
}

function resolveYFinanceSidecar() {
  const configured = process.env.JFTRADE_YFINANCE_SIDECAR?.trim();
  if (configured) return configured;

  const staged = stagedYFinanceSidecarPath();
  if (existsSync(staged)) return staged;
  if (process.env.JFTRADE_DESKTOP_DEV_DRY_RUN === "1") return "";

  console.log(`Preparing native yfinance helper for desktop development: ${staged}`);
  const python =
    process.env.JFTRADE_YFINANCE_BUILD_PYTHON?.trim() ||
    defaultYFinanceBuildPython();
  const result = spawnSync(
    process.execPath,
    ["scripts/build-yfinance-sidecar.mjs"],
    {
      cwd: rootDir,
      env: {
        ...process.env,
        JFTRADE_YFINANCE_BUILD_PYTHON: python,
      },
      stdio: "inherit",
    },
  );
  if (result.error) throw result.error;
  if (result.status !== 0) {
    throw new Error(
      `yfinance sidecar build failed with status ${result.status ?? "unknown"}`,
    );
  }
  if (!existsSync(staged)) {
    throw new Error(`yfinance sidecar build did not produce ${staged}`);
  }
  return staged;
}

function stagedYFinanceSidecarPath() {
  const goos = { darwin: "darwin", linux: "linux", win32: "windows" }[
    process.platform
  ] || process.platform;
  const goarch = { arm64: "arm64", x64: "amd64" }[process.arch] || process.arch;
  const extension = goos === "windows" ? ".exe" : "";
  const binaryBase = `yfinance-sidecar-${goos}-${goarch}`;
  return path.join(
    rootDir,
    "internal",
    "yfinanceassets",
    "assets",
    "bin",
    binaryBase,
    `${binaryBase}${extension}`,
  );
}

function defaultYFinanceBuildPython() {
  const relative =
    process.platform === "win32"
      ? ["workers", "yfinance-sidecar", ".venv", "Scripts", "python.exe"]
      : ["workers", "yfinance-sidecar", ".venv", "bin", "python"];
  const bundled = path.join(rootDir, ...relative);
  if (existsSync(bundled)) return bundled;
  return process.platform === "win32" ? "python" : "python3";
}
