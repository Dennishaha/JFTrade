import { describe, expect, it } from "vitest";

import {
  strategyPineEditorCompletions,
  strategyPineEditorHoverItems,
} from "../src/features/strategyPineEditorIntelliSense";

const executableTACompletionLabels = [
  "ta.ema",
  "ta.sma",
  "ta.rma",
  "ta.wma",
  "ta.hma",
  "ta.vwma",
  "ta.rsi",
  "ta.atr",
  "ta.cci",
  "ta.highest",
  "ta.lowest",
  "ta.highestbars",
  "ta.lowestbars",
  "ta.change",
  "ta.mom",
  "ta.roc",
  "ta.range",
  "ta.mode",
  "ta.rising",
  "ta.falling",
  "ta.stdev",
  "ta.variance",
  "ta.cum",
  "ta.wpr",
  "ta.mfi",
  "ta.stoch",
  "ta.tr",
  "ta.barssince",
  "ta.valuewhen",
  "ta.crossover",
  "ta.crossunder",
  "ta.cross",
  "ta.bbw",
  "ta.cog",
  "ta.vwap",
  "ta.sar",
  "ta.linreg",
  "ta.pivothigh",
  "ta.pivotlow",
  "ta.kcw",
  "ta.alma",
  "ta.cmo",
  "ta.tsi",
  "ta.correlation",
  "ta.dev",
  "ta.median",
  "ta.percentile_linear_interpolation",
  "ta.percentile_nearest_rank",
  "ta.percentrank",
  "ta.swma",
];

const executableTATupleCompletionLabels = [
  "ta.macd",
  "ta.bb",
  "ta.dmi",
  "ta.supertrend",
  "ta.kc",
];

const executableUtilityCompletionLabels = [
  "input",
  "input.int",
  "input.float",
  "input.bool",
  "input.string",
  "input.source",
  "input.time",
  "input.timeframe",
  "input.color",
  "math.abs",
  "math.min",
  "math.max",
  "math.avg",
  "math.round",
  "math.round_to_mintick",
  "math.floor",
  "math.ceil",
  "math.sqrt",
  "math.pow",
  "math.log",
  "math.sign",
  "str.length",
  "str.tostring",
  "str.contains",
  "str.pos",
  "str.substring",
  "str.replace",
  "str.upper",
  "str.lower",
  "str.format",
  "timeframe.change",
  "timeframe.in_seconds",
];

const pineTSVisualCompletionLabels = [
  "plot",
  "plotshape",
  "plotchar",
  "label.new",
  "line.new",
  "box.new",
  "table.new",
  "alertcondition",
];

describe("strategyPineEditorIntelliSense", () => {
  it("suggests the executable Pine v6 TA public surface", () => {
    const labels = new Set(strategyPineEditorCompletions.map((completion) => completion.label));

    for (const label of [...executableTACompletionLabels, ...executableTATupleCompletionLabels]) {
      expect(labels.has(label), `${label} should be suggested`).toBe(true);
    }
  });

  it("documents executable TA completions with hover signatures", () => {
    const hoverTargets = new Set(strategyPineEditorHoverItems.map((item) => item.target));

    for (const label of [...executableTACompletionLabels, ...executableTATupleCompletionLabels]) {
      expect(hoverTargets.has(label), `${label} should have hover documentation`).toBe(true);
    }
  });

  it("suggests and documents executable Pine v6 utility helpers", () => {
    const labels = new Set(strategyPineEditorCompletions.map((completion) => completion.label));
    const hoverTargets = new Set(strategyPineEditorHoverItems.map((item) => item.target));

    for (const label of executableUtilityCompletionLabels) {
      expect(labels.has(label), `${label} should be suggested`).toBe(true);
      expect(hoverTargets.has(label), `${label} should have hover documentation`).toBe(true);
    }
  });

  it("suggests and documents PineTS visual and alert outputs without treating them as trading blocks", () => {
    const labels = new Set(strategyPineEditorCompletions.map((completion) => completion.label));
    const hoverTargets = new Set(strategyPineEditorHoverItems.map((item) => item.target));

    for (const label of pineTSVisualCompletionLabels) {
      expect(labels.has(label), `${label} should be suggested`).toBe(true);
      expect(hoverTargets.has(label), `${label} should have hover documentation`).toBe(true);
    }
  });

  it("does not suggest JFTrade-only helper shortcuts as public Pine v6 entrypoints", () => {
    const labels = new Set(strategyPineEditorCompletions.map((completion) => completion.label));
    const hoverTargets = new Set(strategyPineEditorHoverItems.map((item) => item.target));

    for (const helper of ["ma", "security_source", "bollinger", "williams_r", "ta.adx"]) {
      expect(labels.has(helper), `${helper} completion should stay internal-only`).toBe(false);
      expect(hoverTargets.has(helper), `${helper} hover should stay internal-only`).toBe(false);
    }
  });
});
