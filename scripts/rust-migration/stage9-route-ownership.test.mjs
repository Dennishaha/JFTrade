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
    shadowRoutes: 26,
    cutoverTestOnlyRoutes: 172,
    cutoverQualifiedRoutes: 0,
    remainingRoutes: 80,
    goProductionOwnerRoutes: 278,
    rustProductionOwnerRoutes: 0,
    removedGoRoutes: 0,
    remainingByCapability: {
      adk: 37,
      backtests: 4,
      brokers: 3,
      execution: 7,
      "market-data": 6,
      research: 1,
      strategies: 7,
      system: 7,
      watchlist: 8,
    },
  });
});

test("Stage 9 ownership ledger rejects missing records and unsafe Go removal", () => {
  const { baseline, ownership } = loadRouteOwnership();
  const changed = structuredClone(ownership);
  changed.operations.pop();
  changed.operations[0].goRemovalStatus = "removed";
  const errors = validateRouteOwnership(baseline, changed);
  assert.ok(errors.some((error) => error.includes("cannot remove Go")));
  assert.ok(errors.some((error) => error.includes("baseline route is missing")));
});
