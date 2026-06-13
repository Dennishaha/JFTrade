import { describe, expect, it } from "vitest";

import { buildADKToolVisualization } from "../src/composables/adkToolVisualizations";

describe("buildADKToolVisualization", () => {
  it("builds portfolio summary cards from known fields", () => {
    const visualization = buildADKToolVisualization("portfolio.summary", {
      brokerStatus: "connected",
      accountCount: 2,
      orderCount: 3,
      checkedAt: "2026-06-08T10:00:00Z",
    });

    expect(visualization?.kind).toBe("summary");
    if (visualization?.kind !== "summary") return;
    expect(visualization.title).toBe("组合摘要");
    expect(visualization.cards.map((card) => card.label)).toEqual(["经纪通道", "账户数", "订单数"]);
    expect(visualization.rows?.[0]).toEqual({ label: "检查时间", value: "2026-06-08T10:00:00Z" });
  });

  it("builds broker order tables and keeps only available preferred columns", () => {
    const visualization = buildADKToolVisualization("broker.orders", {
      orders: [
        {
          symbol: "HK.00700",
          side: "BUY",
          status: "SUBMITTED",
          quantity: 100,
          price: 388.12345,
          ignored: "not shown",
        },
      ],
    });

    expect(visualization?.kind).toBe("table");
    if (visualization?.kind !== "table") return;
    expect(visualization.title).toBe("经纪商订单");
    expect(visualization.columns.map((column) => column.key)).toEqual(["symbol", "side", "status", "quantity", "price"]);
    expect(visualization.rows[0]).toMatchObject({
      symbol: "HK.00700",
      side: "买入",
      status: "已提交",
      quantity: "100",
      price: "388.1235",
    });
  });

  it("builds market depth rows with relative bar percentages", () => {
    const visualization = buildADKToolVisualization("market.depth", {
      symbol: "HK.00700",
      bids: [[388, 100], [387.8, 50]],
      asks: [{ price: 388.2, quantity: 200 }],
    });

    expect(visualization?.kind).toBe("depth");
    if (visualization?.kind !== "depth") return;
    expect(visualization.title).toBe("盘口深度");
    expect(visualization.bids[0]).toMatchObject({ price: "388", quantity: "100", percent: 50 });
    expect(visualization.asks[0]).toMatchObject({ price: "388.2", quantity: "200", percent: 100 });
  });

  it("builds risk state summary with kill switch tone", () => {
    const visualization = buildADKToolVisualization("risk.state", {
      killSwitch: true,
      realTradingEnabled: false,
      riskConfigSource: "workspace",
    });

    expect(visualization?.kind).toBe("summary");
    if (visualization?.kind !== "summary") return;
    expect(visualization.title).toBe("风险状态");
    expect(visualization.cards.find((card) => card.label === "熔断开关")).toMatchObject({
      value: "是",
      tone: "danger",
    });
  });

  it("builds execution timelines from event arrays", () => {
    const visualization = buildADKToolVisualization("execution.order_events", {
      events: [
        { type: "submitted", createdAt: "2026-06-08T10:00:00Z", symbol: "HK.00700" },
        { type: "failed", reason: "rejected" },
      ],
    });

    expect(visualization?.kind).toBe("timeline");
    if (visualization?.kind !== "timeline") return;
    expect(visualization.events).toHaveLength(2);
    expect(visualization.events[1]).toMatchObject({ label: "失败", detail: "已拒绝", tone: "danger" });
  });

  it("builds strategy Pine workflow summaries for new strategy tools", () => {
    const spec = buildADKToolVisualization("strategy.pine_spec", {
      version: "v6",
      sourceFormat: "pine-v6",
      runtime: "pine-go-plan",
      selectedSection: "support-matrix",
      sections: [{ id: "support-matrix", title: "支持矩阵" }],
      supportedHooks: ["on_kline_close"],
      supportMatrix: [{ capability: "Source-aware indicators", parser: true }],
      unsupportedPatterns: ["array.* 暂不支持"],
      goldenScripts: [{ id: "golden-ma-cross", title: "均线交叉" }],
      examples: [],
    });
    expect(spec?.kind).toBe("summary");
    if (spec?.kind !== "summary") return;
    expect(spec.title).toBe("JFTrade Pine Script v6 规范");
    expect(spec.subtitle).toBe("章节：支持矩阵");
    expect(spec.rows?.find((row) => row.label === "不支持写法数")?.value).toBe("1");

    const validation = buildADKToolVisualization("strategy.validate_pine", {
      ok: true,
      runtime: "pine-go-plan",
      hooks: ["on_kline_close"],
      metadata: {
        name: "Mean Revert",
        version: "0.1.0",
        symbol: "US.TME",
        interval: "5m",
      },
      requirements: {
        indicators: [{ alias: "rsi14", kind: "rsi" }],
      },
    });
    expect(validation?.kind).toBe("summary");
    if (validation?.kind !== "summary") return;
    expect(validation.title).toBe("Pine 校验");
    expect(validation.cards.find((card) => card.label === "校验结果")).toMatchObject({
      value: "有效",
      tone: "ok",
    });

    const save = buildADKToolVisualization("strategy.save_definition", {
      operation: "updated",
      definition: {
        id: "def-1",
        name: "Mean Revert",
        version: "0.1.2",
        symbol: "US.TME",
        interval: "15m",
        runtime: "pine-go-plan",
        sourceFormat: "pine-v6",
        updatedAt: "2026-06-10T10:00:00Z",
      },
    });
    expect(save?.kind).toBe("summary");
    if (save?.kind !== "summary") return;
    expect(save.title).toBe("策略定义已保存");
    expect(save.cards.find((card) => card.label === "操作")?.value).toBe("已更新");

    const updateMode = buildADKToolVisualization("strategy.update_instance_mode", {
      updatedFields: ["executionMode"],
      instance: {
        id: "inst-1",
        runtime: "pine-go-plan",
        status: "STOPPED",
        definition: { name: "Mean Revert" },
        binding: {
          executionMode: "notify_only",
          interval: "15m",
          symbols: ["US.TME"],
        },
      },
    });
    expect(updateMode?.kind).toBe("summary");
    if (updateMode?.kind !== "summary") return;
    expect(updateMode.title).toBe("策略实例模式已更新");
    expect(updateMode.rows?.find((row) => row.label === "已修改字段")?.value).toBe("执行模式");
  });

  it("returns null for unknown tools and malformed known outputs", () => {
    expect(buildADKToolVisualization("unknown.tool", { ok: true })).toBeNull();
    expect(buildADKToolVisualization("broker.orders", { orders: "not an array" })).toBeNull();
    expect(buildADKToolVisualization("portfolio.summary", {})).toBeNull();
    expect(buildADKToolVisualization("market.depth", { bids: [{ price: 1 }], asks: [] })).toBeNull();
  });
});
