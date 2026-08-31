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

function isRecord(value) {
  return value !== null && typeof value === "object" && !Array.isArray(value);
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

/**
 * Validate the endpoint/public-key pair without logging either value.
 * The public key is intentionally not parsed cryptographically here: actual
 * Minisign verification belongs to tauri-plugin-updater and the signing
 * workflow. This check prevents an accidental partial or private-key config.
 */
export function validateUpdaterConfiguration({ endpoint, publicKey } = {}) {
  const endpointValue = String(endpoint ?? "").trim();
  const publicKeyValue = String(publicKey ?? "").trim();
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
  if (publicKeyValue.length < 8 || /[\u0000-\u0008\u000b\u000c\u000e-\u001f]/.test(publicKeyValue)) {
    throw new Error("updater public key must be a non-empty text value");
  }
  return {
    endpoint: endpointUrl.href,
    publicKeyConfigured: true,
    publicKeySha256: createHash("sha256").update(publicKeyValue).digest("hex"),
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
  const signature = requireString(value, label).replaceAll("\r\n", "\n");
  if (signature.length < 8) throw new Error(`${label} is too short to be a signature`);
  return signature;
}

function archiveNameFromUrl(value, label) {
  const url = validateHttpsUrl(value, label);
  let name;
  try {
    name = decodeURIComponent(path.posix.basename(url.pathname));
  } catch (error) {
    throw new Error(`${label} contains an invalid escaped artifact name: ${error.message}`);
  }
  if (!name || name === "." || name === ".." || name.includes("/")) {
    throw new Error(`${label} must identify an updater archive filename`);
  }
  if (!UPDATER_ARCHIVE_EXTENSIONS.some((extension) => name.endsWith(extension))) {
    throw new Error(`${label} must reference a .tar.gz or .zip updater archive`);
  }
  return { name, url };
}

function collectArtifacts(artifactRoot) {
  if (!fs.existsSync(artifactRoot)) throw new Error(`updater artifact directory is missing: ${artifactRoot}`);
  const archives = new Map();
  const signatures = new Map();
  for (const filePath of walkFiles(artifactRoot)) {
    const name = path.basename(filePath);
    if (UPDATER_ARCHIVE_EXTENSIONS.some((extension) => name.endsWith(extension))) {
      if (archives.has(name)) throw new Error(`duplicate updater archive filename: ${name}`);
      archives.set(name, filePath);
    } else if (name.endsWith(".sig")) {
      if (signatures.has(name)) throw new Error(`duplicate updater signature filename: ${name}`);
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
  if ("pub_date" in feed && typeof feed.pub_date !== "string") throw new Error("feed.pub_date must be a string");
  return { version, platforms: feed.platforms };
}

/**
 * Verify a real Tauri updater feed against the archives and .sig files emitted
 * by `createUpdaterArtifacts`. The feed signature text must exactly match its
 * sidecar file after newline normalization; no placeholder or generated
 * signature is accepted as evidence of cryptographic validity.
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
  const expected = expectedTargets ? [...expectedTargets] : Object.keys(parsedFeed.platforms);
  const missingTargets = expected.filter((target) => !(target in parsedFeed.platforms));
  if (missingTargets.length > 0) throw new Error(`feed is missing target(s): ${missingTargets.join(", ")}`);

  const seenArchives = new Set();
  const entries = [];
  for (const [target, rawEntry] of Object.entries(parsedFeed.platforms)) {
    if (!isRecord(rawEntry)) throw new Error(`feed.platforms.${target} must be an object`);
    const { name, url } = archiveNameFromUrl(rawEntry.url, `feed.platforms.${target}.url`);
    const archivePath = archives.get(name);
    if (!archivePath) throw new Error(`feed.platforms.${target} archive is missing locally: ${name}`);
    const signaturePath = `${archivePath}.sig`;
    if (!fs.existsSync(signaturePath)) {
      throw new Error(`feed.platforms.${target} signature sidecar is missing: ${path.basename(signaturePath)}`);
    }
    const sidecar = normalizeSignature(fs.readFileSync(signaturePath, "utf8"), `${name}.sig`);
    const feedSignature = normalizeSignature(rawEntry.signature, `feed.platforms.${target}.signature`);
    if (feedSignature !== sidecar) {
      throw new Error(`feed.platforms.${target} signature does not match ${path.basename(signaturePath)}`);
    }
    seenArchives.add(name);
    entries.push({
      target,
      url: url.href,
      archive: name,
      archiveSha256: sha256(archivePath),
      archiveBytes: fs.statSync(archivePath).size,
      signatureSha256: createHash("sha256").update(sidecar).digest("hex"),
    });
  }

  for (const [name, archivePath] of archives) {
    const signaturePath = `${archivePath}.sig`;
    if (!fs.existsSync(signaturePath)) throw new Error(`updater archive is missing sidecar signature: ${name}`);
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
      "The feed signature text and artifact sidecars are compared byte-for-byte after newline normalization.",
      "Minisign cryptographic verification, HTTPS certificate/availability, native install/upgrade/rollback and external publication require the release signing workflow.",
    ],
  };
}

function parseArgs(args) {
  const values = {};
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
    values[{ artifacts: "artifactRoot", feed: "feedPath", config: "configPath" }[key] ?? key] = value;
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
  console.log(`Verified signed Tauri updater feed v${report.version} with ${report.feed.entryCount} target(s).`);
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
