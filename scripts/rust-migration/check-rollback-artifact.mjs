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

const SHA256 = /^[a-f0-9]{64}$/;
const SAFE_TARGET = /^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/;

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
  if (!/^(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)$/.test(version)) {
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

function regularFileStat(filePath, label, { allowEmpty = false } = {}) {
  let stat;
  try {
    stat = fs.lstatSync(filePath);
  } catch (error) {
    throw new Error(`${label} is missing: ${filePath}: ${error.message}`);
  }
  if (stat.isSymbolicLink()) throw new Error(`${label} must not be a symbolic link: ${filePath}`);
  if (!stat.isFile()) throw new Error(`${label} must be a regular file: ${filePath}`);
  if (!allowEmpty && stat.size === 0) throw new Error(`${label} must not be empty: ${filePath}`);
  return stat;
}

function directoryStat(directory, label) {
  let stat;
  try {
    stat = fs.lstatSync(directory);
  } catch (error) {
    throw new Error(`${label} is missing: ${directory}: ${error.message}`);
  }
  if (stat.isSymbolicLink()) throw new Error(`${label} must not be a symbolic link: ${directory}`);
  if (!stat.isDirectory()) throw new Error(`${label} must be a directory: ${directory}`);
  return stat;
}

function rejectDuplicateJsonKeys(content, label) {
  let index = 0;
  const stack = [];
  const skipWhitespace = () => {
    while (/\s/.test(content[index] ?? "")) index += 1;
  };
  const parseString = () => {
    const start = index;
    index += 1;
    while (index < content.length) {
      if (content[index] === "\\") index += 2;
      else if (content[index] === '"') {
        index += 1;
        return JSON.parse(content.slice(start, index));
      } else index += 1;
    }
    throw new Error("unterminated JSON string");
  };
  const scanValue = () => {
    skipWhitespace();
    if (content[index] === "{") return scanObject();
    if (content[index] === "[") return scanArray();
    if (content[index] === '"') {
      parseString();
      return;
    }
    const start = index;
    while (index < content.length && !/[\s,\]}]/.test(content[index])) index += 1;
    if (start === index) throw new Error("invalid JSON value");
  };
  const scanObject = () => {
    index += 1;
    const objectPath = stack.join(".");
    const keys = new Set();
    stack.push("<object>");
    skipWhitespace();
    if (content[index] === "}") {
      index += 1;
      stack.pop();
      return;
    }
    while (index < content.length) {
      skipWhitespace();
      if (content[index] !== '"') throw new Error("object key must be a string");
      const key = parseString();
      if (keys.has(key)) {
        const location = objectPath ? `${objectPath}.${key}` : key;
        throw new Error(`${label} contains duplicate JSON object key: ${location}`);
      }
      keys.add(key);
      skipWhitespace();
      if (content[index] !== ":") throw new Error("object key is missing a colon");
      index += 1;
      stack[stack.length - 1] = key;
      scanValue();
      stack[stack.length - 1] = "<object>";
      skipWhitespace();
      if (content[index] === "}") {
        index += 1;
        stack.pop();
        return;
      }
      if (content[index] !== ",") throw new Error("object member is missing a comma");
      index += 1;
    }
    throw new Error("unterminated JSON object");
  };
  const scanArray = () => {
    index += 1;
    stack.push("<array>");
    skipWhitespace();
    if (content[index] === "]") {
      index += 1;
      stack.pop();
      return;
    }
    while (index < content.length) {
      scanValue();
      skipWhitespace();
      if (content[index] === "]") {
        index += 1;
        stack.pop();
        return;
      }
      if (content[index] !== ",") throw new Error("array value is missing a comma");
      index += 1;
    }
    throw new Error("unterminated JSON array");
  };
  scanValue();
  skipWhitespace();
  if (index !== content.length) throw new Error("trailing JSON content");
}

function readJson(filePath, label) {
  let content;
  try {
    regularFileStat(filePath, label);
    content = fs.readFileSync(filePath, "utf8");
  } catch (error) {
    throw new Error(`cannot read ${label} ${filePath}: ${error.message}`);
  }
  try {
    const parsed = JSON.parse(content);
    rejectDuplicateJsonKeys(content, label);
    return parsed;
  } catch (error) {
    throw new Error(`cannot parse ${label} ${filePath}: ${error.message}`);
  }
}

function sha256(filePath) {
  return createHash("sha256").update(fs.readFileSync(filePath)).digest("hex");
}

function rejectSymlinkTraversal(root, target, label) {
  const resolvedRoot = path.resolve(root);
  const resolvedTarget = path.resolve(target);
  const relative = path.relative(resolvedRoot, resolvedTarget);
  if (path.isAbsolute(relative) || relative === ".." || relative.startsWith(`..${path.sep}`)) {
    throw new Error(`${label} escapes its release root`);
  }
  let current = resolvedRoot;
  for (const segment of relative.split(path.sep).filter(Boolean)) {
    current = path.join(current, segment);
    let stat;
    try {
      stat = fs.lstatSync(current);
    } catch (error) {
      if (error.code === "ENOENT") return resolvedTarget;
      throw new Error(`${label} is unavailable: ${current}: ${error.message}`);
    }
    if (stat.isSymbolicLink()) {
      throw new Error(`${label} must not traverse a symbolic link: ${current}`);
    }
    if (current !== resolvedTarget && !stat.isDirectory()) {
      throw new Error(`${label} path component is not a directory: ${current}`);
    }
  }
  return resolvedTarget;
}

function pathContains(root, target) {
  const relative = path.relative(path.resolve(root), path.resolve(target));
  return relative === "" || (relative !== ".."
    && !relative.startsWith(`..${path.sep}`) && !path.isAbsolute(relative));
}

function commonPath(left, right) {
  let candidate = path.resolve(left);
  const target = path.resolve(right);
  while (!pathContains(candidate, target)) {
    const parent = path.dirname(candidate);
    if (parent === candidate) return null;
    candidate = parent;
  }
  return candidate;
}

function validateRetentionDestination(source, destination) {
  if (pathContains(source, destination)) {
    throw new Error("retained rollback destination must not be inside rollback source");
  }
  const destinationParent = path.dirname(destination);
  const sharedRoot = commonPath(source, destinationParent);
  if (sharedRoot) {
    rejectSymlinkTraversal(sharedRoot, source, "rollback source");
    rejectSymlinkTraversal(sharedRoot, destinationParent, "retained rollback destination parent");
  } else {
    rejectSymlinkTraversal(path.parse(source).root, source, "rollback source");
    rejectSymlinkTraversal(
      path.parse(destinationParent).root,
      destinationParent,
      "retained rollback destination parent",
    );
  }
}

function walkFiles(directory) {
  directoryStat(directory, "release artifact directory");
  const files = [];
  const pending = [directory];
  while (pending.length > 0) {
    const current = pending.pop();
    for (const entry of fs.readdirSync(current, { withFileTypes: true })) {
      const filePath = path.join(current, entry.name);
      const stat = fs.lstatSync(filePath);
      if (stat.isSymbolicLink()) {
        throw new Error(`release artifact tree must not contain a symbolic link: ${filePath}`);
      }
      if (stat.isDirectory()) pending.push(filePath);
      else if (stat.isFile()) files.push(filePath);
      else throw new Error(`release artifact tree contains a non-regular entry: ${filePath}`);
    }
  }
  return files.sort();
}

function assertDirectory(directory, label) {
  directoryStat(directory, label);
  return path.resolve(directory);
}

function validateRelativePath(value, label) {
  if (typeof value !== "string" || value !== value.trim()) {
    throw new Error(`${label}.path must not contain surrounding whitespace`);
  }
  const relative = requireString(value, `${label}.path`);
  if (path.isAbsolute(relative) || path.win32.isAbsolute(relative)
    || /^[A-Za-z]:/.test(relative) || relative.includes("\\") || relative.includes("\0")) {
    throw new Error(`${label}.path must be a safe relative POSIX path`);
  }
  const parts = relative.split("/");
  if (parts.some((part) => part === "" || part === "." || part === "..")) {
    throw new Error(`${label}.path must not contain empty, dot or parent path segments`);
  }

  let decoded = relative;
  for (let attempt = 0; attempt < 4; attempt += 1) {
    let next;
    try {
      next = decodeURIComponent(decoded);
    } catch (error) {
      throw new Error(`${label}.path contains an invalid escaped path: ${error.message}`);
    }
    if (next === decoded) break;
    decoded = next;
    if (decoded.includes("\\") || /[\u0000-\u001f\u007f]/.test(decoded)) {
      throw new Error(`${label}.path must not contain encoded path separators or control characters`);
    }
    const decodedParts = decoded.split("/");
    if (decodedParts.length !== parts.length
      || decodedParts.some((part) => part === "" || part === "." || part === "..")) {
      throw new Error(`${label}.path must not contain encoded path traversal segments`);
    }
    parts.splice(0, parts.length, ...decodedParts);
  }
  return relative;
}

function resolveContained(root, relativePath, label) {
  const value = validateRelativePath(relativePath, label);
  const resolvedRoot = path.resolve(root);
  const resolved = path.resolve(resolvedRoot, value);
  const relative = path.relative(resolvedRoot, resolved);
  if (relative === ".." || relative.startsWith(`..${path.sep}`)) {
    throw new Error(`${label}.path escapes its release root: ${value}`);
  }
  return rejectSymlinkTraversal(resolvedRoot, resolved, `${label}.path`);
}

// The publish job flattens uploaded artifacts before attaching them to the
// release, while tauri-release manifests retain paths from the build tree.
// Prefer the declared path, then accept one unambiguous basename match so a
// flattened release can still be audited without weakening traversal checks.
function resolveArtifact(root, relativePath, label) {
  const declared = resolveContained(root, relativePath, label);
  try {
    fs.lstatSync(declared);
    return declared;
  } catch (error) {
    if (error.code !== "ENOENT") {
      throw new Error(`${label}.path is unavailable: ${declared}: ${error.message}`);
    }
  }
  const basename = path.basename(declared);
  const matches = walkFiles(root).filter((filePath) => path.basename(filePath) === basename);
  if (matches.length > 1) {
    throw new Error(`${label}.path has duplicate basename under its release root: ${basename}`);
  }
  if (matches.length === 1) return matches[0];
  return declared;
}

function requireFile(filePath, label) {
  regularFileStat(filePath, label);
  return filePath;
}

function requireDigest(entry, filePath, label) {
  if (!isRecord(entry)) throw new Error(`${label} must be an object`);
  const expected = requireString(entry.sha256, `${label}.sha256`);
  if (!SHA256.test(expected)) throw new Error(`${label}.sha256 must be lowercase SHA-256`);
  const stat = regularFileStat(filePath, label);
  const actual = sha256(filePath);
  if (actual !== expected) throw new Error(`${label}.sha256 does not match ${path.basename(filePath)}`);
  if (!Number.isInteger(entry.size) || entry.size <= 0) throw new Error(`${label}.size must be a positive integer`);
  if (stat.size !== entry.size) throw new Error(`${label}.size does not match ${path.basename(filePath)}`);
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
  if (override !== undefined) return resolveContained(root, override, `${platform} manifest`);
  return findNamedFile(root, [`tauri-release-${platform}.json`], `${platform} package manifest`);
}

function normalizeSignature(value, label) {
  const signature = requireString(value, label).replaceAll("\r\n", "\n");
  if (signature.includes("\r") || /[\u0000-\u0008\u000b\u000c\u000e-\u001f\u007f]/.test(signature)) {
    throw new Error(`${label} contains unsupported control characters`);
  }
  if (signature.length < 8) throw new Error(`${label} is too short to be a signature`);
  return signature;
}

function feedMetadataPath(root, override) {
  if (override !== undefined) return resolveContained(root, override, "updater metadata");
  return findNamedFile(root, ["latest.json", "updater.json"], "updater metadata");
}

function versionInArtifactName(name, version, label) {
  const escaped = version.replaceAll(".", "\\.");
  const pattern = new RegExp(`(?:^|[-_])v?${escaped}(?=$|[-_.])`);
  if (!pattern.test(name)) throw new Error(`${label} must contain release version ${version}`);
}

const packageExtensions = Object.freeze({
  dmg: ".dmg",
  appimage: ".AppImage",
  deb: ".deb",
  rpm: ".rpm",
  nsis: "-setup.exe",
});

const nativePackageArchitectures = Object.freeze({
  "macos-arm64": Object.freeze({ dmg: ["aarch64", "arm64"] }),
  "linux-x64": Object.freeze({
    appimage: ["amd64", "x86_64"],
    deb: ["amd64", "x86_64"],
    rpm: ["x86_64", "amd64"],
  }),
  "windows-x64": Object.freeze({ nsis: ["x64", "amd64"] }),
  "windows-arm64": Object.freeze({ nsis: ["arm64", "aarch64"] }),
});

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function nativePackageArtifactPattern(version, platform, kind) {
  const architectures = nativePackageArchitectures[platform]?.[kind];
  if (!architectures) return null;
  const architecture = architectures.map(escapeRegExp).join("|");
  const stem = `^JFTrade[_-]v?${escapeRegExp(version)}`;
  if (kind === "rpm") {
    return new RegExp(`${stem}-[0-9]+[._-](?:${architecture})\\.rpm$`);
  }
  if (kind === "nsis") {
    return new RegExp(`${stem}[_-](?:${architecture})-setup\\.exe$`);
  }
  return new RegExp(`${stem}[_-](?:${architecture})${escapeRegExp(packageExtensions[kind])}$`);
}

function validatePackageArtifactName(name, version, platform, kind, label) {
  versionInArtifactName(name, version, label);
  const extension = packageExtensions[kind];
  if (!extension || !name.endsWith(extension)) {
    throw new Error(`${label} must use ${extension ?? "the expected"} package extension`);
  }
  const escapedVersion = version.replaceAll(".", "\\.");
  const canonicalPrefix = new RegExp(`^JFTrade-v?${escapedVersion}-${platform}(?=$|[-_.])`);
  const nativePattern = nativePackageArtifactPattern(version, platform, kind);
  if (!canonicalPrefix.test(name) && !(nativePattern && nativePattern.test(name))) {
    throw new Error(`${label} must contain platform ${platform}`);
  }
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
  const seenPackageBasenames = new Set();
  for (const [index, entry] of manifest.packages.entries()) {
    const entryLabel = `${label}.packages[${index}]`;
    if (!isRecord(entry)) throw new Error(`${entryLabel} must be an object`);
    const kind = requireString(entry.kind, `${entryLabel}.kind`);
    if (!expectedKinds.has(kind)) throw new Error(`${entryLabel}.kind is not expected for ${platform}: ${kind}`);
    if (seenKinds.has(kind)) throw new Error(`${label} contains duplicate package kind: ${kind}`);
    seenKinds.add(kind);
    const filePath = resolveArtifact(root, entry.path, entryLabel);
    requireFile(filePath, entryLabel);
    const basename = path.basename(filePath);
    if (seenPackageBasenames.has(basename)) {
      throw new Error(`${label} contains duplicate package basename: ${basename}`);
    }
    seenPackageBasenames.add(basename);
    validatePackageArtifactName(basename, version, platform, kind, `${entryLabel}.path`);
    requireDigest(entry, filePath, entryLabel);
  }
  for (const kind of expectedKinds) if (!seenKinds.has(kind)) throw new Error(`${label} is missing package kind: ${kind}`);
  const archives = new Map();
  const signatureArchives = new Set();
  const hasUpdaterArchives = Object.hasOwn(manifest, "updaterArchives");
  if (hasUpdaterArchives && !Array.isArray(manifest.updaterArchives)) {
    throw new Error(`${label}.updaterArchives must be an array when present`);
  }
  if (hasUpdaterArchives && manifest.updaterArchives.length === 0) {
    throw new Error(`${label}.updaterArchives must be a non-empty array when present`);
  }
  if (hasUpdaterArchives) {
    for (const [index, entry] of manifest.updaterArchives.entries()) {
      const entryLabel = `${label}.updaterArchives[${index}]`;
      if (!isRecord(entry)) throw new Error(`${entryLabel} must be an object`);
      const filePath = resolveArtifact(root, entry.path, entryLabel);
      requireFile(filePath, entryLabel);
      if (!(filePath.endsWith(".tar.gz") || filePath.endsWith(".zip"))) throw new Error(`${entryLabel}.path must be a .tar.gz or .zip archive`);
      const archiveName = path.basename(filePath);
      versionInArtifactName(archiveName, version, `${entryLabel}.path`);
      requireDigest(entry, filePath, entryLabel);
      if (archives.has(archiveName)) throw new Error(`${label} contains duplicate updater archive: ${archiveName}`);
      const sidecarPath = `${filePath}.sig`;
      requireFile(sidecarPath, `${entryLabel} sidecar signature`);
      archives.set(archiveName, { filePath, sidecarPath });
    }
  }
  if (!Array.isArray(manifest.updaterSignatures) || manifest.updaterSignatures.length === 0) {
    throw new Error(`${label}.updaterSignatures must contain sidecar signatures`);
  }
  const seenSignatures = new Set();
  for (const [index, entry] of manifest.updaterSignatures.entries()) {
    const entryLabel = `${label}.updaterSignatures[${index}]`;
    if (!isRecord(entry)) throw new Error(`${entryLabel} must be an object`);
    const filePath = resolveArtifact(root, entry.path, entryLabel);
    requireFile(filePath, entryLabel);
    if (!filePath.endsWith(".sig")) throw new Error(`${entryLabel}.path must end with .sig`);
    requireDigest(entry, filePath, entryLabel);
    const archiveName = path.basename(filePath).slice(0, -4);
    if (seenSignatures.has(archiveName)) {
      throw new Error(`${label} contains duplicate updater signature: ${path.basename(filePath)}`);
    }
    seenSignatures.add(archiveName);
    if (!archives.has(archiveName)) {
      if (hasUpdaterArchives) {
        throw new Error(`${entryLabel} has no matching updater archive`);
      }
      const archivePath = resolveArtifact(root, entry.path.slice(0, -4), `${entryLabel} archive`);
      requireFile(archivePath, `${entryLabel} archive`);
      if (!(archivePath.endsWith(".tar.gz") || archivePath.endsWith(".zip"))) throw new Error(`${entryLabel} archive must be a .tar.gz or .zip file`);
      versionInArtifactName(path.basename(archivePath), version, `${entryLabel} archive`);
      const sidecarPath = `${archivePath}.sig`;
      if (path.resolve(sidecarPath) !== path.resolve(filePath)) {
        throw new Error(`${entryLabel} does not match its updater archive sidecar`);
      }
      archives.set(archiveName, { filePath: archivePath, sidecarPath });
    } else if (path.resolve(archives.get(archiveName).sidecarPath) !== path.resolve(filePath)) {
      throw new Error(`${entryLabel} does not match its updater archive sidecar`);
    }
    signatureArchives.add(archiveName);
  }
  if (archives.size === 0) throw new Error(`${label} has no updater archives`);
  for (const [archiveName, archive] of archives) {
    if (!signatureArchives.has(archiveName)) throw new Error(`${label} is missing sidecar metadata for ${archiveName}`);
    requireFile(archive.sidecarPath, `${archiveName} sidecar signature`);
  }
  return {
    platform,
    architecture: manifest.target.architecture,
    manifestPath: path.relative(root, filePath).split(path.sep).join("/"),
    packageCount: manifest.packages.length,
    archiveNames: [...archives.keys()].sort(),
    archiveFiles: [...archives.values()].map(({ filePath }) => filePath),
  };
}

function feedTargetForPlatform(platform, feedPlatforms) {
  const target = updaterTargetAliases[platform].find((candidate) => Object.hasOwn(feedPlatforms, candidate));
  if (!target) {
    throw new Error(`updater metadata is missing target for ${platform} (${updaterTargetAliases[platform].join(", ")})`);
  }
  return target;
}

function canonicalFeedTarget(target, label) {
  if (typeof target !== "string" || !SAFE_TARGET.test(target)
    || ["__proto__", "constructor", "prototype"].includes(target)) {
    throw new Error(`${label} must be a safe updater target name`);
  }
  const platform = ROLLBACK_PLATFORMS.find((candidate) => updaterTargetAliases[candidate].includes(target));
  if (!platform) throw new Error(`${label} is not an expected rollback target: ${target}`);
  return platform;
}

function archiveNameFromUrl(value, label) {
  const raw = requireString(value, label);
  let url;
  try {
    url = new URL(raw);
  } catch (error) {
    throw new Error(`${label} must be a valid URL: ${error.message}`);
  }
  if (url.protocol !== "https:" || !url.hostname || url.username || url.password) {
    throw new Error(`${label} must use an HTTPS URL without credentials`);
  }
  const rawPath = raw.slice(raw.indexOf("//") + 2).split(/[?#]/, 1)[0].replace(/^[^/]*/, "");
  if (rawPath.includes("\\")) throw new Error(`${label} must not contain path separators or control characters`);
  const rawSegmentCount = rawPath.split("/").length;
  let decodedPath;
  try {
    decodedPath = decodeURIComponent(rawPath);
  } catch (error) {
    throw new Error(`${label} contains an invalid escaped artifact path: ${error.message}`);
  }
  if (decodedPath.includes("\\") || /[\u0000-\u001f\u007f]/.test(decodedPath)) {
    throw new Error(`${label} must not contain path separators or control characters`);
  }
  let inspectedPath = decodedPath;
  for (let attempt = 0; attempt < 4; attempt += 1) {
    if (inspectedPath.split("/").length !== rawSegmentCount) {
      throw new Error(`${label} must not contain encoded path separators`);
    }
    const segments = inspectedPath.split("/");
    if (segments.some((segment) => segment === "." || segment === "..")) {
      throw new Error(`${label} must not contain path traversal segments`);
    }
    let next;
    try {
      next = decodeURIComponent(inspectedPath);
    } catch (error) {
      throw new Error(`${label} contains an invalid escaped artifact path: ${error.message}`);
    }
    if (next === inspectedPath) break;
    inspectedPath = next;
    if (inspectedPath.includes("\\") || /[\u0000-\u001f\u007f]/.test(inspectedPath)) {
      throw new Error(`${label} must not contain path separators or control characters`);
    }
  }
  let name;
  try {
    name = decodeURIComponent(path.posix.basename(url.pathname));
  } catch (error) {
    throw new Error(`${label} contains an invalid escaped artifact name: ${error.message}`);
  }
  if (!name || name === "." || name === ".." || name.includes("/") || name.includes("\\")
    || /[\u0000-\u001f\u007f]/.test(name)) {
    throw new Error(`${label} must identify an updater archive filename`);
  }
  if (!(name.endsWith(".tar.gz") || name.endsWith(".zip"))) {
    throw new Error(`${label} must identify an updater archive`);
  }
  return { name, url };
}

function validateUpdaterMetadata({ root, feedPath, expectedVersion, manifests }) {
  const feed = readJson(feedPath, "Tauri updater metadata");
  if (!isRecord(feed)) throw new Error("Tauri updater metadata must be an object");
  const version = requireSemver(feed.version, "updater metadata.version");
  if (version !== expectedVersion) throw new Error(`updater metadata.version ${version} does not match ${expectedVersion}`);
  if (!isRecord(feed.platforms) || Object.keys(feed.platforms).length === 0) throw new Error("updater metadata.platforms must be a non-empty object");
  const targetIdentities = new Map();
  for (const target of Object.keys(feed.platforms)) {
    const canonical = canonicalFeedTarget(target, `updater metadata.platforms.${target}`);
    if (targetIdentities.has(canonical)) {
      throw new Error(`updater metadata.platforms contains duplicate target identity: ${target}`);
    }
    targetIdentities.set(canonical, target);
  }
  const archives = new Map();
  for (const [platform, manifest] of Object.entries(manifests)) {
    for (const filePath of manifest.archiveFiles) {
      const name = path.basename(filePath);
      if (archives.has(name)) throw new Error(`updater archive filename is shared by multiple platforms: ${name}`);
      archives.set(name, { filePath, platform });
    }
  }
  const targets = {};
  const seenFeedArchives = new Set();
  for (const platform of ROLLBACK_PLATFORMS) {
    const target = feedTargetForPlatform(platform, feed.platforms);
    const entry = feed.platforms[target];
    if (!isRecord(entry)) throw new Error(`updater metadata.platforms.${target} must be an object`);
    const { name: archiveName, url } = archiveNameFromUrl(
      entry.url,
      `updater metadata.platforms.${target}.url`,
    );
    const archive = archives.get(archiveName);
    if (!archive) throw new Error(`updater metadata.platforms.${target} archive is not represented by a package manifest: ${archiveName}`);
    if (seenFeedArchives.has(archiveName)) {
      throw new Error(`updater metadata.platforms contains duplicate updater archive reference: ${archiveName}`);
    }
    seenFeedArchives.add(archiveName);
    if (archive.platform !== platform) throw new Error(`updater metadata.platforms.${target} archive belongs to ${archive.platform}, not ${platform}`);
    const feedSignature = normalizeSignature(entry.signature, `updater metadata.platforms.${target}.signature`);
    const sidecarPath = `${archive.filePath}.sig`;
    requireFile(sidecarPath, `${archiveName}.sig`);
    const sidecar = normalizeSignature(fs.readFileSync(sidecarPath, "utf8"), `${archiveName}.sig`);
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
  if (override !== undefined) return path.resolve(requireString(override, "instructionsPath"));
  for (const root of [currentRoot, previousRoot]) {
    for (const name of ["rollback.md", "ROLLBACK.md", "rollback-instructions.md"]) {
      const candidate = path.join(root, name);
      try {
        fs.lstatSync(candidate);
        return candidate;
      } catch (error) {
        if (error.code !== "ENOENT") throw error;
      }
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
  const currentPath = path.resolve(requireString(currentRoot, "currentRoot"));
  const previousPath = path.resolve(requireString(previousRoot, "previousRoot"));
  if (currentPath === previousPath) {
    throw new Error("current and previous release roots must differ");
  }
  const current = inspectRelease({
    root: currentPath,
    expectedVersion: transition.currentVersion,
    manifestPaths: currentManifestPaths,
    updaterMetadata: currentUpdaterMetadata,
  });
  const previous = inspectRelease({
    root: previousPath,
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
  walkFiles(source);
  const destination = path.resolve(destinationRoot);
  if (source === destination) throw new Error("rollback source and retained destination must differ");
  validateRetentionDestination(source, destination);
  const destinationParent = path.dirname(destination);
  assertDirectoryOrCreate(destinationParent, "retained rollback destination parent");
  try {
    fs.lstatSync(destination);
    throw new Error(`retained rollback artifact already exists; refusing overwrite: ${destination}`);
  } catch (error) {
    if (error.code !== "ENOENT") throw error;
  }
  const stagingPrefix = path.join(destinationParent, `.${path.basename(destination)}.staging-`);
  const staging = fs.mkdtempSync(stagingPrefix);
  try {
    fs.cpSync(source, staging, {
      recursive: true,
      dereference: false,
      errorOnExist: true,
      force: false,
    });
    walkFiles(staging);
    try {
      fs.lstatSync(destination);
      throw new Error(`retained rollback artifact already exists; refusing overwrite: ${destination}`);
    } catch (error) {
      if (error.code !== "ENOENT") throw error;
    }
    fs.renameSync(staging, destination);
  } catch (error) {
    fs.rmSync(staging, { recursive: true, force: true });
    throw new Error(`atomic rollback artifact retention failed: ${error.message}`);
  }
  directoryStat(source, "rollback source");
  return {
    source,
    destination,
    sourcePreserved: true,
    atomicRename: true,
  };
}

export function atomicallyRetainRollbackArtifact({ sourceRoot, retainedRoot, version } = {}) {
  const releaseVersion = requireSemver(version, "version");
  const source = assertDirectory(sourceRoot, "rollback source");
  walkFiles(source);
  const retainedRootPath = path.resolve(requireString(retainedRoot, "retainedRoot"));
  const destination = path.join(retainedRootPath, releaseVersion);
  validateRetentionDestination(source, destination);
  return copyAndRenameAtomically(source, destination);
}

function assertDirectoryOrCreate(directory, label) {
  if (!directory) throw new Error(`${label} directory is required`);
  const resolved = path.resolve(directory);
  const missing = [];
  let current = resolved;
  while (true) {
    let stat;
    try {
      stat = fs.lstatSync(current);
    } catch (error) {
      if (error.code !== "ENOENT") {
        throw new Error(`${label} is unavailable: ${current}: ${error.message}`);
      }
      const parent = path.dirname(current);
      if (parent === current) throw new Error(`${label} is unavailable: ${current}`);
      missing.unshift(path.basename(current));
      current = parent;
      continue;
    }
    if (stat.isSymbolicLink()) throw new Error(`${label} must not traverse a symbolic link: ${current}`);
    if (!stat.isDirectory()) throw new Error(`${label} must be a directory: ${current}`);
    break;
  }
  for (const segment of missing) {
    current = path.join(current, segment);
    try {
      fs.mkdirSync(current);
    } catch (error) {
      if (error.code !== "EEXIST") {
        throw new Error(`${label} cannot create ${current}: ${error.message}`);
      }
    }
    const stat = fs.lstatSync(current);
    if (stat.isSymbolicLink()) throw new Error(`${label} must not traverse a symbolic link: ${current}`);
    if (!stat.isDirectory()) throw new Error(`${label} must be a directory: ${current}`);
  }
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
