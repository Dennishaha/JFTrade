import assert from "node:assert/strict";
import test from "node:test";

import { evaluateStage7Performance, summarizeStage7Samples } from "./benchmark-stage7.mjs";

test("Stage 7 sample summary computes deterministic percentiles", () => {
  const summary = summarizeStage7Samples([
    { elapsedMillis: 1, peakRssBytes: 10, cpuMillis: 1 },
    { elapsedMillis: 2, peakRssBytes: 20, cpuMillis: 2 },
    { elapsedMillis: 3, peakRssBytes: 15, cpuMillis: 3 },
  ], 1);
  assert.equal(summary.elapsedMillis.p50, 2);
  assert.equal(summary.elapsedMillis.p95, 3);
  assert.equal(summary.peakRssBytes, 20);
});

test("Stage 7 gate rejects an RSS regression", () => {
  const gate = evaluateStage7Performance(
    { elapsedMillis: { p95: 10 }, peakRssBytes: 100 },
    { elapsedMillis: { p95: 10 }, peakRssBytes: 111 },
  );
  assert.equal(gate.p95RegressionGatePassed, true);
  assert.equal(gate.rssRegressionGatePassed, false);
});
