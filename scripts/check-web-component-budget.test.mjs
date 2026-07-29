import assert from "node:assert/strict";
import test from "node:test";

import {
  compareBudgetToMergeBase,
  compareWebComponentBudget,
  inspectVueSource,
} from "./check-web-component-budget.mjs";

test("counts local scoped style sources as effective component size", () => {
  assert.deepEqual(
    inspectVueSource('<template />\n<style scoped src="./panel.css"></style>\n', () => ".x {\n color: red;\n}\n"),
    { sourceLines: 2, scopedStyleLines: 3, externalStyleLines: 3, effectiveLines: 5 },
  );
});

test("rejects new growth and stale exceptions", () => {
  const result = compareWebComponentBudget([
    { name: "new.vue", effectiveLines: 801, scopedStyleLines: 4 },
    { name: "legacy.vue", effectiveLines: 902, scopedStyleLines: 3 },
    { name: "small.vue", effectiveLines: 700, scopedStyleLines: 2 },
  ], {
    defaultMaxLines: 800,
    scopedStyleLinesMax: 8,
    exceptions: {
      "legacy.vue": { maxLines: 900, reason: "remaining responsibilities are being split" },
      "small.vue": { maxLines: 850, reason: "this exception should now be removed" },
    },
  });
  assert.deepEqual(result.failures, [
    "scoped style lines 9 exceed budget 8",
    "new.vue has 801 effective lines, limit 800",
    "legacy.vue grew to 902 effective lines, budget 900",
    "small.vue has a stale exception at 700 lines",
  ]);
});

test("accepts a shrinking documented baseline", () => {
  const result = compareWebComponentBudget(
    [{ name: "legacy.vue", effectiveLines: 850, scopedStyleLines: 10 }],
    { defaultMaxLines: 800, scopedStyleLinesMax: 10, exceptions: {
      "legacy.vue": { maxLines: 850, reason: "split the remaining editor state next" },
    } },
  );
  assert.deepEqual(result.failures, []);
});

test("rejects slack ceilings that could hide growth", () => {
  const result = compareWebComponentBudget(
    [{ name: "legacy.vue", effectiveLines: 850, scopedStyleLines: 10 }],
    { defaultMaxLines: 801, scopedStyleLinesMax: 11, exceptions: {
      "legacy.vue": { maxLines: 900, reason: "split the remaining editor state next" },
    } },
  );
  assert.deepEqual(result.failures, [
    "defaultMaxLines must remain 800",
    "scopedStyleLinesMax is stale at 11; reduce it to 10",
    "legacy.vue exception is stale at 900; reduce it to 850",
  ]);
});

test("rejects exception and style budget growth relative to merge-base", () => {
  const failures = compareBudgetToMergeBase(
    {
      defaultMaxLines: 800,
      scopedStyleLinesMax: 101,
      exceptions: {
        "legacy.vue": { maxLines: 901 },
        "new.vue": { maxLines: 820 },
      },
    },
    {
      defaultMaxLines: 800,
      scopedStyleLinesMax: 100,
      exceptions: { "legacy.vue": { maxLines: 900 } },
    },
  );
  assert.deepEqual(failures, [
    "scopedStyleLinesMax grew from 100 to 101",
    "legacy.vue exception grew from 900 to 901",
    "new.vue is a new component budget exception relative to merge-base",
  ]);
});
