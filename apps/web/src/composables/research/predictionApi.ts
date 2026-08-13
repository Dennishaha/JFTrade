import type {
  BrokerFeatureResultDto,
} from "@/contracts";

import {
  apiDeletePath,
  apiGetPath,
  apiPostPath,
  type RequestBodyFor,
  type ResponseDataFor,
} from "@/composables/shared/apiClient";

export type PredictionResource =
  | "categories"
  | "competitions"
  | "series"
  | "events"
  | "event-contracts"
  | "snapshot"
  | "order-book"
  | "candles"
  | "candle-history"
  | "ticks"
  | "milestones"
  | "eligible-events";

export interface PredictionRequest {
  scope: "prediction";
  resource: PredictionResource;
  brokerId?: string;
  accountId?: string;
  tradingEnvironment?: string;
  category?: string;
  tag?: string;
  seriesId?: string;
  eventId?: string;
  code?: string;
  cursor?: string;
  pageSize?: number;
  refresh?: boolean;
}

export interface PredictionSubscriptionRequest {
  code: string;
  brokerId?: string;
  accountId?: string;
  dataTypes: string[];
}

export type PredictionComboQuoteRequest = RequestBodyFor<
  "/api/v1/market-data/prediction/combos/quotes",
  "post"
>;
type PredictionSubscriptionLease = ResponseDataFor<
  "/api/v1/market-data/prediction/contracts/{code}/subscriptions",
  "post"
>;

function queryString(request: PredictionRequest): string {
  const params = new URLSearchParams();
  const values: Array<[string, string | number | boolean | undefined]> = [
    ["brokerId", request.brokerId],
    ["accountId", request.accountId],
    ["tradingEnvironment", request.tradingEnvironment],
    ["category", request.category],
    ["tag", request.tag],
    ["seriesId", request.seriesId],
    ["cursor", request.cursor],
    ["pageSize", request.pageSize],
    ["refresh", request.refresh ? true : undefined],
  ];
  for (const [key, value] of values) {
    if (value !== undefined && String(value).trim() !== "") {
      params.set(key, String(value));
    }
  }
  const query = params.toString();
  return query ? `?${query}` : "";
}

export function predictionTarget(request: PredictionRequest): {
  template:
    | "/api/v1/market-data/prediction/categories"
    | "/api/v1/market-data/prediction/competitions"
    | "/api/v1/market-data/prediction/series"
    | "/api/v1/market-data/prediction/events"
    | "/api/v1/market-data/prediction/events/{eventId}/contracts"
    | "/api/v1/market-data/prediction/contracts/{code}/snapshot"
    | "/api/v1/market-data/prediction/contracts/{code}/order-book"
    | "/api/v1/market-data/prediction/contracts/{code}/candles"
    | "/api/v1/market-data/prediction/contracts/{code}/candles/history"
    | "/api/v1/market-data/prediction/contracts/{code}/ticks"
    | "/api/v1/market-data/prediction/contracts/{code}/milestones"
    | "/api/v1/market-data/prediction/combos/eligible-events";
  path: string;
} {
  const query = queryString(request);
  const base = "/api/v1/market-data/prediction";
  if (request.resource === "event-contracts") {
    if (!request.eventId?.trim()) throw new Error("Prediction contracts require eventId");
    return {
      template: "/api/v1/market-data/prediction/events/{eventId}/contracts",
      path: `${base}/events/${encodeURIComponent(request.eventId)}/contracts${query}`,
    };
  }
  if (["snapshot", "order-book", "candles", "candle-history", "ticks", "milestones"].includes(request.resource)) {
    if (!request.code?.trim()) throw new Error(`Prediction ${request.resource} requires code`);
    const suffix = request.resource === "candle-history" ? "candles/history" : request.resource;
    return {
      template: `/api/v1/market-data/prediction/contracts/{code}/${suffix}` as ReturnType<typeof predictionTarget>["template"],
      path: `${base}/contracts/${encodeURIComponent(request.code)}/${suffix}${query}`,
    };
  }
  if (request.resource === "eligible-events") {
    return {
      template: "/api/v1/market-data/prediction/combos/eligible-events",
      path: `${base}/combos/eligible-events${query}`,
    };
  }
  return {
    template: `${base}/${request.resource}` as ReturnType<typeof predictionTarget>["template"],
    path: `${base}/${request.resource}${query}`,
  };
}

function subscriptionQuery(request: Pick<PredictionSubscriptionRequest, "brokerId" | "accountId">): string {
  const params = new URLSearchParams();
  if (request.brokerId) params.set("brokerId", request.brokerId);
  if (request.accountId) params.set("accountId", request.accountId);
  const query = params.toString();
  return query ? `?${query}` : "";
}

export const predictionApi = {
  async query(request: PredictionRequest): Promise<BrokerFeatureResultDto> {
    const target = predictionTarget(request);
    return apiGetPath(target.template, target.path) as Promise<BrokerFeatureResultDto>;
  },

  acquireSubscription(
    request: PredictionSubscriptionRequest,
  ): Promise<PredictionSubscriptionLease> {
    return apiPostPath(
      "/api/v1/market-data/prediction/contracts/{code}/subscriptions",
      `/api/v1/market-data/prediction/contracts/${encodeURIComponent(request.code)}/subscriptions${subscriptionQuery(request)}`,
      { dataTypes: request.dataTypes },
    );
  },

  async releaseSubscription(code: string, leaseId: string): Promise<void> {
    await apiDeletePath(
      "/api/v1/market-data/prediction/contracts/{code}/subscriptions/{leaseId}",
      `/api/v1/market-data/prediction/contracts/${encodeURIComponent(code)}/subscriptions/${encodeURIComponent(leaseId)}`,
    );
  },

  quoteCombo(request: PredictionComboQuoteRequest): Promise<BrokerFeatureResultDto> {
    return apiPostPath(
      "/api/v1/market-data/prediction/combos/quotes",
      "/api/v1/market-data/prediction/combos/quotes",
      request,
    ) as Promise<BrokerFeatureResultDto>;
  },
};
