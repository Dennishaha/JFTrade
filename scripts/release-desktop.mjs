import fs from "node:fs";
import path from "node:path";
import process from "node:process";

import { spawnChecked } from "./lib/spawn.mjs";
import { macBundleIdentifier } from "./lib/mac-app-bundle.mjs";

const rootDir = path.resolve(import.meta.dirname, "..");
const desktopDistDir = path.join(rootDir, "dist", "desktop");
const desktopReleaseDir = path.join(rootDir, "dist", "desktop-release");

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
const requestedSpec = parseTargetSpec(process.argv[2] || currentTarget);
const requestedTarget = requestedSpec.target;

if (!requestedTarget) {
  console.error(`Unknown desktop release target: ${process.argv[2]}`);
  process.exit(1);
}

const arch = process.env.GOARCH || process.argv[3] || requestedSpec.arch || defaultArch(requestedTarget);
const releaseName = releaseTargetName(requestedTarget, arch);
const status = spawnChecked("node", ["scripts/build-desktop.mjs", releaseName], { cwd: rootDir });
if (status !== 0) process.exit(status);

const releaseDir = path.join(desktopReleaseDir, releaseName);
const sourceBundle = path.join(targetOutputDir(requestedTarget, arch), "JFTrade.app");
const source = targetOutputPath(requestedTarget, arch);
const destination =
  requestedTarget === "darwin" ? path.join(releaseDir, "JFTrade.app") : path.join(releaseDir, path.basename(source));

cleanupLegacyReleaseOutputs();
fs.mkdirSync(releaseDir, { recursive: true });
if (requestedTarget === "darwin") {
  fs.rmSync(destination, { recursive: true, force: true });
  fs.cpSync(sourceBundle, destination, { recursive: true });
} else {
  fs.copyFileSync(source, destination);
}
verifyReleaseArtifact(requestedTarget, destination);

console.log(`Desktop ${releaseName} release artifact verified at ${destination}`);

function parseTargetSpec(value) {
  const normalized = String(value || "").toLowerCase();
  const archSuffix = normalized.match(/^(.*?)[-_:](amd64|arm64)$/);
  if (archSuffix) {
    return { target: platformAliases[archSuffix[1]] || "", arch: archSuffix[2] };
  }
  return { target: platformAliases[normalized] || "", arch: "" };
}

function defaultArch(target) {
  if (target === "darwin" && currentTarget === "darwin") {
    return process.arch === "arm64" ? "arm64" : "amd64";
  }
  return "amd64";
}

function releaseTargetName(target, targetArch) {
  return `${target}-${targetArch}`;
}

function cleanupLegacyReleaseOutputs() {
  for (const entry of ["darwin", "windows", "linux"]) {
    fs.rmSync(path.join(desktopReleaseDir, entry), { recursive: true, force: true });
  }
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
  return path.join(desktopDistDir, releaseTargetName(target, targetArch));
}

function verifyReleaseArtifact(target, artifactPath) {
  if (!fs.existsSync(artifactPath)) {
    console.error(`Desktop release artifact is missing: ${artifactPath}`);
    process.exit(1);
  }
  const stat = fs.statSync(artifactPath);
  if (target !== "darwin" && !stat.isFile()) {
    console.error(`Desktop release artifact is not a file: ${artifactPath}`);
    process.exit(1);
  }
  if (target !== "darwin" && stat.size <= 0) {
    console.error(`Desktop release artifact is empty: ${artifactPath}`);
    process.exit(1);
  }
  if (target === "darwin") {
    if (!stat.isDirectory()) {
      console.error(`macOS release artifact is not an app bundle: ${artifactPath}`);
      process.exit(1);
    }
    const plistPath = path.join(artifactPath, "Contents", "Info.plist");
    const plist = fs.readFileSync(plistPath, "utf8");
    if (!plist.includes("<key>CFBundleIdentifier</key>") || !plist.includes(`<string>${macBundleIdentifier}</string>`)) {
      console.error(`macOS Info.plist is missing bundle identifier ${macBundleIdentifier}`);
      process.exit(1);
    }
  }
}
