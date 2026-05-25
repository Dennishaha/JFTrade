import { describe, expect, it } from "vitest";

import {
  buildStrategyVisualNodeSummary,
} from "../src/features/strategyVisualBuilderNodePresentation";

describe("strategyVisualBuilderNodePresentation", () => {
  it("builds a getter summary with output details and data chip", () => {
    const summary = buildStrategyVisualNodeSummary({
      properties: {
        blockKind: "getTechnicalIndicator",
        indicatorType: "movingAverage",
        movingAverageType: "EMA",
        windowSize: 20,
      },
    });

    expect(summary.eyebrow).toBe("技术指标");
    expect(summary.title).toBe("获取 均线 EMA 20");
    expect(summary.details).toContainEqual({
      label: "输出",
      value: "快线 / 慢线",
    });
    expect(summary.chips).toEqual(["数据"]);
  });

  it("builds a condition summary with true false chips", () => {
    const summary = buildStrategyVisualNodeSummary({
      properties: {
        blockKind: "technicalIndicatorCondition",
        indicatorType: "rsi",
        conditionMode: "numeric",
        operator: "<",
        threshold: 30,
      },
    });

    expect(summary.eyebrow).toBe("指标条件判断");
    expect(summary.title).toBe("RSI < 30");
    expect(summary.details).toContainEqual({
      label: "判断",
      value: "< 30",
    });
    expect(summary.chips).toEqual(["True", "False"]);
  });

  it("builds a place-order summary with direction order type and quantity", () => {
    const summary = buildStrategyVisualNodeSummary({
      text: "下单 · 买入开多 · 100 股",
      properties: {
        blockKind: "placeOrder",
        side: "BUY",
        orderType: "LIMIT",
        limitPrice: 518.5,
        quantityMode: "shares",
        quantityValue: 100,
      },
    });

    expect(summary.eyebrow).toBe("交易动作");
    expect(summary.details).toContainEqual({
      label: "方向",
      value: "买入开多",
    });
    expect(summary.details).toContainEqual({
      label: "委托",
      value: "限价 518.5",
    });
    expect(summary.details).toContainEqual({
      label: "数量",
      value: "100 股",
    });
  });

  it("builds a place-order summary for current symbol position sizing", () => {
    const summary = buildStrategyVisualNodeSummary({
      text: "下单 · 卖出平多 · 25% 当前标的仓位",
      properties: {
        blockKind: "placeOrder",
        side: "SELL",
        orderType: "MARKET",
        quantityMode: "symbolPositionPercent",
        quantityValue: 25,
      },
    });

    expect(summary.details).toContainEqual({
      label: "数量",
      value: "25% 当前标的仓位",
    });
  });

  it("builds a place-order summary for account position sizing", () => {
    const summary = buildStrategyVisualNodeSummary({
      text: "下单 · 买入开多 · 10% 账户仓位",
      properties: {
        blockKind: "placeOrder",
        side: "BUY",
        orderType: "MARKET",
        quantityMode: "accountPositionPercent",
        quantityValue: 10,
      },
    });

    expect(summary.details).toContainEqual({
      label: "数量",
      value: "10% 账户仓位",
    });
  });
});