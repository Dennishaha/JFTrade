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
  ExecutionSettingsResponse,
  NormalizeInstrumentRequest,
  RealTradeRiskSnapshot,
  WebSession,
} from "@/contracts";
import type { components } from "@/generated/openapi";
import type { RequestBodyFor } from "@/composables/apiClient";

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
      components["schemas"]["servercore.WebSessionData"]
    >();
    expectTypeOf<
      RequestBodyFor<"/api/v1/adk/chat/stream", "post">
    >().toEqualTypeOf<
      components["schemas"]["assistant.ADKChatRequest"]
    >();
  });
});
