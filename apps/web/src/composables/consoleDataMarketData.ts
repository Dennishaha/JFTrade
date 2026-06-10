import { ref, type Ref } from "vue";

import {
  type BrokerOrdersResponse,
  type BrokerPositionsResponse,
  type ExecutionOrdersResponse,
  type MarketDataSubscriptionsResponse,
  type PortfolioPositionsResponse,
  emptyMarketDataSubscriptions,
} from "@/contracts";

import {
  createConsoleDataMarketInstrumentsController,
} from "./consoleDataMarketInstruments";
import {
  createConsoleDataMarketSubscriptionsController,
} from "./consoleDataMarketSubscriptions";
import type { MarketInstrumentReference } from "./consoleDataSystemState";

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
    loadMarketDataSubscriptions,
    loadMarketInstrumentReferences,
    releaseMarketDataSubscription,
    subscribeCurrentMarketData,
    unsubscribeAllMarketData,
  } = marketSubscriptionsController;

  return {
    acquireMarketDataSubscription,
    heartbeatMarketDataConsumer,
    isLoadingMarketData,
    loadMarketDataSubscriptions,
    loadMarketInstrumentReferences,
    marketDataError,
    marketDataSubscriptions,
    marketInstrumentReferences,
    marketInstrumentSearchOptions,
    releaseMarketDataSubscription,
    subscribeCurrentMarketData,
    unsubscribeAllMarketData,
  };
}