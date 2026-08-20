#!/usr/bin/env node
import fs from "node:fs";
import { fileURLToPath } from "node:url";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));
const baseline = JSON.parse(fs.readFileSync(
  `${repositoryRoot}/tests/fixtures/rust-migration/stage7/api-control-plane-corpus.json`,
  "utf8",
));
const ownership = JSON.parse(fs.readFileSync(
  `${repositoryRoot}/tests/fixtures/rust-migration/stage9/route-ownership.json`,
  "utf8",
));

function fail(message) {
  throw new Error(`Stage 9 route ownership gate: ${message}`);
}

function routeKey(route) {
  return `${route.method} ${route.path}`;
}

function groupFor(route) {
  return route.path.slice("/api/v1/".length).split("/")[0];
}

if (baseline.version !== ownership.baselineVersion) {
  fail(`baseline version ${baseline.version} does not match ${ownership.baselineVersion}`);
}
if (baseline.routes.length !== ownership.baselineOperations) {
  fail(`expected ${ownership.baselineOperations} baseline operations, found ${baseline.routes.length}`);
}

const baselineKeys = new Set(baseline.routes.map(routeKey));
const claimed = new Set();
for (const [bucket, routes] of [
  ["shadowRoutes", ownership.shadowRoutes],
  ["cutoverTestRoutes", ownership.cutoverTestRoutes],
]) {
  for (const route of routes) {
    const key = routeKey(route);
    if (!baselineKeys.has(key)) fail(`${bucket} contains non-OpenAPI route ${key}`);
    if (claimed.has(key)) fail(`route is claimed more than once: ${key}`);
    claimed.add(key);
  }
}

const remaining = baseline.routes.filter((route) => !claimed.has(routeKey(route)));
const remainingByGroup = {};
for (const route of remaining) {
  const group = groupFor(route);
  remainingByGroup[group] = (remainingByGroup[group] ?? 0) + 1;
}
const orderedRemaining = Object.fromEntries(Object.entries(remainingByGroup).sort());
if (JSON.stringify(orderedRemaining) !== JSON.stringify(ownership.remainingByGroup)) {
  fail(`remaining route ledger drifted: ${JSON.stringify(orderedRemaining)}`);
}
if (claimed.size + remaining.length !== ownership.baselineOperations) {
  fail("route classification is not exhaustive");
}

console.log(
  `Stage 9 route ownership gate passed: ${ownership.shadowRoutes.length} read-only shadow, `
    + `${ownership.cutoverTestRoutes.length} cutover-test-only, ${remaining.length} remaining operations.`,
);
