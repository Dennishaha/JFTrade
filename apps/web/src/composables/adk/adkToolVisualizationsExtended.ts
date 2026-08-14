import { isRecord, type UnknownRecord } from "./adkToolVisualizationHelpers";
import {
  buildRecordTable,
  buildStringTable,
  buildToolSummary,
  buildToolTable,
  findArray,
  formatValue,
  optionalValue,
  pick,
  row,
  summaryCard,
  toneForKillSwitch,
  toneForValue,
  type ADKSummaryVisualization,
  type ADKToolVisualization,
} from "./adkToolVisualizations";

export function buildMarketProviders(output: UnknownRecord): ADKToolVisualization | null {
  const providers = findArray(output, ["providers"]);
  const liveHealth = isRecord(output.liveHealth) ? output.liveHealth : {};
  const liveProvider = optionalValue(output.liveProvider);
  const rows = providers.filter(isRecord).slice(0, 20).map((provider) => ({
    providerId: formatValue(pick(provider, ["selectionId", "providerId"])),
    source: formatValue(pick(provider, ["displayName", "source"])),
    markets: formatValue(pick(provider, ["supportedMarkets", "defaultMarket"])),
    quotes: formatValue(isRecord(provider.capabilities) ? provider.capabilities.streamingQuotes : undefined),
    candles: formatValue(isRecord(provider.capabilities) ? provider.capabilities.streamingCandles : undefined),
    history: formatValue(isRecord(provider.capabilities) ? provider.capabilities.historicalCandles : undefined),
    constraints: providerConstraintLabel(provider),
    health: providerHealthLabel(provider, liveProvider, liveHealth),
  }));
  if (rows.length === 0) return buildToolSummary("行情提供者", output, ["liveProvider", "backtestProvider", "liveHealth", "checkedAt"]);
  return {
    kind: "table", title: "行情提供者", subtitle: `实时 ${formatValue(output.liveProvider)} · 回测 ${formatValue(output.backtestProvider)}`,
    columns: [
      { key: "providerId", label: "提供者" }, { key: "source", label: "来源" }, { key: "markets", label: "市场" },
      { key: "quotes", label: "实时报价" }, { key: "candles", label: "实时 K 线" }, { key: "history", label: "历史 K 线" },
      { key: "constraints", label: "约束" }, { key: "health", label: "运行健康" },
    ], rows,
  };
}

export function buildRuntimeDependencies(output: UnknownRecord): ADKToolVisualization | null {
  const dependencies = findArray(output, ["dependencies", "items"]).filter(isRecord).map((dependency) => ({
    id: formatValue(dependency.id),
    name: formatValue(pick(dependency, ["displayName", "name"])),
    status: formatValue(dependency.status),
    version: formatValue(pick(dependency, ["detectedVersion", "version"])),
    message: formatValue(pick(dependency, ["message", "error"])),
  }));
  const openD = isRecord(output.openD) ? output.openD : null;
  if (openD && !dependencies.some((dependency) => dependency.id.toLowerCase() === "opend")) {
    const runtime = isRecord(openD.runtime) ? openD.runtime : {};
    const diagnosis = isRecord(openD.diagnosis) ? openD.diagnosis : {};
    dependencies.push({
      id: "opend",
      name: "Futu OpenD",
      status: formatValue(pick(openD, ["status"]) ?? runtime.connectivity),
      version: formatValue(runtime.serverVersion),
      message: formatValue(pick(diagnosis, ["summary"]) ?? pick(runtime, ["lastError"]) ?? openD.error),
    });
  }
  if (dependencies.length === 0) return buildToolSummary("运行依赖诊断", output, ["allRequiredSatisfied", "checkedAt", "openD"]);
  return {
    kind: "table", title: "运行依赖诊断",
    subtitle: `必需依赖 ${output.allRequiredSatisfied === true ? "已满足" : "未完全满足"}`,
    columns: [{ key: "id", label: "依赖" }, { key: "name", label: "名称" }, { key: "status", label: "状态" }, { key: "version", label: "版本" }, { key: "message", label: "诊断" }],
    rows: dependencies,
  };
}

export function buildScreenCatalog(output: UnknownRecord): ADKToolVisualization | null {
  return buildToolTable("筛选因子目录", output, ["factors"], [["key", "因子"], ["label", "名称"], ["category", "分类"], ["valueType", "类型"], ["operators", "运算符"], ["availability", "可用性"]]) ?? buildToolSummary("筛选因子目录", output, ["version", "schemaVersion", "querySchemaVersion", "provider", "rateLimit", "factors"]);
}

export function buildResearchScreen(output: UnknownRecord): ADKToolVisualization | null {
  const entries = findArray(output, ["entries", "results", "items", "data"]).filter(isRecord).slice(0, 20);
  if (entries.length === 0) {
    return buildToolSummary("研究筛选", output, ["provider", "asOf", "catalogVersion", "hasMore", "total", "warnings", "partialErrors"]);
  }
  const availableFixedColumns: Array<[string, string]> = [
    ["instrumentId", "标的"], ["name", "名称"], ["market", "市场"], ["symbol", "代码"], ["industry", "行业"],
  ];
  const fixedColumns = availableFixedColumns.filter(([key]) => entries.some((entry) => entry[key] !== undefined));
  const dynamicColumns = findArray(output, ["columns"]).filter(isRecord).slice(0, 8).flatMap((column) => {
    const columnId = optionalValue(column.columnId);
    if (!columnId) return [];
    return [{ key: `cell:${columnId}`, label: optionalValue(column.label) ?? optionalValue(column.factorKey) ?? columnId, columnId }];
  });
  const columns = [
    ...fixedColumns.map(([key, label]) => ({ key, label })),
    ...dynamicColumns.map(({ key, label }) => ({ key, label })),
  ];
  const rows = entries.map((entry) => {
    const result: Record<string, string> = Object.fromEntries(fixedColumns.map(([key]) => [key, formatValue(entry[key])]));
    for (const column of dynamicColumns) result[column.key] = researchScreenCellValue(entry, column.columnId);
    return result;
  });
  const provider = isRecord(output.provider) ? pick(output.provider, ["source", "providerId", "brokerId"]) : output.provider;
  const warningCount = findArray(output, ["warnings"]).length;
  return {
    kind: "table", title: "研究筛选",
    subtitle: [optionalValue(provider), optionalValue(output.asOf), `${rows.length} 行`, output.hasMore === true ? "还有下一页" : undefined, warningCount > 0 ? `${warningCount} 条警告` : undefined].filter(Boolean).join(" · "),
    columns, rows,
  };
}

export function buildStrategyInstance(output: UnknownRecord): ADKToolVisualization | null {
  const instance = isRecord(output.instance) ? output.instance : output;
  const binding = isRecord(instance.binding) ? instance.binding : {};
  const observation = isRecord(instance.runtimeObservation) ? instance.runtimeObservation : {};
  const risk = isRecord(binding.runtimeRisk) ? binding.runtimeRisk : undefined;
  const cards = [
    summaryCard("状态", pick(instance, ["status", "actualStatus"]), toneForValue(pick(instance, ["status", "actualStatus"]))),
    summaryCard("模式", binding.executionMode), summaryCard("周期", binding.interval), summaryCard("图表", binding.chartType),
    summaryCard("风控", risk), summaryCard("最新错误", pick(observation, ["lastError"]), "danger"),
  ].filter((card): card is NonNullable<typeof card> => card !== null);
  const rows = [row("实例 ID", instance.id), row("定义", isRecord(instance.definition) ? instance.definition.name : undefined), row("账户", isRecord(binding.brokerAccount) ? binding.brokerAccount.accountId : undefined), row("实时标的", observation.activeSymbols), row("定义同步", instance.definitionSync)].filter((item): item is { label: string; value: string } => item !== null);
  if (cards.length === 0 && rows.length === 0) return null;
  return { kind: "summary", title: "策略实例", cards, rows };
}

export function buildRiskState(output: UnknownRecord): ADKToolVisualization | null {
  const killSwitch = pick(output, ["killSwitch", "kill_switch"]);
  const riskLimits = pick(output, ["riskLimits", "limits"]);
  const cards = [
    summaryCard("熔断开关", killSwitch, toneForKillSwitch(killSwitch)),
    summaryCard("风险限制", riskLimits),
    summaryCard("实盘交易", pick(output, ["realTradingEnabled", "realTrading", "enabled"]), toneForValue(pick(output, ["realTradingEnabled", "realTrading", "enabled"]))),
  ].filter((card): card is NonNullable<typeof card> => card !== null);
  const rows = [
    row("检查时间", pick(output, ["checkedAt", "updatedAt", "at"])),
    row("来源", pick(output, ["riskConfigSource", "source"])),
  ].filter((item): item is { label: string; value: string } => item !== null);

  if (cards.length === 0 && rows.length === 0) return null;
  return { kind: "summary", title: "风险状态", cards, rows };
}

export function buildBacktestResultView(output: UnknownRecord): ADKToolVisualization | null {
  const view = optionalValue(output.view) ?? "summary";
  const series = isRecord(output.series) ? output.series : {};
  if (view === "chart") {
    const candles = findArray(series, ["candles"]);
    if (candles.length > 0) {
      return buildRecordTable("回测蜡烛窗口", output, candles, [
        ["time", "时间"], ["open", "开"], ["high", "高"], ["low", "低"], ["close", "收"], ["volume", "量"],
      ]);
    }
    const trades = findArray(series, ["trades"]);
    if (trades.length > 0) {
      return buildRecordTable("回测交易窗口", output, trades, [
        ["time", "时间"], ["side", "方向"], ["price", "价格"], ["qty", "数量"], ["positionQty", "持仓"],
      ]);
    }
  }
  if (view === "orders") {
    const orders = findArray(series, ["orderBook"]);
    if (orders.length > 0) {
      return buildRecordTable("回测订单窗口", output, orders, [
        ["orderId", "订单"], ["symbol", "标的"], ["side", "方向"], ["status", "状态"], ["quantity", "数量"], ["price", "价格"], ["submittedAt", "提交时间"], ["filledAt", "成交时间"],
      ]);
    }
  }
  if (view === "logs" || view === "warnings" || view === "errors") {
    const keys = view === "logs" ? ["logs"] : view === "warnings" ? ["warnings"] : ["runtimeErrors"];
    const items = findArray(series, keys);
    const title = view === "logs" ? "回测日志窗口" : view === "warnings" ? "回测数据警告" : "回测错误窗口";
    if (items.length > 0) return buildStringTable(title, output, items);
  }
  return buildBacktestResultSummary(output);
}

function buildBacktestResultSummary(output: UnknownRecord): ADKToolVisualization | null {
  const run = isRecord(output.run) ? output.run : {};
  const summary = isRecord(output.summary) ? output.summary : {};
  const cards = [
    summaryCard("状态", run.status, toneForValue(run.status)), summaryCard("最终资产", summary.finalBalance), summaryCard("盈亏", summary.pnl),
    summaryCard("收益", summary.totalReturn), summaryCard("最大回撤", summary.maxDrawdown), summaryCard("成交数", summary.totalTrades),
  ].filter((card): card is NonNullable<typeof card> => card !== null);
  const rows = [
    row("运行 ID", run.id), row("标的", run.symbol), row("周期", run.interval), row("行情提供者", run.marketDataProvider),
    row("图表类型", run.chartType), row("标的类型", run.instrumentType), row("扩展时段", run.useExtendedHours), row("复权", run.rehabType),
    row("执行模型", run.executionModel), row("费用", run.tradingCosts), row("开始", run.startTime), row("结束", run.endTime),
    row("数据警告数", summary.warningCount), row("最新警告", summary.latestWarning), row("错误", summary.error), row("最新日志", summary.latestLog),
  ].filter((item): item is { label: string; value: string } => item !== null);
  if (cards.length === 0 && rows.length === 0) return null;
  const visualization: ADKSummaryVisualization = { kind: "summary", title: "回测结果视图", cards, rows };
  const subtitle = optionalValue(output.view);
  if (subtitle) visualization.subtitle = subtitle;
  return visualization;
}

function providerConstraintLabel(provider: UnknownRecord): string {
  const constraints = isRecord(provider.constraints) ? provider.constraints : {};
  const labels = [
    constraints.requiresOpenD === true ? "需 OpenD" : undefined,
    constraints.requiresMarketDataRight === true ? "需行情权限" : undefined,
    constraints.usesSubscriptionQuota === true ? "占订阅额度" : undefined,
  ].filter((label): label is string => label !== undefined);
  return labels.length > 0 ? labels.join("、") : "无额外约束";
}

function providerHealthLabel(provider: UnknownRecord, liveProvider: string | undefined, health: UnknownRecord): string {
  const providerId = optionalValue(pick(provider, ["selectionId", "providerId"]));
  if (!providerId || providerId !== liveProvider) return "未探测";
  const connected = health.connected === true ? "已连接" : health.connected === false ? "未连接" : undefined;
  const readiness = optionalValue(health.readiness);
  const error = optionalValue(pick(health, ["lastError", "error"]));
  return [connected, readiness, error].filter(Boolean).join(" · ") || optionalValue(health.status) || "未知";
}

function researchScreenCellValue(entry: UnknownRecord, columnId: string): string {
  const cells = isRecord(entry.cells) ? entry.cells : {};
  const cell = isRecord(cells[columnId]) ? cells[columnId] : null;
  const value = cell && isRecord(cell.value) ? cell.value : null;
  if (!value) return "-";
  const typedValue = pick(value, ["string", "integer", "number", "integers", "enumName"]);
  const rendered = formatValue(typedValue);
  const unit = optionalValue(value.unit);
  return unit && rendered !== "-" ? `${rendered} ${unit}` : rendered;
}
