import { ref, type Ref } from "vue";

import {
  type ExecutionOrdersResponse,
  type MarketDataSubscriptionsResponse,
  emptyMarketDataSubscriptions,
} from "@/types";
import type {
  BrokerOrdersResponse,
  BrokerPositionsResponse,
  PortfolioPositionsResponse,
} from "@/contracts";

import {
  createConsoleDataMarketInstrumentsController,
} from "@/composables/market-data/consoleDataMarketInstruments";
import {
  createConsoleDataMarketSubscriptionsController,
} from "@/composables/market-data/consoleDataMarketSubscriptions";
import type { MarketInstrumentReference } from "@/composables/settings/consoleDataSystemState";

interface CreateConsoleDataMarketDataSliceOptions {
  marketDataQueryMarket: Ref<string>;
  marketDataQuerySymbol: Ref<string>;
  selectedBrokerAccount: Ref<{ market?: string | null } | null | undefined>;
  portfolioPositions: Ref<PortfolioPositionsResponse>;
  brokerPositions: Ref<BrokerPositionsResponse>;
  brokerOrders: Ref<BrokerOrdersResponse>;
  activeExecutionOrders: Ref<ExecutionOrdersResponse>;
}

export function createConsoleDataMarketDataSlice(
  options: CreateConsoleDataMarketDataSliceOptions,
) {
  const marketDataSubscriptions = ref<MarketDataSubscriptionsResponse>(
    emptyMarketDataSubscriptions,
  );
  const marketInstrumentReferences = ref<MarketInstrumentReference[]>([]);
  const isLoadingMarketData = ref(false);
  const marketDataError = ref("");

  const marketInstrumentsController =
    createConsoleDataMarketInstrumentsController({
      marketDataQueryMarket: options.marketDataQueryMarket,
      selectedBrokerAccount: options.selectedBrokerAccount,
      marketInstrumentReferences,
      marketDataSubscriptions,
      portfolioPositions: options.portfolioPositions,
      brokerPositions: options.brokerPositions,
      brokerOrders: options.brokerOrders,
      activeExecutionOrders: options.activeExecutionOrders,
    });
  const { marketInstrumentSearchOptions } = marketInstrumentsController;

  const marketSubscriptionsController =
    createConsoleDataMarketSubscriptionsController({
      marketDataSubscriptions,
      marketInstrumentReferences,
      marketDataQueryMarket: options.marketDataQueryMarket,
      marketDataQuerySymbol: options.marketDataQuerySymbol,
      isLoadingMarketData,
      marketDataError,
    });
  const {
    acquireMarketDataSubscription,
    heartbeatMarketDataConsumer,
    loadMarketInstrumentReferences,
    releaseMarketDataSubscription,
    subscribeCurrentMarketData,
    unsubscribeAllMarketData,
  } = marketSubscriptionsController;

  return {
    acquireMarketDataSubscription,
    heartbeatMarketDataConsumer,
    loadMarketInstrumentReferences,
    marketInstrumentReferences,
    marketInstrumentSearchOptions,
    releaseMarketDataSubscription,
    subscribeCurrentMarketData,
    unsubscribeAllMarketData,
  };
}
