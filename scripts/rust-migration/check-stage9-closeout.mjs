#!/usr/bin/env node

import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

export const STAGE9_CLOSEOUT_SCHEMA_VERSION = "jftrade.stage9.closeout-evidence.v1";
export const REQUIRED_PLATFORMS = Object.freeze([
  "macos-arm64",
  "linux-x64",
  "windows-x64",
  "windows-arm64",
]);

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));
const manifestRelativePath = "tests/fixtures/rust-migration/stage9/closeout-evidence.json";
const requiredGates = Object.freeze([
  "allRouteGroups",
  "uniqueWriteOwner",
  "platformRelease",
  "signedUpdaterArtifact",
  "securityReview",
  "sbom",
  "recoveryDrill",
  "observationWindow",
]);
const platformChecks = Object.freeze([
  "package",
  "signed",
  "install",
  "upgrade",
  "uninstall",
  "rollback",
  "runtimeSmoke",
]);
const statuses = new Set(["passed", "open", "blocked"]);

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

function addError(errors, message) {
  errors.push(message);
}

function requireRecord(value, label, errors) {
  if (!isRecord(value)) {
    addError(errors, `${label} must be an object`);
    return null;
  }
  return value;
}

function requireString(value, label, errors) {
  if (typeof value !== "string" || value.trim() === "") {
    addError(errors, `${label} must be a non-empty string`);
  }
}

function requireStatus(value, label, errors) {
  if (typeof value !== "string" || !statuses.has(value)) {
    addError(errors, `${label} must be one of passed, open or blocked`);
  }
}

function requireStringArray(value, label, errors) {
  if (!Array.isArray(value) || value.length === 0) {
    addError(errors, `${label} must be a non-empty string array`);
    return;
  }
  value.forEach((item, index) => requireString(item, `${label}[${index}]`, errors));
}

function requireInteger(value, label, errors) {
  if (!Number.isInteger(value) || value < 0) {
    addError(errors, `${label} must be a non-negative integer`);
  }
}

function checkKeys(value, label, required, allowed, errors) {
  const object = requireRecord(value, label, errors);
  if (!object) return null;
  for (const key of required) {
    if (!(key in object)) addError(errors, `${label}.${key} is required`);
  }
  for (const key of Object.keys(object)) {
    if (!allowed.includes(key)) addError(errors, `${label}.${key} is not allowed`);
  }
  return object;
}

function validateEvidence(value, label, errors) {
  const evidence = checkKeys(
    value,
    label,
    ["id", "status", "scope", "command", "evidence"],
    ["id", "status", "scope", "command", "evidence"],
    errors,
  );
  if (!evidence) return;
  requireString(evidence.id, `${label}.id`, errors);
  requireStatus(evidence.status, `${label}.status`, errors);
  requireString(evidence.scope, `${label}.scope`, errors);
  requireString(evidence.command, `${label}.command`, errors);
  requireString(evidence.evidence, `${label}.evidence`, errors);
}

function validateGate(value, label, errors) {
  const gate = checkKeys(
    value,
    label,
    ["status", "evidence"],
    ["status", "evidence", "owner", "next"],
    errors,
  );
  if (!gate) return;
  requireStatus(gate.status, `${label}.status`, errors);
  requireStringArray(gate.evidence, `${label}.evidence`, errors);
  if ("owner" in gate) requireString(gate.owner, `${label}.owner`, errors);
  if ("next" in gate) requireString(gate.next, `${label}.next`, errors);
}

function validatePlatform(value, label, errors) {
  const platform = checkKeys(
    value,
    label,
    [...platformChecks, "evidence"],
    [...platformChecks, "evidence"],
    errors,
  );
  if (!platform) return;
  for (const check of platformChecks) {
    requireStatus(platform[check], `${label}.${check}`, errors);
  }
  requireStringArray(platform.evidence, `${label}.evidence`, errors);
}

function validatePlatformGate(value, label, errors) {
  const gate = checkKeys(
    value,
    label,
    ["status", "evidence", "platforms"],
    ["status", "evidence", "platforms"],
    errors,
  );
  if (!gate) return;
  requireStatus(gate.status, `${label}.status`, errors);
  requireStringArray(gate.evidence, `${label}.evidence`, errors);
  const platforms = requireRecord(gate.platforms, `${label}.platforms`, errors);
  if (!platforms) return;
  for (const platform of REQUIRED_PLATFORMS) {
    if (!(platform in platforms)) {
      addError(errors, `${label}.platforms.${platform} is required`);
    }
  }
  for (const platform of Object.keys(platforms)) {
    if (!REQUIRED_PLATFORMS.includes(platform)) {
      addError(errors, `${label}.platforms.${platform} is not allowed`);
      continue;
    }
    validatePlatform(platforms[platform], `${label}.platforms.${platform}`, errors);
  }
}

function validateOwnerDeletion(value, label, errors) {
  const owner = checkKeys(
    value,
    label,
    ["status", "evidence", "conditions"],
    ["status", "evidence", "conditions"],
    errors,
  );
  if (!owner) return;
  requireStatus(owner.status, `${label}.status`, errors);
  requireStringArray(owner.evidence, `${label}.evidence`, errors);
  requireStringArray(owner.conditions, `${label}.conditions`, errors);
}

export function validateManifest(manifest) {
  const errors = [];
  const root = checkKeys(
    manifest,
    "manifest",
    [
      "$schema",
      "schemaVersion",
      "stage",
      "status",
      "routeOwnership",
      "localEvidence",
      "gates",
      "ownerDeletion",
    ],
    [
      "$schema",
      "schemaVersion",
      "stage",
      "status",
      "routeOwnership",
      "localEvidence",
      "gates",
      "ownerDeletion",
    ],
    errors,
  );
  if (!root) return errors;
  requireString(root.$schema, "manifest.$schema", errors);
  if (root.schemaVersion !== STAGE9_CLOSEOUT_SCHEMA_VERSION) {
    addError(errors, `manifest.schemaVersion must be ${STAGE9_CLOSEOUT_SCHEMA_VERSION}`);
  }
  if (root.stage !== 9) addError(errors, "manifest.stage must be 9");
  if (root.status !== "in_progress" && root.status !== "closed") {
    addError(errors, "manifest.status must be in_progress or closed");
  }

  const routeOwnership = checkKeys(
    root.routeOwnership,
    "manifest.routeOwnership",
    ["baselineOperations", "shadowRoutes", "cutoverTestOnlyRoutes", "remainingRoutes"],
    ["baselineOperations", "shadowRoutes", "cutoverTestOnlyRoutes", "remainingRoutes"],
    errors,
  );
  if (routeOwnership) {
    for (const key of [
      "baselineOperations",
      "shadowRoutes",
      "cutoverTestOnlyRoutes",
      "remainingRoutes",
    ]) {
      requireInteger(routeOwnership[key], `manifest.routeOwnership.${key}`, errors);
    }
  }

  if (!Array.isArray(root.localEvidence) || root.localEvidence.length === 0) {
    addError(errors, "manifest.localEvidence must be a non-empty array");
  } else {
    root.localEvidence.forEach((item, index) => {
      validateEvidence(item, `manifest.localEvidence[${index}]`, errors);
    });
  }

  const gates = checkKeys(
    root.gates,
    "manifest.gates",
    requiredGates,
    requiredGates,
    errors,
  );
  if (gates) {
    for (const gate of requiredGates) {
      if (gate === "platformRelease") {
        validatePlatformGate(gates[gate], `manifest.gates.${gate}`, errors);
      } else {
        validateGate(gates[gate], `manifest.gates.${gate}`, errors);
      }
    }
  }

  const ownerDeletion = checkKeys(
    root.ownerDeletion,
    "manifest.ownerDeletion",
    ["go", "wails"],
    ["go", "wails"],
    errors,
  );
  if (ownerDeletion) {
    validateOwnerDeletion(ownerDeletion.go, "manifest.ownerDeletion.go", errors);
    validateOwnerDeletion(ownerDeletion.wails, "manifest.ownerDeletion.wails", errors);
  }
  return errors;
}

export function routeOwnershipSnapshot(root = repositoryRoot) {
  const baseline = readJson(
    path.join(root, "tests/fixtures/rust-migration/stage7/api-control-plane-corpus.json"),
    "OpenAPI route baseline",
  );
  const ownership = readJson(
    path.join(root, "tests/fixtures/rust-migration/stage9/route-ownership.json"),
    "route ownership fixture",
  );
  if (!Array.isArray(baseline.routes)) {
    throw new Error("OpenAPI route baseline must contain routes");
  }
  if (!Array.isArray(ownership.shadowRoutes) || !Array.isArray(ownership.cutoverTestRoutes)) {
    throw new Error("route ownership fixture must contain shadowRoutes and cutoverTestRoutes");
  }
  const routeKey = (route) => `${route.method} ${route.path}`;
  const baselineKeys = new Set(baseline.routes.map(routeKey));
  const claimed = new Set();
  for (const [bucket, routes] of [
    ["shadowRoutes", ownership.shadowRoutes],
    ["cutoverTestRoutes", ownership.cutoverTestRoutes],
  ]) {
    for (const route of routes) {
      const key = routeKey(route);
      if (!baselineKeys.has(key)) {
        throw new Error(`${bucket} contains non-baseline route ${key}`);
      }
      if (claimed.has(key)) throw new Error(`route is claimed more than once: ${key}`);
      claimed.add(key);
    }
  }
  const remaining = baseline.routes.length - claimed.size;
  return {
    baselineOperations: baseline.routes.length,
    shadowRoutes: ownership.shadowRoutes.length,
    cutoverTestOnlyRoutes: ownership.cutoverTestRoutes.length,
    remainingRoutes: remaining,
  };
}

function sameRouteOwnership(left, right) {
  return [
    "baselineOperations",
    "shadowRoutes",
    "cutoverTestOnlyRoutes",
    "remainingRoutes",
  ].every((key) => left[key] === right[key]);
}

export function evaluateCloseout(manifest, options = {}) {
  const errors = validateManifest(manifest);
  if (errors.length > 0) {
    return {
      valid: false,
      complete: false,
      errors,
      blockers: [],
      expectedRouteOwnership: null,
    };
  }

  const expectedRouteOwnership = options.expectedRouteOwnership ?? routeOwnershipSnapshot(
    options.repositoryRoot ?? repositoryRoot,
  );
  const blockers = [];
  if (!sameRouteOwnership(manifest.routeOwnership, expectedRouteOwnership)) {
    return {
      valid: false,
      complete: false,
      errors: [
        `route ownership ledger does not match fixtures (expected ${expectedRouteOwnership.shadowRoutes} shadow / `
          + `${expectedRouteOwnership.cutoverTestOnlyRoutes} cutover-test-only / ${expectedRouteOwnership.remainingRoutes} remaining)`,
      ],
      blockers: [],
      expectedRouteOwnership,
    };
  }
  if (manifest.routeOwnership.remainingRoutes !== 0) {
    blockers.push(`route ownership still has ${manifest.routeOwnership.remainingRoutes} remaining operation(s)`);
  }

  for (const gate of requiredGates) {
    if (manifest.gates[gate].status !== "passed") {
      blockers.push(`gate ${gate} is ${manifest.gates[gate].status}`);
    }
  }
  const platformGate = manifest.gates.platformRelease;
  for (const platform of REQUIRED_PLATFORMS) {
    const checks = platformGate.platforms[platform];
    for (const check of platformChecks) {
      if (checks[check] !== "passed") {
        blockers.push(`platform ${platform} ${check} is ${checks[check]}`);
      }
    }
  }
  for (const owner of ["go", "wails"]) {
    if (manifest.ownerDeletion[owner].status !== "passed") {
      blockers.push(`owner deletion ${owner} is ${manifest.ownerDeletion[owner].status}`);
    }
  }
  if (manifest.status !== "closed") blockers.push("manifest status is not closed");
  return {
    valid: true,
    complete: blockers.length === 0,
    errors: [],
    blockers,
    expectedRouteOwnership,
  };
}

function parseArguments(args) {
  let check = false;
  let manifestPath = path.join(repositoryRoot, manifestRelativePath);
  for (let index = 0; index < args.length; index += 1) {
    const argument = args[index];
    if (argument === "--check") {
      check = true;
      continue;
    }
    if (argument === "--manifest") {
      manifestPath = path.resolve(args[++index] ?? "");
      continue;
    }
    if (argument.startsWith("--manifest=")) {
      manifestPath = path.resolve(argument.slice("--manifest=".length));
      continue;
    }
    throw new Error(`unknown argument: ${argument}`);
  }
  return { check, manifestPath };
}

export function main(args = process.argv.slice(2)) {
  let parsed;
  try {
    parsed = parseArguments(args);
    const manifest = readJson(parsed.manifestPath, "Stage 9 closeout evidence manifest");
    const result = evaluateCloseout(manifest);
    if (!result.valid) {
      console.error("Stage 9 closeout evidence manifest is invalid:");
      for (const error of result.errors) console.error(`- ${error}`);
      return 1;
    }
    const state = result.complete ? "ready for formal close" : "in progress; formal close blocked";
    console.log(`Stage 9 closeout evidence: ${state}.`);
    console.log(
      `Route ownership: ${manifest.routeOwnership.shadowRoutes} shadow / `
        + `${manifest.routeOwnership.cutoverTestOnlyRoutes} cutover-test-only / `
        + `${manifest.routeOwnership.remainingRoutes} remaining.`,
    );
    if (!result.complete) {
      for (const blocker of result.blockers) console.log(`- ${blocker}`);
    }
    return parsed.check && !result.complete ? 1 : 0;
  } catch (error) {
    console.error(`Stage 9 closeout evidence checker failed: ${error.message}`);
    return 1;
  }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  process.exitCode = main();
}
