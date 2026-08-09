import {
  normalizeQuoteWorkbenchPeriod,
  normalizeQuoteWorkbenchProductClass,
  type QuoteWorkbenchPeriod,
  type QuoteWorkbenchTab,
} from "../components/domain/market-data/quoteWorkbench";
import {
  RESEARCH_SECTIONS,
  type ResearchSection,
  type ResearchSectionConfig,
} from "../components/research/researchNavigation";
import {
  normalizeResearchQuoteTarget,
  type ResearchQuoteTarget,
} from "../components/research/researchQuote";

export type PredictionContractView =
  | "snapshot"
  | "depth"
  | "candles"
  | "ticks"
  | "milestones";

export type InstrumentResearchOperation =
  | "profile"
  | "financials"
  | "valuation"
  | "analyst"
  | "ownership"
  | "corporate_actions"
  | "short_interest"
  | "news";

export type SecuritiesWorkspaceProductClass =
  | "equity"
  | "fund"
  | "warrant"
  | "cbbc"
  | "index"
  | "bond"
  | "unknown";

export function configFor(section: ResearchSection): ResearchSectionConfig {
  return RESEARCH_SECTIONS.find((item) => item.value === section)!;
}

export function operationFor(section: ResearchSection, value: unknown): string {
  const config = configFor(section);
  const rawCandidate = String(value ?? "");
  const candidate =
    section === "industries" &&
    ["chain_detail", "chains_by_plate"].includes(rawCandidate)
      ? "chains"
      : rawCandidate;
  return config.operations.some((item) => item.value === candidate)
    ? candidate
    : config.operations[0]?.value ?? "";
}

export function firstQueryValue(value: unknown): string {
  if (Array.isArray(value)) return String(value[0] ?? "").trim();
  return String(value ?? "").trim();
}

export function quotePeriodFromQuery(value: unknown): QuoteWorkbenchPeriod {
  return normalizeQuoteWorkbenchPeriod(firstQueryValue(value));
}

export function predictionContractViewFromQuery(
  value: unknown,
): PredictionContractView {
  const candidate = firstQueryValue(value);
  return ["snapshot", "depth", "candles", "ticks", "milestones"].includes(
    candidate,
  )
    ? (candidate as PredictionContractView)
    : "snapshot";
}

export function quoteTargetFromQuery(
  query: Record<string, unknown>,
): ResearchQuoteTarget | null {
  const instrumentId = firstQueryValue(query.quote);
  if (instrumentId === "") return null;
  return normalizeResearchQuoteTarget({
    kind: firstQueryValue(query.quoteKind) === "plate" ? "plate" : "instrument",
    instrumentId,
    name: firstQueryValue(query.quoteName),
    productClass: normalizeQuoteWorkbenchProductClass(
      firstQueryValue(query.quoteClass),
    ),
  });
}

export function quoteTabFromQuery(
  query: Record<string, unknown>,
  target: ResearchQuoteTarget | null,
): QuoteWorkbenchTab {
  return target?.kind !== "plate" && firstQueryValue(query.quoteTab) === "news"
    ? "news"
    : "quote";
}

export function queryWith(
  query: Record<string, unknown>,
  patch: Record<string, string | undefined>,
): Record<string, string | string[]> {
  const next: Record<string, string | string[]> = {};
  for (const [key, value] of Object.entries({ ...query, ...patch })) {
    if (value == null || value === "") continue;
    next[key] = Array.isArray(value) ? value.map(String) : String(value);
  }
  return next;
}

export function activeScreenMarketFor(
  value: unknown,
  activeMarketCode: string,
): "US" | "HK" | "SH" | "SZ" {
  const candidate = firstQueryValue(value).toUpperCase();
  if (["US", "HK", "SH", "SZ"].includes(candidate)) {
    return candidate as "US" | "HK" | "SH" | "SZ";
  }
  return activeMarketCode === "HK" ? "HK" : activeMarketCode === "CN" ? "SH" : "US";
}

export function queryMarketFor(input: {
  section: ResearchSection;
  operation: string;
  activeMarketCode: string;
  activeScreenMarket: string;
  activeInstrumentId: string;
  workspaceMarket: string;
}): string {
  if (input.section === "market" || input.section === "calendar") {
    return input.activeMarketCode;
  }
  if (input.section === "screens") return input.activeScreenMarket;
  if (input.section === "derivatives") {
    return input.operation === "warrant" ? "HK" : "US";
  }
  if (input.section === "institutions") {
    return input.activeMarketCode === "HK" ? "HK" : "US";
  }
  if (input.section === "industries") return input.activeMarketCode;
  if (input.section === "instrument") {
    return input.activeInstrumentId.split(".", 1)[0] || input.workspaceMarket;
  }
  return "US";
}

export function optionResearchOperation(
  operation: string,
): "unusual" | "zero_dte" | "earnings" | "seller" {
  return ["unusual", "zero_dte", "earnings", "seller"].includes(operation)
    ? (operation as "unusual" | "zero_dte" | "earnings" | "seller")
    : "unusual";
}

export function macroResearchOperation(
  operation: string,
): "indicators" | "fed_target_rate" | "fed_dot_plot" {
  return ["indicators", "fed_target_rate", "fed_dot_plot"].includes(operation)
    ? (operation as "indicators" | "fed_target_rate" | "fed_dot_plot")
    : "indicators";
}

export function arkResearchOperation(
  operation: string,
): "ark_fund_holdings" | "ark_transactions" {
  return operation === "ark_fund_holdings"
    ? "ark_fund_holdings"
    : "ark_transactions";
}

export function derivativeScreenOperation(
  operation: string,
): "option_screen" | "warrant" {
  return operation === "warrant" ? "warrant" : "option_screen";
}

const instrumentResearchOperations = new Set<InstrumentResearchOperation>([
  "profile",
  "financials",
  "valuation",
  "analyst",
  "ownership",
  "corporate_actions",
  "short_interest",
  "news",
]);

export function instrumentResearchOperation(
  operation: string,
): InstrumentResearchOperation {
  return instrumentResearchOperations.has(operation as InstrumentResearchOperation)
    ? (operation as InstrumentResearchOperation)
    : "profile";
}

export function researchEntry(value: unknown): Record<string, unknown> | null {
  return value != null && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;
}

export function quoteTargetWorkspaceProductClass(
  value: string,
): SecuritiesWorkspaceProductClass {
  return ["equity", "fund", "warrant", "cbbc", "index", "bond"].includes(value)
    ? (value as SecuritiesWorkspaceProductClass)
    : "unknown";
}

export function researchWorkspaceDestination(
  productClassHint: "option" | "equity" | "unknown",
  targetProductClass: string,
): {
  marketSegment: "securities" | "derivatives";
  productClass: SecuritiesWorkspaceProductClass | "option";
} {
  if (productClassHint === "option") {
    return { marketSegment: "derivatives", productClass: "option" };
  }
  const resolved = quoteTargetWorkspaceProductClass(targetProductClass);
  return {
    marketSegment: "securities",
    productClass: resolved === "unknown" ? "equity" : resolved,
  };
}
