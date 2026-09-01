#!/usr/bin/env node

import { createHash } from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));
const defaultConfigPath = path.join(repositoryRoot, "apps/desktop/src-tauri/tauri.conf.json");

export const SIGNED_UPDATER_SCHEMA = "jftrade.tauri-signed-updater.v1";
export const UPDATER_ARCHIVE_EXTENSIONS = Object.freeze([".tar.gz", ".zip"]);

const UPDATER_TARGET_ALIASES = Object.freeze({
  "darwin-aarch64": "darwin-aarch64",
  "darwin-arm64": "darwin-aarch64",
  "darwin-x86_64": "darwin-x86_64",
  "darwin-amd64": "darwin-x86_64",
  "darwin-x64": "darwin-x86_64",
  "linux-aarch64": "linux-aarch64",
  "linux-arm64": "linux-aarch64",
  "linux-x86_64": "linux-x86_64",
  "linux-amd64": "linux-x86_64",
  "linux-x64": "linux-x86_64",
  "windows-aarch64": "windows-aarch64",
  "windows-arm64": "windows-aarch64",
  "windows-x86_64": "windows-x86_64",
  "windows-amd64": "windows-x86_64",
  "windows-x64": "windows-x86_64",
});

const SAFE_PLATFORM_NAME = /^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/;
const SHA256 = /^[a-f0-9]{64}$/;
const MINISIGN_ALGORITHMS = new Set(["Ed", "ED"]);

function isRecord(value) {
  return value !== null && typeof value === "object" && !Array.isArray(value);
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
      if (content[index] === "\\") {
        index += 2;
      } else if (content[index] === '"') {
        index += 1;
        return JSON.parse(content.slice(start, index));
      } else {
        index += 1;
      }
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

function walkFiles(directory) {
  directoryStat(directory, "updater artifact directory");
  const files = [];
  const pending = [directory];
  while (pending.length > 0) {
    const current = pending.pop();
    for (const entry of fs.readdirSync(current, { withFileTypes: true })) {
      const filePath = path.join(current, entry.name);
      const stat = fs.lstatSync(filePath);
      if (stat.isSymbolicLink()) {
        throw new Error(`updater artifact tree must not contain a symbolic link: ${filePath}`);
      }
      if (stat.isDirectory()) pending.push(filePath);
      else if (stat.isFile()) files.push(filePath);
      else throw new Error(`updater artifact tree contains a non-regular entry: ${filePath}`);
    }
  }
  return files.sort();
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

function validateHttpsUrl(value, label) {
  const raw = requireString(value, label);
  let url;
  try {
    url = new URL(raw);
  } catch (error) {
    throw new Error(`${label} must be a valid HTTPS URL: ${error.message}`);
  }
  if (url.protocol !== "https:" || !url.hostname || url.username || url.password) {
    throw new Error(`${label} must be an HTTPS URL without credentials`);
  }
  return url;
}

function decodeBase64(value, label, expectedLength) {
  if (!/^[A-Za-z0-9+/]+={0,2}$/.test(value) || value.length % 4 !== 0) {
    throw new Error(`${label} must be canonical base64`);
  }
  const decoded = Buffer.from(value, "base64");
  if (decoded.toString("base64") !== value || decoded.length !== expectedLength) {
    throw new Error(`${label} must decode to ${expectedLength} bytes`);
  }
  return decoded;
}

function normalizedText(value, label) {
  if (typeof value !== "string" || value.trim() === "") {
    throw new Error(`${label} must be a non-empty text value`);
  }
  const normalized = value.replaceAll("\r\n", "\n");
  if (normalized.includes("\r") || /[\u0000-\u0008\u000b\u000c\u000e-\u001f]/.test(normalized)) {
    throw new Error(`${label} contains unsupported control characters`);
  }
  if (normalized.startsWith("\n") || normalized.endsWith("\n\n")) {
    throw new Error(`${label} has invalid surrounding newlines`);
  }
  return normalized.endsWith("\n") ? normalized.slice(0, -1) : normalized;
}

function parseMinisignPublicKey(value, label) {
  const normalized = normalizedText(value, label);
  const lines = normalized.split("\n");
  let encoded;
  if (lines.length === 1) {
    encoded = lines[0];
  } else {
    if (lines.length !== 2 || !/^untrusted comment: minisign public key(?: .*)?$/.test(lines[0])) {
      throw new Error(`${label} must be a Minisign public key or its base64 body`);
    }
    encoded = lines[1];
  }
  const decoded = decodeBase64(encoded, `${label} body`, 42);
  const algorithm = String.fromCharCode(decoded[0], decoded[1]);
  if (!MINISIGN_ALGORITHMS.has(algorithm)) {
    throw new Error(`${label} uses an unsupported Minisign algorithm`);
  }
  return {
    normalized,
    algorithm,
    keyId: Buffer.from(decoded.subarray(2, 10)).toString("hex"),
  };
}

/**
 * Validate the endpoint/public-key pair without logging either value.
 * The public key is parsed only for its Minisign encoding and key identity;
 * actual signature verification belongs to tauri-plugin-updater and the
 * signing workflow. This check prevents an accidental partial or private-key
 * config without claiming cryptographic validity.
 */
export function validateUpdaterConfiguration({ endpoint, publicKey } = {}) {
  const endpointValue = typeof endpoint === "string" ? endpoint.trim() : "";
  const publicKeyValue = typeof publicKey === "string" ? publicKey.trim() : "";
  if (!endpointValue && !publicKeyValue) {
    throw new Error("signed updater requires both HTTPS endpoint and public key");
  }
  if (!endpointValue || !publicKeyValue) {
    throw new Error("signed updater endpoint and public key must be configured together");
  }
  const endpointUrl = validateHttpsUrl(endpointValue, "updater endpoint");
  if (/private\s+key|secret\s+key/i.test(publicKeyValue)) {
    throw new Error("updater public key appears to contain a private/secret key");
  }
  const parsedPublicKey = parseMinisignPublicKey(publicKeyValue, "updater public key");
  return {
    endpoint: endpointUrl.href,
    publicKeyConfigured: true,
    publicKeySha256: createHash("sha256").update(publicKeyValue).digest("hex"),
    publicKeyAlgorithm: parsedPublicKey.algorithm,
    publicKeyKeyId: parsedPublicKey.keyId,
  };
}

export function inspectTauriUpdaterConfig(configPath = defaultConfigPath) {
  const config = readJson(configPath, "Tauri config");
  if (!isRecord(config.plugins) || !isRecord(config.plugins.updater)) {
    throw new Error("Tauri config must declare plugins.updater");
  }
  if (!("pubkey" in config.plugins.updater)) {
    throw new Error("Tauri config updater must declare pubkey (runtime release value is injected)");
  }
  const configuredPublicKey = config.plugins.updater.pubkey;
  if (configuredPublicKey !== undefined && configuredPublicKey !== null
    && typeof configuredPublicKey !== "string") {
    throw new Error("Tauri config updater pubkey must be a string");
  }
  if (typeof configuredPublicKey === "string" && configuredPublicKey.trim() !== "") {
    parseMinisignPublicKey(configuredPublicKey, "Tauri config updater pubkey");
  }
  if (!isRecord(config.bundle) || config.bundle.createUpdaterArtifacts !== true) {
    throw new Error("Tauri config bundle.createUpdaterArtifacts must be true");
  }
  return {
    configPath,
    createUpdaterArtifacts: true,
    runtimePublicKeyInjected: String(config.plugins.updater.pubkey ?? "").trim() === "",
  };
}

function normalizeSignature(value, label) {
  const signature = normalizedText(value, label);
  const lines = signature.split("\n");
  if (lines.length !== 4 || !/^untrusted comment: signature from minisign(?: .*)?$/.test(lines[0])) {
    throw new Error(`${label} must contain a four-line Minisign signature`);
  }
  if (!/^trusted comment: .+$/.test(lines[2])) {
    throw new Error(`${label} must contain a trusted comment`);
  }
  const body = decodeBase64(lines[1], `${label} signature body`, 74);
  const global = decodeBase64(lines[3], `${label} trusted signature`, 64);
  const algorithm = String.fromCharCode(body[0], body[1]);
  if (!MINISIGN_ALGORITHMS.has(algorithm)) {
    throw new Error(`${label} uses an unsupported Minisign algorithm`);
  }
  return {
    text: signature,
    algorithm,
    keyId: Buffer.from(body.subarray(2, 10)).toString("hex"),
    globalSignature: global,
  };
}

function archiveNameFromUrl(value, label) {
  const raw = requireString(value, label);
  const url = validateHttpsUrl(raw, label);
  const rawPath = raw.slice(raw.indexOf("//") + 2).split(/[?#]/, 1)[0].replace(/^[^/]*/, "");
  let decodedPath;
  try {
    decodedPath = decodeURIComponent(rawPath);
  } catch (error) {
    throw new Error(`${label} contains an invalid escaped artifact path: ${error.message}`);
  }
  const pathSegments = decodedPath.split("/");
  if (pathSegments.some((segment) => segment === "." || segment === "..")) {
    throw new Error(`${label} must not contain path traversal segments`);
  }
  if (decodedPath.includes("\\") || /[\u0000-\u001f\u007f]/.test(decodedPath)) {
    throw new Error(`${label} must not contain path separators or control characters`);
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
  if (!UPDATER_ARCHIVE_EXTENSIONS.some((extension) => name.endsWith(extension))) {
    throw new Error(`${label} must reference a .tar.gz or .zip updater archive`);
  }
  return { name, url };
}

function collectArtifacts(artifactRoot) {
  directoryStat(artifactRoot, "updater artifact directory");
  const archives = new Map();
  const signatures = new Map();
  for (const filePath of walkFiles(artifactRoot)) {
    const name = path.basename(filePath);
    if (UPDATER_ARCHIVE_EXTENSIONS.some((extension) => name.endsWith(extension))) {
      if (archives.has(name)) throw new Error(`duplicate updater archive filename: ${name}`);
      regularFileStat(filePath, `updater archive ${name}`);
      archives.set(name, filePath);
    } else if (name.endsWith(".sig")) {
      if (signatures.has(name)) throw new Error(`duplicate updater signature filename: ${name}`);
      regularFileStat(filePath, `updater signature ${name}`);
      signatures.set(name, filePath);
    }
  }
  if (archives.size === 0) throw new Error(`no .tar.gz or .zip updater archive found under ${artifactRoot}`);
  if (signatures.size === 0) throw new Error(`no .sig updater signature found under ${artifactRoot}`);
  return { archives, signatures };
}

function requireFeed(feed) {
  if (!isRecord(feed)) throw new Error("Tauri updater feed must be a JSON object");
  const version = requireSemver(feed.version, "feed.version");
  if (!isRecord(feed.platforms) || Object.keys(feed.platforms).length === 0) {
    throw new Error("feed.platforms must be a non-empty object");
  }
  if ("notes" in feed && typeof feed.notes !== "string") throw new Error("feed.notes must be a string");
  if ("pub_date" in feed) {
    const date = requireString(feed.pub_date, "feed.pub_date");
    if (!Number.isFinite(Date.parse(date))) throw new Error("feed.pub_date must be a valid date string");
  }
  return { version, platforms: feed.platforms };
}

function canonicalTarget(target, label) {
  if (typeof target !== "string" || !SAFE_PLATFORM_NAME.test(target)
    || ["__proto__", "constructor", "prototype"].includes(target)) {
    throw new Error(`${label} must be a safe updater target name`);
  }
  return UPDATER_TARGET_ALIASES[target] ?? target;
}

function parseExpectedTargets(expectedTargets) {
  if (expectedTargets === undefined) return null;
  if (!Array.isArray(expectedTargets) || expectedTargets.length === 0) {
    throw new Error("expectedTargets must be a non-empty array");
  }
  const targets = [];
  const seen = new Set();
  for (const [index, target] of expectedTargets.entries()) {
    const canonical = canonicalTarget(target, `expectedTargets[${index}]`);
    if (seen.has(canonical)) throw new Error(`expectedTargets contains duplicate target: ${target}`);
    seen.add(canonical);
    targets.push({ target, canonical });
  }
  return targets;
}

function requireDigest(value, label) {
  if (typeof value !== "string" || !SHA256.test(value)) {
    throw new Error(`${label} must be a lowercase SHA-256 digest`);
  }
  return value;
}

function requirePositiveSize(value, label) {
  if (!Number.isSafeInteger(value) || value <= 0) {
    throw new Error(`${label} must be a positive safe integer`);
  }
  return value;
}

function archiveVersionMatches(name, version, label) {
  const pattern = new RegExp(`(?:^|[-_])v?${version.replaceAll(".", "\\.")}(?=$|[-_.])`);
  if (!pattern.test(name)) throw new Error(`${label} must contain feed version ${version}`);
}

function optionalArtifactMetadata(rawEntry, archivePath, label) {
  const hasDigest = Object.hasOwn(rawEntry, "sha256");
  const hasSize = Object.hasOwn(rawEntry, "size");
  if (hasDigest !== hasSize) {
    throw new Error(`${label} must provide both sha256 and size when artifact metadata is present`);
  }
  const stat = regularFileStat(archivePath, `updater archive ${path.basename(archivePath)}`);
  const actualSha256 = sha256(archivePath);
  if (hasDigest) {
    const expectedSha256 = requireDigest(rawEntry.sha256, `${label}.sha256`);
    if (expectedSha256 !== actualSha256) {
      throw new Error(`${label}.sha256 does not match ${path.basename(archivePath)}`);
    }
    const expectedSize = requirePositiveSize(rawEntry.size, `${label}.size`);
    if (expectedSize !== stat.size) {
      throw new Error(`${label}.size does not match ${path.basename(archivePath)}`);
    }
  }
  return { stat, sha256: actualSha256 };
}

/**
 * Verify a real Tauri updater feed against the archives and .sig files emitted
 * by `createUpdaterArtifacts`. The feed signature text must exactly match its
 * sidecar file after newline normalization. Minisign structure is checked
 * locally, while cryptographic validity remains an external workflow concern.
 */
export function inspectSignedTauriUpdaterArtifacts({
  artifactRoot,
  feedPath,
  feed,
  endpoint,
  publicKey,
  expectedVersion,
  expectedTargets,
  configPath = defaultConfigPath,
} = {}) {
  if (!artifactRoot) throw new Error("artifactRoot is required");
  const config = inspectTauriUpdaterConfig(configPath);
  const configuration = validateUpdaterConfiguration({ endpoint, publicKey });
  const parsedFeed = requireFeed(feed ?? readJson(feedPath, "Tauri updater feed"));
  if (expectedVersion !== undefined && parsedFeed.version !== requireSemver(expectedVersion, "expectedVersion")) {
    throw new Error(`feed.version ${parsedFeed.version} does not match expected version ${expectedVersion}`);
  }
  const { archives, signatures } = collectArtifacts(artifactRoot);
  const expected = parseExpectedTargets(expectedTargets);
  const feedTargets = Object.keys(parsedFeed.platforms).map((target) => ({
    target,
    canonical: canonicalTarget(target, `feed.platforms.${target}`),
  }));
  const seenFeedTargets = new Set();
  for (const { target, canonical } of feedTargets) {
    if (seenFeedTargets.has(canonical)) {
      throw new Error(`feed.platforms contains duplicate target identity: ${target}`);
    }
    seenFeedTargets.add(canonical);
  }
  const expectedTargetNames = expected?.map((entry) => entry.target) ?? feedTargets.map((entry) => entry.target);
  const expectedCanonicalTargets = expected?.map((entry) => entry.canonical) ?? feedTargets.map((entry) => entry.canonical);
  const missingTargets = expectedTargetNames.filter((target, index) => !seenFeedTargets.has(expectedCanonicalTargets[index]));
  if (missingTargets.length > 0) throw new Error(`feed is missing target(s): ${missingTargets.join(", ")}`);
  if (expected) {
    const unexpectedTargets = feedTargets
      .filter(({ canonical }) => !expectedCanonicalTargets.includes(canonical))
      .map(({ target }) => target);
    if (unexpectedTargets.length > 0) {
      throw new Error(`feed contains unexpected target(s): ${unexpectedTargets.join(", ")}`);
    }
  }

  const seenArchives = new Set();
  const entries = [];
  for (const [target, rawEntry] of Object.entries(parsedFeed.platforms)) {
    if (!isRecord(rawEntry)) throw new Error(`feed.platforms.${target} must be an object`);
    const entryKeys = Object.keys(rawEntry);
    const allowedEntryKeys = ["signature", "url", "sha256", "size"];
    const unknownEntryKeys = entryKeys.filter((key) => !allowedEntryKeys.includes(key));
    if (unknownEntryKeys.length > 0) {
      throw new Error(`feed.platforms.${target} contains unsupported field(s): ${unknownEntryKeys.join(", ")}`);
    }
    const { name, url } = archiveNameFromUrl(rawEntry.url, `feed.platforms.${target}.url`);
    const archivePath = archives.get(name);
    if (!archivePath) throw new Error(`feed.platforms.${target} archive is missing locally: ${name}`);
    archiveVersionMatches(name, parsedFeed.version, `feed.platforms.${target}.url`);
    const archiveMetadata = optionalArtifactMetadata(rawEntry, archivePath, `feed.platforms.${target}`);
    if (seenArchives.has(name)) {
      throw new Error(`feed.platforms contains duplicate updater archive reference: ${name}`);
    }
    const signaturePath = `${archivePath}.sig`;
    const signatureStat = regularFileStat(signaturePath, `feed.platforms.${target} signature sidecar`);
    const sidecar = normalizeSignature(fs.readFileSync(signaturePath, "utf8"), `${name}.sig`);
    const feedSignature = normalizeSignature(rawEntry.signature, `feed.platforms.${target}.signature`);
    if (feedSignature.text !== sidecar.text) {
      throw new Error(`feed.platforms.${target} signature does not match ${path.basename(signaturePath)}`);
    }
    if (feedSignature.algorithm !== configuration.publicKeyAlgorithm
      || feedSignature.keyId !== configuration.publicKeyKeyId) {
      throw new Error(`feed.platforms.${target} signature key does not match configured updater public key`);
    }
    seenArchives.add(name);
    entries.push({
      target,
      url: url.href,
      archive: name,
      archiveSha256: archiveMetadata.sha256,
      archiveBytes: archiveMetadata.stat.size,
      signatureSha256: createHash("sha256").update(sidecar.text).digest("hex"),
      signatureBytes: signatureStat.size,
    });
  }

  for (const [name, archivePath] of archives) {
    const signaturePath = `${archivePath}.sig`;
    regularFileStat(signaturePath, `updater archive ${name} sidecar signature`);
    if (!seenArchives.has(name)) throw new Error(`updater archive is not represented in feed: ${name}`);
  }
  for (const [name] of signatures) {
    if (!archives.has(name.slice(0, -4))) throw new Error(`signature has no matching updater archive: ${name}`);
  }
  return {
    schemaVersion: SIGNED_UPDATER_SCHEMA,
    version: parsedFeed.version,
    config,
    endpoint: configuration.endpoint,
    publicKeyConfigured: configuration.publicKeyConfigured,
    publicKeySha256: configuration.publicKeySha256,
    feed: {
      path: feedPath ?? null,
      targets: entries.map((entry) => entry.target).sort(),
      entryCount: entries.length,
    },
    artifacts: entries,
    limitations: [
      "Archive paths, non-empty files, versions, optional feed sha256/size metadata and Minisign text structure are validated locally.",
      "The feed signature text and artifact sidecars are compared byte-for-byte after newline normalization.",
      "Minisign cryptographic verification, HTTPS certificate/availability, four-platform coverage, native install/upgrade/rollback and external publication require the release signing workflow.",
    ],
  };
}

function parseArgs(args) {
  const values = {};
  const aliases = {
    artifacts: "artifactRoot",
    feed: "feedPath",
    config: "configPath",
    "public-key": "publicKey",
    "expected-version": "expectedVersion",
    "expected-target": "expectedTargets",
    "expected-targets": "expectedTargets",
  };
  for (let index = 0; index < args.length; index += 1) {
    const argument = args[index];
    if (!argument.startsWith("--")) throw new Error(`unknown argument: ${argument}`);
    const [key, inline] = argument.slice(2).split("=", 2);
    if (key === "config-only") {
      values.configOnly = true;
      continue;
    }
    const value = inline ?? args[++index];
    if (!value) throw new Error(`missing value for --${key}`);
    const normalizedKey = aliases[key] ?? key;
    if (normalizedKey === "expectedTargets") {
      (values.expectedTargets ??= []).push(value);
    } else {
      values[normalizedKey] = value;
    }
  }
  return values;
}

function main(args = process.argv.slice(2)) {
  const options = parseArgs(args);
  if (options.configOnly) {
    const result = inspectTauriUpdaterConfig(options.configPath ?? defaultConfigPath);
    console.log(`Verified Tauri updater configuration (createUpdaterArtifacts=${result.createUpdaterArtifacts}).`);
    return 0;
  }
  const report = inspectSignedTauriUpdaterArtifacts({
    ...options,
    endpoint: options.endpoint ?? process.env.JFTRADE_TAURI_UPDATER_ENDPOINT,
    publicKey: options.publicKey ?? process.env.JFTRADE_TAURI_UPDATER_PUBKEY,
  });
  console.log(`Validated local Tauri updater feed inputs v${report.version} with ${report.feed.entryCount} target(s); external signature and platform release evidence remain required.`);
  return 0;
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    process.exitCode = main();
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  }
}
