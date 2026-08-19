import assert from "node:assert/strict";
import test from "node:test";

import { summarizeSamples } from "./benchmark-readonly.mjs";

test("summarizes representative elapsed CPU and RSS samples", () => {
  const result = summarizeSamples([
    { elapsedMillis: 1, cpuMillis: 3, peakRssBytes: 100 },
    { elapsedMillis: 2, cpuMillis: 2, peakRssBytes: 150 },
    { elapsedMillis: 9, cpuMillis: 4, peakRssBytes: 120 },
  ], 2);
  assert.deepEqual(result, {
    iterations: 3,
    warmups: 2,
    elapsedMillis: { p50: 2, p95: 9 },
    cpuMillisP50: 3,
    peakRssBytes: 150,
  });
});
