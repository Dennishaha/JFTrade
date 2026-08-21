#!/usr/bin/env node

import { routeOwnershipSnapshot } from "./stage9-route-ownership.mjs";

try {
  const snapshot = routeOwnershipSnapshot();
  console.log(
    `Stage 9 route ownership gate passed: ${snapshot.shadowRoutes} read-only shadow, `
      + `${snapshot.cutoverTestOnlyRoutes} cutover-test-only, `
      + `${snapshot.cutoverQualifiedRoutes} cutover-qualified, `
      + `${snapshot.remainingRoutes} remaining operations; `
      + `${snapshot.rustProductionOwnerRoutes} Rust production owner.`,
  );
} catch (error) {
  console.error(`Stage 9 route ownership gate failed: ${error.message}`);
  process.exitCode = 1;
}
