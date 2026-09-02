#!/usr/bin/env node

import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

import { routeOwnershipSnapshot } from "./stage9-route-ownership.mjs";

export { routeOwnershipSnapshot } from "./stage9-route-ownership.mjs";

export const STAGE9_CLOSEOUT_SCHEMA_VERSION = "jftrade.stage9.closeout-evidence.v2";
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
  "hardCutReadiness",
  "rollbackArtifact",
  "backupRestoreDrill",
  "postReleaseSmoke",
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
const ownerEntryStatuses = new Set(["removed", "retained", "unknown"]);

// Candidate admission runs before a release exists.  It proves the static
// migration prerequisites and route ownership, while final release evidence
// (including post-release smoke and hard-cut readiness) is intentionally
// evaluated only by the full closeout check after publication.
export const POST_RELEASE_GATES = Object.freeze([
  "postReleaseSmoke",
  "hardCutReadiness",
]);
export const CANDIDATE_PREREQUISITE_GATES = Object.freeze(
  requiredGates.filter((gate) => !POST_RELEASE_GATES.includes(gate)),
);

// Hard-cut readiness is a release assertion and therefore depends on every
// release-safety prerequisite. Source/entrypoint deletion is a repository
// state assertion: it depends only on complete route ownership and the unique
// production writer, while release qualification remains fail-closed.
const hardCutPrerequisiteGates = Object.freeze(
  requiredGates.filter((gate) => gate !== "hardCutReadiness"),
);
const ownerDeletionPrerequisiteGates = Object.freeze([
  "allRouteGroups",
  "uniqueWriteOwner",
]);

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
    ["status", "evidence", "conditions", "entryStatus"],
    errors,
  );
  if (!owner) return;
  requireStatus(owner.status, `${label}.status`, errors);
  requireStringArray(owner.evidence, `${label}.evidence`, errors);
  requireStringArray(owner.conditions, `${label}.conditions`, errors);
  if ("entryStatus" in owner
    && (typeof owner.entryStatus !== "string" || !ownerEntryStatuses.has(owner.entryStatus))) {
    addError(errors, `${label}.entryStatus must be one of removed, retained or unknown`);
  }
}

function gateIsPassed(manifest, gateName) {
  const gate = manifest.gates[gateName];
  if (gate.status !== "passed") return false;
  if (gateName !== "platformRelease") return true;
  return REQUIRED_PLATFORMS.every((platform) =>
    platformChecks.every((check) => gate.platforms[platform][check] === "passed"),
  );
}

function addPrerequisiteBlockers(manifest, blockers) {
  if (manifest.gates.hardCutReadiness.status === "passed") {
    for (const gate of hardCutPrerequisiteGates) {
      if (!gateIsPassed(manifest, gate)) {
        blockers.push(
          `hardCutReadiness is passed while prerequisite gate ${gate} is not fully passed`,
        );
      }
    }
  }
  for (const owner of ["go", "wails"]) {
    const ownerEvidence = manifest.ownerDeletion[owner];
    if (ownerEvidence.status !== "passed") continue;
    if (ownerEvidence.entryStatus && ownerEvidence.entryStatus !== "removed") {
      blockers.push(
        `owner deletion ${owner} is passed while entrypoint status is ${ownerEvidence.entryStatus}`,
      );
    }
    for (const gate of ownerDeletionPrerequisiteGates) {
      if (!gateIsPassed(manifest, gate)) {
        blockers.push(
          `owner deletion ${owner} is passed while prerequisite gate ${gate} is not fully passed`,
        );
      }
    }
  }
}

function routeOwnershipBlockers(expectedRouteOwnership) {
  const blockers = [];
  if (expectedRouteOwnership.remainingRoutes !== 0) {
    blockers.push(`route ownership still has ${expectedRouteOwnership.remainingRoutes} remaining operation(s)`);
  }
  if (expectedRouteOwnership.cutoverQualifiedRoutes !== expectedRouteOwnership.baselineOperations) {
    blockers.push(
      `route ownership has ${expectedRouteOwnership.baselineOperations - expectedRouteOwnership.cutoverQualifiedRoutes} operation(s) not cutover-qualified`,
    );
  }
  if (expectedRouteOwnership.rustProductionOwnerRoutes !== expectedRouteOwnership.baselineOperations) {
    blockers.push(
      `production ownership remains Go for ${expectedRouteOwnership.goProductionOwnerRoutes} operation(s)`,
    );
  }
  if (expectedRouteOwnership.removedGoRoutes !== expectedRouteOwnership.baselineOperations) {
    blockers.push(
      `Go implementation removal remains incomplete for ${expectedRouteOwnership.baselineOperations - expectedRouteOwnership.removedGoRoutes} operation(s)`,
    );
  }
  return blockers;
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
      "localEvidence",
      "gates",
      "ownerDeletion",
    ],
    [
      "$schema",
      "schemaVersion",
      "stage",
      "status",
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
  const blockers = routeOwnershipBlockers(expectedRouteOwnership);

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
  addPrerequisiteBlockers(manifest, blockers);
  if (manifest.status !== "closed") blockers.push("manifest status is not closed");
  return {
    valid: true,
    complete: blockers.length === 0,
    errors: [],
    blockers,
    expectedRouteOwnership,
  };
}

/**
 * Validate the release-candidate admission boundary.  Candidate admission is
 * deliberately weaker than formal closeout: it requires every pre-release
 * gate, but permits evidence that can only be collected after publication.
 * It never changes the manifest or claims a release is formally closed.
 */
export function evaluateCandidate(manifest, options = {}) {
  const errors = validateManifest(manifest);
  if (errors.length > 0) {
    return {
      phase: "candidate",
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
  const blockers = routeOwnershipBlockers(expectedRouteOwnership);
  if (manifest.status !== "in_progress") {
    blockers.push("release candidate requires manifest status in_progress");
  }
  for (const gate of CANDIDATE_PREREQUISITE_GATES) {
    if (!gateIsPassed(manifest, gate)) {
      blockers.push(`candidate prerequisite gate ${gate} is ${manifest.gates[gate].status}`);
    }
  }
  for (const gate of POST_RELEASE_GATES) {
    if (manifest.gates[gate].status === "passed") {
      blockers.push(`candidate cannot accept post-release gate ${gate} as passed before publication`);
    }
  }
  addPrerequisiteBlockers(manifest, blockers);
  return {
    phase: "candidate",
    valid: true,
    complete: blockers.length === 0,
    errors: [],
    blockers,
    expectedRouteOwnership,
  };
}

/**
 * Validate only the repository-local admission boundary used before release
 * artifacts exist.  This deliberately does not inspect any external release
 * gate or claim that a candidate is qualified; artifact-bound evidence is
 * checked by check-release-candidate.mjs after all platform jobs complete.
 */
export function evaluateCandidateStatic(manifest, options = {}) {
  const errors = validateManifest(manifest);
  if (errors.length > 0) {
    return {
      phase: "candidate-static",
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
  const blockers = routeOwnershipBlockers(expectedRouteOwnership);
  if (manifest.status !== "in_progress") {
    blockers.push("static release admission requires manifest status in_progress");
  }
  for (const gate of ["allRouteGroups", "uniqueWriteOwner"]) {
    if (!gateIsPassed(manifest, gate)) {
      blockers.push(`static admission gate ${gate} is ${manifest.gates[gate].status}`);
    }
  }
  return {
    phase: "candidate-static",
    valid: true,
    complete: blockers.length === 0,
    errors: [],
    blockers,
    expectedRouteOwnership,
  };
}

function parseArguments(args) {
  let check = false;
  let mode = "full";
  let manifestPath = path.join(repositoryRoot, manifestRelativePath);
  for (let index = 0; index < args.length; index += 1) {
    const argument = args[index];
    if (argument === "--check") {
      check = true;
      mode = "full";
      continue;
    }
    if (argument === "--candidate" || argument === "--check-candidate") {
      check = true;
      mode = "candidate";
      continue;
    }
    if (argument === "--candidate-static" || argument === "--check-candidate-static") {
      check = true;
      mode = "candidate-static";
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
  return { check, mode, manifestPath };
}

export function main(args = process.argv.slice(2)) {
  let parsed;
  try {
    parsed = parseArguments(args);
    const manifest = readJson(parsed.manifestPath, "Stage 9 closeout evidence manifest");
    const result = parsed.mode === "candidate"
      ? evaluateCandidate(manifest)
      : parsed.mode === "candidate-static"
        ? evaluateCandidateStatic(manifest)
        : evaluateCloseout(manifest);
    if (!result.valid) {
      console.error("Stage 9 closeout evidence manifest is invalid:");
      for (const error of result.errors) console.error(`- ${error}`);
      return 1;
    }
    const state = result.complete
      ? (parsed.mode === "candidate"
        ? "candidate admission passed"
        : parsed.mode === "candidate-static"
          ? "static candidate admission passed"
          : "ready for formal close")
      : (parsed.mode === "candidate"
        ? "candidate admission blocked"
        : parsed.mode === "candidate-static"
          ? "static candidate admission blocked"
          : "in progress; formal close blocked");
    const label = parsed.mode === "candidate"
      ? "Stage 9 release-candidate evidence"
      : parsed.mode === "candidate-static"
        ? "Stage 9 static release-candidate evidence"
        : "Stage 9 closeout evidence";
    console.log(`${label}: ${state}.`);
    const ownership = result.expectedRouteOwnership;
    console.log(
      `Route ownership: ${ownership.shadowRoutes} shadow / `
        + `${ownership.cutoverTestOnlyRoutes} cutover-test-only / `
        + `${ownership.cutoverQualifiedRoutes} cutover-qualified / `
        + `${ownership.remainingRoutes} remaining / `
        + `${ownership.rustProductionOwnerRoutes} Rust production owner.`,
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
