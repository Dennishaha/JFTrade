import { describe, expect, it } from "vitest";

import { buildADKToolVisualization } from "@/composables/adk/adkToolVisualizations";

describe("ADK new capability visualizations", () => {
  it("renders provider and runtime dependency diagnostics", () => {
    const providers = buildADKToolVisualization("market.providers", {
      liveProvider: "futu",
      backtestProvider: "yfinance",
      providers: [
        {
          selectionId: "futu",
          displayName: "Futu OpenD",
          supportedMarkets: ["HK", "US"],
          capabilities: { streamingQuotes: true, streamingCandles: true, historicalCandles: true },
          constraints: { requiresOpenD: true, usesSubscriptionQuota: true },
        },
        {
          providerId: "yfinance",
          source: "Yahoo Finance",
          defaultMarket: "US",
          capabilities: {},
        },
        null,
      ],
      liveHealth: { connected: true, readiness: "ready" },
    });
    expect(providers?.kind).toBe("table");
    if (providers?.kind !== "table") return;
    expect(providers.title).toBe("行情提供者");
    expect(providers.subtitle).toBe("实时 futu · 回测 yfinance");
    expect(providers.rows).toHaveLength(2);
    expect(providers.rows[0]).toMatchObject({ providerId: "futu", source: "Futu OpenD", quotes: "是", candles: "是", health: "已连接 · ready" });
    expect(providers.rows[1]).toMatchObject({ providerId: "yfinance", markets: "US", history: "-" });

    const providerSummary = buildADKToolVisualization("market.providers", {
      liveProvider: "futu",
      backtestProvider: "akshare",
      liveHealth: { status: "unknown" },
      checkedAt: "2026-08-14T10:00:00Z",
    });
    expect(providerSummary?.kind).toBe("summary");

    const dependencies = buildADKToolVisualization("system.runtime_dependencies", {
      dependencies: [
        { id: "node", displayName: "Node.js", status: "ok", detectedVersion: "22.18.0", message: "available" },
      ],
      openD: {
        status: "online",
        runtime: { connectivity: "connected", serverVersion: "9.0" },
        diagnosis: { summary: "OpenD ready" },
      },
      allRequiredSatisfied: false,
    });
    expect(dependencies?.kind).toBe("table");
    if (dependencies?.kind !== "table") return;
    expect(dependencies.rows[0]).toMatchObject({ id: "node", name: "Node.js", version: "22.18.0" });
    expect(dependencies.rows[1]).toMatchObject({ id: "opend", name: "Futu OpenD", status: "online", version: "9.0" });

    const dependencySummary = buildADKToolVisualization("system.runtime_dependencies", {
      items: [null],
      allRequiredSatisfied: false,
      checkedAt: "2026-08-14T10:00:00Z",
    });
    expect(dependencySummary?.kind).toBe("summary");
    expect(buildADKToolVisualization("system.runtime_dependencies", { allRequiredSatisfied: true })?.kind).toBe("summary");
  });

  it("renders degraded provider and OpenD diagnostic fallbacks", () => {
    const providers = buildADKToolVisualization("market.providers", {
      liveProvider: "futu",
      backtestProvider: "akshare",
      providers: [
        { selectionId: "futu", displayName: "Futu OpenD", capabilities: "unavailable", constraints: "none" },
        { selectionId: "akshare" },
      ],
      liveHealth: { connected: false, lastError: "OpenD offline" },
    });
    expect(providers?.kind).toBe("table");
    if (providers?.kind !== "table") return;
    expect(providers.rows[0]).toMatchObject({ quotes: "-", candles: "-", history: "-", health: "未连接 · OpenD offline", constraints: "无额外约束" });

    const connectivityFallback = buildADKToolVisualization("system.runtime_dependencies", {
      openD: { runtime: { connectivity: "disconnected", lastError: "quote connection lost" } },
      allRequiredSatisfied: true,
    });
    expect(connectivityFallback?.kind).toBe("table");
    if (connectivityFallback?.kind === "table") {
      expect(connectivityFallback.subtitle).toBe("必需依赖 已满足");
      expect(connectivityFallback.rows[0]).toMatchObject({ status: "未连接", message: "quote connection lost" });
    }

    const errorFallback = buildADKToolVisualization("system.runtime_dependencies", {
      dependencies: [{ id: "opend", name: "Existing OpenD", status: "ok" }],
      openD: { error: "health probe failed" },
    });
    expect(errorFallback?.kind).toBe("table");
    if (errorFallback?.kind === "table") {
      expect(errorFallback.rows).toHaveLength(1);
      expect(errorFallback.rows[0]).toMatchObject({ id: "opend", name: "Existing OpenD" });
    }
  });

  it("renders the typed research screen catalog and safe fallbacks", () => {
    const catalog = buildADKToolVisualization("research.screen_catalog", {
      version: "2026-08",
      factors: [
        {
          key: "pe",
          label: "市盈率",
          category: "valuation",
          valueType: "number",
          operators: ["lt", "between"],
          availability: "US",
        },
      ],
    });
    expect(catalog?.kind).toBe("table");
    if (catalog?.kind !== "table") return;
    expect(catalog.title).toBe("筛选因子目录");
    expect(catalog.rows[0]).toMatchObject({ key: "pe", label: "市盈率", operators: "2" });

    const fallback = buildADKToolVisualization("research.screen_catalog", {
      schemaVersion: "screen-v2",
      querySchemaVersion: "2",
      provider: "platform",
      rateLimit: { remaining: 9 },
      factors: [],
    });
    expect(fallback?.kind).toBe("summary");
    expect(buildADKToolVisualization("research.screen_catalog", {})).toBeNull();

    const screen = buildADKToolVisualization("research.screen", {
      provider: { providerId: "futu-opend" },
      asOf: "2026-08-14T10:00:00Z",
      columns: [{ columnId: "pe", factorKey: "valuation.pe", label: "市盈率" }],
      entries: [{
        instrumentId: "US.AAPL", name: "Apple", market: "US",
        cells: { pe: { value: { type: "number", number: 31.25, unit: "x" } } },
      }],
      hasMore: true,
      warnings: ["partial data"],
    });
    expect(screen?.kind).toBe("table");
    if (screen?.kind !== "table") return;
    expect(screen.columns).toContainEqual({ key: "cell:pe", label: "市盈率" });
    expect(screen.rows[0]).toMatchObject({ instrumentId: "US.AAPL", "cell:pe": "31.25 x" });
    expect(screen.subtitle).toContain("还有下一页");

    expect(buildADKToolVisualization("research.screen", { catalogVersion: "2026-08", entries: [] })?.kind).toBe("summary");
    const sparseScreen = buildADKToolVisualization("research.screen", {
      provider: "fixture",
      columns: [
        { label: "ignored" },
        { columnId: "factor", factorKey: "quality.roe" },
        { columnId: "raw" },
      ],
      entries: [{
        instrumentId: "US.MSFT",
        cells: {
          factor: {},
          raw: { value: { type: "string", string: "available" } },
        },
      }],
      hasMore: false,
      warnings: [],
    });
    expect(sparseScreen?.kind).toBe("table");
    if (sparseScreen?.kind === "table") {
      expect(sparseScreen.columns).toEqual(expect.arrayContaining([
        { key: "cell:factor", label: "quality.roe" },
        { key: "cell:raw", label: "raw" },
      ]));
      expect(sparseScreen.rows[0]).toMatchObject({ "cell:factor": "-", "cell:raw": "available" });
    }
  });

  it("renders backtest provider metadata and cancellation outcomes", () => {
    const runs = buildADKToolVisualization("backtest.runs", {
      runs: [{
        id: "bt-1",
        status: "COMPLETED",
        symbol: "US.AAPL",
        marketDataProvider: "akshare",
        chartType: "heikinashi",
        executionModel: "conservative-bar-v1",
        totalReturn: 0.08,
      }],
    });
    expect(runs?.kind).toBe("table");
    if (runs?.kind !== "table") return;
    expect(runs.columns.map((column) => column.key)).toContain("marketDataProvider");
    expect(runs.rows[0]).toMatchObject({ marketDataProvider: "akshare", chartType: "heikinashi" });

    const research = buildADKToolVisualization("strategy.research_backtest", {
      status: "completed",
      marketDataProvider: "yfinance",
      chartType: "standard",
      instrumentType: "etf",
      useExtendedHours: true,
      executionModel: "conservative-bar-v1",
      tradingCosts: { commissionRate: 0.001 },
      validation: { metadata: { name: "ETF strategy" }, hooks: [] },
    });
    expect(research?.kind).toBe("summary");
    if (research?.kind !== "summary") return;
    expect(research.rows?.map((item) => item.label)).toEqual(expect.arrayContaining([
      "行情提供者", "图表类型", "标的类型", "扩展时段", "执行模型", "交易费用",
    ]));

    const result = buildADKToolVisualization("backtest.result_view", {
      view: "summary",
      run: {
        id: "bt-1",
        status: "COMPLETED",
        symbol: "US.AAPL",
        marketDataProvider: "yfinance",
        chartType: "standard",
        instrumentType: "etf",
        useExtendedHours: true,
        rehabType: "forward",
        executionModel: "conservative-bar-v1",
        tradingCosts: { commissionRate: 0.001 },
      },
      summary: { totalReturn: 0.1, warningCount: 1, latestWarning: "fallback tick size" },
    });
    expect(result?.kind).toBe("summary");
    if (result?.kind !== "summary") return;
    expect(result.rows?.map((item) => item.label)).toEqual(expect.arrayContaining([
      "行情提供者", "图表类型", "标的类型", "扩展时段", "复权", "执行模型", "费用", "数据警告数", "最新警告",
    ]));

    const warningView = buildADKToolVisualization("backtest.result_view", {
      view: "warnings",
      run: { id: "bt-1", symbol: "US.AAPL" },
      series: { warnings: ["adjustment unavailable"] },
    });
    expect(warningView?.kind).toBe("table");
    if (warningView?.kind === "table") expect(warningView.title).toBe("回测数据警告");

    const chartView = buildADKToolVisualization("backtest.result_view", {
      view: "chart",
      run: { id: "bt-chart", symbol: "US.MSFT" },
      series: { candles: [], trades: [{ time: "2026-08-14T10:00:00Z", side: "buy", price: 400 }] },
    });
    expect(chartView?.kind).toBe("table");
    const ordersView = buildADKToolVisualization("backtest.result_view", {
      view: "orders",
      series: { orderBook: [{ orderId: "order-1", side: "buy", status: "filled" }] },
    });
    expect(ordersView?.kind).toBe("table");
    const errorsView = buildADKToolVisualization("backtest.result_view", {
      view: "errors",
      series: { runtimeErrors: ["worker failed"] },
    });
    expect(errorsView?.kind).toBe("table");
    expect(buildADKToolVisualization("backtest.result_view", {})).toBeNull();

    const optimize = buildADKToolVisualization("strategy.optimize", {
      runs: [{ definitionId: "def-1", runId: "bt-2", status: "queued", marketDataProvider: "akshare", chartType: "heikinashi", instrumentType: "stock", useExtendedHours: false, executionModel: "conservative-bar-v1" }],
    });
    expect(optimize?.kind).toBe("table");
    if (optimize?.kind === "table") expect(optimize.columns.map((column) => column.key)).toEqual(expect.arrayContaining(["marketDataProvider", "chartType", "instrumentType", "useExtendedHours", "executionModel"]));

    const cancellation = buildADKToolVisualization("backtest.cancel", { runId: "bt-1", cancelled: false, cancelRequested: true });
    expect(cancellation?.kind).toBe("summary");
    expect(buildADKToolVisualization("backtest.cancel", { runId: "bt-1", cancelled: true })?.kind).toBe("summary");
  });

  it("renders every strategy instance lifecycle result and activity timeline", () => {
    const output = {
      instance: {
        id: "instance-1",
        status: "RUNNING",
        definition: { name: "Trend" },
        definitionSync: "current",
        binding: {
          executionMode: "live",
          interval: "5m",
          chartType: "heikinashi",
          brokerAccount: { accountId: "ACC-1" },
          runtimeRisk: { maxPositionValue: 10000 },
        },
        runtimeObservation: { activeSymbols: ["US.AAPL"], lastError: "none" },
      },
    };
    for (const toolName of [
      "strategy.instantiate",
      "strategy.instance_start",
      "strategy.instance_stop",
      "strategy.instance_refresh_definition",
      "strategy.instance_risk.update",
    ]) {
      const visualization = buildADKToolVisualization(toolName, output);
      expect(visualization?.kind).toBe("summary");
    }
    const activity = buildADKToolVisualization("strategy.instance_activity", {
      entries: [
        { type: "started", at: "2026-08-14T10:00:00Z", message: "started", status: "success" },
        { action: "pause", timestamp: "2026-08-14T10:01:00Z", reason: "manual" },
      ],
    });
    expect(activity?.kind).toBe("timeline");
    if (activity?.kind !== "timeline") return;
    expect(activity.events[0]).toMatchObject({ label: "started", detail: "started", tone: "ok" });
    expect(activity.events[1]).toMatchObject({ label: "pause", detail: "manual" });

    const logs = buildADKToolVisualization("strategy.instance_activity", { logs: ["worker started", "signal emitted"] });
    expect(logs?.kind).toBe("table");
    if (logs?.kind === "table") expect(logs.rows[0]).toMatchObject({ message: "worker started" });

    expect(buildADKToolVisualization("strategy.instance_activity", { logs: [null] })).toBeNull();
    expect(buildADKToolVisualization("strategy.instance_start", {
      status: "STOPPED",
      binding: { executionMode: "notify_only" },
    })?.kind).toBe("summary");
    expect(buildADKToolVisualization("strategy.instance_start", {}) ).toBeNull();
  });
});
