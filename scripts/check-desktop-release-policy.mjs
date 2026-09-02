#!/usr/bin/env node

import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

export const RELEASE_SIGNING_REQUIREMENTS = Object.freeze({
  macos: Object.freeze([
    "JFTRADE_MACOS_CERTIFICATE_BASE64",
    "JFTRADE_MACOS_CERTIFICATE_PASSWORD",
    "JFTRADE_MACOS_SIGN_IDENTITY",
    "JFTRADE_MACOS_NOTARY_APPLE_ID",
    "JFTRADE_MACOS_NOTARY_PASSWORD",
    "JFTRADE_MACOS_NOTARY_TEAM_ID",
  ]),
  windows: Object.freeze([
    "JFTRADE_WINDOWS_CERTIFICATE_BASE64",
    "JFTRADE_WINDOWS_CERTIFICATE_PASSWORD",
  ]),
  updater: Object.freeze([
    "TAURI_SIGNING_PRIVATE_KEY",
    "TAURI_SIGNING_PRIVATE_KEY_PASSWORD",
    "JFTRADE_TAURI_UPDATER_PUBKEY",
    "JFTRADE_TAURI_UPDATER_ENDPOINT",
  ]),
});

export const DESKTOP_RELEASE_OPERATIONS = Object.freeze(["rehearsal", "candidate", "publish"]);

function releaseOperation(environment) {
  const explicit = String(environment.JFTRADE_DESKTOP_OPERATION ?? "").trim();
  if (explicit) return explicit;
  return String(environment.JFTRADE_DESKTOP_PUBLISH ?? "").trim() === "true"
    ? "publish"
    : "rehearsal";
}

function missingSigningValues(environment) {
  return Object.values(RELEASE_SIGNING_REQUIREMENTS)
    .flat()
    .filter((name) => String(environment[name] ?? "").trim() === "");
}

function validateUpdaterEndpoint(environment) {
  const endpoint = String(environment.JFTRADE_TAURI_UPDATER_ENDPOINT ?? "").trim();
  if (!endpoint) return null;
  let url;
  try {
    url = new URL(endpoint);
  } catch {
    return "JFTRADE_TAURI_UPDATER_ENDPOINT must be a valid HTTPS URL";
  }
  if (url.protocol !== "https:" || url.username || url.password || !url.hostname) {
    return "JFTRADE_TAURI_UPDATER_ENDPOINT must be an HTTPS URL without credentials";
  }
  return null;
}

export function evaluateDesktopReleasePolicy({
  environment = process.env,
} = {}) {
  const operation = releaseOperation(environment);
  if (!DESKTOP_RELEASE_OPERATIONS.includes(operation)) {
    return {
      operation,
      publish: false,
      valid: false,
      blockers: [`unsupported desktop release operation: ${operation}`],
    };
  }

  const blockers = [];
  if (operation === "candidate") {
    blockers.push(...missingSigningValues(environment).map(
      (name) => `candidate requires configured signing secret or updater value ${name}`,
    ));
    const endpointError = validateUpdaterEndpoint(environment);
    if (endpointError) blockers.push(endpointError);
  }
  return { operation, publish: operation === "publish", valid: blockers.length === 0, blockers };
}

function main(args = process.argv.slice(2)) {
  const operationIndex = args.indexOf("--operation");
  const legacyPublish = args.includes("--publish");
  const operation = operationIndex >= 0 ? args[operationIndex + 1] : legacyPublish ? "publish" : undefined;
  if (operationIndex >= 0 && (!operation || operation.startsWith("--"))) {
    console.error("--operation requires rehearsal, candidate, or publish");
    return 1;
  }
  const environment = operation
    ? { ...process.env, JFTRADE_DESKTOP_OPERATION: operation }
    : process.env;
  const result = evaluateDesktopReleasePolicy({ environment });
  if (result.valid) {
    console.log(`Desktop release ${result.operation} policy passed.`);
    return 0;
  }
  console.error(`Desktop release ${result.operation || "unknown"} policy failed closed:`);
  for (const blocker of result.blockers) console.error(`- ${blocker}`);
  return 1;
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  process.exitCode = main();
}
