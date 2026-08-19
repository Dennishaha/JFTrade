import assert from "node:assert/strict";
import test from "node:test";

import { evaluateStage8Performance, summarizeStage8Samples } from "./benchmark-stage8.mjs";

test("Stage 8 sample summary computes deterministic percentiles", () => {
  const summary = summarizeStage8Samples([
    { elapsedMillis: 1, peakRssBytes: 10, cpuMillis: 1 },
    { elapsedMillis: 2, peakRssBytes: 20, cpuMillis: 2 },
    { elapsedMillis: 3, peakRssBytes: 15, cpuMillis: 3 },
  ], 1);
  assert.equal(summary.elapsedMillis.p50, 2);
  assert.equal(summary.elapsedMillis.p95, 3);
  assert.equal(summary.peakRssBytes, 20);
});

test("Stage 8 gate rejects a p95 regression", () => {
  const gate = evaluateStage8Performance(
    { elapsedMillis: { p95: 100 }, peakRssBytes: 100 },
    { elapsedMillis: { p95: 106 }, peakRssBytes: 100 },
  );
  assert.equal(gate.p95RegressionGatePassed, false);
  assert.equal(gate.rssRegressionGatePassed, true);
});
