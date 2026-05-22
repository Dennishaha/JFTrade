import { computed, type Ref } from "vue";

import {
  type BrokerOrdersResponse,
  type BrokerPositionsResponse,
  type ExecutionOrdersResponse,
  type MarketDataSubscriptionsResponse,
  type PortfolioPositionsResponse,
} from "@jftrade/ui-contracts";

import type { MarketInstrumentReference } from "./consoleDataSystemState";

interface MarketInstrumentInput {
  market?: string | null;
  symbol?: string | null;
}

interface MarketInstrumentSearchSourceInput extends MarketInstrumentInput {
  name?: string | null;
}

export interface MarketInstrumentSearchOption {
  market: string;
  symbol: string;
  instrumentId: string;
  name: string | null;
  label: string;
  lookupValue: string;
  sources: string[];
}

interface ConsoleDataMarketInstrumentsControllerOptions {
  prefs: Ref<{ market: string }>;
  marketDataQueryMarket: Ref<string>;
  selectedBrokerAccount: Ref<{ market?: string | null } | null | undefined>;
  marketInstrumentReferences: Ref<MarketInstrumentReference[]>;
  marketDataSubscriptions: Ref<MarketDataSubscriptionsResponse>;
  portfolioPositions: Ref<PortfolioPositionsResponse>;
  brokerPositions: Ref<BrokerPositionsResponse>;
  brokerOrders: Ref<BrokerOrdersResponse>;
  executionOrders: Ref<ExecutionOrdersResponse>;
}

export function parseMarketInstrumentInput(
  value: string,
  fallbackMarket: string,
): { market: string; symbol: string } | null {
  const normalized = value.trim().toUpperCase();
  if (normalized === "") {
    return null;
  }

  const separator = normalized.includes(":") ? ":" : ".";
  if (normalized.includes(separator)) {
    const [market, symbol] = normalized.split(separator, 2);
    if (market != null && symbol != null && market !== "" && symbol !== "") {
      return { market, symbol };
    }
  }

  const market = fallbackMarket.trim().toUpperCase();
  if (market === "") {
    return null;
  }

  return { market, symbol: normalized };
}

export function normalizeInstrumentParts(
  input: MarketInstrumentInput,
  fallbackMarket?: string,
): { market: string; symbol: string } | null {
  const rawSymbol = input.symbol?.trim().toUpperCase();
  const rawMarket = input.market?.trim().toUpperCase() ?? "";

  if (rawSymbol == null || rawSymbol === "") {
    return null;
  }

  const embeddedSeparator = rawSymbol.includes(":") ? ":" : ".";
  if (rawSymbol.includes(embeddedSeparator)) {
    const [symbolMarket, symbol] = rawSymbol.split(embeddedSeparator, 2);
    if (
      symbolMarket != null &&
      symbol != null &&
      symbolMarket !== "" &&
      symbol !== ""
    ) {
      return {
        market: rawMarket === "" ? symbolMarket : rawMarket,
        symbol,
      };
    }
  }

  const market = rawMarket || fallbackMarket?.trim().toUpperCase() || "";
  if (market === "") {
    return null;
  }

  return { market, symbol: rawSymbol };
}

export function createConsoleDataMarketInstrumentsController(
  options: ConsoleDataMarketInstrumentsControllerOptions,
) {
  const marketInstrumentSearchOptions = computed<MarketInstrumentSearchOption[]>(
    () => {
      const byId = new Map<
        string,
        {
          market: string;
          symbol: string;
          name: string | null;
          sources: Set<string>;
        }
      >();

      function addInstrument(
        input: MarketInstrumentSearchSourceInput,
        source: string,
      ): void {
        const parsed = normalizeInstrumentParts(
          input,
          options.selectedBrokerAccount.value?.market ??
            options.marketDataQueryMarket.value,
        );
        if (parsed == null) {
          return;
        }

        const instrumentId = `${parsed.market}.${parsed.symbol}`;
        const existing = byId.get(instrumentId);
        if (existing == null) {
          byId.set(instrumentId, {
            market: parsed.market,
            symbol: parsed.symbol,
            name: input.name ?? null,
            sources: new Set([source]),
          });
          return;
        }

        if (existing.name == null && input.name != null) {
          existing.name = input.name;
        }
        existing.sources.add(source);
      }

      for (const reference of options.marketInstrumentReferences.value) {
        addInstrument(reference, "reference");
        for (const mapping of reference.brokerMappings) {
          addInstrument(
            {
              market: mapping.brokerMarket,
              symbol: mapping.brokerSymbol,
              name: mapping.displayName ?? reference.name,
            },
            `broker:${mapping.brokerId}`,
          );
        }
      }
      for (const entry of options.marketDataSubscriptions.value.entries) {
        addInstrument(entry, "subscription");
      }
      for (const position of options.portfolioPositions.value.positions) {
        addInstrument(position, "portfolio");
      }
      for (const position of options.brokerPositions.value.positions) {
        addInstrument(position, "broker-position");
      }
      for (const order of options.brokerOrders.value.orders) {
        addInstrument(order, "broker-order");
      }
      for (const order of options.executionOrders.value.orders) {
        addInstrument(order, "execution-order");
      }

      return [...byId.entries()]
        .map(([instrumentId, item]) => {
          const sources = [...item.sources].sort();
          const nameSuffix = item.name == null ? "" : ` · ${item.name}`;
          return {
            market: item.market,
            symbol: item.symbol,
            instrumentId,
            name: item.name,
            label: `${instrumentId}${nameSuffix} · ${sources.join(", ")}`,
            lookupValue: `${item.market}:${item.symbol}`,
            sources,
          };
        })
        .sort((left, right) =>
          left.instrumentId.localeCompare(right.instrumentId),
        );
    },
  );

  function resolveMarketInstrumentInput(
    value: string,
  ): { market: string; symbol: string } | null {
    return parseMarketInstrumentInput(value, options.prefs.value.market);
  }

  return {
    marketInstrumentSearchOptions,
    resolveMarketInstrumentInput,
  };
}