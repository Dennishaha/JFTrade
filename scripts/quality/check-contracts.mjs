#!/usr/bin/env node
import crypto from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

import { buildApiTransportCorpus } from "../compatibility/generate-api-transport-corpus.mjs";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));
const expectedRouteCount = 278;
const publicRoutes = new Set([
  "GET /api/v1/auth/session",
  "POST /api/v1/auth/login",
]);
const writeMethods = new Set(["POST", "PUT", "PATCH", "DELETE"]);

export function routeKey(route) {
  return `${route.method} ${route.path}`;
}

export function routeDigest(routes) {
  const hash = crypto.createHash("sha256");
  for (const route of routes) hash.update(`${routeKey(route)}\n`);
  return hash.digest("hex");
}

export function validateRouteContracts(openapi, manifest, routerSource) {
  const errors = [];
  const routes = buildApiTransportCorpus(openapi).routes;
  if (manifest?.version !== "production.v1") {
    errors.push("production route manifest version must be production.v1");
  }
  if (!Array.isArray(manifest?.operations)) {
    return [...errors, "production route manifest must contain operations"];
  }
  const actual = manifest.operations;
  const actualKeys = actual.map(routeKey);
  const expectedKeys = routes.map(routeKey);
  if (routes.length !== expectedRouteCount || actual.length !== expectedRouteCount) {
    errors.push(`route contracts must contain ${expectedRouteCount} operations`);
  }
  if (new Set(actualKeys).size !== actualKeys.length) {
    errors.push("production route manifest contains duplicate operations");
  }
  if (JSON.stringify(actualKeys) !== JSON.stringify(expectedKeys)) {
    errors.push("OpenAPI and Rust production route manifest differ");
  }
  if (actual.some((route) => typeof route.capability !== "string" || route.capability === "")) {
    errors.push("every production route must declare a capability");
  }
  if (manifest.routeDigest !== routeDigest(actual)) {
    errors.push("production route manifest digest does not match its operations");
  }
  for (const route of publicRoutes) {
    if (!actualKeys.includes(route)) errors.push(`public authentication route is missing: ${route}`);
  }
  for (const token of [
    'path != "/api/v1/auth/login"',
    'path != "/api/v1/auth/session"',
    "Method::POST | Method::PUT | Method::PATCH | Method::DELETE",
    '"ORIGIN_FORBIDDEN"',
    '"CSRF_FAILED"',
  ]) {
    if (!routerSource.includes(token)) errors.push(`Rust authentication policy is missing ${token}`);
  }
  return errors;
}

export function routeContractSnapshot(root = repositoryRoot) {
  const openapi = JSON.parse(fs.readFileSync(path.join(root, "contracts/openapi/openapi.json"), "utf8"));
  const manifest = JSON.parse(fs.readFileSync(
    path.join(root, "crates/jftrade-engine/src/product_production_route_manifest.json"),
    "utf8",
  ));
  const routerSource = fs.readFileSync(path.join(root, "crates/jftrade-api/src/router.rs"), "utf8");
  const errors = validateRouteContracts(openapi, manifest, routerSource);
  if (errors.length > 0) throw new Error(errors.join("\n"));
  const keys = manifest.operations.map(routeKey);
  return {
    operations: keys.length,
    authenticated: keys.filter((key) => !publicRoutes.has(key)).length,
    public: keys.filter((key) => publicRoutes.has(key)).length,
    browserWrites: manifest.operations.filter((route) => (
      writeMethods.has(route.method) && !publicRoutes.has(routeKey(route))
    )).length,
    digest: manifest.routeDigest,
  };
}

if (pathToFileURL(path.resolve(process.argv[1] ?? "")).href === import.meta.url) {
  try {
    const snapshot = routeContractSnapshot();
    console.log(
      `Route contracts passed: ${snapshot.operations}/${expectedRouteCount} Rust operations, `
      + `${snapshot.authenticated} authenticated, ${snapshot.public} public, `
      + `${snapshot.browserWrites} browser writes protected by origin and CSRF policy.`,
    );
  } catch (error) {
    console.error(`Route contracts failed: ${error.message}`);
    process.exitCode = 1;
  }
}
