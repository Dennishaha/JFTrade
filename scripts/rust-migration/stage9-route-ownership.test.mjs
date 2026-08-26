import assert from "node:assert/strict";
import test from "node:test";

import {
  loadRouteOwnership,
  routeOwnershipSnapshot,
  validateRouteOwnership,
} from "./stage9-route-ownership.mjs";

test("Stage 9 ownership ledger records every baseline operation and derives all counts", () => {
  const { baseline, ownership } = loadRouteOwnership();
  assert.deepEqual(validateRouteOwnership(baseline, ownership), []);
  assert.deepEqual(routeOwnershipSnapshot(), {
    baselineOperations: 278,
    shadowRoutes: 0,
    cutoverTestOnlyRoutes: 0,
    cutoverQualifiedRoutes: 278,
    remainingRoutes: 0,
    goProductionOwnerRoutes: 0,
    rustProductionOwnerRoutes: 278,
    removedGoRoutes: 0,
    remainingByCapability: {},
  });
});

test("Stage 9 ownership ledger rejects missing records and unsafe Go removal", () => {
  const { baseline, ownership } = loadRouteOwnership();
  const changed = structuredClone(ownership);
  changed.operations.pop();
  changed.operations[0].productionOwner = "go";
  changed.operations[0].goRemovalStatus = "removed";
  const errors = validateRouteOwnership(baseline, changed);
  assert.ok(errors.some((error) => error.includes("cannot remove Go")));
  assert.ok(errors.some((error) => error.includes("baseline route is missing")));
});
