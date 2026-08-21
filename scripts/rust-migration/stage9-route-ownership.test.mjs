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
    cutoverTestOnlyRoutes: 30,
    cutoverQualifiedRoutes: 0,
    remainingRoutes: 222,
    goProductionOwnerRoutes: 278,
    rustProductionOwnerRoutes: 0,
    removedGoRoutes: 0,
    remainingByCapability: {
      adk: 63,
      alerts: 4,
      auth: 3,
      backtests: 8,
      brokers: 16,
      execution: 10,
      "market-data": 46,
      plugins: 4,
      portfolio: 2,
      research: 20,
      strategies: 10,
      "strategy-definitions": 9,
      "strategy-pine": 1,
      system: 9,
      watchlist: 14,
      watchlists: 2,
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
