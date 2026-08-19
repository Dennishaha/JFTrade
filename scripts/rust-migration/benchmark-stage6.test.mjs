import assert from "node:assert/strict";
import test from "node:test";

import { evaluateStage6Performance, summarizeStage6Samples } from "./benchmark-stage6.mjs";

test("Stage 6 sample summary computes deterministic percentiles", () => {
  const summary = summarizeStage6Samples([
    { elapsedMillis: 1, peakRssBytes: 10, cpuMillis: 1 },
    { elapsedMillis: 2, peakRssBytes: 20, cpuMillis: 2 },
    { elapsedMillis: 3, peakRssBytes: 15, cpuMillis: 3 },
  ], 1);
  assert.equal(summary.elapsedMillis.p50, 2);
  assert.equal(summary.elapsedMillis.p95, 3);
  assert.equal(summary.peakRssBytes, 20);
});

test("Stage 6 gate rejects a latency regression", () => {
  const gate = evaluateStage6Performance(
    { elapsedMillis: { p95: 10 }, peakRssBytes: 100 },
    { elapsedMillis: { p95: 11 }, peakRssBytes: 100 },
  );
  assert.equal(gate.p95RegressionGatePassed, false);
  assert.equal(gate.rssRegressionGatePassed, true);
});
