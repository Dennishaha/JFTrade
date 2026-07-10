import fs from "node:fs";
import path from "node:path";
import process from "node:process";

import {
  requireDesktopReleaseMetadata,
  resolveDesktopBuildMetadata,
} from "./lib/desktop-release-metadata.mjs";
import { macBundleIdentifier } from "./lib/mac-app-bundle.mjs";
import { spawnChecked } from "./lib/spawn.mjs";

const rootDir = path.resolve(import.meta.dirname, "..");
const desktopDistDir = path.join(rootDir, "dist", "desktop");
const desktopReleaseDir = path.join(rootDir, "dist", "desktop-release");
const metadata = requireReleaseMetadata();

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
if (!requestedSpec.target) {
  fail(`Unknown desktop release target: ${process.argv[2]}`);
}
if (requestedSpec.target !== currentTarget) {
  fail(
    `Desktop releases must run on their native OS (${requestedSpec.target} requested on ${currentTarget}).`,
  );
}

const artifacts =
  requestedSpec.target === "darwin"
    ? releaseMacArm64(requestedSpec.arch)
    : requestedSpec.target === "windows"
      ? releaseWindows(requestedSpec.arch || "amd64")
      : releaseLinux(requestedSpec.arch || "amd64");

for (const artifact of artifacts) {
  verifyNonEmptyFile(artifact);
  console.log(`Desktop release artifact verified at ${artifact}`);
}

function releaseMacArm64(requestedArch) {
  if (requestedArch && requestedArch !== "arm64") {
    fail("macOS releases only support arm64.");
  }
  buildTarget("darwin", "arm64");

  const releaseDir = freshReleaseDir("darwin-arm64");
  const appPath = path.join(releaseDir, "JFTrade.app");
  const arm64App = path.join(targetOutputDir("darwin", "arm64"), "JFTrade.app");
  fs.cpSync(arm64App, appPath, { recursive: true });

  const executable = path.join(appPath, "Contents", "MacOS", "JFTrade");
  run("lipo", ["-verify_arch", "arm64", executable]);

  verifyMacBundle(appPath);

  const dmgPath = path.join(
    releaseDir,
    `JFTrade-${metadata.version}-macos-arm64-unsigned.dmg`,
  );
  const temporaryDmgPath = path.join(
    desktopReleaseDir,
    `.JFTrade-${metadata.version}-macos-arm64-unsigned.tmp.dmg`,
  );
  fs.rmSync(temporaryDmgPath, { force: true });
  run("diskutil", [
    "image",
    "create",
    "from",
    "--format",
    "UDZO",
    "--volumeName",
    "JFTrade",
    releaseDir,
    temporaryDmgPath,
  ]);
  fs.renameSync(temporaryDmgPath, dmgPath);
  return [dmgPath];
}

function releaseWindows(arch) {
  buildTarget("windows", arch);
  const releaseDir = freshReleaseDir(`windows-${arch}`);
  const source = targetOutputPath("windows", arch);

  if (arch !== "amd64" && arch !== "arm64")
    fail(`Unsupported Windows release architecture: ${arch}`);

  const applicationPath = path.join(releaseDir, "JFTrade.exe");
  fs.copyFileSync(source, applicationPath);

  const installer = path.join(
    releaseDir,
    arch === "arm64"
      ? `JFTrade-${metadata.version}-windows-arm64-preview-unsigned-setup.exe`
      : `JFTrade-${metadata.version}-windows-x64-unsigned-setup.exe`,
  );
  run(process.env.MAKENSIS || "makensis", [
    `/DVERSION=${metadata.numericVersion}`,
    `/DSOURCE_EXE=${applicationPath}`,
    `/DOUTPUT_EXE=${installer}`,
    path.join(rootDir, "build", "desktop", "windows", "installer.nsi"),
  ]);
  return [installer];
}

function releaseLinux(arch) {
  buildTarget("linux", arch);
  const releaseDir = freshReleaseDir(`linux-${arch}`);
  const source = targetOutputPath("linux", arch);
  const destination = path.join(releaseDir, `jftrade-desktop-linux-${arch}`);
  fs.copyFileSync(source, destination);
  fs.chmodSync(destination, 0o755);
  return [destination];
}

function buildTarget(target, arch) {
  run(process.execPath, ["scripts/build-desktop.mjs", `${target}-${arch}`], {
    env: {
      ...process.env,
      JFTRADE_DESKTOP_RELEASE_TAG: metadata.tag,
      JFTRADE_DESKTOP_BUILD_TIME: metadata.buildTime,
      JFTRADE_DESKTOP_COMMIT: metadata.commit,
    },
  });
}

function verifyMacBundle(appPath) {
  const plist = fs.readFileSync(
    path.join(appPath, "Contents", "Info.plist"),
    "utf8",
  );
  for (const expected of [
    macBundleIdentifier,
    metadata.version,
    metadata.commit,
    metadata.buildTime,
  ]) {
    if (!plist.includes(expected))
      fail(`macOS Info.plist is missing ${expected}`);
  }
}

function requireReleaseMetadata() {
  try {
    return requireDesktopReleaseMetadata(resolveDesktopBuildMetadata());
  } catch (error) {
    fail(error instanceof Error ? error.message : String(error));
  }
}

function freshReleaseDir(name) {
  const directory = path.join(desktopReleaseDir, name);
  fs.rmSync(directory, { recursive: true, force: true });
  fs.mkdirSync(directory, { recursive: true });
  return directory;
}

function parseTargetSpec(value) {
  const normalized = String(value || "").toLowerCase();
  const match = normalized.match(/^(.*?)[-_:](amd64|arm64)$/);
  if (match) return { target: platformAliases[match[1]] || "", arch: match[2] };
  return { target: platformAliases[normalized] || "", arch: "" };
}

function targetOutputPath(target, arch) {
  const directory = targetOutputDir(target, arch);
  if (target === "windows")
    return path.join(directory, `jftrade-desktop-windows-${arch}.exe`);
  if (target === "linux")
    return path.join(directory, `jftrade-desktop-linux-${arch}`);
  return path.join(directory, ".build", "JFTrade");
}

function targetOutputDir(target, arch) {
  return path.join(desktopDistDir, `${target}-${arch}`);
}

function verifyNonEmptyFile(file) {
  if (
    !fs.existsSync(file) ||
    !fs.statSync(file).isFile() ||
    fs.statSync(file).size === 0
  ) {
    fail(`Desktop release artifact is missing or empty: ${file}`);
  }
}

function run(command, args, options = {}) {
  const status = spawnChecked(command, args, { cwd: rootDir, ...options });
  if (status !== 0) process.exit(status);
}

function fail(message) {
  console.error(message);
  process.exit(1);
}
