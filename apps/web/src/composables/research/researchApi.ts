import type { BrokerFeatureResultDto } from "@/contracts";

import { apiGetPath, type ApiGetPath } from "@/composables/shared/apiClient";

export type ResearchFamily =
  | "calendar"
  | "industries"
  | "institutions"
  | "macro"
  | "rankings"
  | "screens"
  | "instrument"
  | "financials"
  | "valuation"
  | "analyst"
  | "ownership"
  | "corporate-actions"
  | "short-interest"
  | "technical-indicators";

export interface ResearchRequest {
  scope: "research";
  family: ResearchFamily;
  brokerId?: string;
  accountId?: string;
  tradingEnvironment?: string;
  market?: string;
  instrumentId?: string;
  operation?: string;
  direction?: string;
  sort?: string;
  date?: string;
  beginDate?: string;
  endDate?: string;
  institutionId?: string;
  indicatorId?: string;
  chainId?: string;
  plateId?: string;
  plateType?: string;
  holdingType?: number;
  cycleType?: number;
  stockScope?: string;
  marketCapMin?: number;
  marketCapMax?: number;
  optionVolumeMin?: number;
  optionVolumeMax?: number;
  ivMin?: number;
  ivMax?: number;
  ivRankMin?: number;
  ivRankMax?: number;
  ivPercentileMin?: number;
  ivPercentileMax?: number;
  cursor?: string;
  pageSize?: number;
  refresh?: boolean;
}

const collectionRoutes = {
  calendar: "/api/v1/research/calendars",
  industries: "/api/v1/research/industries",
  institutions: "/api/v1/research/institutions",
  macro: "/api/v1/research/macro",
  rankings: "/api/v1/research/rankings",
  screens: "/api/v1/research/screens",
} as const satisfies Partial<Record<ResearchFamily, ApiGetPath>>;

const instrumentRoutes = {
  instrument: "/api/v1/research/instruments/{instrumentId}",
  financials: "/api/v1/research/financials/{instrumentId}",
  valuation: "/api/v1/research/valuation/{instrumentId}",
  analyst: "/api/v1/research/analyst/{instrumentId}",
  ownership: "/api/v1/research/ownership/{instrumentId}",
  "corporate-actions": "/api/v1/research/corporate-actions/{instrumentId}",
  "short-interest": "/api/v1/research/short-interest/{instrumentId}",
  "technical-indicators": "/api/v1/research/technical-indicators/{instrumentId}",
} as const satisfies Partial<Record<ResearchFamily, ApiGetPath>>;

function queryString(request: ResearchRequest): string {
  const params = new URLSearchParams();
  const values: Array<[string, string | number | boolean | undefined]> = [
    ["accountId", request.accountId],
    ["tradingEnvironment", request.tradingEnvironment],
    ["market", request.market],
    ["operation", request.operation],
    ["direction", request.direction],
    ["sort", request.sort],
    ["date", request.date],
    ["beginDate", request.beginDate],
    ["endDate", request.endDate],
    ["institutionId", request.institutionId],
    ["indicatorId", request.indicatorId],
    ["chainId", request.chainId],
    ["plateId", request.plateId],
    ["instrumentId", request.family === "industries" ? request.instrumentId : undefined],
    ["plateType", request.plateType],
    ["holdingType", request.holdingType],
    ["cycleType", request.cycleType],
    ["stockScope", request.stockScope],
    ["marketCapMin", request.marketCapMin],
    ["marketCapMax", request.marketCapMax],
    ["optionVolumeMin", request.optionVolumeMin],
    ["optionVolumeMax", request.optionVolumeMax],
    ["ivMin", request.ivMin],
    ["ivMax", request.ivMax],
    ["ivRankMin", request.ivRankMin],
    ["ivRankMax", request.ivRankMax],
    ["ivPercentileMin", request.ivPercentileMin],
    ["ivPercentileMax", request.ivPercentileMax],
    ["cursor", request.cursor],
    ["pageSize", request.pageSize],
    ["refresh", request.refresh ? true : undefined],
    ["brokerId", request.brokerId],
  ];
  for (const [key, value] of values) {
    if (value !== undefined && String(value).trim() !== "") {
      params.set(key, String(value));
    }
  }
  const query = params.toString();
  return query ? `?${query}` : "";
}

export function researchTarget(request: ResearchRequest): {
  template: ApiGetPath;
  path: string;
} {
  const collection = collectionRoutes[
    request.family as keyof typeof collectionRoutes
  ];
  if (collection != null) {
    return { template: collection, path: `${collection}${queryString(request)}` };
  }
  const template = instrumentRoutes[
    request.family as keyof typeof instrumentRoutes
  ];
  const instrumentId = request.instrumentId?.trim();
  if (template == null || !instrumentId) {
    throw new Error(`Research family ${request.family} requires instrumentId`);
  }
  const family = request.family === "instrument" ? "instruments" : request.family;
  return {
    template,
    path: `/api/v1/research/${family}/${encodeURIComponent(instrumentId)}${queryString(request)}`,
  };
}

export const researchApi = {
  async query(request: ResearchRequest): Promise<BrokerFeatureResultDto> {
    const target = researchTarget(request);
    return apiGetPath(target.template, target.path) as Promise<BrokerFeatureResultDto>;
  },
};
