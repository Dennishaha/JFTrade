import type { Ref } from "vue";

import { type MarketDataSubscriptionsResponse } from "@jftrade/ui-contracts";

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

export function createStableWebConsumerId(scope: string): string {
  const storageKey = `jftrade.market-data.consumer.${scope}`;
  const fallback = `web:${scope}:${createRandomConsumerSuffix()}`;

  if (typeof window === "undefined" || window.sessionStorage == null) {
    return fallback;
  }

  const existing = window.sessionStorage.getItem(storageKey);
  if (existing != null && existing.trim() !== "") {
    return existing;
  }

  window.sessionStorage.setItem(storageKey, fallback);
  return fallback;
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
          : "Failed to load market data subscriptions.";
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
        "Market and symbol are required to acquire a realtime subscription.";
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
          : "Failed to acquire market data subscription.";
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
          : "Failed to release market data subscription.";
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
          : "Failed to heartbeat market data subscription.";
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
          : "Failed to cancel market data subscriptions.";
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