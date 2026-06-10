import { describe, expect, it } from "vitest";

import type { StrategyDefinitionDocument } from "@/contracts";

import { migrateLegacyMovingAverageDefinition } from "../src/features/strategyVisualBuilderMigration";

describe("migrateLegacyMovingAverageDefinition", () => {
  it("migrates legacy MA5/20 scripts and visual nodes to day semantics", () => {
    const definition: StrategyDefinitionDocument = {
      id: "legacy-ma",
      name: "Legacy MA",
      version: "0.1.0",
      description: "legacy MA definition",
      runtime: "dsl-go-plan",
      symbol: "00700",
      interval: "1m",
      script: 'function onKLineClosed(ctx) { const fast = ctx.indicators["ma:EMA:5"]; const slow = ctx.indicators["ma:20"]; return fast && slow; }',
      visualModel: {
        engine: "logic-flow",
        version: 1,
        nodes: [
          {
            id: "fast-ma",
            type: "rect",
            x: 120,
            y: 100,
            text: "获取 均线 EMA 5",
            properties: {
              blockKind: "getTechnicalIndicator",
              indicatorType: "movingAverage",
              movingAverageType: "EMA",
              windowSize: 5,
            },
          },
          {
            id: "slow-ma",
            type: "rect",
            x: 320,
            y: 100,
            text: "自定义均线节点",
            properties: {
              blockKind: "getTechnicalIndicator",
              indicatorType: "movingAverage",
              movingAverageType: "MA",
              windowSize: 20,
            },
          },
        ],
        edges: [],
      },
      createdAt: "2026-05-26T00:00:00.000Z",
      updatedAt: "2026-05-26T00:00:00.000Z",
    };

    const migrated = migrateLegacyMovingAverageDefinition(definition);

    expect(migrated.script).toContain('ctx.indicators["ma:EMA:5:day"]');
    expect(migrated.script).toContain('ctx.indicators["ma:MA:20:day"]');
    expect(migrated.visualModel?.nodes[0]?.properties.periodUnit).toBe("day");
    expect(migrated.visualModel?.nodes[0]?.text).toBe("获取 均线 EMA 5日");
    expect(migrated.visualModel?.nodes[1]?.properties.periodUnit).toBe("day");
    expect(migrated.visualModel?.nodes[1]?.text).toBe("自定义均线节点");
  });
});