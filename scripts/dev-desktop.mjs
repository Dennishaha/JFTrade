import { spawn, spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import {
  existsSync,
  mkdirSync,
  readFileSync,
  readdirSync,
  renameSync,
  rmSync,
  statSync,
  writeFileSync,
} from "node:fs";
import path from "node:path";
import process from "node:process";

import {
  nativeBundleCacheReusable,
  selectYFinanceDevelopmentRuntime,
} from "./lib/desktop-dev-fast-path.mjs";

const rootDir = path.resolve(import.meta.dirname, "..");
const desktopRuntimeDir = path.join(rootDir, "var", "jftrade-api");
const dryRun = process.env.JFTRADE_DESKTOP_DEV_DRY_RUN === "1";
const packageManagerCommand = process.env.npm_execpath
  ? [process.execPath, [process.env.npm_execpath]]
  : [process.platform === "win32" ? "pnpm.cmd" : "pnpm", []];

const apiBind = process.env.JFTRADE_API_BIND || "127.0.0.1:3008";
const apiBaseUrl = apiBaseURLForBind(apiBind);
const frontendURL =
  process.env.FRONTEND_DEVSERVER_URL || "http://127.0.0.1:3003";
const yfinanceRuntime = resolveYFinanceRuntime();
const devEnv = {
  JFTRADE_DESKTOP_MODE: "1",
  FRONTEND_DEVSERVER_URL: frontendURL,
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
  ...yfinanceRuntime.environment,
};

if (dryRun) {
  printDryRun(devEnv, yfinanceRuntime.mode);
  process.exit(0);
}

const children = [];
let shuttingDown = false;

try {
  const web = launchLongRunning(
    packageManagerCommand[0],
    [...packageManagerCommand[1], "run", "dev:web"],
    devEnv,
  );
  children.push(web);

  const nativePreparation = prepareDesktopExecutable();
  await Promise.all([nativePreparation, waitForURL(frontendURL, web)]);
  const desktop = await nativePreparation;
  const app = launchLongRunning(desktop.command, desktop.args, devEnv);
  children.push(app);
} catch (error) {
  shutdownChildren();
  console.error(error instanceof Error ? error.message : String(error));
  process.exit(1);
}

for (const signal of ["SIGINT", "SIGTERM"]) {
  process.on(signal, () => {
    shutdownChildren(signal);
    process.exit(0);
  });
}

function launchLongRunning(command, args, extraEnv) {
  const child = spawn(command, args, {
    stdio: "inherit",
    shell:
      process.platform === "win32" && /\.(?:cmd|bat)$/i.test(command),
    env: { ...process.env, ...extraEnv },
    cwd: rootDir,
  });
  child.on("exit", (code, signal) => {
    if (shuttingDown) return;
    shuttingDown = true;
    shutdownChildren();
    if (signal) {
      process.kill(process.pid, signal);
    } else {
      process.exit(code ?? 0);
    }
  });
  return child;
}

function shutdownChildren(signal = "SIGTERM") {
  shuttingDown = true;
  for (const child of children) {
    if (!child.killed) child.kill(signal);
  }
}

async function prepareDesktopExecutable() {
  if (process.platform !== "darwin") {
    return { command: "go", args: ["run", "./cmd/jftrade-desktop"] };
  }
  const appPath = path.join(rootDir, "dist", "dev", "JFTrade Dev.app");
  const executable = path.join(
    appPath,
    "Contents",
    "MacOS",
    "JFTrade Dev",
  );
  const fingerprintPath = path.join(
    rootDir,
    "dist",
    "dev",
    ".jftrade-native-fingerprint",
  );
  let fingerprint = nativeDesktopFingerprint();
  const executableAvailable = existsSync(executable);
  if (nativeBundleCacheReusable({
    executableAvailable,
    fingerprint,
    signatureValid: executableAvailable && validCodeSignature(appPath),
    storedFingerprint: readOptionalText(fingerprintPath),
  })) {
    console.log("JFTrade native development bundle cache hit");
    return { command: executable, args: [] };
  }

  console.log("JFTrade native development bundle cache miss; rebuilding");
  fingerprint = nativeDesktopFingerprint(true) || fingerprint;
  await spawnAndWait(process.execPath, [
    "scripts/wails3.mjs",
    "task",
    "darwin:build:dev",
  ]);
  if (!existsSync(executable) || !validCodeSignature(appPath)) {
    throw new Error("JFTrade native development bundle failed validation");
  }
  if (fingerprint) writeAtomicText(fingerprintPath, fingerprint);
  return { command: executable, args: [] };
}

function nativeDesktopFingerprint(refreshDependencies = false) {
  const goEnvironment = spawnSync(
    "go",
    ["env", "GOVERSION", "GOOS", "GOARCH", "CGO_ENABLED"],
    { cwd: rootDir, encoding: "utf8" },
  );
  if (goEnvironment.status !== 0) return "";
  const dependencyKey = nativeDependencyManifestKey(goEnvironment.stdout);
  const dependencyFiles = nativeDependencyFiles(
    dependencyKey,
    refreshDependencies,
  );
  if (!dependencyFiles) return "";
  const files = new Set([
    ...dependencyFiles,
    "go.mod",
    "go.sum",
    "Taskfile.yml",
    "build/Taskfile.yml",
    "build/darwin/Taskfile.yml",
    "build/darwin/Info.dev.plist",
    "build/config.yml",
    "build/desktop/appicon.png",
    "LICENSE",
    "docs/legal/third-party-notices.md",
    "scripts/dev-desktop.mjs",
    "scripts/lib/desktop-dev-fast-path.mjs",
  ]);
  const hash = createHash("sha256");
  hash.update(goEnvironment.stdout);
  hash.update(process.env.GOFLAGS || "");
  hash.update(process.env.CGO_CFLAGS || "");
  hash.update(process.env.CGO_LDFLAGS || "");
  for (const relative of [...files].sort()) {
    const absolute = path.join(rootDir, relative);
    if (!existsSync(absolute) || !statSync(absolute).isFile()) continue;
    hash.update(relative);
    hash.update("\0");
    hash.update(readFileSync(absolute));
    hash.update("\0");
  }
  return hash.digest("hex");
}

function nativeDependencyManifestKey(goEnvironment) {
  const hash = createHash("sha256");
  hash.update("jftrade-native-dependencies-v1\0");
  hash.update(goEnvironment || "");
  hash.update(process.env.GOFLAGS || "");
  for (const name of ["go.mod", "go.sum"]) {
    hash.update(readFileSync(path.join(rootDir, name)));
  }
  return hash.digest("hex");
}

function nativeDependencyFiles(key, refresh) {
  const manifestPath = path.join(
    rootDir,
    "dist",
    "dev",
    ".jftrade-native-dependencies.json",
  );
  if (!refresh) {
    const manifest = readNativeDependencyManifest(manifestPath);
    if (
      manifest?.key === key &&
      nativeDependencyDirectoriesUnchanged(manifest.directories)
    ) {
      return manifest.files;
    }
  }
  const dependencyList = spawnSync(
    "go",
    [
      "list",
      "-deps",
      "-f",
      "{{if not .Standard}}{{.Dir}}|{{join .GoFiles \",\"}}|{{join .CgoFiles \",\"}}|{{join .EmbedFiles \",\"}}{{end}}",
      "./cmd/jftrade-desktop",
    ],
    { cwd: rootDir, encoding: "utf8" },
  );
  if (dependencyList.status !== 0) return null;
  const files = new Set();
  const watchedDirectories = new Set();
  for (const line of dependencyList.stdout.split(/\r?\n/)) {
    if (!line.trim()) continue;
    const [directory, ...groups] = line.split("|");
    const packageRelative = path.relative(rootDir, directory);
    if (!packageRelative.startsWith("..")) {
      watchedDirectories.add(packageRelative || ".");
    }
    for (const group of groups) {
      for (const name of group.split(",").filter(Boolean)) {
        const absolute = path.join(directory, name);
        const relative = path.relative(rootDir, absolute);
        if (!relative.startsWith("..")) {
          files.add(relative);
          watchedDirectories.add(path.dirname(relative));
        }
      }
    }
  }
  const directories = {};
  for (const relative of [...watchedDirectories].sort()) {
    directories[relative] = readDirectoryNames(relative);
  }
  const manifest = { key, files: [...files].sort(), directories };
  mkdirSync(path.dirname(manifestPath), { recursive: true });
  writeAtomicText(manifestPath, JSON.stringify(manifest));
  return manifest.files;
}

function readNativeDependencyManifest(file) {
  try {
    const parsed = JSON.parse(readFileSync(file, "utf8"));
    if (
      !Array.isArray(parsed.files) ||
      parsed.directories === null ||
      typeof parsed.directories !== "object" ||
      Array.isArray(parsed.directories)
    ) {
      return null;
    }
    return parsed;
  } catch {
    return null;
  }
}

function nativeDependencyDirectoriesUnchanged(directories) {
  return Object.entries(directories).every(
    ([relative, names]) =>
      JSON.stringify(readDirectoryNames(relative)) === JSON.stringify(names),
  );
}

function readDirectoryNames(relative) {
  try {
    return readdirSync(path.join(rootDir, relative)).sort();
  } catch {
    return [];
  }
}

function validCodeSignature(appPath) {
  const result = spawnSync(
    "codesign",
    ["--verify", "--strict", appPath],
    { cwd: rootDir, stdio: "ignore" },
  );
  return result.status === 0;
}

function spawnAndWait(command, args) {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, { cwd: rootDir, stdio: "inherit" });
    child.on("error", reject);
    child.on("exit", (code) => {
      if (code === 0) resolve();
      else reject(new Error(`${command} exited with status ${code ?? 1}`));
    });
  });
}

async function waitForURL(url, processToWatch) {
  const deadline = Date.now() + 30_000;
  while (Date.now() < deadline) {
    if (processToWatch.exitCode != null) {
      throw new Error("Vite exited before becoming ready");
    }
    try {
      const response = await fetch(url, { signal: AbortSignal.timeout(500) });
      if (response.ok) return;
    } catch {
      // Vite is still binding its loopback listener.
    }
    await new Promise((resolve) => setTimeout(resolve, 100));
  }
  throw new Error(`Vite did not become ready within 30s: ${url}`);
}

function resolveYFinanceRuntime() {
  const configured = process.env.JFTRADE_YFINANCE_SIDECAR?.trim();
  const configuredPython = process.env.JFTRADE_YFINANCE_DEV_PYTHON?.trim();
  const configuredSource = process.env.JFTRADE_YFINANCE_DEV_PYTHONPATH?.trim();
  const python = defaultYFinancePython();
  const source = path.join(rootDir, "workers", "yfinance-sidecar", "src");
  const staged = stagedYFinanceSidecarPath();
  const explicitPythonRequested = Boolean(configuredPython || configuredSource);
  const inspectDefaultRuntime = !configured && !explicitPythonRequested;
  const selection = selectYFinanceDevelopmentRuntime({
    explicitHelper: configured,
    explicitHelperUsable:
      dryRun || (Boolean(configured) && usableRegularFile(configured)),
    explicitPython: configuredPython,
    explicitSource: configuredSource,
    explicitPythonUsable:
      dryRun ||
      (!configured &&
        Boolean(configuredPython) &&
        Boolean(configuredSource) &&
        usableRegularFile(configuredPython) &&
        existsSync(configuredSource) &&
        pythonRuntimeUsable(configuredPython, configuredSource)),
    defaultPython: python,
    defaultSource: source,
    defaultPythonUsable:
      inspectDefaultRuntime &&
      existsSync(python) &&
      existsSync(source) &&
      pythonRuntimeUsable(python, source),
    frozenAvailable: existsSync(staged),
    frozenHelper: staged,
    allowUnavailable: dryRun,
  });
  if (selection.kind === "python-source") {
    return {
      mode: "python-source",
      environment: {
        JFTRADE_YFINANCE_DEV_PYTHON: selection.python,
        JFTRADE_YFINANCE_DEV_PYTHONPATH: selection.source,
      },
    };
  }
  if (selection.kind === "explicit-helper" || selection.kind === "frozen-helper") {
    return {
      mode: selection.kind,
      environment: { JFTRADE_YFINANCE_SIDECAR: selection.executable },
    };
  }
  return { mode: "unavailable", environment: {} };
}

function usableRegularFile(file) {
  try {
    return statSync(file).isFile();
  } catch {
    return false;
  }
}

function pythonRuntimeUsable(python, source) {
  const probe = [
    "import importlib.util, sys",
    "required = ('fastapi', 'uvicorn', 'yfinance', 'curl_cffi')",
    "sys.exit(0 if all(importlib.util.find_spec(name) for name in required) else 1)",
  ].join("; ");
  const result = spawnSync(python, ["-c", probe], {
    cwd: rootDir,
    env: { ...process.env, PYTHONPATH: source },
    stdio: "ignore",
  });
  return result.status === 0;
}

function defaultYFinancePython() {
  const relative =
    process.platform === "win32"
      ? ["workers", "yfinance-sidecar", ".venv", "Scripts", "python.exe"]
      : ["workers", "yfinance-sidecar", ".venv", "bin", "python"];
  return path.join(rootDir, ...relative);
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

function printDryRun(environment, mode) {
  for (const key of [
    "FRONTEND_DEVSERVER_URL",
    "JFTRADE_DESKTOP_MODE",
    "JFTRADE_SETTINGS_PATH",
    "JFTRADE_BACKTEST_DB",
    "JFTRADE_API_BIND",
    "DISABLE_MARKETS_CACHE",
    "JFTRADE_YFINANCE_SIDECAR",
    "JFTRADE_YFINANCE_DEV_PYTHON",
    "JFTRADE_YFINANCE_DEV_PYTHONPATH",
    "VITE_API_BASE_URL",
    "VITE_DEV_API_TARGET",
  ]) {
    if (environment[key]) console.log(`${key}=${environment[key]}`);
  }
  console.log(`JFTRADE_YFINANCE_DEV_MODE=${mode}`);
}

function apiBaseURLForBind(bind) {
  const match = bind.trim().match(/^(.*):(\d+)$/);
  if (!match) return "";
  let host = match[1].trim();
  if (host === "" || host === "0.0.0.0" || host === "::" || host === "[::]") {
    host = "127.0.0.1";
  }
  host = host.replace(/^\[(.*)\]$/, "$1");
  return `http://${host}:${match[2]}`;
}

function readOptionalText(file) {
  try {
    return readFileSync(file, "utf8").trim();
  } catch {
    return "";
  }
}

function writeAtomicText(file, value) {
  const temporary = `${file}.${process.pid}.tmp`;
  writeFileSync(temporary, `${value}\n`, { mode: 0o600 });
  renameSync(temporary, file);
  rmSync(temporary, { force: true });
}
