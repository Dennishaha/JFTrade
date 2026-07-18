import { describe, expect, it } from "vitest";

import type { MarketSecurityDetails } from "../src/contracts";
import { resolveProductUnderlying } from "../src/composables/productUnderlying";

function details(
  value: Partial<MarketSecurityDetails>,
): MarketSecurityDetails {
  return value as MarketSecurityDetails;
}

describe("product underlying resolution", () => {
  it("uses the current stock but resolves derivative owners", () => {
    expect(
      resolveProductUnderlying("US.AAPL", "equity", null, false),
    ).toEqual({
      instrumentId: "US.AAPL",
      source: "current_instrument",
      pending: false,
    });
    expect(
      resolveProductUnderlying(
        "US.AAPL260117C00200000",
        "option",
        details({
          option: {
            owner: {
              instrumentId: "US.AAPL",
              market: "US",
              symbol: "AAPL",
            },
          } as MarketSecurityDetails["option"],
        }),
        false,
      ),
    ).toEqual({
      instrumentId: "US.AAPL",
      source: "option_owner",
      pending: false,
    });
    expect(
      resolveProductUnderlying(
        "HK.12345",
        "cbbc",
        details({
          warrant: {
            owner: {
              instrumentId: "",
              market: "HK",
              symbol: "00700",
            },
          } as MarketSecurityDetails["warrant"],
        }),
        false,
      ),
    ).toEqual({
      instrumentId: "HK.00700",
      source: "warrant_owner",
      pending: false,
    });
  });

  it("never falls back to an option contract while its owner is unresolved", () => {
    expect(
      resolveProductUnderlying(
        "US.AAPL260117C00200000",
        "option",
        null,
        true,
      ),
    ).toEqual({
      instrumentId: "",
      source: "unresolved",
      pending: true,
    });
    expect(
      resolveProductUnderlying(
        "US.AAPL260117C00200000",
        "option",
        details({ option: {} as MarketSecurityDetails["option"] }),
        false,
      ),
    ).toEqual({
      instrumentId: "",
      source: "unresolved",
      pending: false,
    });
  });

  it("does not resolve an unclassified instrument while identity loads", () => {
    expect(
      resolveProductUnderlying("US.OPTION-PENDING", "unknown", null, true),
    ).toEqual({
      instrumentId: "",
      source: "unresolved",
      pending: true,
    });
  });
});
