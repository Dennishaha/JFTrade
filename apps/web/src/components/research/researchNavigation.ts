import {
  capabilitySurfaceID,
  type CapabilitySurfaceID,
} from "../../features/capabilitySurfaces";

export type ResearchSection =
  | "market"
  | "screens"
  | "derivatives"
  | "calendar"
  | "macro"
  | "institutions"
  | "industries"
  | "instrument"
  | "prediction";

export interface ResearchOperation {
  value: string;
  label: string;
  path: string;
}

export interface ResearchSectionConfig {
  value: ResearchSection;
  label: string;
  description: string;
  surfaceId: CapabilitySurfaceID;
  capabilities: string[];
  operations: ResearchOperation[];
}

export const RESEARCH_SECTIONS: ResearchSectionConfig[] = [
  {
    value: "market",
    surfaceId: capabilitySurfaceID("research.market"),
    label: "市场",
    description: "指数、榜单与行业概念板块",
    capabilities: ["指数快照", "市场榜单", "板块热力", "板块成分"],
    operations: [
      { value: "top_movers", label: "涨跌榜", path: "/api/v1/research/rankings?market=US&operation=top_movers&pageSize=50" },
      { value: "hot", label: "热门榜", path: "/api/v1/research/rankings?market=US&operation=hot&pageSize=50" },
    ],
  },
  {
    value: "screens",
    surfaceId: capabilitySurfaceID("research.screens"),
    label: "筛选器",
    description: "按财务、行情和技术指标筛选证券",
    capabilities: ["完整因子目录", "筛选预设", "自选结果列"],
    operations: [
      { value: "stock_v2", label: "股票筛选", path: "/api/v1/research/screens" },
    ],
  },
  {
    value: "derivatives",
    surfaceId: capabilitySurfaceID("research.options"),
    label: "衍生品",
    description: "期权事件、期权筛选与港股轮证",
    capabilities: ["IV/HV", "0DTE", "财报期权", "港股轮证"],
    operations: [
      { value: "unusual", label: "期权异动", path: "/api/v1/market-data/options/events?market=US&operation=unusual&pageSize=50" },
      { value: "zero_dte", label: "0DTE", path: "/api/v1/market-data/options/events?market=US&operation=zero_dte&pageSize=50" },
      { value: "earnings", label: "财报期权", path: "/api/v1/market-data/options/events?market=US&operation=earnings&pageSize=50" },
      { value: "seller", label: "卖方筛选", path: "/api/v1/market-data/options/events?market=US&operation=seller&pageSize=50" },
      { value: "option_screen", label: "期权筛选", path: "/api/v1/market-data/options/screens?market=US&operation=screen&pageSize=50" },
      { value: "warrant", label: "港股轮证", path: "/api/v1/market-data/warrants?market=HK&operation=screen&pageSize=50" },
    ],
  },
  {
    value: "calendar",
    surfaceId: capabilitySurfaceID("research.calendar"),
    label: "日历",
    description: "财报、派息、经济事件与 IPO",
    capabilities: ["财报", "派息", "经济事件", "IPO"],
    operations: [
      { value: "earnings", label: "财报日历", path: "/api/v1/research/calendars?market=US&operation=earnings&pageSize=50" },
      { value: "dividends", label: "派息日历", path: "/api/v1/research/calendars?market=US&operation=dividends&pageSize=50" },
      { value: "economic", label: "经济事件", path: "/api/v1/research/calendars?market=US&operation=economic&pageSize=50" },
      { value: "ipos", label: "IPO 日历", path: "/api/v1/research/calendars?market=US&operation=ipos&pageSize=50" },
    ],
  },
  {
    value: "macro",
    surfaceId: capabilitySurfaceID("research.macro"),
    label: "宏观",
    description: "宏观指标、FedWatch 利率与点阵图",
    capabilities: ["宏观指标", "FedWatch", "点阵图"],
    operations: [
      { value: "indicators", label: "宏观指标", path: "/api/v1/research/macro?market=US&operation=indicators&pageSize=50" },
      { value: "fed_target_rate", label: "FedWatch 利率", path: "/api/v1/research/macro?market=US&operation=fed_target_rate&pageSize=50" },
      { value: "fed_dot_plot", label: "点阵图", path: "/api/v1/research/macro?market=US&operation=fed_dot_plot&pageSize=50" },
    ],
  },
  {
    value: "institutions",
    surfaceId: capabilitySurfaceID("research.institutions"),
    label: "机构",
    description: "美港机构资料、持仓与 ARK 动态",
    capabilities: ["机构资料", "持仓变化", "ARK 动态"],
    operations: [
      { value: "list", label: "机构列表", path: "/api/v1/research/institutions?market=US&operation=list&pageSize=50" },
      { value: "holding_changes", label: "持仓变化", path: "/api/v1/research/institutions?market=US&operation=holding_changes&pageSize=50" },
      { value: "ark_fund_holdings", label: "ARK 持仓", path: "/api/v1/research/institutions?market=US&operation=ark_fund_holdings&pageSize=50" },
      { value: "ark_transactions", label: "ARK 动态", path: "/api/v1/research/institutions?market=US&operation=ark_transactions&pageSize=50" },
    ],
  },
  {
    value: "industries",
    surfaceId: capabilitySurfaceID("research.industries"),
    label: "产业链",
    description: "产业链关系、节点与关联证券",
    capabilities: ["产业链", "链路详情", "关联股票"],
    operations: [
      { value: "chains", label: "产业链", path: "/api/v1/research/industries?market=US&operation=chains&pageSize=50" },
    ],
  },
  {
    value: "instrument",
    surfaceId: capabilitySurfaceID("research.market"),
    label: "个股研究",
    description: "当前工作区标的的财务、估值、分析师与股权资料",
    capabilities: ["公司资料", "财务", "估值", "分析师", "股权", "资讯"],
    operations: [
      { value: "profile", label: "公司资料", path: "/api/v1/research/instruments/:instrumentId?operation=profile&pageSize=50" },
      { value: "financials", label: "财务", path: "/api/v1/research/financials/:instrumentId?operation=statements&pageSize=50" },
      { value: "valuation", label: "估值", path: "/api/v1/research/valuation/:instrumentId?operation=detail&pageSize=50" },
      { value: "analyst", label: "分析师", path: "/api/v1/research/analyst/:instrumentId?operation=consensus&pageSize=50" },
      { value: "ownership", label: "股权", path: "/api/v1/research/ownership/:instrumentId?operation=overview&pageSize=50" },
      { value: "corporate_actions", label: "公司行动", path: "/api/v1/research/corporate-actions/:instrumentId?operation=dividends&pageSize=50" },
      { value: "short_interest", label: "卖空数据", path: "/api/v1/research/short-interest/:instrumentId?operation=daily_volume&pageSize=50" },
      { value: "news", label: "资讯", path: "/api/v1/market-data/news?market=US&code=:instrumentId&operation=search&pageSize=30" },
    ],
  },
  {
    value: "prediction",
    surfaceId: capabilitySurfaceID("research.prediction"),
    label: "预测市场",
    description: "分类、赛事、系列、事件与合约",
    capabilities: ["事件发现", "YES/NO 合约", "Parlay RFQ"],
    operations: [
      { value: "categories", label: "事件发现与 Parlay", path: "/api/v1/market-data/prediction/categories?pageSize=50" },
    ],
  },
];

export type MarketView = "home" | "rankings" | "sectors";

export const MARKET_VIEWS: Array<{ value: MarketView; label: string }> = [
  { value: "home", label: "总览" },
  { value: "rankings", label: "榜单" },
  { value: "sectors", label: "板块" },
];

export const MARKET_CODE_OPTIONS: Array<{ value: string; label: string }> = [
  { value: "US", label: "美股" },
  { value: "HK", label: "港股" },
  { value: "CN", label: "沪深" },
];

const INSTRUMENT_FEATURE_IDS: Record<string, string> = {
  profile: "research.instrument",
  financials: "research.financials",
  valuation: "research.valuation",
  analyst: "research.analyst",
  ownership: "research.ownership",
  corporate_actions: "research.corporate_actions",
  short_interest: "research.short_interest",
  news: "research.news",
};

/** Broker runtime capabilities required by the currently visible research view. */
export function researchFeatureIds(
  section: ResearchSection,
  operation: string,
  marketView: MarketView,
): string[] {
  if (section === "market") {
    if (marketView === "home") {
      return ["market.snapshots", "research.rankings", "research.industry"];
    }
    return marketView === "rankings"
      ? ["research.rankings", "market.snapshots"]
      : ["research.industry", "market.snapshots"];
  }
  if (section === "derivatives") {
    return [operation === "warrant"
      ? "derivatives.warrants"
      : operation === "option_screen"
        ? "derivatives.option_screen"
        : "derivatives.option_events"];
  }
  const featureBySection: Partial<Record<ResearchSection, string>> = {
    screens: "research.screen",
    calendar: "research.calendar",
    macro: "research.macro",
    institutions: "research.institutions",
    industries: "research.industry",
    prediction: "prediction.discover",
  };
  if (section === "instrument") {
    return [INSTRUMENT_FEATURE_IDS[operation] ?? "research.instrument"];
  }
  return [featureBySection[section] ?? "market.snapshots"];
}

export function validResearchSection(value: unknown): ResearchSection {
  const candidate = String(value ?? "");
  return RESEARCH_SECTIONS.some((item) => item.value === candidate)
    ? (candidate as ResearchSection)
    : "market";
}

export function validMarketView(value: unknown): MarketView {
  return MARKET_VIEWS.some((item) => item.value === value)
    ? (value as MarketView)
    : "home";
}

export function validMarketCode(value: unknown): string {
  return MARKET_CODE_OPTIONS.some((item) => item.value === value)
    ? String(value)
    : "US";
}
