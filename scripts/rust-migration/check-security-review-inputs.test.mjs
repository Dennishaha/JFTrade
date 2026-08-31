import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import {
  inspectSecurityReviewInputs,
  RUST_ENTRYPOINTS,
  REQUIRED_SECURITY_INPUTS,
} from "./check-security-review-inputs.mjs";

function fixtureRoot() {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-security-inputs-"));
  const write = (relativePath, content) => {
    const target = path.join(root, relativePath);
    fs.mkdirSync(path.dirname(target), { recursive: true });
    fs.writeFileSync(target, content);
  };
  for (const entrypoint of RUST_ENTRYPOINTS) write(entrypoint, "#![forbid(unsafe_code)]\nfn main() {}\n");
  write(REQUIRED_SECURITY_INPUTS.denyPolicy, [
    "[advisories]",
    'yanked = "deny"',
    "[licenses]",
    'allow = ["MIT"]',
    "[bans]",
    'wildcards = "deny"',
    "[sources]",
    'unknown-registry = "deny"',
    'unknown-git = "deny"',
    'allow-registry = ["https://github.com/rust-lang/crates.io-index"]',
  ].join("\n"));
  write(REQUIRED_SECURITY_INPUTS.capability, JSON.stringify({
    local: true,
    windows: ["*"],
    remote: { urls: [
      "http://127.0.0.1:3003",
      "http://127.0.0.1:3003/*",
      "http://localhost:3003",
      "http://localhost:3003/*",
    ] },
    permissions: ["core:default", "opener:default", "notification:default", "updater:default"],
  }));
  write(REQUIRED_SECURITY_INPUTS.tauriConfig, JSON.stringify({
    build: { devUrl: "http://127.0.0.1:3003" },
    app: { security: { csp: {
      "connect-src": "ipc: http://ipc.localhost http://127.0.0.1:3008 http://127.0.0.1:6699 ws://127.0.0.1:3008 ws://127.0.0.1:6699",
    } } },
  }));
  write(REQUIRED_SECURITY_INPUTS.desktopProfile, [
    'const DEVELOPMENT_API_BIND: &str = "127.0.0.1:3008";',
    'const RELEASE_API_BIND: &str = "127.0.0.1:6699";',
  ].join("\n"));
  write(REQUIRED_SECURITY_INPUTS.engine, [
    "#![forbid(unsafe_code)]",
    'const DEFAULT_BIND_ADDRESS: &str = "127.0.0.1:0";',
    "if bind_address.ip().is_loopback() { Ok(()) } else { Err(()) }",
  ].join("\n"));
  write(REQUIRED_SECURITY_INPUTS.productServer, [
    'let host = if record.public_access_enabled() { "0.0.0.0" } else { "127.0.0.1" };',
  ].join("\n"));
  write(REQUIRED_SECURITY_INPUTS.updater, [
    "UPDATER_ENDPOINT_ENV",
    "UPDATER_PUBLIC_KEY_ENV",
    'endpoint.scheme() != "https"',
    "endpoint.username().is_empty()",
    "endpoint.password().is_some()",
    "endpoint and signing public key must be configured together",
  ].join("\n"));
  write(REQUIRED_SECURITY_INPUTS.lifecycle, "NativeUpdaterConfig::Ready from_environment\n");
  write(REQUIRED_SECURITY_INPUTS.settingsSecurity, "password_hash hash_argon2id\n");
  write(REQUIRED_SECURITY_INPUTS.adkSecrets, [
    "write_adk_secrets read_adk_secrets",
    'object.remove("apiKey")',
    "from_mode(0o600)",
  ].join("\n"));
  return root;
}

test("accepts complete repository security-review inputs without claiming sign-off", () => {
  const result = inspectSecurityReviewInputs(fixtureRoot());
  assert.equal(result.valid, true);
  assert.equal(result.status, "repository_inputs_verified");
  assert.equal(result.independentReviewStatus, "independent_review_required");
  assert.equal(result.releaseQualification, "external_security_review_required");
});

test("rejects Tauri capability permission expansion", () => {
  const root = fixtureRoot();
  const file = path.join(root, REQUIRED_SECURITY_INPUTS.capability);
  const capability = JSON.parse(fs.readFileSync(file, "utf8"));
  capability.permissions.push("shell:default");
  fs.writeFileSync(file, JSON.stringify(capability));
  const result = inspectSecurityReviewInputs(root);
  assert.equal(result.valid, false);
  assert.match(result.errors.join("\n"), /permissions are broader/);
});

test("rejects a desktop profile that exposes a public listener", () => {
  const root = fixtureRoot();
  const file = path.join(root, REQUIRED_SECURITY_INPUTS.desktopProfile);
  fs.writeFileSync(file, fs.readFileSync(file, "utf8").replaceAll("127.0.0.1", "0.0.0.0"));
  const result = inspectSecurityReviewInputs(root);
  assert.equal(result.valid, false);
  assert.match(result.errors.join("\n"), /loopback bind/);
});

test("rejects a Rust production entrypoint without unsafe-code forbid", () => {
  const root = fixtureRoot();
  const file = path.join(root, "crates/jftrade-engine/src/bin/jftrade-api-rust.rs");
  fs.writeFileSync(file, "fn main() {}\n");
  const result = inspectSecurityReviewInputs(root);
  assert.equal(result.valid, false);
  assert.match(result.errors.join("\n"), /lacks #!\[forbid\(unsafe_code\)\]/);
});
