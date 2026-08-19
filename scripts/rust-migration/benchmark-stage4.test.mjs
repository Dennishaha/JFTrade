import assert from "node:assert/strict";
import test from "node:test";

import {
  evaluateStage4Performance,
  summarizeStage4Samples,
} from "./benchmark-stage4.mjs";

test("stage 4 samples report tail latency and maximum RSS", () => {
  const summary = summarizeStage4Samples([
    { elapsedMillis: 10, cpuMillis: 5, peakRssBytes: 100 },
    { elapsedMillis: 30, cpuMillis: 10, peakRssBytes: 300 },
    { elapsedMillis: 20, cpuMillis: 8, peakRssBytes: 200 },
  ], 2);
  assert.deepEqual(summary.elapsedMillis, { p50: 20, p95: 30, p99: 30 });
  assert.equal(summary.cpuMillisP50, 8);
  assert.equal(summary.peakRssBytes, 300);
  assert.equal(summary.warmups, 2);
});

test("stage 4 gates reject latency and RSS regressions independently", () => {
  const baseline = { elapsedMillis: { p95: 100 }, peakRssBytes: 1000 };
  assert.deepEqual(
    evaluateStage4Performance(baseline, {
      elapsedMillis: { p95: 105 },
      peakRssBytes: 1100,
    }),
    {
      rustToGoP95: 1.05,
      rustToGoPeakRss: 1.1,
      p95RegressionGatePassed: true,
      rssRegressionGatePassed: true,
    },
  );
  const failed = evaluateStage4Performance(baseline, {
    elapsedMillis: { p95: 106 },
    peakRssBytes: 1101,
  });
  assert.equal(failed.p95RegressionGatePassed, false);
  assert.equal(failed.rssRegressionGatePassed, false);
});
