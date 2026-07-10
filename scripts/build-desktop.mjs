import fs from "node:fs";
import path from "node:path";
import process from "node:process";

import { spawnChecked } from "./lib/spawn.mjs";
import { writeMacAppBundle } from "./lib/mac-app-bundle.mjs";
import { resolveDesktopBuildMetadata } from "./lib/desktop-release-metadata.mjs";
import {
  assertPreparedDesktopReleaseInputs,
  usesPreparedDesktopReleaseInputs,
} from "./lib/desktop-release-inputs.mjs";
import { withWindowsDesktopManifestVersion } from "./lib/windows-resource-metadata.mjs";

const rootDir = path.resolve(import.meta.dirname, "..");
const desktopDistDir = path.join(rootDir, "dist", "desktop");
const buildMetadata = resolveDesktopBuildMetadata();

const platformAliases = {
  darwin: "darwin",
  macos: "darwin",
  mac: "darwin",
  win32: "windows",
  windows: "windows",
  win: "windows",
  linux: "linux",
};

const currentTarget = platformAliases[process.platform] || process.platform;
const requestedSpec = parseTargetSpec(
  process.env.JFTRADE_DESKTOP_TARGET || process.argv[2] || currentTarget,
);
const requestedTarget = requestedSpec.target;
const arch =
  process.env.GOARCH ||
  process.argv[3] ||
  requestedSpec.arch ||
  (requestedTarget === currentTarget ? currentGoArch() : "") ||
  defaultArch(requestedTarget);

if (!requestedTarget) {
  console.error(
    `Unknown desktop target: ${process.env.JFTRADE_DESKTOP_TARGET || process.argv[2]}`,
  );
  process.exit(1);
}

preflightTargetToolchain(requestedTarget, arch);

function run(command, args, options = {}) {
  const status = runStatus(command, args, options);
  if (status !== 0) process.exit(status);
}

function runStatus(command, args, options = {}) {
  return spawnChecked(command, args, { cwd: rootDir, ...options });
}

prepareDesktopReleaseInputs();
cleanupLegacyDesktopOutputs();

const outputDir = targetOutputDir(requestedTarget, arch);
fs.rmSync(outputDir, { recursive: true, force: true });
fs.mkdirSync(outputDir, { recursive: true });
const outputPath = targetOutputPath(requestedTarget, arch);
fs.mkdirSync(path.dirname(outputPath), { recursive: true });
const goEnv = targetGoEnv(requestedTarget, arch);
const buildTags =
  requestedTarget === "linux" ? "release_assets,gtk3" : "release_assets";
const generatedWindowsSyso = generateWindowsSyso(requestedTarget, arch);
let buildStatus = 0;
try {
  buildStatus = runStatus(
    "go",
    [
      "build",
      "-buildvcs=false",
      "-tags",
      buildTags,
      "-ldflags",
      desktopGoLDFlags(buildMetadata),
      "-o",
      outputPath,
      "./cmd/jftrade-desktop",
    ],
    { env: { ...process.env, ...goEnv } },
  );
} finally {
  if (generatedWindowsSyso) {
    fs.rmSync(generatedWindowsSyso, { force: true });
  }
}
if (buildStatus !== 0) {
  process.exit(buildStatus);
}
if (generatedWindowsSyso && fs.existsSync(generatedWindowsSyso)) {
  console.error(
    `Windows syso file was not cleaned up: ${generatedWindowsSyso}`,
  );
  process.exit(1);
}

if (requestedTarget === "darwin") {
  const appPath = path.join(outputDir, "JFTrade.app");
  writeMacAppBundle(appPath, outputPath, buildMetadata);
  console.log(`macOS app bundle written to ${appPath}`);
} else {
  console.log(`Desktop artifact written to ${outputPath}`);
}

function normalizeTarget(value) {
  return platformAliases[String(value || "").toLowerCase()] || "";
}

function parseTargetSpec(value) {
  const normalized = String(value || "").toLowerCase();
  const archSuffix = normalized.match(/^(.*?)[-_:](amd64|arm64)$/);
  if (archSuffix) {
    return { target: normalizeTarget(archSuffix[1]), arch: archSuffix[2] };
  }
  return { target: normalizeTarget(normalized), arch: "" };
}

function defaultArch(target) {
  if (target === "darwin" && process.platform === "darwin") {
    return process.arch === "arm64" ? "arm64" : "amd64";
  }
  return "amd64";
}

function currentGoArch() {
  if (process.arch === "arm64") return "arm64";
  if (process.arch === "x64") return "amd64";
  return "";
}

function hostGoEnvironment() {
  const environment = { ...process.env };
  for (const name of [
    "GOOS",
    "GOARCH",
    "GOARM64",
    "CGO_ENABLED",
    "CC",
    "CXX",
  ]) {
    delete environment[name];
  }
  return environment;
}

function prepareDesktopReleaseInputs() {
  let prepared;
  try {
    prepared = usesPreparedDesktopReleaseInputs();
    if (prepared) {
      assertPreparedDesktopReleaseInputs(rootDir);
    }
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exit(1);
  }
  if (prepared) {
    console.log("Using prepared desktop release inputs.");
    return;
  }
  run("npm", ["run", "prepare:desktop-release"]);
  run("npm", ["run", "generate:wails-bindings"], {
    env: hostGoEnvironment(),
  });
  run("npm", ["run", "build:pineworker"]);
}

function preflightTargetToolchain(target, targetArch) {
  if (target === "linux" && currentTarget !== "linux" && !process.env.CC) {
    console.error(
      [
        "Linux desktop builds require a Linux CGO toolchain with GTK/WebKit development headers.",
        "Run this target on Linux, or set CC/pkg-config for a full Linux cross toolchain.",
        "On Ubuntu runners, install: pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev",
      ].join("\n"),
    );
    process.exit(1);
  }
  if (
    target === "windows" &&
    targetArch === "arm64" &&
    !(currentTarget === "windows" && currentGoArch() === "arm64")
  ) {
    console.error(
      "Windows arm64 desktop releases require a native Windows arm64 CGO toolchain.",
    );
    process.exit(1);
  }
  if (target === "darwin" && currentTarget !== "darwin") {
    console.error("macOS desktop builds require a native macOS CGO toolchain.");
    process.exit(1);
  }
}

function generateWindowsSyso(target, targetArch) {
  if (target !== "windows") {
    return "";
  }

  const sysoPath = path.join(
    rootDir,
    "cmd",
    "jftrade-desktop",
    `jftrade_windows_${targetArch}.syso`,
  );
  const generatedInfo = path.join(outputDir, "windows-info.json");
  const generatedManifest = path.join(outputDir, "wails.exe.manifest");
  fs.rmSync(sysoPath, { force: true });
  writeWindowsMetadata(generatedInfo, generatedManifest, buildMetadata);
  run(process.execPath, [
    "scripts/wails3.mjs",
    "generate",
    "syso",
    "-arch",
    targetArch,
    "-icon",
    "build/desktop/windows/icon.ico",
    "-manifest",
    generatedManifest,
    "-info",
    generatedInfo,
    "-out",
    sysoPath,
  ]);
  return sysoPath;
}

function writeWindowsMetadata(infoPath, manifestPath, metadata) {
  const info = JSON.parse(
    fs.readFileSync(
      path.join(rootDir, "build", "desktop", "windows", "info.json"),
      "utf8",
    ),
  );
  info.fixed.file_version = metadata.numericVersion;
  info.info["0000"].ProductVersion = metadata.numericVersion;
  info.info["0000"].Comments =
    `Wails v3 desktop shell for JFTrade; commit ${metadata.commit}; built ${metadata.buildTime}`;
  fs.writeFileSync(infoPath, `${JSON.stringify(info, null, 2)}\n`, "utf8");

  const manifest = withWindowsDesktopManifestVersion(
    fs.readFileSync(
      path.join(rootDir, "build", "desktop", "windows", "wails.exe.manifest"),
      "utf8",
    ),
    metadata.numericVersion,
  );
  fs.writeFileSync(manifestPath, manifest, "utf8");
}

function desktopGoLDFlags(metadata) {
  const packagePath = "github.com/jftrade/jftrade-main/internal/buildinfo";
  return [
    `-X ${packagePath}.Version=${metadata.version}`,
    `-X ${packagePath}.Commit=${metadata.commit}`,
    `-X ${packagePath}.BuildTime=${metadata.buildTime}`,
  ].join(" ");
}

function cleanupLegacyDesktopOutputs() {
  for (const entry of [
    ".desktop-build",
    "JFTrade.app",
    "jftrade-desktop",
    "jftrade-desktop.exe",
    "jftrade-desktop-windows-amd64.exe",
    "jftrade-desktop-windows-arm64.exe",
    "jftrade-desktop-linux-amd64",
    "jftrade-desktop-linux-arm64",
  ]) {
    fs.rmSync(path.join(rootDir, "dist", entry), {
      recursive: true,
      force: true,
    });
  }
}

function targetGoEnv(target, targetArch) {
  const env = { GOARCH: targetArch };
  if (target === "windows") {
    env.GOOS = "windows";
    env.CGO_ENABLED = process.env.CGO_ENABLED || "1";
  } else if (target === "linux") {
    env.GOOS = "linux";
    env.CGO_ENABLED = process.env.CGO_ENABLED || "1";
  } else if (target === "darwin") {
    env.GOOS = "darwin";
    env.CGO_ENABLED = process.env.CGO_ENABLED || "1";
  }
  return env;
}

function targetOutputPath(target, targetArch) {
  const outputDir = targetOutputDir(target, targetArch);
  if (target === "darwin") {
    return path.join(outputDir, ".build", "JFTrade");
  }
  if (target === "windows") {
    return path.join(outputDir, `jftrade-desktop-windows-${targetArch}.exe`);
  }
  return path.join(outputDir, `jftrade-desktop-linux-${targetArch}`);
}

function targetOutputDir(target, targetArch) {
  return path.join(desktopDistDir, `${target}-${targetArch}`);
}
