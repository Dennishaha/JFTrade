// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

import {
  commitWatchlistImport,
  getWatchlistMembership,
  getWatchlistQuotes,
  listWatchlistGroups,
  listWatchlistItems,
  listWatchlistSourceGroups,
  listWatchlistSources,
  updateWatchlistGroup,
} from "../src/composables/watchlistApi";

function ok(data: unknown): Response {
  return new Response(
    JSON.stringify({ ok: true, data, timestamp: "2026-07-11T00:00:00Z" }),
    { status: 200, headers: { "Content-Type": "application/json" } },
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("watchlistApi", () => {
  it("normalizes generated watchlist transport fields for the UI", async () => {
    const fetchMock = vi.fn(async (input: string | URL | Request) => {
      const url = String(input);
      if (url.endsWith("/groups")) {
        return ok({ groups: [{ groupId: "g-1", name: "自选股", revision: 4 }] });
      }
      if (url.includes("/items")) {
        return ok({
          items: [{
            instrumentId: "HK.00700",
            market: "HK",
            symbol: "00700",
            name: "腾讯控股",
            type: "EQUITY",
            sourceIds: ["futu:default"],
            groups: [{ groupId: "g-1", name: "自选股" }],
          }],
        });
      }
      return ok({
        instrumentId: "HK.00700",
        revision: 7,
        groups: [{ groupId: "g-1", name: "自选股" }],
      });
    });
    vi.stubGlobal("fetch", fetchMock);

    await expect(listWatchlistGroups()).resolves.toMatchObject([
      { id: "g-1", name: "自选股", revision: 4 },
    ]);
    await expect(listWatchlistItems()).resolves.toMatchObject({
      items: [{
        instrumentId: "HK.00700",
        securityType: "EQUITY",
        groupIds: ["g-1"],
        sources: [{ sourceId: "futu:default" }],
      }],
    });
    await expect(getWatchlistMembership("HK", "00700")).resolves.toMatchObject({
      revision: 7,
      groupIds: ["g-1"],
    });
  });

  it("normalizes source status, remote system groups and extended quotes", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: string | URL | Request) => {
        const url = String(input);
        if (url.endsWith("/sources")) {
          return ok({
            sources: [
              { sourceId: "futu:default", displayName: "富途 OpenD", status: "ready" },
              { sourceId: "futu:offline", displayName: "离线连接", status: "unavailable", error: "OpenD 未连接" },
            ],
          });
        }
        if (url.includes("/sources/futu%3Adefault/groups")) {
          return ok({ groups: [{ remoteGroupId: "all", name: "全部", type: "SYSTEM", ambiguous: false }] });
        }
        return ok({
          quotes: [{
            instrumentId: "US.AAPL",
            price: 210,
            extended: { pre: { price: 211, observedAt: "2026-07-11T00:00:00Z" } },
          }],
          errors: [],
          observedAt: "2026-07-11T00:00:00Z",
        });
      }),
    );

    const sources = await listWatchlistSources();
    expect(sources.map((source) => [source.id, source.available])).toEqual([
      ["futu:default", true],
      ["futu:offline", false],
    ]);
    await expect(listWatchlistSourceGroups("futu:default")).resolves.toMatchObject([
      { remoteGroupId: "all", system: true },
    ]);
    await expect(getWatchlistQuotes(["US.AAPL"])).resolves.toMatchObject({
      quotes: [{ instrumentId: "US.AAPL", preMarket: { price: 211 } }],
    });
  });

  it("maps selected local-only removals to the commit transport contract", async () => {
    const fetchMock = vi.fn(async () =>
      ok({ runId: "run-1", sourceId: "futu:default", status: "completed", removedCount: 1 }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      commitWatchlistImport("preview-1", {
        deleteLocalOnlyInstrumentIds: ["HK.00005"],
      }),
    ).resolves.toMatchObject({ id: "run-1", deletedCount: 1 });
    expect(JSON.parse(fetchMock.mock.calls[0]?.[1]?.body as string)).toEqual({
      deleteInstrumentIds: ["HK.00005"],
    });
  });

  it("uses the shared typed client for dynamic PATCH operations", async () => {
    const fetchMock = vi.fn(async () =>
      ok({ groupId: "g-tech", name: "科技观察", revision: 3 }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      updateWatchlistGroup("g-tech", {
        name: "科技观察",
        expectedRevision: 2,
      }),
    ).resolves.toMatchObject({
      id: "g-tech",
      name: "科技观察",
      revision: 3,
    });
    expect(fetchMock.mock.calls[0]?.[1]).toMatchObject({
      method: "PATCH",
      credentials: "include",
    });
    expect(JSON.parse(fetchMock.mock.calls[0]?.[1]?.body as string)).toEqual({
      name: "科技观察",
      expectedRevision: 2,
    });
  });
});
