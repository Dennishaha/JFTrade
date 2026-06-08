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
    case "portfolio.summary":
      return buildPortfolioSummary(output);
    case "broker.orders":
      return buildToolTable("Broker orders", output, ["orders", "items", "data"], [
        ["symbol", "Symbol"],
        ["side", "Side"],
        ["status", "Status"],
        ["quantity", "Qty"],
        ["price", "Price"],
        ["orderIdEx", "Order ID"],
        ["createdAt", "Created"],
        ["updatedAt", "Updated"],
      ]);
    case "broker.fills":
      return buildToolTable("Broker fills", output, ["fills", "items", "data"], [
        ["symbol", "Symbol"],
        ["side", "Side"],
        ["quantity", "Qty"],
        ["price", "Price"],
        ["amount", "Amount"],
        ["fillId", "Fill ID"],
        ["createdAt", "Created"],
      ]);
    case "broker.fees":
      return buildToolTable("Broker fees", output, ["fees", "items", "data"], [
        ["orderIdEx", "Order ID"],
        ["feeType", "Type"],
        ["amount", "Amount"],
        ["currency", "Currency"],
        ["description", "Description"],
      ]);
    case "broker.cash_flows":
      return buildToolTable("Cash flows", output, ["cashFlows", "flows", "items", "data"], [
        ["clearingDate", "Date"],
        ["direction", "Direction"],
        ["amount", "Amount"],
        ["currency", "Currency"],
        ["description", "Description"],
      ]);
    case "market.depth":
      return buildDepth(output);
    case "risk.state":
      return buildRiskState(output);
    case "risk.events":
      return buildTimeline("Risk events", output, ["events", "riskEvents", "items", "data"]);
    case "execution.order_events":
      return buildTimeline("Order events", output, ["events", "orderEvents", "items", "data", "orders"]);
    case "backtest.runs":
      return buildToolTable("Backtest runs", output, ["runs", "items", "data"], [
        ["id", "Run"],
        ["status", "Status"],
        ["symbol", "Symbol"],
        ["interval", "Interval"],
        ["totalReturn", "Return"],
        ["maxDrawdown", "Drawdown"],
        ["tradeCount", "Trades"],
        ["createdAt", "Created"],
      ]);
    case "strategy.optimize":
      return buildToolTable("Optimization candidates", output, ["runs", "candidates", "tasks", "items", "data"], [
        ["definitionId", "Definition"],
        ["runId", "Run"],
        ["status", "Status"],
        ["totalReturn", "Return"],
        ["maxDrawdown", "Drawdown"],
        ["tradeCount", "Trades"],
      ]);
    default:
      return null;
  }
}

function buildPortfolioSummary(output: UnknownRecord): ADKToolVisualization | null {
  const cards = [
    summaryCard("Broker", pick(output, ["brokerStatus", "brokerEnabled", "connected", "status"])),
    summaryCard("Accounts", pick(output, ["accountCount", "accountsTotal", "accounts"])),
    summaryCard("Orders", pick(output, ["orderCount", "ordersTotal", "orders"])),
    summaryCard("Positions", pick(output, ["positionCount", "positionsTotal", "positions"])),
  ].filter((card): card is NonNullable<typeof card> => card !== null);
  const rows = [
    row("Checked at", pick(output, ["checkedAt", "updatedAt", "at"])),
    row("Trading environment", pick(output, ["tradingEnvironment", "environment"])),
    row("Account", pick(output, ["accountId", "accountName"])),
  ].filter((item): item is { label: string; value: string } => item !== null);

  if (cards.length === 0 && rows.length === 0) return null;
  return { kind: "summary", title: "Portfolio summary", cards, rows };
}

function buildRiskState(output: UnknownRecord): ADKToolVisualization | null {
  const killSwitch = pick(output, ["killSwitch", "kill_switch"]);
  const riskLimits = pick(output, ["riskLimits", "limits"]);
  const cards = [
    summaryCard("Kill switch", killSwitch, toneForKillSwitch(killSwitch)),
    summaryCard("Risk limits", riskLimits),
    summaryCard("Real trading", pick(output, ["realTradingEnabled", "realTrading", "enabled"]), toneForValue(pick(output, ["realTradingEnabled", "realTrading", "enabled"]))),
  ].filter((card): card is NonNullable<typeof card> => card !== null);
  const rows = [
    row("Checked at", pick(output, ["checkedAt", "updatedAt", "at"])),
    row("Source", pick(output, ["riskConfigSource", "source"])),
  ].filter((item): item is { label: string; value: string } => item !== null);

  if (cards.length === 0 && rows.length === 0) return null;
  return { kind: "summary", title: "Risk state", cards, rows };
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
    subtitle: `${records.length}${items.length > records.length ? ` of ${items.length}` : ""} rows`,
    columns: columns.map(([key, label]) => ({ key, label })),
    rows: records.map((record) => Object.fromEntries(columns.map(([key]) => [key, formatValue(record[key])]))),
  };
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
    title: "Market depth",
    subtitle: formatValue(pick(output, ["symbol", "instrumentId", "market"])),
    bids: bids.map((row) => ({ ...row, percent: depthPercent(row, maxQuantity) })),
    asks: asks.map((row) => ({ ...row, percent: depthPercent(row, maxQuantity) })),
  };
}

function buildTimeline(title: string, output: UnknownRecord, arrayKeys: string[]): ADKTimelineVisualization | null {
  const items = findArray(output, arrayKeys);
  const events = items.filter(isRecord).slice(0, 20).map((item) => {
    const event: ADKTimelineVisualization["events"][number] = {
      label: formatValue(pick(item, ["kind", "type", "status", "event", "action", "orderId", "id"])),
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
  return { kind: "timeline", title, subtitle: `${events.length}${items.length > events.length ? ` of ${items.length}` : ""} events`, events };
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
  if (typeof value === "boolean") return value ? "Yes" : "No";
  if (typeof value === "number") return Number.isFinite(value) ? value.toLocaleString(undefined, { maximumFractionDigits: 4 }) : "-";
  if (typeof value === "string") return value.trim() || "-";
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
  if (["yes", "enabled", "active", "ok", "healthy", "connected", "succeeded", "completed", "done"].includes(text)) return "ok";
  if (["no", "disabled", "inactive", "pending", "running", "todo"].includes(text)) return "muted";
  if (text.includes("warn") || text.includes("blocked") || text.includes("limited")) return "warning";
  if (text.includes("error") || text.includes("failed") || text.includes("denied") || text.includes("kill")) return "danger";
  return undefined;
}

function toneForKillSwitch(value: unknown): "ok" | "warning" | "danger" | "muted" | undefined {
  if (typeof value === "boolean") return value ? "danger" : "ok";
  const text = formatValue(value).toLowerCase();
  if (["yes", "true", "enabled", "active", "on", "engaged"].includes(text)) return "danger";
  if (["no", "false", "disabled", "inactive", "off"].includes(text)) return "ok";
  return toneForValue(value);
}

function labelFromKey(key: string): string {
  return key.replace(/([A-Z])/g, " $1").replace(/[_-]+/g, " ").replace(/\b\w/g, (char) => char.toUpperCase()).trim();
}

function isRecord(value: unknown): value is UnknownRecord {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
