#!/usr/bin/env node

import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

import { evaluateCloseout } from "./rust-migration/check-stage9-closeout.mjs";

const repositoryRoot = fileURLToPath(new URL("..", import.meta.url));
const defaultManifestPath = path.join(
  repositoryRoot,
  "tests/fixtures/rust-migration/stage9/closeout-evidence.json",
);

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

function isPublish(environment) {
  return String(environment.JFTRADE_DESKTOP_PUBLISH ?? "").trim() === "true";
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
  closeoutManifest,
  expectedRouteOwnership,
  checkCloseout = true,
} = {}) {
  if (!isPublish(environment)) {
    return { publish: false, valid: true, blockers: [] };
  }

  const blockers = missingSigningValues(environment).map(
    (name) => `publish requires configured signing secret or updater value ${name}`,
  );
  const endpointError = validateUpdaterEndpoint(environment);
  if (endpointError) blockers.push(endpointError);

  if (checkCloseout) {
    let manifest = closeoutManifest;
    if (manifest === undefined) {
      try {
        manifest = JSON.parse(fs.readFileSync(defaultManifestPath, "utf8"));
      } catch (error) {
        blockers.push(`cannot read Stage 9 closeout evidence: ${error.message}`);
      }
    }
    if (manifest !== undefined) {
      const closeout = evaluateCloseout(manifest, { expectedRouteOwnership });
      if (!closeout.valid) {
        blockers.push(...closeout.errors.map((error) => `invalid closeout evidence: ${error}`));
      } else if (!closeout.complete) {
        blockers.push(...closeout.blockers.map((blocker) => `Stage 9 closeout gate: ${blocker}`));
      }
    }
  }
  return { publish: true, valid: blockers.length === 0, blockers };
}

function main(args = process.argv.slice(2)) {
  const forcePublish = args.includes("--publish");
  const signingOnly = args.includes("--signing-only");
  const environment = forcePublish
    ? { ...process.env, JFTRADE_DESKTOP_PUBLISH: "true" }
    : process.env;
  const result = evaluateDesktopReleasePolicy({ environment, checkCloseout: !signingOnly });
  if (result.valid) {
    console.log(result.publish ? "Desktop release publish policy passed." : "Desktop release dry-run policy passed.");
    return 0;
  }
  console.error("Desktop release publish policy failed closed:");
  for (const blocker of result.blockers) console.error(`- ${blocker}`);
  return 1;
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  process.exitCode = main();
}
