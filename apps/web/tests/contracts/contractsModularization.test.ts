import { describe, expect, expectTypeOf, it } from "vitest";

import {
  architectureCards,
  emptyBrokerRuntime,
  emptySystemStatus,
  type BrokerMarketCapability,
  type BrokerReadFeatureCapability,
  type BrokerReadFeatureKey,
} from "@/types";
import type {
  BacktestSyncRequestDto,
  BrokerDescriptorDto,
  DataManagementOverviewResponse,
  ExecutionSettingsResponse,
  ExecutionOrdersDto,
  MarketDataDepthDto,
  NormalizeInstrumentRequest,
  ObservabilityEventDto,
  PluginCatalogDto,
  RealTradeRiskSnapshot,
  ResearchScreenPresetDto,
  StrategyDefinitionDto,
  SystemStatusResponseDto,
  WatchlistGroupDto,
  WebSession,
} from "@/contracts";
import type { components } from "@/generated/openapi";
import type { RequestBodyFor } from "@/composables/shared/apiClient";

describe("contract and view-model boundaries", () => {
  it("keeps runtime view-model exports available through @/types", () => {
    expect(architectureCards).not.toHaveLength(0);
    expect(emptySystemStatus.name).toBe("JFTrade");
    expect(emptyBrokerRuntime.session.connectivity).toBe("disconnected");
  });

  it("keeps broker readFeatures optional and individually partial", () => {
    const quoteOnly: BrokerMarketCapability = {
      market: "US",
      supportsQuote: true,
      supportsTrade: false,
    };
    const fundsOnly: BrokerMarketCapability = {
      market: "HK",
      supportsQuote: true,
      supportsTrade: true,
      readFeatures: {
        funds: {
          supportedEnvironments: ["SIMULATE"],
        },
      },
    };

    expect(quoteOnly.readFeatures).toBeUndefined();
    expect(fundsOnly.readFeatures?.funds?.supportedEnvironments).toEqual([
      "SIMULATE",
    ]);
    expectTypeOf(fundsOnly.readFeatures).toEqualTypeOf<
      | Partial<Record<BrokerReadFeatureKey, BrokerReadFeatureCapability>>
      | undefined
    >();
  });

  it("exports wire-equivalent DTOs from generated schema aliases", () => {
    expectTypeOf<NormalizeInstrumentRequest>().toEqualTypeOf<
      components["schemas"]["marketdata.NormalizeInstrumentRequest"]
    >();
    expectTypeOf<ExecutionSettingsResponse>().toEqualTypeOf<
      components["schemas"]["jftsettings.ExecutionSettings"]
    >();
    expectTypeOf<RealTradeRiskSnapshot>().toEqualTypeOf<
      components["schemas"]["trading.RealTradeRiskSnapshot"]
    >();
    expectTypeOf<WebSession>().toEqualTypeOf<
      components["schemas"]["webaccess.WebSessionData"]
    >();
    expectTypeOf<BacktestSyncRequestDto>().toEqualTypeOf<
      components["schemas"]["backtest.SyncRequest"]
    >();
    expectTypeOf<BrokerDescriptorDto>().toEqualTypeOf<
      components["schemas"]["broker.Descriptor"]
    >();
    expectTypeOf<DataManagementOverviewResponse>().toEqualTypeOf<
      components["schemas"]["settings.DataManagementOverviewResponse"]
    >();
    expectTypeOf<ExecutionOrdersDto>().toEqualTypeOf<
      components["schemas"]["trading.ExecutionOrders"]
    >();
    expectTypeOf<MarketDataDepthDto>().toEqualTypeOf<
      components["schemas"]["marketdata.DepthData"]
    >();
    expectTypeOf<ObservabilityEventDto>().toEqualTypeOf<
      components["schemas"]["observability.Event"]
    >();
    expectTypeOf<PluginCatalogDto>().toEqualTypeOf<
      components["schemas"]["strategy.PluginCatalog"]
    >();
    expectTypeOf<ResearchScreenPresetDto>().toEqualTypeOf<
      components["schemas"]["research.ResearchScreenPreset"]
    >();
    expectTypeOf<StrategyDefinitionDto>().toEqualTypeOf<
      components["schemas"]["strategy.Definition"]
    >();
    expectTypeOf<SystemStatusResponseDto>().toEqualTypeOf<
      components["schemas"]["system.SystemStatusResponse"]
    >();
    expectTypeOf<WatchlistGroupDto>().toEqualTypeOf<
      components["schemas"]["watchlist.WatchlistGroup"]
    >();
    expectTypeOf<
      RequestBodyFor<"/api/v1/adk/chat/stream", "post">
    >().toEqualTypeOf<
      components["schemas"]["assistant.ADKChatRequest"]
    >();
  });
});
