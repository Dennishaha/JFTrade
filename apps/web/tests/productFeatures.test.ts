// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

import {
  featureEntryTitle,
  fetchProductFeature,
  instrumentIDFromFeatureEntry,
} from "../src/composables/productFeatures";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("productFeatures", () => {
  it("fetches through the shared envelope client and recognizes normalized identities", async () => {
    const data = { entries: [], asOf: "now" };
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({
        ok: true,
        text: async () => JSON.stringify({ ok: true, data }),
      }),
    );
    await expect(fetchProductFeature("/api/product")).resolves.toEqual(data);

    expect(instrumentIDFromFeatureEntry({ instrumentId: "us.aapl" })).toBe(
      "US.AAPL",
    );
    expect(instrumentIDFromFeatureEntry({ code: "hk.00700" })).toBe("HK.00700");
    expect(
      instrumentIDFromFeatureEntry({
        security: { market: "US", code: "MSFT" },
      }),
    ).toBe("US.MSFT");
    expect(instrumentIDFromFeatureEntry({ security: "bad" })).toBeNull();
    expect(instrumentIDFromFeatureEntry({ security: { code: "AAPL" } })).toBeNull();

    expect(featureEntryTitle({ name: "Apple" }, 0)).toBe("Apple");
    expect(featureEntryTitle({ title: "News" }, 0)).toBe("News");
    expect(featureEntryTitle({ instrumentId: "US.AAPL" }, 0)).toBe("US.AAPL");
    expect(featureEntryTitle({ name: "  ", code: "AAPL" }, 0)).toBe("AAPL");
    expect(featureEntryTitle({}, 4)).toBe("结果 5");
  });
});
