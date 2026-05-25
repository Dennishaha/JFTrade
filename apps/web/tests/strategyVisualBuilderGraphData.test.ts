import { describe, expect, it } from "vitest";

import {
  fromLogicFlowGraphData,
  getStrategyBlockKind,
} from "../src/features/strategyVisualBuilder";

describe("strategyVisualBuilderGraphData", () => {
  it("normalizes freshly created technical indicator nodes from graph data", () => {
    const model = fromLogicFlowGraphData({
      nodes: [
        {
          id: "indicator-node",
          type: "rect",
          x: 420,
          y: 260,
          text: "",
          properties: {
            blockKind: "technicalIndicator",
          },
        },
      ],
      edges: [],
    });

    const node = model.nodes[0];
    expect(node).toBeDefined();
    expect(getStrategyBlockKind(node)).toBe("technicalIndicator");
    expect(node?.text).toBe("RSI 14 < 30");
    expect(node?.properties.indicatorType).toBe("rsi");
    expect(node?.properties.conditionMode).toBe("numeric");
    expect(node?.properties.operator).toBe("<");
    expect(node?.properties.threshold).toBe(30);
    expect(node?.properties.period).toBe(14);
  });

  it("fills missing defaults for non-indicator palette nodes", () => {
    const model = fromLogicFlowGraphData({
      nodes: [
        {
          id: "notify-node",
          type: "rect",
          x: 420,
          y: 260,
          text: "",
          properties: {
            blockKind: "notify",
          },
        },
      ],
      edges: [],
    });

    const node = model.nodes[0];
    expect(node?.text).toBe("发送通知");
    expect(node?.properties.message).toBe("策略条件命中，准备处理后续动作");
  });
});