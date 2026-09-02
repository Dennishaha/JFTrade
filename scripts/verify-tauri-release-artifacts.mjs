#!/usr/bin/env node

import { createHash } from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

import { inspectArtifact } from "./check-zero-go.mjs";

const repositoryRoot = fileURLToPath(new URL("..", import.meta.url));
const manifestSchema = "jftrade.tauri-release-artifacts.v1";
const platformNames = new Set([
  "macos-arm64",
  "linux-x64",
  "windows-x64",
  "windows-arm64",
]);

const packageSpecs = Object.freeze({
  "macos-arm64": [
    { directory: "dmg", extension: ".dmg", kind: "dmg" },
  ],
  "linux-x64": [
    { directory: "appimage", extension: ".AppImage", kind: "appimage" },
    { directory: "deb", extension: ".deb", kind: "deb" },
    { directory: "rpm", extension: ".rpm", kind: "rpm" },
  ],
  "windows-x64": [
    { directory: "nsis", extension: "-setup.exe", kind: "nsis" },
  ],
  "windows-arm64": [
    { directory: "nsis", extension: "-setup.exe", kind: "nsis" },
  ],
});

function sha256(filePath) {
  return createHash("sha256").update(fs.readFileSync(filePath)).digest("hex");
}

function requireVersion(version) {
  if (!/^\d+\.\d+\.\d+$/.test(version)) {
    throw new Error(`Tauri release version must be numeric semver: ${version}`);
  }
  return version;
}

function requirePlatform(platform) {
  if (!platformNames.has(platform)) {
    throw new Error(`Unsupported Tauri release platform: ${platform}`);
  }
  return platform;
}

function listFiles(directory) {
  if (!fs.existsSync(directory)) return [];
  return fs
    .readdirSync(directory, { withFileTypes: true })
    .filter((entry) => entry.isFile())
    .map((entry) => path.join(directory, entry.name));
}

function findPackage(bundleRoot, version, platform, spec, relativeRoot) {
  const directory = path.join(bundleRoot, spec.directory);
  const candidates = listFiles(directory).filter((filePath) => {
    const name = path.basename(filePath);
    return name.includes(version) && name.endsWith(spec.extension);
  });
  if (candidates.length !== 1) {
    throw new Error(
      `${platform} ${spec.kind} package count is ${candidates.length}; expected exactly one in ${directory}`,
    );
  }
  const filePath = candidates[0];
  const stat = fs.statSync(filePath);
  if (stat.size === 0) throw new Error(`Tauri package is empty: ${filePath}`);
  return {
    kind: spec.kind,
    path: path.relative(relativeRoot, filePath).split(path.sep).join("/"),
    sha256: sha256(filePath),
    size: stat.size,
  };
}

function findAppBundle(bundleRoot, version, relativeRoot) {
  const directory = path.join(bundleRoot, "macos");
  const candidates = fs.existsSync(directory)
    ? fs
        .readdirSync(directory, { withFileTypes: true })
        .filter((entry) => entry.isDirectory() && entry.name.endsWith(".app"))
        .map((entry) => path.join(directory, entry.name))
    : [];
  if (candidates.length !== 1) {
    throw new Error(`macos app bundle count is ${candidates.length}; expected exactly one in ${directory}`);
  }
  const app = candidates[0];
  const executable = path.join(app, "Contents/MacOS/jftrade-desktop");
  if (!fs.existsSync(executable) || fs.statSync(executable).size === 0) {
    throw new Error(`macos app bundle executable is missing or empty: ${executable}`);
  }
  return {
    kind: "app-bundle",
    path: path.relative(relativeRoot, app).split(path.sep).join("/"),
    version,
  };
}

function collectUpdaterSignatures(bundleRoot, relativeRoot) {
  const files = [];
  const pending = [bundleRoot];
  while (pending.length > 0) {
    const directory = pending.pop();
    for (const entry of fs.readdirSync(directory, { withFileTypes: true })) {
      const filePath = path.join(directory, entry.name);
      if (entry.isDirectory()) pending.push(filePath);
      else if (entry.name.endsWith(".sig")) files.push(filePath);
    }
  }
  return files.sort().map((filePath) => ({
    path: path.relative(relativeRoot, filePath).split(path.sep).join("/"),
    sha256: sha256(filePath),
    size: fs.statSync(filePath).size,
  }));
}

function collectUpdaterArchives(bundleRoot, relativeRoot) {
  const files = [];
  const pending = [bundleRoot];
  while (pending.length > 0) {
    const directory = pending.pop();
    for (const entry of fs.readdirSync(directory, { withFileTypes: true })) {
      const filePath = path.join(directory, entry.name);
      if (entry.isDirectory()) pending.push(filePath);
      else if (entry.name.endsWith(".tar.gz") || entry.name.endsWith(".zip")) files.push(filePath);
    }
  }
  return files.sort().map((filePath) => ({
    path: path.relative(relativeRoot, filePath).split(path.sep).join("/"),
    sha256: sha256(filePath),
    size: fs.statSync(filePath).size,
  }));
}

export function inspectTauriReleaseArtifacts({
  root = repositoryRoot,
  bundleRoot = path.join(root, "apps/desktop/src-tauri/target/release/bundle"),
  platform,
  version,
  architecture,
  requireUpdater = false,
} = {}) {
  requirePlatform(platform);
  requireVersion(version);
  const relativeRoot = path.resolve(root);
  if (!fs.existsSync(bundleRoot)) throw new Error(`Tauri bundle directory is missing: ${bundleRoot}`);
  const zeroGoErrors = inspectArtifact(bundleRoot);
  if (zeroGoErrors.length > 0) {
    throw new Error(`Tauri bundle failed the zero-Go gate:\n${zeroGoErrors.map((error) => `- ${error}`).join("\n")}`);
  }
  const packages = packageSpecs[platform].map((spec) =>
    findPackage(bundleRoot, version, platform, spec, relativeRoot),
  );
  const appBundle = platform === "macos-arm64"
    ? findAppBundle(bundleRoot, version, relativeRoot)
    : null;
  const signatures = collectUpdaterSignatures(bundleRoot, relativeRoot);
  const updaterArchives = collectUpdaterArchives(bundleRoot, relativeRoot);
  if (requireUpdater && (signatures.length === 0 || updaterArchives.length === 0)) {
    throw new Error(`${platform} publish requires Tauri updater archive(s) and signature(s)`);
  }
  return {
    schemaVersion: manifestSchema,
    target: { architecture, platform },
    version,
    scope: "package-and-integrity",
    packages,
    appBundle,
    updaterSignatures: signatures,
    updaterArchives,
    limitations: [
      "Package presence and SHA-256 are verified by this script.",
      "Code-signing validity, native install/upgrade/uninstall/rollback and post-release runtime require the matching platform runner.",
    ],
  };
}

export function writeTauriReleaseArtifactManifest(options = {}) {
  const manifest = inspectTauriReleaseArtifacts(options);
  const outputPath = options.outputPath ?? path.join(
    options.root ?? repositoryRoot,
    "artifacts",
    `tauri-release-${manifest.target.platform}.json`,
  );
  fs.mkdirSync(path.dirname(outputPath), { recursive: true });
  fs.writeFileSync(outputPath, `${JSON.stringify(manifest, null, 2)}\n`, "utf8");
  return manifest;
}

function parseArgs(args) {
  const values = {};
  for (let index = 0; index < args.length; index += 1) {
    const argument = args[index];
    if (!argument.startsWith("--")) throw new Error(`unknown argument: ${argument}`);
    const [key, inline] = argument.slice(2).split("=", 2);
    if (key === "require-updater") {
      values.requireUpdater = true;
      continue;
    }
    const value = inline ?? args[++index];
    if (!value) throw new Error(`missing value for --${key}`);
    values[
      { "bundle-root": "bundleRoot", output: "outputPath" }[key] ?? key
    ] = value;
  }
  for (const key of ["platform", "version", "architecture"]) {
    if (!values[key]) throw new Error(`missing required --${key}`);
  }
  return values;
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const manifest = writeTauriReleaseArtifactManifest(parseArgs(process.argv.slice(2)));
    console.log(
      `Verified Tauri ${manifest.target.platform} package manifest with ${manifest.packages.length} package(s) and ${manifest.updaterSignatures.length} updater signature(s).`,
    );
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  }
}
