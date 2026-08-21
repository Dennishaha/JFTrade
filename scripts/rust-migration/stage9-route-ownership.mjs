import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

export const IMPLEMENTATION_STATUSES = Object.freeze([
  "remaining",
  "shadow",
  "cutover-test-only",
  "cutover-qualified",
]);
export const PRODUCTION_OWNERS = Object.freeze(["go", "rust"]);
export const GO_REMOVAL_STATUSES = Object.freeze(["retained", "removed"]);

export const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));
const baselineRelativePath = "tests/fixtures/rust-migration/stage7/api-control-plane-corpus.json";
const ownershipRelativePath = "tests/fixtures/rust-migration/stage9/route-ownership.json";

function readJson(filePath, label) {
  try {
    return JSON.parse(fs.readFileSync(filePath, "utf8"));
  } catch (error) {
    throw new Error(`cannot read ${label} ${filePath}: ${error.message}`);
  }
}

export function routeKey(route) {
  return `${route.method} ${route.path}`;
}

export function loadRouteOwnership(root = repositoryRoot) {
  return {
    baseline: readJson(path.join(root, baselineRelativePath), "OpenAPI route baseline"),
    ownership: readJson(path.join(root, ownershipRelativePath), "route ownership fixture"),
  };
}

function requireString(value, label, errors) {
  if (typeof value !== "string" || value.trim() === "") {
    errors.push(`${label} must be a non-empty string`);
  }
}

function requireStringArray(value, label, errors) {
  if (!Array.isArray(value)) {
    errors.push(`${label} must be an array`);
    return;
  }
  value.forEach((item, index) => requireString(item, `${label}[${index}]`, errors));
  if (new Set(value).size !== value.length) errors.push(`${label} must not contain duplicates`);
}

export function validateRouteOwnership(baseline, ownership) {
  const errors = [];
  if (!baseline || !Array.isArray(baseline.routes)) {
    return ["OpenAPI route baseline must contain routes"];
  }
  if (!ownership || typeof ownership !== "object" || Array.isArray(ownership)) {
    return ["route ownership fixture must be an object"];
  }
  const allowedRootKeys = new Set(["version", "baselineVersion", "operations"]);
  for (const key of Object.keys(ownership)) {
    if (!allowedRootKeys.has(key)) errors.push(`route ownership.${key} is not allowed`);
  }
  if (ownership.version !== "stage9.route-ownership.v2") {
    errors.push("route ownership.version must be stage9.route-ownership.v2");
  }
  if (ownership.baselineVersion !== baseline.version) {
    errors.push(
      `baseline version ${baseline.version} does not match ${ownership.baselineVersion}`,
    );
  }
  if (!Array.isArray(ownership.operations)) {
    errors.push("route ownership.operations must be an array");
    return errors;
  }

  const baselineKeys = new Set(baseline.routes.map(routeKey));
  const operationKeys = new Set();
  ownership.operations.forEach((operation, index) => {
    const label = `route ownership.operations[${index}]`;
    if (!operation || typeof operation !== "object" || Array.isArray(operation)) {
      errors.push(`${label} must be an object`);
      return;
    }
    const requiredKeys = [
      "method",
      "path",
      "capability",
      "implementationStatus",
      "productionOwner",
      "goRemovalStatus",
      "dependencies",
      "evidence",
    ];
    for (const key of requiredKeys) {
      if (!(key in operation)) errors.push(`${label}.${key} is required`);
    }
    for (const key of Object.keys(operation)) {
      if (!requiredKeys.includes(key)) errors.push(`${label}.${key} is not allowed`);
    }
    requireString(operation.method, `${label}.method`, errors);
    requireString(operation.path, `${label}.path`, errors);
    requireString(operation.capability, `${label}.capability`, errors);
    requireStringArray(operation.dependencies, `${label}.dependencies`, errors);
    requireStringArray(operation.evidence, `${label}.evidence`, errors);
    if (!IMPLEMENTATION_STATUSES.includes(operation.implementationStatus)) {
      errors.push(`${label}.implementationStatus is invalid`);
    }
    if (!PRODUCTION_OWNERS.includes(operation.productionOwner)) {
      errors.push(`${label}.productionOwner is invalid`);
    }
    if (!GO_REMOVAL_STATUSES.includes(operation.goRemovalStatus)) {
      errors.push(`${label}.goRemovalStatus is invalid`);
    }
    if (operation.productionOwner === "go" && operation.goRemovalStatus !== "retained") {
      errors.push(`${label} cannot remove Go while Go is the production owner`);
    }
    if (operation.goRemovalStatus === "removed" && operation.productionOwner !== "rust") {
      errors.push(`${label} can remove Go only after Rust owns production`);
    }
    const key = routeKey(operation);
    if (!baselineKeys.has(key)) errors.push(`${label} contains non-baseline route ${key}`);
    if (operationKeys.has(key)) errors.push(`route is recorded more than once: ${key}`);
    operationKeys.add(key);
  });
  for (const route of baseline.routes) {
    const key = routeKey(route);
    if (!operationKeys.has(key)) errors.push(`baseline route is missing from ledger: ${key}`);
  }
  if (operationKeys.size !== baseline.routes.length) {
    errors.push(
      `route ledger must contain exactly ${baseline.routes.length} operations, found ${operationKeys.size}`,
    );
  }
  return errors;
}

export function routeOwnershipSnapshot(root = repositoryRoot) {
  const { baseline, ownership } = loadRouteOwnership(root);
  const errors = validateRouteOwnership(baseline, ownership);
  if (errors.length > 0) throw new Error(errors.join("\n"));

  const byStatus = Object.fromEntries(IMPLEMENTATION_STATUSES.map((status) => [status, 0]));
  const byProductionOwner = Object.fromEntries(PRODUCTION_OWNERS.map((owner) => [owner, 0]));
  const remainingByCapability = {};
  let removedGoRoutes = 0;
  for (const operation of ownership.operations) {
    byStatus[operation.implementationStatus] += 1;
    byProductionOwner[operation.productionOwner] += 1;
    if (operation.goRemovalStatus === "removed") removedGoRoutes += 1;
    if (operation.implementationStatus === "remaining") {
      remainingByCapability[operation.capability] =
        (remainingByCapability[operation.capability] ?? 0) + 1;
    }
  }
  return {
    baselineOperations: baseline.routes.length,
    shadowRoutes: byStatus.shadow,
    cutoverTestOnlyRoutes: byStatus["cutover-test-only"],
    cutoverQualifiedRoutes: byStatus["cutover-qualified"],
    remainingRoutes: byStatus.remaining,
    goProductionOwnerRoutes: byProductionOwner.go,
    rustProductionOwnerRoutes: byProductionOwner.rust,
    removedGoRoutes,
    remainingByCapability: Object.fromEntries(Object.entries(remainingByCapability).sort()),
  };
}
