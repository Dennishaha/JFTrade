#!/usr/bin/env node

import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));

export const SECURITY_REVIEW_INPUTS_SCHEMA = "jftrade.security-review-inputs.v1";

// These are the production crate roots and API entrypoint.  Shadow binaries
// and test-only crates are deliberately out of scope for this release input
// check; cargo's workspace lint still applies to them when they are built.
export const RUST_ENTRYPOINTS = Object.freeze([
  "apps/desktop/src-tauri/src/lib.rs",
  "crates/jftrade-engine/src/lib.rs",
  "crates/jftrade-engine/src/main.rs",
  "crates/jftrade-engine/src/bin/jftrade-api-rust.rs",
]);

export const REQUIRED_SECURITY_INPUTS = Object.freeze({
  denyPolicy: "deny.toml",
  capability: "apps/desktop/src-tauri/capabilities/default.json",
  tauriConfig: "apps/desktop/src-tauri/tauri.conf.json",
  desktopProfile: "apps/desktop/src-tauri/src/profile.rs",
  engine: "crates/jftrade-engine/src/lib.rs",
  productServer: "crates/jftrade-engine/src/product_server.rs",
  updater: "apps/desktop/src-tauri/src/native_notification_updater.rs",
  lifecycle: "apps/desktop/src-tauri/src/native_lifecycle.rs",
  settingsSecurity: "crates/jftrade-settings/src/security.rs",
  adkSecrets: "crates/jftrade-engine/src/product_production_ports_adk_mutation_provider.rs",
});

const EXPECTED_CAPABILITY_PERMISSIONS = Object.freeze([
  "core:default",
  "opener:default",
  "notification:default",
  "updater:default",
]);

const EXPECTED_CSP_CONNECT_SOURCES = new Set([
  "ipc:",
  "http://ipc.localhost",
  "http://127.0.0.1:3008",
  "http://127.0.0.1:6699",
  "ws://127.0.0.1:3008",
  "ws://127.0.0.1:6699",
]);

function isRecord(value) {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function safeValue(value) {
  const text = String(value);
  try {
    const url = new URL(text.replace(/\/\*$/, ""));
    if (url.username || url.password) {
      url.username = "<redacted>";
      url.password = "<redacted>";
      return url.toString();
    }
  } catch {
    // Not a URL; there is no credential-bearing URL to redact.
  }
  return text;
}

function readText(root, relativePath, errors) {
  const absolutePath = path.join(root, relativePath);
  try {
    return fs.readFileSync(absolutePath, "utf8");
  } catch (error) {
    errors.push(`${relativePath} is missing or unreadable: ${error.message}`);
    return null;
  }
}

function readJson(root, relativePath, errors) {
  const text = readText(root, relativePath, errors);
  if (text === null) return null;
  try {
    return JSON.parse(text);
  } catch (error) {
    errors.push(`${relativePath} is not valid JSON: ${error.message}`);
    return null;
  }
}

function checkText(text, relativePath, requirements, errors) {
  if (text === null) return;
  for (const [label, requirement] of requirements) {
    const matches = typeof requirement === "string"
      ? text.includes(requirement)
      : requirement.test(text);
    if (!matches) errors.push(`${relativePath} is missing required ${label}`);
  }
}

function checkRustEntrypoints(root, errors, checks) {
  const missing = [];
  const invalid = [];
  for (const relativePath of RUST_ENTRYPOINTS) {
    const absolutePath = path.join(root, relativePath);
    let text;
    try {
      text = fs.readFileSync(absolutePath, "utf8");
    } catch {
      missing.push(relativePath);
      continue;
    }
    if (!/^#!\[forbid\(unsafe_code\)\]/m.test(text)) invalid.push(relativePath);
  }
  if (missing.length > 0) {
    errors.push(`Rust production entrypoint is missing: ${missing.join(", ")}`);
  }
  if (invalid.length > 0) {
    errors.push(`Rust production entrypoint lacks #![forbid(unsafe_code)]: ${invalid.join(", ")}`);
  }
  checks.push({
    id: "rust-unsafe-code",
    status: missing.length === 0 && invalid.length === 0 ? "passed" : "failed",
    inputs: [...RUST_ENTRYPOINTS],
  });
}

function checkDenyPolicy(text, errors, checks) {
  const requirements = [
    ["advisory yanked deny policy", /^\s*yanked\s*=\s*["']deny["']/m],
    ["license allow list", /^\s*allow\s*=\s*\[/m],
    ["wildcard dependency deny policy", /^\s*wildcards\s*=\s*["']deny["']/m],
    ["unknown registry deny policy", /^\s*unknown-registry\s*=\s*["']deny["']/m],
    ["unknown git source deny policy", /^\s*unknown-git\s*=\s*["']deny["']/m],
    ["crates.io registry allowlist", "https://github.com/rust-lang/crates.io-index"],
  ];
  const before = errors.length;
  checkText(text, "deny.toml", requirements, errors);
  checks.push({
    id: "cargo-deny-policy",
    status: errors.length === before ? "passed" : "failed",
    inputs: ["deny.toml"],
  });
}

function loopbackUrl(value, expectedPort = null) {
  if (typeof value !== "string") return false;
  const hasPathWildcard = value.endsWith("/*");
  const normalized = hasPathWildcard ? value.slice(0, -2) : value;
  if (normalized.includes("*")) return false;
  let url;
  try {
    url = new URL(normalized);
  } catch {
    return false;
  }
  if (url.protocol !== "http:" || !["127.0.0.1", "localhost"].includes(url.hostname)) return false;
  if (url.username || url.password || (expectedPort !== null && url.port !== expectedPort)) return false;
  return url.pathname === "" || url.pathname === "/";
}

function checkCapabilities(capability, errors, checks) {
  const before = errors.length;
  if (!isRecord(capability)) {
    errors.push("Tauri capability must be an object");
  } else {
    if (capability.local !== true) errors.push("Tauri capability must mark local content as local");
    if (!Array.isArray(capability.windows) || capability.windows.length !== 1 || capability.windows[0] !== "*") {
      errors.push("Tauri capability must explicitly cover the desktop window set");
    }
    if (!Array.isArray(capability.permissions)) {
      errors.push("Tauri capability permissions must be an array");
    } else {
      const actual = capability.permissions.map(String);
      if (actual.length !== EXPECTED_CAPABILITY_PERMISSIONS.length
        || actual.some((permission, index) => permission !== EXPECTED_CAPABILITY_PERMISSIONS[index])) {
        errors.push(`Tauri capability permissions are broader than the approved set: ${JSON.stringify(actual)}`);
      }
    }
    const urls = capability.remote?.urls;
    if (!Array.isArray(urls) || urls.length === 0) {
      errors.push("Tauri capability must declare loopback remote URLs");
    } else {
      for (const url of urls) {
        if (!loopbackUrl(url, "3003")) {
          errors.push(`Tauri capability remote URL is not loopback-only: ${safeValue(url)}`);
        }
      }
    }
  }
  checks.push({
    id: "tauri-capabilities-acl",
    status: errors.length === before ? "passed" : "failed",
    inputs: [REQUIRED_SECURITY_INPUTS.capability],
  });
}

function checkCsp(config, errors, checks) {
  const before = errors.length;
  if (!isRecord(config)) {
    errors.push("Tauri configuration must be an object");
  } else {
    if (!loopbackUrl(config.build?.devUrl, "3003")) {
      errors.push("Tauri devUrl must remain a loopback HTTP origin");
    }
    const csp = config.app?.security?.csp;
    const connectSrc = typeof csp?.["connect-src"] === "string"
      ? csp["connect-src"].split(/\s+/).filter(Boolean)
      : [];
    if (connectSrc.length === 0) errors.push("Tauri CSP must define connect-src");
    for (const source of connectSrc) {
      if (!EXPECTED_CSP_CONNECT_SOURCES.has(source)) {
        errors.push(`Tauri CSP connect-src is not an approved loopback source: ${source}`);
      }
    }
    if (connectSrc.some((source) => source === "*" || source.includes("0.0.0.0"))) {
      errors.push("Tauri CSP connect-src may not use a public wildcard listener");
    }
  }
  checks.push({
    id: "loopback-csp",
    status: errors.length === before ? "passed" : "failed",
    inputs: [REQUIRED_SECURITY_INPUTS.tauriConfig],
  });
}

function checkLoopbackListeners(profile, engine, productServer, errors, checks) {
  const before = errors.length;
  checkText(profile, REQUIRED_SECURITY_INPUTS.desktopProfile, [
    ["development loopback bind", /DEVELOPMENT_API_BIND\s*:\s*&str\s*=\s*"127\.0\.0\.1:\d+"/],
    ["release loopback bind", /RELEASE_API_BIND\s*:\s*&str\s*=\s*"127\.0\.0\.1:\d+"/],
  ], errors);
  checkText(engine, REQUIRED_SECURITY_INPUTS.engine, [
    ["loopback bind validation", /bind_address\.ip\(\)\.is_loopback\(\)/],
    ["loopback default bind", /DEFAULT_BIND_ADDRESS\s*:\s*&str\s*=\s*"127\.0\.0\.1:/],
  ], errors);
  checkText(productServer, REQUIRED_SECURITY_INPUTS.productServer, [
    ["explicit public-access guard", /record\.public_access_enabled\(\)/],
    ["private listener bind", /"127\.0\.0\.1"/],
    ["explicit public listener branch", /"0\.0\.0\.0"/],
  ], errors);
  checks.push({
    id: "loopback-listeners",
    status: errors.length === before ? "passed" : "failed",
    inputs: [
      REQUIRED_SECURITY_INPUTS.desktopProfile,
      REQUIRED_SECURITY_INPUTS.engine,
      REQUIRED_SECURITY_INPUTS.productServer,
    ],
  });
}

function checkCredentialUpdaterBoundary(updater, lifecycle, settingsSecurity, adkSecrets, errors, checks) {
  const before = errors.length;
  checkText(updater, REQUIRED_SECURITY_INPUTS.updater, [
    ["endpoint environment input", "UPDATER_ENDPOINT_ENV"],
    ["public-key environment input", "UPDATER_PUBLIC_KEY_ENV"],
    ["HTTPS endpoint validation", /scheme\(\)\s*!=\s*["']https["']/],
    ["URL credential rejection", /username\(\)\.is_empty\(\)/],
    ["URL password rejection", /password\(\)\.is_some\(\)/],
    ["endpoint/key pairing", /endpoint and signing public key must be configured together/],
  ], errors);
  checkText(lifecycle, REQUIRED_SECURITY_INPUTS.lifecycle, [
    ["conditional updater installation", /NativeUpdaterConfig::Ready/],
    ["updater endpoint is not hard-coded", /from_environment/],
  ], errors);
  checkText(settingsSecurity, REQUIRED_SECURITY_INPUTS.settingsSecurity, [
    ["password hash storage", "password_hash"],
    ["Argon2 password hashing", "hash_argon2id"],
  ], errors);
  checkText(adkSecrets, REQUIRED_SECURITY_INPUTS.adkSecrets, [
    ["credential sidecar writer", "write_adk_secrets"],
    ["credential sidecar reader", "read_adk_secrets"],
    ["API key removal from durable payload", /object\.remove\("apiKey"\)/],
    ["restrictive credential file permissions", /from_mode\(0o600\)/],
  ], errors);
  checks.push({
    id: "credential-updater-boundary",
    status: errors.length === before ? "passed" : "failed",
    inputs: [
      REQUIRED_SECURITY_INPUTS.updater,
      REQUIRED_SECURITY_INPUTS.lifecycle,
      REQUIRED_SECURITY_INPUTS.settingsSecurity,
      REQUIRED_SECURITY_INPUTS.adkSecrets,
    ],
  });
}

/**
 * Inspect repository inputs for an independent security review.
 *
 * A successful result means only that the expected source/policy inputs are
 * present and satisfy these lightweight static checks.  It is intentionally
 * not a security sign-off and never claims that an independent review passed.
 */
export function inspectSecurityReviewInputs(root = repositoryRoot) {
  const errors = [];
  const checks = [];
  checkRustEntrypoints(root, errors, checks);

  const denyPolicy = readText(root, REQUIRED_SECURITY_INPUTS.denyPolicy, errors);
  if (denyPolicy !== null) checkDenyPolicy(denyPolicy, errors, checks);
  else checks.push({ id: "cargo-deny-policy", status: "failed", inputs: [REQUIRED_SECURITY_INPUTS.denyPolicy] });

  const capability = readJson(root, REQUIRED_SECURITY_INPUTS.capability, errors);
  checkCapabilities(capability, errors, checks);
  const tauriConfig = readJson(root, REQUIRED_SECURITY_INPUTS.tauriConfig, errors);
  checkCsp(tauriConfig, errors, checks);

  const profile = readText(root, REQUIRED_SECURITY_INPUTS.desktopProfile, errors);
  const engine = readText(root, REQUIRED_SECURITY_INPUTS.engine, errors);
  const productServer = readText(root, REQUIRED_SECURITY_INPUTS.productServer, errors);
  checkLoopbackListeners(profile, engine, productServer, errors, checks);

  const updater = readText(root, REQUIRED_SECURITY_INPUTS.updater, errors);
  const lifecycle = readText(root, REQUIRED_SECURITY_INPUTS.lifecycle, errors);
  const settingsSecurity = readText(root, REQUIRED_SECURITY_INPUTS.settingsSecurity, errors);
  const adkSecrets = readText(root, REQUIRED_SECURITY_INPUTS.adkSecrets, errors);
  checkCredentialUpdaterBoundary(updater, lifecycle, settingsSecurity, adkSecrets, errors, checks);

  const valid = errors.length === 0;
  return {
    schemaVersion: SECURITY_REVIEW_INPUTS_SCHEMA,
    status: valid ? "repository_inputs_verified" : "repository_inputs_incomplete",
    valid,
    checks,
    errors,
    independentReview: "required",
    independentReviewStatus: "independent_review_required",
    releaseQualification: "external_security_review_required",
  };
}

export const evaluateSecurityReviewInputs = inspectSecurityReviewInputs;
export const checkSecurityReviewInputs = inspectSecurityReviewInputs;

function parseRoot(args) {
  const index = args.indexOf("--root");
  if (index === -1) return repositoryRoot;
  const value = args[index + 1];
  if (!value || value.startsWith("--")) throw new Error("--root requires a directory");
  return path.resolve(value);
}

export function main(args = process.argv.slice(2)) {
  let result;
  try {
    result = inspectSecurityReviewInputs(parseRoot(args));
  } catch (error) {
    result = {
      schemaVersion: SECURITY_REVIEW_INPUTS_SCHEMA,
      status: "repository_inputs_incomplete",
      valid: false,
      checks: [],
      errors: [error.message],
      independentReview: "required",
      independentReviewStatus: "independent_review_required",
      releaseQualification: "external_security_review_required",
    };
  }
  console.log(JSON.stringify(result, null, 2));
  return result.valid ? 0 : 1;
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  process.exitCode = main();
}
