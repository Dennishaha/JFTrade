import { afterEach, describe, expect, it } from "vitest";

import {
  brokerProviderCapabilityPresentation,
  brokerProviderOptions,
  resetBrokerProviderSelectionForTests,
  resolveBrokerProviderDisplayName,
  useBrokerProviderSelection,
} from "@/composables/trading/brokerProviderSelection";

afterEach(() => {
  resetBrokerProviderSelectionForTests();
});

describe("broker provider presentation", () => {
  it("uses built-in names when a provider descriptor has no display name", () => {
    const selection = useBrokerProviderSelection();
    selection.brokerDescriptors.value = [
      { id: "yfinance", displayName: "", capabilities: [] },
      { id: "akshare", displayName: "", capabilities: [] },
      { id: "futu", displayName: "", capabilities: [] },
    ];

    expect(resolveBrokerProviderDisplayName("yfinance")).toBe("Yahoo");
    expect(resolveBrokerProviderDisplayName("akshare")).toBe("AKShare");
    expect(resolveBrokerProviderDisplayName("futu")).toBe("Futu OpenD");
    expect(resolveBrokerProviderDisplayName("unknown")).toBe("UNKNOWN");
    expect(resolveBrokerProviderDisplayName("missing")).toBe("MISSING");
  });

  it("keeps a static degraded capability normal for presentation", () => {
    const selection = useBrokerProviderSelection();
    selection.brokerDescriptors.value = [
      {
        id: "yfinance",
        displayName: "",
        capabilities: [
          {
            market: "US",
            supportsQuote: true,
            supportsTrade: false,
            features: [{ id: "market.candles", state: "degraded" }],
          },
        ],
      },
    ];

    expect(brokerProviderOptions("market.candles", "US")[0]).toMatchObject({
      state: "degraded",
      displayState: "available",
      tone: "success",
      label: "Yahoo",
      shortLabel: "Yahoo",
    });
    expect(
      brokerProviderCapabilityPresentation("yfinance", "market.candles", "US"),
    ).toEqual({ displayState: "available", tone: "success" });
  });

  it("surfaces runtime degradation and unavailability separately", () => {
    const selection = useBrokerProviderSelection();
    selection.brokerDescriptors.value = [
      {
        id: "futu",
        displayName: "Futu",
        capabilities: [
          {
            market: "US",
            supportsQuote: true,
            supportsTrade: false,
            features: [{ id: "market.snapshot", state: "available" }],
          },
        ],
      },
    ];
    selection.brokerRuntimeCapabilities.value = [
      {
        brokerId: "futu",
        market: "US",
        featureId: "market.snapshot",
        capability: { id: "market.snapshot", state: "available" },
        evaluation: { state: "degraded", reason: "权限状态待确认" },
      },
    ];

    expect(brokerProviderOptions("market.snapshot", "US")[0]).toMatchObject({
      state: "degraded",
      displayState: "degraded",
      tone: "warning",
    });

    selection.brokerRuntimeCapabilities.value[0]!.evaluation = {
      state: "unavailable",
      reason: "行情权限不可用",
    };
    expect(
      brokerProviderCapabilityPresentation("futu", "market.snapshot", "US"),
    ).toEqual({ displayState: "unavailable", tone: "error" });
  });
});
