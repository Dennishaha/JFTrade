export type ADKToolVisualization =
  | ADKSummaryVisualization
  | ADKTableVisualization
  | ADKDepthVisualization
  | ADKTimelineVisualization;

export interface ADKSummaryVisualization {
  kind: "summary";
  title: string;
  subtitle?: string;
  cards: Array<{ label: string; value: string; tone?: "ok" | "warning" | "danger" | "muted" }>;
  rows?: Array<{ label: string; value: string }>;
}

export interface ADKTableVisualization {
  kind: "table";
  title: string;
  subtitle?: string;
  columns: Array<{ key: string; label: string }>;
  rows: Array<Record<string, string>>;
}

export interface ADKDepthVisualization {
  kind: "depth";
  title: string;
  subtitle?: string;
  bids: ADKDepthRow[];
  asks: ADKDepthRow[];
}

export interface ADKDepthRow {
  price: string;
  quantity: string;
  percent: number;
}

export interface ADKTimelineVisualization {
  kind: "timeline";
  title: string;
  subtitle?: string;
  events: Array<{ label: string; time?: string; detail?: string; tone?: "ok" | "warning" | "danger" | "muted" }>;
}

type UnknownRecord = Record<string, unknown>;

export function buildADKToolVisualization(toolName: string, output: unknown): ADKToolVisualization | null {
  const normalizedToolName = toolName.trim();
  if (!isRecord(output)) return null;

  switch (normalizedToolName) {
    case "strategy.pine_spec":
      return buildStrategyPineSpec(output);
    case "strategy.validate_pine":
      return buildStrategyValidateDSL(output);
    case "strategy.research_backtest":
      return buildStrategyResearchBacktest(output);
    case "strategy.save_definition":
      return buildStrategySaveDefinition(output);
    case "strategy.update_instance_mode":
      return buildStrategyUpdateInstanceMode(output);
    case "portfolio.summary":
      return buildPortfolioSummary(output);
    case "broker.orders":
      return buildToolTable("经纪商订单", output, ["orders", "items", "data"], [
        ["symbol", "标的"],
        ["side", "方向"],
        ["status", "状态"],
        ["quantity", "数量"],
        ["price", "价格"],
        ["orderIdEx", "订单号"],
        ["createdAt", "创建时间"],
        ["updatedAt", "更新时间"],
      ]);
    case "broker.fills":
      return buildToolTable("经纪商成交", output, ["fills", "items", "data"], [
        ["symbol", "标的"],
        ["side", "方向"],
        ["quantity", "数量"],
        ["price", "价格"],
        ["amount", "金额"],
        ["fillId", "成交号"],
        ["createdAt", "创建时间"],
      ]);
    case "broker.fees":
      return buildToolTable("订单费用", output, ["fees", "items", "data"], [
        ["orderIdEx", "订单号"],
        ["feeType", "类型"],
        ["amount", "金额"],
        ["currency", "币种"],
        ["description", "说明"],
      ]);
    case "broker.cash_flows":
      return buildToolTable("资金流水", output, ["cashFlows", "flows", "items", "data"], [
        ["clearingDate", "日期"],
        ["direction", "方向"],
        ["amount", "金额"],
        ["currency", "币种"],
        ["description", "说明"],
      ]);
    case "market.depth":
      return buildDepth(output);
    case "risk.state":
      return buildRiskState(output);
    case "risk.events":
      return buildTimeline("风险事件", output, ["events", "riskEvents", "items", "data"]);
    case "execution.order_events":
      return buildTimeline("订单事件", output, ["events", "orderEvents", "items", "data", "orders"]);
    case "backtest.runs":
      return buildToolTable("回测运行", output, ["runs", "items", "data"], [
        ["id", "运行 ID"],
        ["status", "状态"],
        ["symbol", "标的"],
        ["interval", "周期"],
        ["totalReturn", "收益"],
        ["maxDrawdown", "回撤"],
        ["tradeCount", "成交笔数"],
        ["createdAt", "创建时间"],
      ]);
    case "backtest.result_view":
      return buildBacktestResultView(output);
    case "strategy.optimize":
      return buildToolTable("优化候选", output, ["runs", "candidates", "tasks", "items", "data"], [
        ["definitionId", "策略定义"],
        ["runId", "运行 ID"],
        ["status", "状态"],
        ["totalReturn", "收益"],
        ["maxDrawdown", "回撤"],
        ["tradeCount", "成交笔数"],
      ]);
    default:
      return null;
  }
}

function buildStrategyPineSpec(output: UnknownRecord): ADKToolVisualization | null {
  const sections = findArray(output, ["sections"]);
  const hooks = findArray(output, ["supportedHooks"]);
  const unsupportedPatterns = findArray(output, ["unsupportedPatterns"]);
  const examples = findArray(output, ["examples"]);
  const selectedSection = optionalValue(output.selectedSection);
  const cards = [
    summaryCard("版本", output.version),
    summaryCard("格式", output.sourceFormat),
    summaryCard("运行时", output.runtime),
    summaryCard("章节数", sections.length),
    summaryCard("Hook 数", hooks.length),
  ].filter((card): card is NonNullable<typeof card> => card !== null);
  const rows = [
    row("当前章节", selectedSection ? translateDSLSection(selectedSection) : undefined),
    row("返回示例数", examples.length),
    row("不支持写法数", unsupportedPatterns.length),
  ].filter((item): item is { label: string; value: string } => item !== null);
  if (cards.length === 0 && rows.length === 0) return null;
  return {
    kind: "summary",
    title: "JFTrade Pine Script v6 规范",
    subtitle: selectedSection ? `章节：${translateDSLSection(selectedSection)}` : "结构化 Pine 定义",
    cards,
    rows,
  };
}

function buildStrategyValidateDSL(output: UnknownRecord): ADKToolVisualization | null {
  const metadata = isRecord(output.metadata) ? output.metadata : null;
  const requirements = isRecord(output.requirements) ? output.requirements : null;
  const indicators = requirements ? findArray(requirements, ["indicators"]) : [];
  const hooks = findArray(output, ["hooks"]);
  const errors = findArray(output, ["errors"]);
  const ok = output.ok === true;
  const cards = [
    { label: "校验结果", value: ok ? "有效" : "无效", tone: ok ? "ok" : "danger" as const },
    summaryCard("运行时", output.runtime),
    summaryCard("Hook 数", hooks.length),
    summaryCard("指标数", indicators.length),
  ].filter((card): card is ADKSummaryVisualization["cards"][number] => card !== null);
  const rows = [
    row("策略名", metadata?.name),
    row("版本", metadata?.version),
    row("标的", metadata?.symbol),
    row("周期", metadata?.interval),
    row("首个错误", errors[0]),
  ].filter((item): item is { label: string; value: string } => item !== null);
  if (cards.length === 0 && rows.length === 0) return null;
  return {
    kind: "summary",
    title: "Pine 校验",
    subtitle: ok ? "可以继续保存策略定义" : "请先修正脚本后再保存",
    cards,
    rows,
  };
}

function buildStrategyResearchBacktest(output: UnknownRecord): ADKToolVisualization | null {
  const validation = isRecord(output.validation) ? output.validation : null;
  const metadata = validation && isRecord(validation.metadata) ? validation.metadata : null;
  const hooks = validation ? findArray(validation, ["hooks"]) : [];
  const resultView = isRecord(output.resultView) ? output.resultView : null;
  const resultSummary = resultView && isRecord(resultView.summary) ? resultView.summary : null;
  const cards = [
    summaryCard("状态", output.status, toneForValue(output.status)),
    summaryCard("运行 ID", output.runId),
    summaryCard("脚本 Hash", output.scriptHash),
    summaryCard("收益", resultSummary?.totalReturn),
    summaryCard("成交数", resultSummary?.totalTrades),
  ].filter((card): card is NonNullable<typeof card> => card !== null);
  const rows = [
    row("策略名", metadata?.name),
    row("标的", metadata?.symbol),
    row("周期", metadata?.interval),
    row("Hook 数", hooks.length),
    row("结果视图错误", output.resultViewError),
    row("保存建议", output.saveRecommendation),
  ].filter((item): item is { label: string; value: string } => item !== null);
  if (cards.length === 0 && rows.length === 0) return null;
  return {
    kind: "summary",
    title: "策略研究回测",
    subtitle: "临时运行，不会保存策略定义",
    cards,
    rows,
  };
}

function buildStrategySaveDefinition(output: UnknownRecord): ADKToolVisualization | null {
  const definition = isRecord(output.definition)
    ? output.definition
    : isRecord(output)
      ? output
      : null;
  if (!definition) return null;
  const operation = optionalValue(output.operation);
  const cards = [
    operation
      ? ({ label: "操作", value: formatOperation(operation), tone: "ok" } satisfies ADKSummaryVisualization["cards"][number])
      : null,
    summaryCard("策略定义", definition.name),
    summaryCard("版本", definition.version),
    summaryCard("标的", definition.symbol),
    summaryCard("周期", definition.interval),
  ].filter((card): card is ADKSummaryVisualization["cards"][number] => card !== null);
  const rows = [
    row("ID", definition.id),
    row("运行时", definition.runtime),
    row("来源格式", definition.sourceFormat),
    row("更新时间", definition.updatedAt),
  ].filter((item): item is { label: string; value: string } => item !== null);
  if (cards.length === 0 && rows.length === 0) return null;
  return {
    kind: "summary",
    title: "策略定义已保存",
    subtitle: operation ? `本次操作：${formatOperation(operation)}` : "策略定义已更新",
    cards,
    rows,
  };
}

function buildStrategyUpdateInstanceMode(output: UnknownRecord): ADKToolVisualization | null {
  const instance = isRecord(output.instance) ? output.instance : null;
  if (!instance) return null;
  const binding = isRecord(instance.binding) ? instance.binding : null;
  const definition = isRecord(instance.definition) ? instance.definition : null;
  const updatedFields = findArray(output, ["updatedFields"]);
  const cards = [
    summaryCard("模式", binding?.executionMode, toneForValue(binding?.executionMode)),
    summaryCard("状态", instance.status),
    summaryCard("运行时", instance.runtime),
    summaryCard("标的数", Array.isArray(binding?.symbols) ? binding?.symbols.length : undefined),
  ].filter((card): card is NonNullable<typeof card> => card !== null);
  const rows = [
    row("实例 ID", instance.id),
    row("策略定义", definition?.name),
    row("周期", binding?.interval),
    row("已修改字段", updatedFields.map((field) => translateUpdatedFieldName(field)).join("、")),
  ].filter((item): item is { label: string; value: string } => item !== null);
  if (cards.length === 0 && rows.length === 0) return null;
  return {
    kind: "summary",
    title: "策略实例模式已更新",
    subtitle: optionalValue(binding?.executionMode) ?? "执行模式已修改",
    cards,
    rows,
  };
}

function buildPortfolioSummary(output: UnknownRecord): ADKToolVisualization | null {
  const cards = [
    summaryCard("经纪通道", pick(output, ["brokerStatus", "brokerEnabled", "connected", "status"])),
    summaryCard("账户数", pick(output, ["accountCount", "accountsTotal", "accounts"])),
    summaryCard("订单数", pick(output, ["orderCount", "ordersTotal", "orders"])),
    summaryCard("持仓数", pick(output, ["positionCount", "positionsTotal", "positions"])),
  ].filter((card): card is NonNullable<typeof card> => card !== null);
  const rows = [
    row("检查时间", pick(output, ["checkedAt", "updatedAt", "at"])),
    row("交易环境", pick(output, ["tradingEnvironment", "environment"])),
    row("账户", pick(output, ["accountId", "accountName"])),
  ].filter((item): item is { label: string; value: string } => item !== null);

  if (cards.length === 0 && rows.length === 0) return null;
  return { kind: "summary", title: "组合摘要", cards, rows };
}

function buildRiskState(output: UnknownRecord): ADKToolVisualization | null {
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

function buildBacktestResultView(output: UnknownRecord): ADKToolVisualization | null {
  const view = optionalValue(output.view) ?? "summary";
  const series = isRecord(output.series) ? output.series : {};
  if (view === "chart") {
    const candles = findArray(series, ["candles"]);
    if (candles.length > 0) {
      return buildRecordTable("回测蜡烛窗口", output, candles, [
        ["time", "时间"],
        ["open", "开"],
        ["high", "高"],
        ["low", "低"],
        ["close", "收"],
        ["volume", "量"],
      ]);
    }
    const trades = findArray(series, ["trades"]);
    if (trades.length > 0) {
      return buildRecordTable("回测交易窗口", output, trades, [
        ["time", "时间"],
        ["side", "方向"],
        ["price", "价格"],
        ["qty", "数量"],
        ["positionQty", "持仓"],
      ]);
    }
  }
  if (view === "orders") {
    const orders = findArray(series, ["orderBook"]);
    if (orders.length > 0) {
      return buildRecordTable("回测订单窗口", output, orders, [
        ["orderId", "订单"],
        ["symbol", "标的"],
        ["side", "方向"],
        ["status", "状态"],
        ["quantity", "数量"],
        ["price", "价格"],
        ["submittedAt", "提交时间"],
        ["filledAt", "成交时间"],
      ]);
    }
  }
  if (view === "logs" || view === "errors") {
    const items = findArray(series, view === "logs" ? ["logs"] : ["runtimeErrors"]);
    if (items.length > 0) {
      return buildStringTable(view === "logs" ? "回测日志窗口" : "回测错误窗口", output, items);
    }
  }
  return buildBacktestResultSummary(output);
}

function buildBacktestResultSummary(output: UnknownRecord): ADKToolVisualization | null {
  const run = isRecord(output.run) ? output.run : {};
  const summary = isRecord(output.summary) ? output.summary : {};
  const cards = [
    summaryCard("状态", run.status, toneForValue(run.status)),
    summaryCard("最终资产", summary.finalBalance),
    summaryCard("盈亏", summary.pnl),
    summaryCard("收益", summary.totalReturn),
    summaryCard("最大回撤", summary.maxDrawdown),
    summaryCard("成交数", summary.totalTrades),
  ].filter((card): card is NonNullable<typeof card> => card !== null);
  const rows = [
    row("运行 ID", run.id),
    row("标的", run.symbol),
    row("周期", run.interval),
    row("开始", run.startTime),
    row("结束", run.endTime),
    row("错误", summary.error),
    row("最新日志", summary.latestLog),
  ].filter((item): item is { label: string; value: string } => item !== null);
  if (cards.length === 0 && rows.length === 0) return null;
  const visualization: ADKSummaryVisualization = { kind: "summary", title: "回测结果视图", cards, rows };
  const subtitle = optionalValue(output.view);
  if (subtitle) visualization.subtitle = subtitle;
  return visualization;
}

function buildToolTable(
  title: string,
  output: UnknownRecord,
  arrayKeys: string[],
  preferredColumns: Array<[string, string]>,
): ADKTableVisualization | null {
  const items = findArray(output, arrayKeys);
  if (items.length === 0) return null;
  const records = items.filter(isRecord).slice(0, 20);
  if (records.length === 0) return null;
  const columns = preferredColumns.filter(([key]) => records.some((record) => hasDisplayValue(record[key])));
  if (columns.length === 0) {
    for (const key of Object.keys(records[0]!).slice(0, 6)) {
      columns.push([key, labelFromKey(key)]);
    }
  }
  return {
    kind: "table",
    title,
    subtitle: `${records.length}${items.length > records.length ? ` / ${items.length}` : ""} 行`,
    columns: columns.map(([key, label]) => ({ key, label })),
    rows: records.map((record) => Object.fromEntries(columns.map(([key]) => [key, formatValue(record[key])]))),
  };
}

function buildRecordTable(
  title: string,
  output: UnknownRecord,
  items: unknown[],
  preferredColumns: Array<[string, string]>,
): ADKTableVisualization | null {
  const records = items.filter(isRecord).slice(0, 20);
  if (records.length === 0) return null;
  const columns = preferredColumns.filter(([key]) => records.some((record) => hasDisplayValue(record[key])));
  if (columns.length === 0) {
    for (const key of Object.keys(records[0]!).slice(0, 6)) {
      columns.push([key, labelFromKey(key)]);
    }
  }
  return {
    kind: "table",
    title,
    subtitle: backtestWindowSubtitle(output, records.length, items.length),
    columns: columns.map(([key, label]) => ({ key, label })),
    rows: records.map((record) => Object.fromEntries(columns.map(([key]) => [key, formatValue(record[key])]))),
  };
}

function buildStringTable(title: string, output: UnknownRecord, items: unknown[]): ADKTableVisualization | null {
  const rows = items.slice(0, 20).map((item, index) => ({ index: String(index + 1), message: formatValue(item) }));
  if (rows.length === 0) return null;
  return {
    kind: "table",
    title,
    subtitle: backtestWindowSubtitle(output, rows.length, items.length),
    columns: [
      { key: "index", label: "#" },
      { key: "message", label: "内容" },
    ],
    rows,
  };
}

function backtestWindowSubtitle(output: UnknownRecord, returned: number, total: number): string {
  const run = isRecord(output.run) ? output.run : {};
  const window = isRecord(output.window) ? output.window : {};
  const parts = [
    optionalValue(run.symbol),
    optionalValue(window.resolution),
    `${returned}${total > returned ? ` / ${total}` : ""} 行`,
  ];
  const nextCursor = optionalValue(window.nextCursor);
  if (nextCursor) parts.push(`next ${nextCursor}`);
  return parts.filter(Boolean).join(" · ");
}

function buildDepth(output: UnknownRecord): ADKDepthVisualization | null {
  const bidsRaw = findArray(output, ["bids", "bid", "bidRows"]);
  const asksRaw = findArray(output, ["asks", "ask", "askRows"]);
  const bids = normalizeDepthRows(bidsRaw).slice(0, 8);
  const asks = normalizeDepthRows(asksRaw).slice(0, 8);
  if (bids.length === 0 && asks.length === 0) return null;
  const maxQuantity = Math.max(...[...bids, ...asks].map((item) => Number.parseFloat(item.quantity.replace(/,/g, ""))).filter(Number.isFinite), 1);
  return {
    kind: "depth",
    title: "盘口深度",
    subtitle: formatValue(pick(output, ["symbol", "instrumentId", "market"])),
    bids: bids.map((row) => ({ ...row, percent: depthPercent(row, maxQuantity) })),
    asks: asks.map((row) => ({ ...row, percent: depthPercent(row, maxQuantity) })),
  };
}

function buildTimeline(title: string, output: UnknownRecord, arrayKeys: string[]): ADKTimelineVisualization | null {
  const items = findArray(output, arrayKeys);
  const events = items.filter(isRecord).slice(0, 20).map((item) => {
    const event: ADKTimelineVisualization["events"][number] = {
      label: formatValue(pick(item, ["kind", "type", "event", "action", "status", "orderId", "id"])),
    };
    const time = optionalValue(pick(item, ["at", "time", "createdAt", "updatedAt", "timestamp"]));
    const detail = optionalValue(pick(item, ["message", "detail", "reason", "description", "symbol"]));
    const tone = toneForValue(pick(item, ["status", "kind", "type"]));
    if (time !== undefined) event.time = time;
    if (detail !== undefined) event.detail = detail;
    if (tone !== undefined) event.tone = tone;
    return event;
  });
  if (events.length === 0) return null;
  return { kind: "timeline", title, subtitle: `${events.length}${items.length > events.length ? ` / ${items.length}` : ""} 条事件`, events };
}

function normalizeDepthRows(items: unknown[]): ADKDepthRow[] {
  return items.map((item) => {
    if (Array.isArray(item)) {
      return { price: formatValue(item[0]), quantity: formatValue(item[1]), percent: 0 };
    }
    if (isRecord(item)) {
      return {
        price: formatValue(pick(item, ["price", "p"])),
        quantity: formatValue(pick(item, ["quantity", "qty", "volume", "size"])),
        percent: 0,
      };
    }
    return null;
  }).filter((item): item is ADKDepthRow => item !== null && item.price !== "-" && item.quantity !== "-");
}

function depthPercent(row: ADKDepthRow, maxQuantity: number): number {
  const quantity = Number.parseFloat(row.quantity.replace(/,/g, ""));
  if (!Number.isFinite(quantity) || maxQuantity <= 0) return 0;
  return Math.max(4, Math.min(100, Math.round((quantity / maxQuantity) * 100)));
}

function summaryCard(label: string, value: unknown, tone: ADKSummaryVisualization["cards"][number]["tone"] = undefined) {
  if (!hasDisplayValue(value)) return null;
  const card: ADKSummaryVisualization["cards"][number] = { label, value: formatValue(value) };
  const resolvedTone = tone ?? toneForValue(value);
  if (resolvedTone !== undefined) card.tone = resolvedTone;
  return card;
}

function row(label: string, value: unknown) {
  if (!hasDisplayValue(value)) return null;
  return { label, value: formatValue(value) };
}

function findArray(record: UnknownRecord, keys: string[]): unknown[] {
  for (const key of keys) {
    const value = record[key];
    if (Array.isArray(value)) return value;
  }
  for (const value of Object.values(record)) {
    if (isRecord(value)) {
      const nested = findArray(value, keys);
      if (nested.length > 0) return nested;
    }
  }
  return [];
}

function pick(record: UnknownRecord, keys: string[]): unknown {
  for (const key of keys) {
    if (hasDisplayValue(record[key])) return record[key];
  }
  return undefined;
}

function optionalValue(value: unknown): string | undefined {
  return hasDisplayValue(value) ? formatValue(value) : undefined;
}

function hasDisplayValue(value: unknown): boolean {
  if (value === null || value === undefined) return false;
  if (typeof value === "string") return value.trim() !== "";
  if (Array.isArray(value)) return value.length > 0;
  return true;
}

function formatValue(value: unknown): string {
  if (value === null || value === undefined) return "-";
  if (typeof value === "boolean") return value ? "是" : "否";
  if (typeof value === "number") return Number.isFinite(value) ? value.toLocaleString(undefined, { maximumFractionDigits: 4 }) : "-";
  if (typeof value === "string") {
    const trimmed = value.trim();
    return trimmed ? translateDisplayText(trimmed) : "-";
  }
  if (Array.isArray(value)) return String(value.length);
  if (isRecord(value)) {
    const status = pick(value, ["status", "state", "enabled", "active", "value"]);
    if (hasDisplayValue(status)) return formatValue(status);
    return Object.keys(value).length === 0 ? "-" : JSON.stringify(value);
  }
  return String(value);
}

function toneForValue(value: unknown): "ok" | "warning" | "danger" | "muted" | undefined {
  const text = formatValue(value).toLowerCase();
  if (["yes", "是", "enabled", "已启用", "active", "活跃", "ok", "healthy", "正常", "connected", "已连接", "succeeded", "success", "成功", "completed", "已完成", "done"].includes(text)) return "ok";
  if (["no", "否", "disabled", "未启用", "inactive", "未激活", "pending", "待处理", "queued", "排队中", "running", "运行中", "todo", "已停止"].includes(text)) return "muted";
  if (text.includes("warn") || text.includes("warning") || text.includes("警告") || text.includes("blocked") || text.includes("limited") || text.includes("受限")) return "warning";
  if (text.includes("error") || text.includes("failed") || text.includes("失败") || text.includes("denied") || text.includes("拒绝") || text.includes("kill")) return "danger";
  return undefined;
}

function toneForKillSwitch(value: unknown): "ok" | "warning" | "danger" | "muted" | undefined {
  if (typeof value === "boolean") return value ? "danger" : "ok";
  const text = formatValue(value).toLowerCase();
  if (["yes", "是", "true", "enabled", "已启用", "active", "活跃", "on", "engaged"].includes(text)) return "danger";
  if (["no", "否", "false", "disabled", "未启用", "inactive", "未激活", "off"].includes(text)) return "ok";
  return toneForValue(value);
}

function translateDSLSection(section: string): string {
  switch (section.trim().toLowerCase()) {
    case "overview":
      return "概览";
    case "syntax":
      return "语法";
    case "expressions":
      return "表达式";
    case "indicators":
      return "指标";
    case "orders":
      return "下单";
    case "protect":
      return "保护";
    case "support-matrix":
      return "支持矩阵";
    case "unsupported":
      return "不支持项";
    case "examples":
      return "示例";
    default:
      return section;
  }
}

function formatOperation(operation: string): string {
  switch (operation.trim().toLowerCase()) {
    case "created":
      return "已创建";
    case "updated":
      return "已更新";
    default:
      return operation;
  }
}

function translateUpdatedFieldName(field: unknown): string {
  const text = typeof field === "string" ? field.trim() : "";
  switch (text) {
    case "executionMode":
      return "执行模式";
    default:
      return text;
  }
}

function translateDisplayText(text: string): string {
  const key = text.trim().toLowerCase();
  const exactMap: Record<string, string> = {
    connected: "已连接",
    disconnected: "未连接",
    enabled: "已启用",
    disabled: "未启用",
    active: "活跃",
    inactive: "未激活",
    valid: "有效",
    invalid: "无效",
    created: "已创建",
    updated: "已更新",
    live: "实盘",
    notify_only: "仅通知",
    stopped: "已停止",
    running: "运行中",
    pending: "待处理",
    submitted: "已提交",
    succeeded: "成功",
    success: "成功",
    failed: "失败",
    rejected: "已拒绝",
    completed: "已完成",
    buy: "买入",
    sell: "卖出",
    short: "做空",
    long: "做多",
    allow: "允许",
    market: "市价",
    limit: "限价",
    current: "当前",
    current_day: "当日",
    true: "是",
    false: "否",
    yes: "是",
    no: "否",
  };
  return exactMap[key] ?? text;
}

function labelFromKey(key: string): string {
  return key.replace(/([A-Z])/g, " $1").replace(/[_-]+/g, " ").replace(/\b\w/g, (char) => char.toUpperCase()).trim();
}

function isRecord(value: unknown): value is UnknownRecord {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
