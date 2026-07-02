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

  it("maps string kill-switch states through dedicated and fallback tones", () => {
    const engaged = buildADKToolVisualization("risk.state", {
      killSwitch: "engaged",
      realTradingEnabled: "off",
    });
    expect(engaged?.kind).toBe("summary");
    if (engaged?.kind !== "summary") return;
    expect(engaged.cards.find((card) => card.label === "熔断开关")).toMatchObject({
      value: "engaged",
      tone: "danger",
    });

    const off = buildADKToolVisualization("risk.state", {
      killSwitch: "off",
      realTradingEnabled: "disabled",
    });
    expect(off?.kind).toBe("summary");
    if (off?.kind !== "summary") return;
    expect(off.cards.find((card) => card.label === "熔断开关")).toMatchObject({
      value: "off",
      tone: "ok",
    });

    const blocked = buildADKToolVisualization("risk.state", {
      killSwitch: "blocked by supervisor",
      realTradingEnabled: "pending",
    });
    expect(blocked?.kind).toBe("summary");
    if (blocked?.kind !== "summary") return;
    expect(blocked.cards.find((card) => card.label === "熔断开关")).toMatchObject({
      tone: "warning",
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
      runtime: "pine-pinets",
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
      runtime: "pine-pinets",
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
        runtime: "pine-pinets",
        sourceFormat: "pine-v6",
        updatedAt: "2026-06-10T10:00:00Z",
      },
    });
    expect(save?.kind).toBe("summary");
    if (save?.kind !== "summary") return;
    expect(save.title).toBe("策略定义已保存");
    expect(save.cards.find((card) => card.label === "操作")?.value).toBe("已更新");

    const created = buildADKToolVisualization("strategy.save_definition", {
      operation: "created",
      definition: {
        id: "def-2",
        name: "Breakout",
        version: "0.0.1",
        symbol: "US.AAPL",
        interval: "5m",
        runtime: "pine-pinets",
        sourceFormat: "pine-v6",
        updatedAt: "2026-06-10T11:00:00Z",
      },
    });
    expect(created?.kind).toBe("summary");
    if (created?.kind !== "summary") return;
    expect(created.cards.find((card) => card.label === "操作")?.value).toBe("已创建");

    const passthroughOperation = buildADKToolVisualization("strategy.save_definition", {
      operation: "archived",
      definition: {
        id: "def-3",
        name: "Archive Me",
        version: "0.0.2",
        symbol: "US.MSFT",
        interval: "15m",
        runtime: "pine-pinets",
        sourceFormat: "pine-v6",
        updatedAt: "2026-06-10T12:00:00Z",
      },
    });
    expect(passthroughOperation?.kind).toBe("summary");
    if (passthroughOperation?.kind !== "summary") return;
    expect(
      passthroughOperation.cards.find((card) => card.label === "操作")?.value,
    ).toBe("archived");

    const updateMode = buildADKToolVisualization("strategy.update_instance_mode", {
      updatedFields: ["executionMode"],
      instance: {
        id: "inst-1",
        runtime: "pine-pinets",
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

  it("builds research backtest summaries and result view tables", () => {
    const research = buildADKToolVisualization("strategy.research_backtest", {
      ok: true,
      status: "queued",
      runId: "bt-1",
      scriptHash: "abc123",
      validation: {
        metadata: { name: "Research Draft", symbol: "US.TME", interval: "1m" },
        hooks: ["on_kline_close"],
        warnings: [],
      },
      resultView: {
        view: "summary",
        summary: { totalReturn: 0.12, totalTrades: 3 },
      },
      saveRecommendation: "仅当用户明确要求保存时再保存。",
    });
    expect(research?.kind).toBe("summary");
    if (research?.kind !== "summary") return;
    expect(research.title).toBe("策略研究回测");
    expect(research.subtitle).toBe("临时运行，不会保存策略定义");
    expect(research.cards.find((card) => card.label === "状态")).toMatchObject({
      value: "queued",
      tone: "muted",
    });

    const chart = buildADKToolVisualization("backtest.result_view", {
      view: "chart",
      run: { id: "bt-1", status: "completed", symbol: "US.TME", interval: "1m" },
      window: { resolution: "5m", nextCursor: "20" },
      series: {
        candles: [
          { time: "2025-01-01T00:00:00Z", open: "10", high: "12", low: "9", close: "11", volume: "300" },
        ],
      },
    });
    expect(chart?.kind).toBe("table");
    if (chart?.kind !== "table") return;
    expect(chart.title).toBe("回测蜡烛窗口");
    expect(chart.subtitle).toBe("US.TME · 5m · 1 行 · next 20");
    expect(chart.columns.map((column) => column.key)).toEqual(["time", "open", "high", "low", "close", "volume"]);
    expect(chart.rows[0]).toMatchObject({ open: "10", close: "11" });

    const summary = buildADKToolVisualization("backtest.result_view", {
      view: "summary",
      run: { id: "bt-1", status: "completed", symbol: "US.TME", interval: "1m" },
      summary: { finalBalance: 110000, pnl: 10000, maxDrawdown: 0.05, totalTrades: 2 },
    });
    expect(summary?.kind).toBe("summary");
    if (summary?.kind !== "summary") return;
    expect(summary.title).toBe("回测结果视图");
    expect(summary.cards.find((card) => card.label === "状态")).toMatchObject({
      value: "已完成",
      tone: "ok",
    });
  });

  it("renders fills, fees, cash flows, runs, and optimization candidates with domain columns", () => {
    const cases = [
      [
        "broker.fills",
        { fills: [{ symbol: "US.AAPL", side: "SELL", quantity: 3, price: 210.5, amount: 631.5 }] },
        "经纪商成交",
        ["symbol", "side", "quantity", "price", "amount"],
      ],
      [
        "broker.fees",
        { fees: [{ orderIdEx: "order-1", feeType: "commission", amount: 1.25, currency: "USD" }] },
        "订单费用",
        ["orderIdEx", "feeType", "amount", "currency"],
      ],
      [
        "broker.cash_flows",
        { flows: [{ clearingDate: "2026-06-08", direction: "current_day", amount: -200, currency: "HKD" }] },
        "资金流水",
        ["clearingDate", "direction", "amount", "currency"],
      ],
      [
        "backtest.runs",
        { runs: [{ id: "bt-1", status: "COMPLETED", symbol: "US.AAPL", totalReturn: 0.08 }] },
        "回测运行",
        ["id", "status", "symbol", "totalReturn"],
      ],
      [
        "strategy.optimize",
        { candidates: [{ definitionId: "def-1", runId: "bt-2", status: "FAILED", maxDrawdown: 0.2 }] },
        "优化候选",
        ["definitionId", "runId", "status", "maxDrawdown"],
      ],
    ] as const;

    for (const [toolName, output, title, columns] of cases) {
      const visualization = buildADKToolVisualization(toolName, output);
      expect(visualization?.kind).toBe("table");
      if (visualization?.kind !== "table") continue;
      expect(visualization.title).toBe(title);
      expect(visualization.columns.map((column) => column.key)).toEqual(columns);
    }
  });

  it("falls back to safe columns and caps oversized broker responses", () => {
    const orders = Array.from({ length: 23 }, (_, index) => ({
      custom_id: `custom-${index}`,
      routeName: "SMART",
      metadata: { state: "active" },
    }));

    const visualization = buildADKToolVisualization("broker.orders", {
      nested: { data: orders },
    });

    expect(visualization?.kind).toBe("table");
    if (visualization?.kind !== "table") return;
    expect(visualization.subtitle).toBe("20 / 23 行");
    expect(visualization.columns).toEqual([
      { key: "custom_id", label: "Custom Id" },
      { key: "routeName", label: "Route Name" },
      { key: "metadata", label: "Metadata" },
    ]);
    expect(visualization.rows[0]?.metadata).toBe("活跃");
  });

  it("renders each backtest window without losing pagination context", () => {
    const common = {
      run: { id: "bt-window", symbol: "US.TSLA" },
      window: { resolution: "1m", nextCursor: "cursor-2" },
    };
    const cases = [
      [
        { view: "chart", ...common, series: { trades: [{ time: "10:00", side: "BUY", price: 200, qty: 2 }] } },
        "回测交易窗口",
        "买入",
      ],
      [
        { view: "orders", ...common, series: { orderBook: [{ orderId: "o-1", side: "SELL", status: "REJECTED" }] } },
        "回测订单窗口",
        "卖出",
      ],
      [
        { view: "logs", ...common, series: { logs: ["strategy started", { status: "running" }] } },
        "回测日志窗口",
        "strategy started",
      ],
      [
        { view: "errors", ...common, series: { runtimeErrors: ["division by zero"] } },
        "回测错误窗口",
        "division by zero",
      ],
    ] as const;

    for (const [output, title, expectedValue] of cases) {
      const visualization = buildADKToolVisualization("backtest.result_view", output);
      expect(visualization?.kind).toBe("table");
      if (visualization?.kind !== "table") continue;
      expect(visualization.title).toBe(title);
      expect(visualization.subtitle).toContain("US.TSLA · 1m ·");
      expect(JSON.stringify(visualization.rows)).toContain(expectedValue);
    }
  });

  it("falls back to a summary when the requested backtest window is empty", () => {
    const visualization = buildADKToolVisualization("backtest.result_view", {
      view: "orders",
      run: {
        id: "bt-empty",
        status: "FAILED",
        startTime: "2026-01-01T00:00:00Z",
        endTime: "2026-01-01T01:00:00Z",
      },
      summary: { error: "data unavailable", latestLog: "market feed closed" },
      series: { orderBook: [] },
    });

    expect(visualization?.kind).toBe("summary");
    if (visualization?.kind !== "summary") return;
    expect(visualization.subtitle).toBe("orders");
    expect(visualization.cards[0]).toMatchObject({ value: "失败", tone: "danger" });
    expect(visualization.rows?.map((item) => item.label)).toEqual([
      "运行 ID",
      "开始",
      "结束",
      "错误",
      "最新日志",
    ]);
  });

  it("normalizes alternate depth shapes and protects bars from invalid quantities", () => {
    const visualization = buildADKToolVisualization("market.depth", {
      instrumentId: "US.NVDA",
      bidRows: [
        { p: 120.1, size: "1,000" },
        { price: 120, volume: "bad" },
        null,
      ],
      askRows: [[120.2, 10], [120.3]],
    });

    expect(visualization?.kind).toBe("depth");
    if (visualization?.kind !== "depth") return;
    expect(visualization.subtitle).toBe("US.NVDA");
    expect(visualization.bids).toEqual([
      { price: "120.1", quantity: "1,000", percent: 100 },
      { price: "120", quantity: "bad", percent: 0 },
    ]);
    expect(visualization.asks[0]?.percent).toBe(4);
  });

  it("maps risk event severity and limits long timelines", () => {
    const events = Array.from({ length: 22 }, (_, index) => ({
      event: index === 0 ? "limit warning" : `event-${index}`,
      timestamp: `2026-06-08T10:${String(index).padStart(2, "0")}:00Z`,
      description: index === 0 ? "position limited" : undefined,
      status: index === 0 ? "blocked" : "success",
    }));
    const visualization = buildADKToolVisualization("risk.events", {
      envelope: { riskEvents: events },
    });

    expect(visualization?.kind).toBe("timeline");
    if (visualization?.kind !== "timeline") return;
    expect(visualization.subtitle).toBe("20 / 22 条事件");
    expect(visualization.events[0]).toMatchObject({
      label: "limit warning",
      detail: "position limited",
      tone: "warning",
    });
  });

  it("shows invalid Pine validation details and untranslated extension fields", () => {
    const validation = buildADKToolVisualization("strategy.validate_pine", {
      ok: false,
      errors: ["strategy.entry is missing"],
      metadata: { name: "Broken" },
    });
    expect(validation?.kind).toBe("summary");
    if (validation?.kind !== "summary") return;
    expect(validation.subtitle).toBe("请先修正脚本后再保存");
    expect(validation.cards[0]).toMatchObject({ value: "无效", tone: "danger" });
    expect(validation.rows?.find((item) => item.label === "首个错误")?.value).toBe(
      "strategy.entry is missing",
    );

    const updated = buildADKToolVisualization("strategy.update_instance_mode", {
      updatedFields: ["executionMode", "riskProfile", 42],
      instance: { id: "instance-1", binding: { executionMode: "live", symbols: [] } },
    });
    expect(updated?.kind).toBe("summary");
    if (updated?.kind !== "summary") return;
    expect(updated.rows?.find((item) => item.label === "已修改字段")?.value).toBe(
      "执行模式、riskProfile、",
    );
  });

  it("translates Pine specification sections and save operations", () => {
    const sectionNames = [
      ["overview", "概览"],
      ["syntax", "语法"],
      ["expressions", "表达式"],
      ["indicators", "指标"],
      ["orders", "下单"],
      ["protect", "保护"],
      ["unsupported", "不支持项"],
      ["examples", "示例"],
      ["custom", "custom"],
    ];
    for (const [section, label] of sectionNames) {
      const visualization = buildADKToolVisualization("strategy.pine_spec", {
        selectedSection: section,
      });
      expect(visualization?.subtitle).toBe(`章节：${label}`);
    }

    const created = buildADKToolVisualization("strategy.save_definition", {
      operation: "created",
      name: "Momentum",
    });
    expect(created?.kind).toBe("summary");
    if (created?.kind !== "summary") return;
    expect(created.cards[0]?.value).toBe("已创建");

    const imported = buildADKToolVisualization("strategy.save_definition", {
      operation: "imported",
      definition: { name: "Imported" },
    });
    expect(imported?.subtitle).toBe("本次操作：imported");
  });

  it("returns null for unknown tools and malformed known outputs", () => {
    expect(buildADKToolVisualization("unknown.tool", { ok: true })).toBeNull();
    expect(buildADKToolVisualization("portfolio.summary", null)).toBeNull();
    expect(buildADKToolVisualization("broker.orders", { orders: "not an array" })).toBeNull();
    expect(buildADKToolVisualization("broker.orders", { orders: [null, 1] })).toBeNull();
    expect(buildADKToolVisualization("portfolio.summary", {})).toBeNull();
    expect(buildADKToolVisualization("strategy.update_instance_mode", {})).toBeNull();
    expect(buildADKToolVisualization("strategy.update_instance_mode", { instance: {} })).toBeNull();
    expect(buildADKToolVisualization("risk.state", {})).toBeNull();
    expect(buildADKToolVisualization("market.depth", { bids: [{ price: 1 }], asks: [] })).toBeNull();
    expect(buildADKToolVisualization("risk.events", { events: [null] })).toBeNull();
    expect(buildADKToolVisualization("backtest.result_view", {})).toBeNull();
  });

  it("falls back to raw backtest record columns and formats array or object values safely", () => {
    const chart = buildADKToolVisualization("backtest.result_view", {
      view: "chart",
      run: { id: "bt-raw" },
      series: {
        candles: [{ foo_bar: "x", routeName: "SMART" }],
      },
    });
    expect(chart?.kind).toBe("table");
    if (chart?.kind !== "table") return;
    expect(chart.columns).toEqual([
      { key: "foo_bar", label: "Foo Bar" },
      { key: "routeName", label: "Route Name" },
    ]);

    expect(
      buildADKToolVisualization("backtest.result_view", {
        view: "chart",
        run: { id: "bt-empty" },
        series: { candles: [null, 1] },
      }),
    ).toBeNull();

    const summary = buildADKToolVisualization("portfolio.summary", {
      accounts: ["a", "b"],
      environment: { region: "APAC" },
      status: { ok: true },
      checkedAt: 10n,
    });
    expect(summary?.kind).toBe("summary");
    if (summary?.kind !== "summary") return;
    expect(summary.cards.find((card) => card.label === "账户数")?.value).toBe("2");
    expect(summary.cards.find((card) => card.label === "经纪通道")?.value).toBe('{"ok":true}');
    expect(summary.rows?.find((row) => row.label === "交易环境")?.value).toBe('{"region":"APAC"}');
    expect(summary.rows?.find((row) => row.label === "检查时间")?.value).toBe("10");
  });
});
