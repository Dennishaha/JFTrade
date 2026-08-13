// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

import {
  featureEntryTitle,
  fetchProductFeature,
  instrumentIDFromFeatureEntry,
  prepareProductFeature,
} from "@/composables/product/productFeatures";
import {
  productFeaturePath,
  queryProductFeature,
} from "@/composables/product/productFeatureApi";
import {
  predictionApi,
  predictionTarget,
} from "@/composables/research/predictionApi";

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
    await expect(
      fetchProductFeature(prepareProductFeature({
        scope: "research",
        family: "rankings",
        market: "US",
      })),
    ).resolves.toEqual(data);

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

  it("routes dynamic product endpoints through their generated operation template", async () => {
    const data = { entries: [], asOf: "now" };
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      text: async () => JSON.stringify({ ok: true, data }),
    });
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      fetchProductFeature(prepareProductFeature({
        scope: "market-feature",
        resource: "instrument-profile",
        instrumentId: "US.AAPL",
        brokerId: "futu",
      })),
    ).resolves.toEqual(data);
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/market-data/instruments/US.AAPL/profile?brokerId=futu",
      expect.objectContaining({ method: "GET" }),
    );

    await expect(
      fetchProductFeature("/api/v1/system/status"),
    ).rejects.toThrow("Product feature request was not prepared");
  });

  it("dispatches every typed product feature scope through its generated route", async () => {
    const data = { entries: [], asOf: "now" };
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      text: async () => JSON.stringify({ ok: true, data }),
    });
    vi.stubGlobal("fetch", fetchMock);

    const requests = [
      { scope: "market-feature" as const, resource: "news" as const, market: "US" },
      { scope: "research" as const, family: "rankings" as const, market: "HK" },
      { scope: "prediction" as const, resource: "categories" as const },
    ];
    for (const request of requests) {
      await expect(queryProductFeature(request)).resolves.toEqual(data);
    }

    expect(productFeaturePath(requests[0]!)).toBe(
      "/api/v1/market-data/news?market=US",
    );
    expect(productFeaturePath(requests[1]!)).toBe(
      "/api/v1/research/rankings?market=HK",
    );
    expect(productFeaturePath(requests[2]!)).toBe(
      "/api/v1/market-data/prediction/categories",
    );
  });

  it("validates prediction targets and preserves typed subscription and quote routes", async () => {
    expect(() =>
      predictionTarget({ scope: "prediction", resource: "event-contracts" }),
    ).toThrow("require eventId");
    expect(() =>
      predictionTarget({ scope: "prediction", resource: "snapshot" }),
    ).toThrow("requires code");

    expect(
      predictionTarget({
        scope: "prediction",
        resource: "event-contracts",
        eventId: "event/one",
        brokerId: "futu",
        accountId: "account-1",
        tradingEnvironment: "REAL",
        category: "sports",
        tag: "live",
        seriesId: "series-1",
        cursor: "next",
        pageSize: 50,
        refresh: true,
      }).path,
    ).toContain(
      "/events/event%2Fone/contracts?brokerId=futu&accountId=account-1&tradingEnvironment=REAL",
    );
    expect(
      predictionTarget({
        scope: "prediction",
        resource: "candle-history",
        code: "EC/HOME",
      }).path,
    ).toBe(
      "/api/v1/market-data/prediction/contracts/EC%2FHOME/candles/history",
    );
    expect(
      predictionTarget({
        scope: "prediction",
        resource: "snapshot",
        code: "EC.HOME",
      }).path,
    ).toContain("/contracts/EC.HOME/snapshot");
    expect(
      predictionTarget({ scope: "prediction", resource: "eligible-events" })
        .path,
    ).toBe("/api/v1/market-data/prediction/combos/eligible-events");

    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      text: async () => JSON.stringify({ ok: true, data: {} }),
    });
    vi.stubGlobal("fetch", fetchMock);
    await predictionApi.acquireSubscription({
      code: "EC/HOME",
      brokerId: "futu",
      accountId: "account-1",
      dataTypes: ["QUOTE"],
    });
    await predictionApi.acquireSubscription({
      code: "EC.AWAY",
      dataTypes: ["ORDER_BOOK"],
    });
    await predictionApi.releaseSubscription("EC/HOME", "lease/one");
    await predictionApi.quoteCombo({
      accountId: "account-1",
      brokerId: "futu",
      legs: [],
      mvc: "1",
      tradingEnvironment: "REAL",
    });

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/market-data/prediction/contracts/EC%2FHOME/subscriptions?brokerId=futu&accountId=account-1",
      expect.objectContaining({ method: "POST" }),
    );
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/market-data/prediction/contracts/EC%2FHOME/subscriptions/lease%2Fone",
      expect.objectContaining({ method: "DELETE" }),
    );
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/market-data/prediction/combos/quotes",
      expect.objectContaining({ method: "POST" }),
    );
  });
});
