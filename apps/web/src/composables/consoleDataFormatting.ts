import type {
  PortfolioReconciliationResponse,
  RealTradeApprovalsResponse,
  RealTradeHardStopEventsResponse,
  RealTradeHardStopsResponse,
  RealTradeKillSwitchEventsResponse,
  RealTradeKillSwitchStateResponse,
  RealTradeRiskEventsResponse,
  RealTradeRiskStateResponse,
  WorkerBrokerOrderUpdateErrorContext,
  WorkerBrokerOrderUpdatesResponse,
} from "@jftrade/ui-contracts";

type RealTradeHardStopScope = "ACCOUNT" | "MARKET" | "SYMBOL";

export function resolvePortfolioReconciliationStatusLabel(
  status: PortfolioReconciliationResponse["positions"][number]["status"],
): string {
  switch (status) {
    case "matched":
      return "已匹配";
    case "different":
      return "存在差异";
    case "missing-in-projection":
      return "内部缺失";
    case "missing-at-broker":
      return "券商缺失";
  }
}

export function resolvePortfolioReconciliationTagType(
  status: PortfolioReconciliationResponse["positions"][number]["status"],
): "success" | "warning" | "danger" | "info" {
  switch (status) {
    case "matched":
      return "success";
    case "different":
      return "warning";
    case "missing-in-projection":
      return "danger";
    case "missing-at-broker":
      return "info";
  }
}

function resolveRealTradeHardStopScope(entry: {
  market: string | null;
  symbol: string | null;
  hardStopScope?: RealTradeHardStopScope | null;
}): RealTradeHardStopScope {
  if (entry.hardStopScope != null) {
    return entry.hardStopScope;
  }

  if (entry.symbol != null) {
    return "SYMBOL";
  }

  if (entry.market != null) {
    return "MARKET";
  }

  return "ACCOUNT";
}

export function formatRealTradeHardStopScope(entry: {
  market: string | null;
  symbol: string | null;
  hardStopScope?: RealTradeHardStopScope | null;
}): string {
  const scope = resolveRealTradeHardStopScope(entry);

  switch (scope) {
    case "SYMBOL":
      return `SYMBOL / ${entry.market ?? "N/A"} / ${entry.symbol ?? "N/A"}`;
    case "MARKET":
      return `MARKET / ${entry.market ?? "N/A"}`;
    case "ACCOUNT":
      return "ACCOUNT";
  }
}

export function resolveRealTradeHardStopScopeTagType(entry: {
  market: string | null;
  symbol: string | null;
  hardStopScope?: RealTradeHardStopScope | null;
}): "info" | "warning" | "danger" {
  switch (resolveRealTradeHardStopScope(entry)) {
    case "ACCOUNT":
      return "info";
    case "MARKET":
      return "warning";
    case "SYMBOL":
      return "danger";
  }
}

export function formatRealTradeKillSwitchSource(
  source: RealTradeKillSwitchStateResponse["killSwitchSource"],
): string {
  switch (source) {
    case "ENV":
      return "ENV";
    case "CONTROL_PLANE":
      return "CONTROL-PLANE";
    default:
      return "INACTIVE";
  }
}

export function resolveRealTradeKillSwitchEventTagType(
  eventType: RealTradeKillSwitchEventsResponse["entries"][number]["eventType"],
): "success" | "warning" | "danger" {
  switch (eventType) {
    case "released":
      return "success";
    case "activated":
      return "warning";
    case "rejected":
      return "danger";
  }
}

export function formatRealTradeRiskSource(
  source: RealTradeRiskStateResponse["riskConfigSource"],
): string {
  switch (source) {
    case "ENV":
      return "ENV";
    case "CONTROL_PLANE":
      return "CONTROL-PLANE";
    case "MERGED":
      return "MERGED";
    default:
      return "INACTIVE";
  }
}

export function resolveRealTradeRiskEventTagType(
  eventType: RealTradeRiskEventsResponse["entries"][number]["eventType"],
): "success" | "warning" | "danger" {
  switch (eventType) {
    case "released":
      return "success";
    case "activated":
      return "warning";
    case "rejected":
      return "danger";
  }
}

export function resolveWorkerBrokerSubscriptionTagType(
  status: WorkerBrokerOrderUpdatesResponse["subscriptions"][number]["status"],
): "success" | "warning" | "info" {
  switch (status) {
    case "active":
      return "success";
    case "retrying":
      return "warning";
    case "inactive":
      return "info";
  }
}

export function formatDateTime(value: string | null | undefined): string {
  if (value == null || value === "") {
    return "N/A";
  }

  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }

  return new Intl.DateTimeFormat(undefined, {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
    timeZoneName: "short",
  }).format(date);
}

export function formatDurationMs(value: number | null | undefined): string {
  if (value == null) {
    return "N/A";
  }

  if (value < 1000) {
    return `${value}ms`;
  }

  if (value < 60_000) {
    return `${Math.round(value / 1000)}s`;
  }

  if (value < 3_600_000) {
    return `${Math.round(value / 60_000)}m`;
  }

  return `${Math.round(value / 3_600_000)}h`;
}

export function formatWorkerBrokerErrorContext(
  context: WorkerBrokerOrderUpdateErrorContext | null,
  fallback: string | null,
): string {
  return context?.summary ?? fallback ?? "No error context";
}

export function resolveRealTradeApprovalDecisionTagType(
  decision: RealTradeApprovalsResponse["entries"][number]["decision"],
): "success" | "danger" {
  return decision === "approved" ? "success" : "danger";
}