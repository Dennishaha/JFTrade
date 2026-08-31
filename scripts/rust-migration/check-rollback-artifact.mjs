#!/usr/bin/env node

import { createHash } from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));

export const ROLLBACK_ARTIFACT_SCHEMA = "jftrade.rollback-artifact.v1";
export const TAURI_RELEASE_MANIFEST_SCHEMA = "jftrade.tauri-release-artifacts.v1";
export const ROLLBACK_PLATFORMS = Object.freeze([
  "macos-arm64",
  "linux-x64",
  "windows-x64",
  "windows-arm64",
]);

const platformSpecs = Object.freeze({
  "macos-arm64": { architecture: "arm64", packageKinds: ["dmg"] },
  "linux-x64": { architecture: "amd64", packageKinds: ["appimage", "deb", "rpm"] },
  "windows-x64": { architecture: "amd64", packageKinds: ["nsis"] },
  "windows-arm64": { architecture: "arm64", packageKinds: ["nsis"] },
});

const updaterTargetAliases = Object.freeze({
  "macos-arm64": ["darwin-aarch64", "darwin-arm64", "macos-arm64"],
  "linux-x64": ["linux-x86_64", "linux-amd64", "linux-x64"],
  "windows-x64": ["windows-x86_64", "windows-amd64", "windows-x64"],
  "windows-arm64": ["windows-aarch64", "windows-arm64"],
});

function isRecord(value) {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function requireString(value, label) {
  if (typeof value !== "string" || value.trim() === "") {
    throw new Error(`${label} must be a non-empty string`);
  }
  return value.trim();
}

function requireSemver(value, label) {
  const version = requireString(value, label);
  if (!/^\d+\.\d+\.\d+$/.test(version)) {
    throw new Error(`${label} must be numeric semver: ${version}`);
  }
  return version;
}

function compareVersions(left, right) {
  const a = requireSemver(left, "left version").split(".").map(Number);
  const b = requireSemver(right, "right version").split(".").map(Number);
  for (let index = 0; index < a.length; index += 1) {
    if (a[index] !== b[index]) return a[index] > b[index] ? 1 : -1;
  }
  return 0;
}

export function validateVersionTransition({
  currentVersion,
  previousVersion,
  allowDowngrade = false,
} = {}) {
  const current = requireSemver(currentVersion, "currentVersion");
  const previous = requireSemver(previousVersion, "previousVersion");
  const order = compareVersions(current, previous);
  if (order < 0) {
    throw new Error(
      `rollback target ${previous} is newer than current version ${current}`,
    );
  }
  if (order === 0) {
    throw new Error(`rollback target and current version are identical: ${current}`);
  }
  if (!allowDowngrade) {
    throw new Error(
      `version downgrade from ${current} to ${previous} is refused without explicit allowDowngrade`,
    );
  }
  return {
    currentVersion: current,
    previousVersion: previous,
    downgrade: true,
    downgradeAllowed: true,
  };
}

function readJson(filePath, label) {
  let content;
  try {
    content = fs.readFileSync(filePath, "utf8");
  } catch (error) {
    throw new Error(`cannot read ${label} ${filePath}: ${error.message}`);
  }
  try {
    return JSON.parse(content);
  } catch (error) {
    throw new Error(`cannot parse ${label} ${filePath}: ${error.message}`);
  }
}

function sha256(filePath) {
  return createHash("sha256").update(fs.readFileSync(filePath)).digest("hex");
}

function walkFiles(directory) {
  if (!fs.existsSync(directory)) return [];
  const files = [];
  const pending = [directory];
  while (pending.length > 0) {
    const current = pending.pop();
    for (const entry of fs.readdirSync(current, { withFileTypes: true })) {
      const filePath = path.join(current, entry.name);
      if (entry.isDirectory()) pending.push(filePath);
      else if (entry.isFile()) files.push(filePath);
    }
  }
  return files.sort();
}

function assertDirectory(directory, label) {
  if (!fs.existsSync(directory) || !fs.statSync(directory).isDirectory()) {
    throw new Error(`${label} directory is missing: ${directory}`);
  }
  return path.resolve(directory);
}

function resolveContained(root, relativePath, label) {
  const value = requireString(relativePath, `${label}.path`);
  if (path.isAbsolute(value)) throw new Error(`${label}.path must be relative to its release root`);
  const resolvedRoot = path.resolve(root);
  const resolved = path.resolve(resolvedRoot, value);
  const relative = path.relative(resolvedRoot, resolved);
  if (relative === ".." || relative.startsWith(`..${path.sep}`)) {
    throw new Error(`${label}.path escapes its release root: ${value}`);
  }
  return resolved;
}

// The publish job flattens uploaded artifacts before attaching them to the
// release, while tauri-release manifests retain paths from the build tree.
// Prefer the declared path, then accept one unambiguous basename match so a
// flattened release can still be audited without weakening traversal checks.
function resolveArtifact(root, relativePath, label) {
  const declared = resolveContained(root, relativePath, label);
  if (fs.existsSync(declared)) return declared;
  const basename = path.basename(declared);
  const matches = walkFiles(root).filter((filePath) => path.basename(filePath) === basename);
  if (matches.length === 1) return matches[0];
  return declared;
}

function requireFile(filePath, label) {
  if (!fs.existsSync(filePath) || !fs.statSync(filePath).isFile()) {
    throw new Error(`${label} is missing: ${filePath}`);
  }
  if (fs.statSync(filePath).size === 0) throw new Error(`${label} is empty: ${filePath}`);
  return filePath;
}

function requireDigest(entry, filePath, label) {
  if (!isRecord(entry)) throw new Error(`${label} must be an object`);
  const expected = requireString(entry.sha256, `${label}.sha256`);
  if (!/^[a-f0-9]{64}$/.test(expected)) throw new Error(`${label}.sha256 must be lowercase SHA-256`);
  const actual = sha256(filePath);
  if (actual !== expected) throw new Error(`${label}.sha256 does not match ${path.basename(filePath)}`);
  if (!Number.isInteger(entry.size) || entry.size <= 0) throw new Error(`${label}.size must be a positive integer`);
  const size = fs.statSync(filePath).size;
  if (size !== entry.size) throw new Error(`${label}.size does not match ${path.basename(filePath)}`);
}

function findNamedFile(root, names, label) {
  const files = walkFiles(root);
  const matches = files.filter((filePath) => names.includes(path.basename(filePath)));
  if (matches.length !== 1) {
    throw new Error(`${label} count is ${matches.length}; expected exactly one under ${root}`);
  }
  return matches[0];
}

function manifestPathFor(root, platform, overrides) {
  const override = overrides?.[platform];
  if (override) return resolveContained(root, override, `${platform} manifest`);
  return findNamedFile(root, [`tauri-release-${platform}.json`], `${platform} package manifest`);
}

function normalizeSignature(value, label) {
  const signature = requireString(value, label).replaceAll("\r\n", "\n");
  if (signature.length < 8) throw new Error(`${label} is too short to be a signature`);
  return signature;
}

function feedMetadataPath(root, override) {
  if (override) return resolveContained(root, override, "updater metadata");
  return findNamedFile(root, ["latest.json", "updater.json"], "updater metadata");
}

function validateManifest(manifest, filePath, platform, root, expectedVersion) {
  const label = `${platform} package manifest`;
  if (!isRecord(manifest)) throw new Error(`${label} must be an object`);
  if (manifest.schemaVersion !== TAURI_RELEASE_MANIFEST_SCHEMA) {
    throw new Error(`${label}.schemaVersion must be ${TAURI_RELEASE_MANIFEST_SCHEMA}`);
  }
  if (manifest.scope !== "package-and-integrity") throw new Error(`${label}.scope must be package-and-integrity`);
  const version = requireSemver(manifest.version, `${label}.version`);
  if (version !== expectedVersion) throw new Error(`${label}.version ${version} does not match ${expectedVersion}`);
  if (!isRecord(manifest.target)) throw new Error(`${label}.target must be an object`);
  if (manifest.target.platform !== platform) throw new Error(`${label}.target.platform must be ${platform}`);
  if (manifest.target.architecture !== platformSpecs[platform].architecture) {
    throw new Error(`${label}.target.architecture must be ${platformSpecs[platform].architecture}`);
  }
  if (!Array.isArray(manifest.packages) || manifest.packages.length === 0) {
    throw new Error(`${label}.packages must be a non-empty array`);
  }
  const expectedKinds = new Set(platformSpecs[platform].packageKinds);
  const seenKinds = new Set();
  for (const [index, entry] of manifest.packages.entries()) {
    const entryLabel = `${label}.packages[${index}]`;
    if (!isRecord(entry)) throw new Error(`${entryLabel} must be an object`);
    const kind = requireString(entry.kind, `${entryLabel}.kind`);
    if (!expectedKinds.has(kind)) throw new Error(`${entryLabel}.kind is not expected for ${platform}: ${kind}`);
    if (seenKinds.has(kind)) throw new Error(`${label} contains duplicate package kind: ${kind}`);
    seenKinds.add(kind);
    const filePath = resolveArtifact(root, entry.path, entryLabel);
    requireFile(filePath, entryLabel);
    if (!path.basename(filePath).includes(version)) throw new Error(`${entryLabel}.path must contain release version ${version}`);
    requireDigest(entry, filePath, entryLabel);
  }
  for (const kind of expectedKinds) if (!seenKinds.has(kind)) throw new Error(`${label} is missing package kind: ${kind}`);
  const archives = new Map();
  const signatureArchives = new Set();
  if ("updaterArchives" in manifest && !Array.isArray(manifest.updaterArchives)) {
    throw new Error(`${label}.updaterArchives must be an array when present`);
  }
  if (Array.isArray(manifest.updaterArchives) && manifest.updaterArchives.length > 0) {
    for (const [index, entry] of manifest.updaterArchives.entries()) {
      const entryLabel = `${label}.updaterArchives[${index}]`;
      const filePath = resolveArtifact(root, entry.path, entryLabel);
      requireFile(filePath, entryLabel);
      if (!(filePath.endsWith(".tar.gz") || filePath.endsWith(".zip"))) throw new Error(`${entryLabel}.path must be a .tar.gz or .zip archive`);
      if (!path.basename(filePath).includes(version)) throw new Error(`${entryLabel}.path must contain release version ${version}`);
      requireDigest(entry, filePath, entryLabel);
      const archiveName = path.basename(filePath);
      if (archives.has(archiveName)) throw new Error(`${label} contains duplicate updater archive: ${archiveName}`);
      archives.set(archiveName, filePath);
      requireFile(`${filePath}.sig`, `${entryLabel} sidecar signature`);
    }
  }
  if (!Array.isArray(manifest.updaterSignatures) || manifest.updaterSignatures.length === 0) {
    throw new Error(`${label}.updaterSignatures must contain sidecar signatures`);
  }
  for (const [index, entry] of manifest.updaterSignatures.entries()) {
    const entryLabel = `${label}.updaterSignatures[${index}]`;
    const filePath = resolveArtifact(root, entry.path, entryLabel);
    requireFile(filePath, entryLabel);
    if (!filePath.endsWith(".sig")) throw new Error(`${entryLabel}.path must end with .sig`);
    requireDigest(entry, filePath, entryLabel);
    const archiveName = path.basename(filePath).slice(0, -4);
    if (!archives.has(archiveName)) {
      if (Array.isArray(manifest.updaterArchives)) {
        throw new Error(`${entryLabel} has no matching updater archive`);
      }
      const archivePath = resolveArtifact(root, entry.path.slice(0, -4), `${entryLabel} archive`);
      requireFile(archivePath, `${entryLabel} archive`);
      if (!(archivePath.endsWith(".tar.gz") || archivePath.endsWith(".zip"))) throw new Error(`${entryLabel} archive must be a .tar.gz or .zip file`);
      if (!path.basename(archivePath).includes(version)) throw new Error(`${entryLabel} archive must contain release version ${version}`);
      archives.set(archiveName, archivePath);
    }
    signatureArchives.add(archiveName);
  }
  if (archives.size === 0) throw new Error(`${label} has no updater archives`);
  for (const [archiveName, archivePath] of archives) {
    if (!signatureArchives.has(archiveName)) throw new Error(`${label} is missing sidecar metadata for ${archiveName}`);
    requireFile(`${archivePath}.sig`, `${archiveName} sidecar signature`);
  }
  return {
    platform,
    architecture: manifest.target.architecture,
    manifestPath: path.relative(root, filePath).split(path.sep).join("/"),
    packageCount: manifest.packages.length,
    archiveNames: [...archives.keys()].sort(),
    archiveFiles: [...archives.values()],
  };
}

function feedTargetForPlatform(platform, feedPlatforms) {
  const target = updaterTargetAliases[platform].find((candidate) => candidate in feedPlatforms);
  if (!target) {
    throw new Error(`updater metadata is missing target for ${platform} (${updaterTargetAliases[platform].join(", ")})`);
  }
  return target;
}

function validateUpdaterMetadata({ root, feedPath, expectedVersion, manifests }) {
  const feed = readJson(feedPath, "Tauri updater metadata");
  if (!isRecord(feed)) throw new Error("Tauri updater metadata must be an object");
  const version = requireSemver(feed.version, "updater metadata.version");
  if (version !== expectedVersion) throw new Error(`updater metadata.version ${version} does not match ${expectedVersion}`);
  if (!isRecord(feed.platforms) || Object.keys(feed.platforms).length === 0) throw new Error("updater metadata.platforms must be a non-empty object");
  const archives = new Map();
  for (const [platform, manifest] of Object.entries(manifests)) {
    for (const filePath of manifest.archiveFiles) {
      const name = path.basename(filePath);
      if (archives.has(name) && archives.get(name).platform !== platform) {
        throw new Error(`updater archive filename is shared by multiple platforms: ${name}`);
      }
      archives.set(name, { filePath, platform });
    }
  }
  const targets = {};
  for (const platform of ROLLBACK_PLATFORMS) {
    const target = feedTargetForPlatform(platform, feed.platforms);
    const entry = feed.platforms[target];
    if (!isRecord(entry)) throw new Error(`updater metadata.platforms.${target} must be an object`);
    let url;
    try {
      url = new URL(requireString(entry.url, `updater metadata.platforms.${target}.url`));
    } catch (error) {
      throw new Error(`updater metadata.platforms.${target}.url must be a valid URL: ${error.message}`);
    }
    if (url.protocol !== "https:") throw new Error(`updater metadata.platforms.${target}.url must use HTTPS`);
    const archiveName = decodeURIComponent(path.posix.basename(url.pathname));
    if (!archiveName.endsWith(".tar.gz") && !archiveName.endsWith(".zip")) throw new Error(`updater metadata.platforms.${target}.url must identify an updater archive`);
    const archive = archives.get(archiveName);
    if (!archive) throw new Error(`updater metadata.platforms.${target} archive is not represented by a package manifest: ${archiveName}`);
    if (archive.platform !== platform) throw new Error(`updater metadata.platforms.${target} archive belongs to ${archive.platform}, not ${platform}`);
    const feedSignature = normalizeSignature(entry.signature, `updater metadata.platforms.${target}.signature`);
    const sidecar = normalizeSignature(fs.readFileSync(`${archive.filePath}.sig`, "utf8"), `${archiveName}.sig`);
    if (feedSignature !== sidecar) throw new Error(`updater metadata.platforms.${target}.signature does not match ${archiveName}.sig`);
    targets[platform] = { target, archive: archiveName, signatureSha256: createHash("sha256").update(sidecar).digest("hex") };
  }
  return {
    path: path.relative(root, feedPath).split(path.sep).join("/"),
    version,
    targets,
    cryptographicVerification: "deferred-to-tauri-plugin-updater",
  };
}

function inspectRelease({ root, expectedVersion, manifestPaths, updaterMetadata } = {}) {
  const releaseRoot = assertDirectory(root, "release");
  const version = requireSemver(expectedVersion, "expectedVersion");
  const manifests = {};
  for (const platform of ROLLBACK_PLATFORMS) {
    const manifestFile = manifestPathFor(releaseRoot, platform, manifestPaths);
    const parsed = readJson(manifestFile, "Tauri package manifest");
    const report = validateManifest(parsed, manifestFile, platform, releaseRoot, version);
    manifests[platform] = { ...report, path: manifestFile };
  }
  const feedPath = feedMetadataPath(releaseRoot, updaterMetadata);
  const feed = validateUpdaterMetadata({ root: releaseRoot, feedPath, expectedVersion: version, manifests });
  return {
    version,
    root: releaseRoot,
    manifests,
    updaterMetadata: feed,
    packageSigning: "not-verified-by-node",
  };
}

function findInstructions(currentRoot, previousRoot, override) {
  if (override) return path.resolve(override);
  for (const root of [currentRoot, previousRoot]) {
    for (const name of ["rollback.md", "ROLLBACK.md", "rollback-instructions.md"]) {
      const candidate = path.join(root, name);
      if (fs.existsSync(candidate)) return candidate;
    }
  }
  throw new Error("rollback instructions are required (pass --instructions or add rollback.md)");
}

function validateInstructions(filePath, currentVersion, previousVersion) {
  requireFile(filePath, "rollback instructions");
  const text = fs.readFileSync(filePath, "utf8").trim();
  if (!/(rollback|revert|downgrade|回退|降级)/i.test(text)) throw new Error("rollback instructions must describe an explicit rollback/downgrade");
  if (!text.includes(currentVersion) || !text.includes(previousVersion)) {
    throw new Error(`rollback instructions must name both ${currentVersion} and ${previousVersion}`);
  }
  return {
    path: filePath,
    bytes: Buffer.byteLength(text),
    namesVersions: true,
  };
}

export function inspectRollbackArtifactPair({
  currentRoot,
  previousRoot,
  currentVersion,
  previousVersion,
  allowDowngrade = false,
  instructionsPath,
  currentManifestPaths,
  previousManifestPaths,
  currentUpdaterMetadata,
  previousUpdaterMetadata,
} = {}) {
  const transition = validateVersionTransition({ currentVersion, previousVersion, allowDowngrade });
  const current = inspectRelease({
    root: currentRoot,
    expectedVersion: transition.currentVersion,
    manifestPaths: currentManifestPaths,
    updaterMetadata: currentUpdaterMetadata,
  });
  const previous = inspectRelease({
    root: previousRoot,
    expectedVersion: transition.previousVersion,
    manifestPaths: previousManifestPaths,
    updaterMetadata: previousUpdaterMetadata,
  });
  const instructions = validateInstructions(
    findInstructions(current.root, previous.root, instructionsPath),
    transition.currentVersion,
    transition.previousVersion,
  );
  return {
    schemaVersion: ROLLBACK_ARTIFACT_SCHEMA,
    scope: "previous-version-integrity-and-pairing",
    versionPolicy: transition,
    current: {
      version: current.version,
      root: current.root,
      platforms: current.manifests,
      updaterMetadata: current.updaterMetadata,
      packageSigning: current.packageSigning,
    },
    previous: {
      version: previous.version,
      root: previous.root,
      platforms: previous.manifests,
      updaterMetadata: previous.updaterMetadata,
      packageSigning: previous.packageSigning,
    },
    rollbackInstructions: instructions,
    limitations: [
      "Package and updater sidecar presence, hashes and version pairing are verified by this script.",
      "Minisign/AuthentiCode/codesign validity and native install, downgrade, rollback and runtime recovery require the matching platform release runner.",
      "This report does not change the Stage 9 closeout manifest or claim a previous release was published.",
    ],
  };
}

export const validateRollbackArtifactPair = inspectRollbackArtifactPair;

function copyAndRenameAtomically(sourceRoot, destinationRoot) {
  const source = assertDirectory(sourceRoot, "rollback source");
  const destination = path.resolve(destinationRoot);
  if (source === destination) throw new Error("rollback source and retained destination must differ");
  if (fs.existsSync(destination)) throw new Error(`retained rollback artifact already exists; refusing overwrite: ${destination}`);
  fs.mkdirSync(path.dirname(destination), { recursive: true });
  const staging = `${destination}.staging-${process.pid}-${Date.now()}`;
  try {
    fs.cpSync(source, staging, { recursive: true, errorOnExist: true, force: false });
    fs.renameSync(staging, destination);
  } catch (error) {
    fs.rmSync(staging, { recursive: true, force: true });
    throw new Error(`atomic rollback artifact retention failed: ${error.message}`);
  }
  return {
    source,
    destination,
    sourcePreserved: fs.existsSync(source),
    atomicRename: true,
  };
}

export function atomicallyRetainRollbackArtifact({ sourceRoot, retainedRoot, version } = {}) {
  const releaseVersion = requireSemver(version, "version");
  const retainedRootPath = assertDirectoryOrCreate(retainedRoot, "retained rollback");
  return copyAndRenameAtomically(sourceRoot, path.join(retainedRootPath, releaseVersion));
}

function assertDirectoryOrCreate(directory, label) {
  if (!directory) throw new Error(`${label} directory is required`);
  const resolved = path.resolve(directory);
  if (fs.existsSync(resolved) && !fs.statSync(resolved).isDirectory()) throw new Error(`${label} path is not a directory: ${resolved}`);
  fs.mkdirSync(resolved, { recursive: true });
  return resolved;
}

export function runRollbackArtifactHarness({
  currentRoot,
  previousRoot,
  retainedRoot,
  currentVersion,
  previousVersion,
  instructionsPath,
  allowDowngrade = true,
  ...options
} = {}) {
  const report = inspectRollbackArtifactPair({
    currentRoot,
    previousRoot,
    currentVersion,
    previousVersion,
    instructionsPath,
    allowDowngrade,
    ...options,
  });
  const retention = atomicallyRetainRollbackArtifact({
    sourceRoot: previousRoot,
    retainedRoot,
    version: report.previous.version,
  });
  return {
    ...report,
    retention: {
      ...retention,
      retainedVersion: report.previous.version,
      rollbackInstall: "not-executed",
    },
  };
}

function parseArgs(args) {
  const options = {};
  for (let index = 0; index < args.length; index += 1) {
    const argument = args[index];
    if (!argument.startsWith("--")) throw new Error(`unknown argument: ${argument}`);
    const [key, inline] = argument.slice(2).split("=", 2);
    if (key === "allow-downgrade") {
      options.allowDowngrade = true;
      continue;
    }
    const value = inline ?? args[++index];
    if (!value) throw new Error(`missing value for --${key}`);
    options[
      {
        "current-dir": "currentRoot",
        "previous-dir": "previousRoot",
        current: "currentVersion",
        previous: "previousVersion",
        instructions: "instructionsPath",
        "current-updater": "currentUpdaterMetadata",
        "previous-updater": "previousUpdaterMetadata",
      }[key] ?? key
    ] = value;
  }
  for (const key of ["currentRoot", "previousRoot", "currentVersion", "previousVersion"]) {
    if (!options[key]) throw new Error(`missing required --${key.replace(/[A-Z]/g, (letter) => `-${letter.toLowerCase()}`)}`);
  }
  return options;
}

export function main(args = process.argv.slice(2)) {
  try {
    const report = inspectRollbackArtifactPair(parseArgs(args));
    console.log(`Verified rollback artifact pairing ${report.current.version} -> ${report.previous.version}.`);
    console.log("Package/updater hashes and rollback instructions are present; cryptographic and native runner evidence remains deferred.");
    return 0;
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    return 1;
  }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  process.exitCode = main();
}
