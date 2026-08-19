import assert from "node:assert/strict";
import test from "node:test";

import {
  evaluateBacktestPerformance,
  summarizeBacktestSamples,
} from "./benchmark-backtest.mjs";

test("summarizes p50 p95 p99 CPU and peak RSS", () => {
  const result = summarizeBacktestSamples([
    { elapsedMillis: 1, cpuMillis: 3, peakRssBytes: 100 },
    { elapsedMillis: 2, cpuMillis: 2, peakRssBytes: 150 },
    { elapsedMillis: 9, cpuMillis: 4, peakRssBytes: 120 },
  ], 2);
  assert.deepEqual(result, {
    iterations: 3,
    warmups: 2,
    elapsedMillis: { p50: 2, p95: 9, p99: 9 },
    cpuMillisP50: 3,
    peakRssBytes: 150,
  });
});

test("enforces regression budgets and the compute improvement target", () => {
  const goResult = { elapsedMillis: { p95: 100 }, peakRssBytes: 1000 };
  assert.deepEqual(
    evaluateBacktestPerformance(goResult, { elapsedMillis: { p95: 60 }, peakRssBytes: 800 }),
    {
      rustToGoP95: 0.6,
      rustToGoPeakRss: 0.8,
      p95RegressionGatePassed: true,
      rssRegressionGatePassed: true,
      computeTargetPassed: true,
    },
  );
  const failed = evaluateBacktestPerformance(
    goResult,
    { elapsedMillis: { p95: 106 }, peakRssBytes: 1110 },
  );
  assert.equal(failed.p95RegressionGatePassed, false);
  assert.equal(failed.rssRegressionGatePassed, false);
  assert.equal(failed.computeTargetPassed, false);
});
