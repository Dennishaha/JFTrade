import { computed, type Ref } from "vue";

import {
  type BrokerOrdersResponse,
  type BrokerPositionsResponse,
  type ExecutionOrdersResponse,
  type MarketDataSubscriptionsResponse,
  type PortfolioPositionsResponse,
} from "@jftrade/ui-contracts";

import type { MarketInstrumentReference } from "./consoleDataSystemState";
import { resolveInstrumentRef } from "./instrumentRef";

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
  sources: string[];
}

interface ConsoleDataMarketInstrumentsControllerOptions {
  marketDataQueryMarket: Ref<string>;
  selectedBrokerAccount: Ref<{ market?: string | null } | null | undefined>;
  marketInstrumentReferences: Ref<MarketInstrumentReference[]>;
  marketDataSubscriptions: Ref<MarketDataSubscriptionsResponse>;
  portfolioPositions: Ref<PortfolioPositionsResponse>;
  brokerPositions: Ref<BrokerPositionsResponse>;
  brokerOrders: Ref<BrokerOrdersResponse>;
  executionOrders: Ref<ExecutionOrdersResponse>;
}

export function normalizeInstrumentParts(
  input: MarketInstrumentInput,
  fallbackMarket?: string,
): { market: string; symbol: string } | null {
  const resolved = resolveInstrumentRef(
    {
      market: input.market ?? null,
      symbol: input.symbol ?? null,
    },
    fallbackMarket,
  );
  if (resolved == null) {
    return null;
  }
  return { market: resolved.market, symbol: resolved.symbol };
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
            sources,
          };
        })
        .sort((left, right) =>
          left.instrumentId.localeCompare(right.instrumentId),
        );
    },
  );

  return {
    marketInstrumentSearchOptions,
  };
}