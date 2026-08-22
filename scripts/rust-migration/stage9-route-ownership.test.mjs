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
    cutoverTestOnlyRoutes: 98,
    cutoverQualifiedRoutes: 0,
    remainingRoutes: 154,
    goProductionOwnerRoutes: 278,
    rustProductionOwnerRoutes: 0,
    removedGoRoutes: 0,
    remainingByCapability: {
      adk: 63,
      alerts: 2,
      auth: 3,
      backtests: 4,
      brokers: 3,
      execution: 7,
      "market-data": 36,
      plugins: 2,
      research: 4,
      strategies: 7,
      "strategy-definitions": 5,
      "strategy-pine": 1,
      system: 7,
      watchlist: 8,
      watchlists: 1,
      ws: 1,
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
