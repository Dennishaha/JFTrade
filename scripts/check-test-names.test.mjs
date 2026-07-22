import assert from "node:assert/strict";
import test from "node:test";

import { hasCoverageNumberName, isTestFile, parseAddedPaths } from "./check-test-names.mjs";

test("recognizes Go and TypeScript test files", () => {
  assert.equal(isTestFile("pkg/trading/order_test.go"), true);
  assert.equal(isTestFile("apps/web/tests/order-flow.spec.ts"), true);
  assert.equal(isTestFile("apps/web/src/order.ts"), false);
});

test("rejects numeric coverage filenames without rejecting behavioral names", () => {
  assert.equal(hasCoverageNumberName("pkg/trading/coverage_98_test.go"), true);
  assert.equal(hasCoverageNumberName("apps/web/tests/order-c95.spec.ts"), true);
  assert.equal(hasCoverageNumberName("apps/web/tests/order-risk.spec.ts"), false);
});

test("reads added and renamed paths from NUL-delimited git output", () => {
  const output = "A\0pkg/trading/order_test.go\0R100\0old_test.go\0apps/web/tests/order-flow.spec.ts\0";
  assert.deepEqual(parseAddedPaths(output), ["pkg/trading/order_test.go", "apps/web/tests/order-flow.spec.ts"]);
});
