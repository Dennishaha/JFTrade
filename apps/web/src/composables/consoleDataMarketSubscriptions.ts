import type { Ref } from "vue";

import { type MarketDataSubscriptionsResponse } from "@/contracts";

import { normalizeKlinePeriod } from "../charting/kline";
import {
  fetchEnvelope,
  fetchEnvelopeWithInit,
} from "./apiClient";
import type {
  MarketInstrumentReference,
  MarketInstrumentReferenceResponse,
} from "./consoleDataSystemState";

type MarketDataChannel = "SNAPSHOT" | "KLINE" | "TICK" | "ORDER_BOOK";

interface CreateConsoleDataMarketSubscriptionsControllerOptions {
  marketDataSubscriptions: Ref<MarketDataSubscriptionsResponse>;
  marketInstrumentReferences: Ref<MarketInstrumentReference[]>;
  marketDataQueryMarket: Ref<string>;
  marketDataQuerySymbol: Ref<string>;
  isLoadingMarketData: Ref<boolean>;
  marketDataError: Ref<string>;
}

function createRandomConsumerSuffix(): string {
  if (
    typeof crypto !== "undefined" &&
    typeof crypto.randomUUID === "function"
  ) {
    return crypto.randomUUID();
  }

  return Math.random().toString(36).slice(2);
}

const consumerWindowSegment = "window";
const pageConsumerInstanceSuffix = createRandomConsumerSuffix();

function normalizeStoredConsumerBase(scope: string, value: string | null): string {
  const trimmed = value?.trim() ?? "";
  if (trimmed === "") {
    return `web:${scope}:${createRandomConsumerSuffix()}`;
  }

  const windowSegmentIndex = trimmed.indexOf(`:${consumerWindowSegment}:`);
  if (windowSegmentIndex >= 0) {
    return trimmed.slice(0, windowSegmentIndex);
  }

  return trimmed;
}

export function createStableWebConsumerId(scope: string): string {
  const storageKey = `jftrade.market-data.consumer.${scope}`;

  if (typeof window === "undefined" || window.sessionStorage == null) {
    const fallback = normalizeStoredConsumerBase(scope, null);
    return `${fallback}:${consumerWindowSegment}:${pageConsumerInstanceSuffix}`;
  }

  const consumerBase = normalizeStoredConsumerBase(
    scope,
    window.sessionStorage.getItem(storageKey),
  );
  if (window.sessionStorage.getItem(storageKey) !== consumerBase) {
    window.sessionStorage.setItem(storageKey, consumerBase);
  }

  return `${consumerBase}:${consumerWindowSegment}:${pageConsumerInstanceSuffix}`;
}

export function createConsoleDataMarketSubscriptionsController(
  options: CreateConsoleDataMarketSubscriptionsControllerOptions,
) {
  async function loadMarketDataSubscriptions(): Promise<void> {
    options.marketDataError.value = "";
    options.isLoadingMarketData.value = true;

    try {
      options.marketDataSubscriptions.value =
        await fetchEnvelope<MarketDataSubscriptionsResponse>(
          "/api/v1/market-data/subscriptions",
        );
    } catch (error) {
      options.marketDataError.value =
        error instanceof Error
          ? error.message
          : "行情订阅加载失败。";
    } finally {
      options.isLoadingMarketData.value = false;
    }
  }

  async function loadMarketInstrumentReferences(
    query = "",
  ): Promise<MarketInstrumentReferenceResponse> {
    const params = new URLSearchParams({
      limit: "50",
      market: options.marketDataQueryMarket.value.trim().toUpperCase() || "HK",
    });
    if (query.trim() !== "") {
      params.set("query", query.trim());
    }

    const response = await fetchEnvelope<MarketInstrumentReferenceResponse>(
      `/api/v1/market-data/instruments?${params.toString()}`,
    );
    const merged = new Map(
      options.marketInstrumentReferences.value.map((entry) => [
        entry.instrumentId,
        entry,
      ]),
    );
    for (const entry of response.entries) {
      merged.set(entry.instrumentId, entry);
    }
    options.marketInstrumentReferences.value = [...merged.values()];
    return response;
  }

  async function acquireMarketDataSubscription(input: {
    consumerId: string;
    market?: string;
    symbol?: string;
    channel?: MarketDataChannel;
    interval?: string;
  }): Promise<void> {
    const market = (input.market ?? options.marketDataQueryMarket.value)
      .trim()
      .toUpperCase();
    const symbol = (input.symbol ?? options.marketDataQuerySymbol.value)
      .trim()
      .toUpperCase();

    options.marketDataError.value = "";

    if (market === "" || symbol === "") {
      options.marketDataError.value =
        "申请实时订阅前请填写市场和标的。";
      return;
    }

    options.isLoadingMarketData.value = true;

    try {
      options.marketDataSubscriptions.value =
        await fetchEnvelopeWithInit<MarketDataSubscriptionsResponse>(
          "/api/v1/market-data/subscriptions",
          {
            method: "POST",
            headers: {
              "Content-Type": "application/json",
            },
            body: JSON.stringify({
              consumerId: input.consumerId,
              market,
              symbol,
              channel: input.channel ?? "SNAPSHOT",
              ...(input.interval == null
                ? {}
                : { interval: normalizeKlinePeriod(input.interval) }),
            }),
          },
        );
    } catch (error) {
      options.marketDataError.value =
        error instanceof Error
          ? error.message
          : "行情订阅申请失败。";
    } finally {
      options.isLoadingMarketData.value = false;
    }
  }

  async function releaseMarketDataSubscription(input: {
    consumerId: string;
    market?: string;
    symbol?: string;
    channel?: MarketDataChannel;
    interval?: string;
    keepalive?: boolean;
  }): Promise<void> {
    const market = (input.market ?? options.marketDataQueryMarket.value)
      .trim()
      .toUpperCase();
    const symbol = (input.symbol ?? options.marketDataQuerySymbol.value)
      .trim()
      .toUpperCase();

    if (market === "" || symbol === "") {
      return;
    }

    try {
      options.marketDataSubscriptions.value =
        await fetchEnvelopeWithInit<MarketDataSubscriptionsResponse>(
          "/api/v1/market-data/subscriptions/release",
          {
            method: "POST",
            headers: {
              "Content-Type": "application/json",
            },
            body: JSON.stringify({
              consumerId: input.consumerId,
              market,
              symbol,
              channel: input.channel ?? "SNAPSHOT",
              ...(input.interval == null
                ? {}
                : { interval: normalizeKlinePeriod(input.interval) }),
            }),
            keepalive: input.keepalive ?? false,
          },
        );
    } catch (error) {
      options.marketDataError.value =
        error instanceof Error
          ? error.message
          : "行情订阅释放失败。";
    }
  }

  async function heartbeatMarketDataConsumer(
    consumerId: string,
  ): Promise<void> {
    if (consumerId.trim() === "") {
      return;
    }

    try {
      options.marketDataSubscriptions.value =
        await fetchEnvelopeWithInit<MarketDataSubscriptionsResponse>(
          "/api/v1/market-data/subscriptions/heartbeat",
          {
            method: "POST",
            headers: {
              "Content-Type": "application/json",
            },
            body: JSON.stringify({ consumerId }),
          },
        );
    } catch (error) {
      options.marketDataError.value =
        error instanceof Error
          ? error.message
          : "行情订阅心跳失败。";
    }
  }

  async function subscribeCurrentMarketData(): Promise<void> {
    await acquireMarketDataSubscription({
      consumerId: "web:manual-market-data",
    });
  }

  async function unsubscribeAllMarketData(): Promise<void> {
    options.marketDataError.value = "";
    options.isLoadingMarketData.value = true;

    try {
      options.marketDataSubscriptions.value =
        await fetchEnvelopeWithInit<MarketDataSubscriptionsResponse>(
          "/api/v1/market-data/subscriptions",
          {
            method: "DELETE",
          },
        );
    } catch (error) {
      options.marketDataError.value =
        error instanceof Error
          ? error.message
          : "取消行情订阅失败。";
    } finally {
      options.isLoadingMarketData.value = false;
    }
  }

  return {
    acquireMarketDataSubscription,
    heartbeatMarketDataConsumer,
    loadMarketDataSubscriptions,
    loadMarketInstrumentReferences,
    releaseMarketDataSubscription,
    subscribeCurrentMarketData,
    unsubscribeAllMarketData,
  };
}
