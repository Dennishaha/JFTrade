import type { MarketSecurityDetails } from "../contracts";
import {
  capabilitySurfaceID,
  type CapabilitySurfaceID,
} from "../features/capabilitySurfaces";
import { normalizeInstrumentSecurityType } from "./instrumentPresentation";

export type WorkspaceProductClass =
  | "equity"
  | "fund"
  | "option"
  | "warrant"
  | "cbbc"
  | "future"
  | "event_contract"
  | "index"
  | "bond"
  | "plate"
  | "unknown";

export type WorkspaceProductTab =
  | "contract"
  | "depth"
  | "chart"
  | "ticks"
  | "rules"
  | "options"
  | "warrants"
  | "news"
  | "company";

export interface WorkspaceProductTabDefinition {
  value: WorkspaceProductTab;
  label: string;
  surfaceId: CapabilitySurfaceID;
  featureId: string;
}

const allWorkspaceProductTabs: readonly WorkspaceProductTabDefinition[] = [
  {
    value: "chart",
    label: "图表",
    surfaceId: capabilitySurfaceID("workspace.chart"),
    featureId: "market.candles",
  },
  {
    value: "options",
    label: "期权",
    surfaceId: capabilitySurfaceID("workspace.options"),
    featureId: "derivatives.option_chain",
  },
  {
    value: "warrants",
    label: "轮证",
    surfaceId: capabilitySurfaceID("workspace.warrants"),
    featureId: "derivatives.warrants",
  },
  {
    value: "news",
    label: "资讯",
    surfaceId: capabilitySurfaceID("workspace.news"),
    featureId: "research.news",
  },
  {
    value: "company",
    label: "公司",
    surfaceId: capabilitySurfaceID("workspace.company"),
    featureId: "research.instrument",
  },
];

const predictionWorkspaceProductTabs: readonly WorkspaceProductTabDefinition[] =
  [
    {
      value: "contract",
      label: "合约",
      surfaceId: capabilitySurfaceID("workspace.contract"),
      featureId: "prediction.snapshot",
    },
    {
      value: "depth",
      label: "盘口",
      surfaceId: capabilitySurfaceID("workspace.depth"),
      featureId: "prediction.depth",
    },
    {
      value: "chart",
      label: "图表",
      surfaceId: capabilitySurfaceID("workspace.chart"),
      featureId: "prediction.history",
    },
    {
      value: "ticks",
      label: "逐笔",
      surfaceId: capabilitySurfaceID("workspace.ticks"),
      featureId: "prediction.history",
    },
    {
      value: "rules",
      label: "规则",
      surfaceId: capabilitySurfaceID("workspace.rules"),
      featureId: "prediction.snapshot",
    },
  ];

const futureWorkspaceProductTabs = allWorkspaceProductTabs.filter(
  (tab) => tab.value === "chart" || tab.value === "news",
);

const knownProductClasses = new Set<WorkspaceProductClass>([
  "equity",
  "fund",
  "option",
  "warrant",
  "cbbc",
  "future",
  "event_contract",
  "index",
  "bond",
  "plate",
  "unknown",
]);

export function resolveWorkspaceProductClass(
  details: MarketSecurityDetails | null | undefined,
  fallbackSecurityType?: string | null,
): WorkspaceProductClass {
  const direct = String(details?.productClass ?? "")
    .trim()
    .toLowerCase() as WorkspaceProductClass;
  if (knownProductClasses.has(direct) && direct !== "unknown") {
    return direct;
  }

  if (details?.future != null) return "future";
  if (details?.option != null) return "option";
  if (details?.warrant != null) {
    const warrantType = String(details.warrant.warrantType ?? "")
      .trim()
      .toUpperCase();
    return warrantType === "BULL" || warrantType === "BEAR"
      ? "cbbc"
      : "warrant";
  }
  if (details?.trust != null) return "fund";
  if (details?.index != null) return "index";
  if (details?.plate != null) return "plate";
  if (details?.equity != null) return "equity";

  const securityType = normalizeInstrumentSecurityType(
    details?.securityType || fallbackSecurityType,
  );
  switch (securityType) {
    case "EQTY":
    case "EQUITY":
    case "STOCK":
      return "equity";
    case "ETF":
    case "FUND":
    case "TRUST":
      return "fund";
    case "DRVT":
    case "OPTION":
      return "option";
    case "WARRANT":
    case "BWRT":
      return "warrant";
    case "FUTURE":
      return "future";
    case "INDEX":
      return "index";
    case "BOND":
      return "bond";
    case "PLATE":
    case "PLATESET":
      return "plate";
    case "EVENT":
    case "EVENTCONTRACT":
      return "event_contract";
    default:
      return "unknown";
  }
}

export function workspaceTabsForProduct(
  market: string,
  productClass: WorkspaceProductClass,
): readonly WorkspaceProductTabDefinition[] {
  if (productClass === "event_contract") {
    return predictionWorkspaceProductTabs;
  }
  if (productClass === "future" || productClass === "unknown") {
    return futureWorkspaceProductTabs;
  }
  const showsWarrants =
    market.trim().toUpperCase() === "HK" &&
    (productClass === "equity" ||
      productClass === "fund" ||
      productClass === "index");
  return showsWarrants
    ? allWorkspaceProductTabs
    : allWorkspaceProductTabs.filter((tab) => tab.value !== "warrants");
}

export function isWorkspaceProductTab(
  value: unknown,
): value is WorkspaceProductTab {
  return [...allWorkspaceProductTabs, ...predictionWorkspaceProductTabs].some(
    (tab) => tab.value === value,
  );
}
