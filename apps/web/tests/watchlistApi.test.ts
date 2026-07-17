// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

import {
  commitWatchlistImport,
  createWatchlistGroup,
  deleteWatchlistBinding,
  deleteWatchlistGroup,
  getWatchlistMembership,
  getWatchlistQuotes,
  listWatchlistBindings,
  listWatchlistGroups,
  listWatchlistImportRuns,
  listWatchlistItems,
  listWatchlistSourceGroups,
  listWatchlistSources,
  previewWatchlistImport,
  replaceWatchlistMembership,
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

  it("normalizes create, paged item, and membership mutation contracts including encoded identifiers", async () => {
    const fetchMock = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/api/v1/watchlist/groups") && init?.method === "POST") {
        return ok({
          groupId: "g/value",
          name: "价值观察",
          isDefault: false,
          protected: true,
          revision: 2,
          itemCount: 4,
          createdAt: "2026-07-16T00:00:00Z",
          updatedAt: "2026-07-16T01:00:00Z",
        });
      }
      if (url.includes("/memberships") && init?.method === "PUT") {
        return ok({ instrumentId: "US.BRK.B", revision: 5, groups: [{ groupId: "g/value", name: "价值观察" }] });
      }
      if (url.includes("/items?")) {
        return ok({
          items: [
            { instrumentId: "", market: "US", symbol: "DROP" },
            {
              instrumentId: "US.BRK.B",
              market: "US",
              symbol: "BRK.B",
              groupIds: ["g/value"],
              groups: [{ groupId: "wrong", name: "自选" }],
              sourceIds: ["futu:default"],
            },
          ],
          nextCursor: "cursor-2",
        });
      }
      return ok({ deleted: true });
    });
    vi.stubGlobal("fetch", fetchMock);

    await expect(createWatchlistGroup("价值观察")).resolves.toMatchObject({
      id: "g/value",
      protected: true,
      itemCount: 4,
    });
    await expect(listWatchlistItems({
      groupId: "g/value",
      cursor: "cursor 1",
      limit: 25,
      query: "  Berkshire  ",
      market: "us",
    })).resolves.toEqual({
      items: [{
        instrumentId: "US.BRK.B",
        market: "US",
        symbol: "BRK.B",
        groupIds: ["g/value"],
        groupNames: ["自选"],
        sources: [{ sourceId: "futu:default" }],
      }],
      nextCursor: "cursor-2",
    });
    await expect(replaceWatchlistMembership("us", "brk.b", {
      groupIds: ["g/value"],
      newGroupNames: ["价值观察"],
      expectedRevision: 4,
    })).resolves.toMatchObject({ instrumentId: "US.BRK.B", groupIds: ["g/value"] });
    await expect(deleteWatchlistGroup("g/value id")).resolves.toBeUndefined();

    const urls = fetchMock.mock.calls.map((call) => String(call[0]));
    expect(urls.some((url) => url.includes("groupId=g%2Fvalue") && url.includes("cursor=cursor+1") && url.includes("query=Berkshire") && url.includes("market=US"))).toBe(true);
    expect(urls.some((url) => url.includes("/instruments/US/BRK.B/memberships"))).toBe(true);
    expect(urls.some((url) => url.endsWith("/groups/g%2Fvalue%20id"))).toBe(true);
  });

  it("keeps empty quote calls local and maps auxiliary import/source contracts faithfully", async () => {
    const fetchMock = vi.fn(async (input: string | URL | Request) => {
      const url = String(input);
      if (url.endsWith("/sources")) {
        return ok({
          sources: [
            { sourceId: "fallback", broker: "Futu", status: "" },
            { sourceId: "bad", sourceName: "ignored", status: "READY", error: "权限不足" },
            { sourceId: "", displayName: "drop" },
          ],
        });
      }
      if (url.includes("/sources/fallback/groups")) {
        return ok({ groups: [
          { remoteGroupId: "system", name: "全部", type: "system", memberCount: 3 },
          { remoteGroupId: "", name: "drop" },
          { remoteGroupId: "missing-name", name: "" },
        ] });
      }
      if (url.endsWith("/bindings")) {
        return ok({ bindings: [
          { bindingId: "binding-1", sourceId: "fallback", remoteGroupId: "system", remoteName: "全部", localGroupId: "g-1" },
          { bindingId: "", sourceId: "drop" },
        ] });
      }
      if (url.endsWith("/imports/preview")) {
        return ok({
          previewId: "preview-1",
          sourceId: "fallback",
          remoteGroupName: "全部",
          newGroupName: "新分组",
          added: [{ instrumentId: "US.AAPL", name: "Apple", selected: true }],
          unchanged: [],
          localOnly: [{ instrumentId: "HK.00700" }],
          expiresAt: "2026-07-16T02:00:00Z",
          remoteHash: "hash",
          localGroupRevision: 8,
        });
      }
      if (url.endsWith("/import-runs")) {
        return ok({ items: [{
          runId: "run-1",
          previewId: "preview-1",
          sourceId: "fallback",
          remoteGroupName: "全部",
          localGroupId: "g-1",
          addedCount: 2,
          removedCount: 1,
          unchangedCount: 3,
          status: "completed",
          createdAt: "2026-07-16T00:00:00Z",
          completedAt: "2026-07-16T00:01:00Z",
        }] });
      }
      if (url.endsWith("/quotes/batch")) {
        return ok({
          quotes: [
            { instrumentId: "", price: 1 },
            { instrumentId: "US.AAPL", name: "Apple", type: "EQUITY", price: 220, previousClose: 210, change: 10, changePercent: 4.76, session: "REGULAR", observedAt: "now", updateTime: "then", source: "futu", extended: { after: { price: 221 }, overnight: { price: 219 } } },
          ],
          errors: [{ instrumentId: "", message: "drop" }, { instrumentId: "US.MSFT" }],
        });
      }
      return ok({ deleted: true });
    });
    vi.stubGlobal("fetch", fetchMock);

    const callsBeforeEmpty = fetchMock.mock.calls.length;
    await expect(getWatchlistQuotes([])).resolves.toMatchObject({ quotes: [], errors: [] });
    expect(fetchMock.mock.calls).toHaveLength(callsBeforeEmpty);
    await expect(getWatchlistQuotes(["US.AAPL"])).resolves.toMatchObject({
      quotes: [{ instrumentId: "US.AAPL", afterHours: { price: 221 }, overnight: { price: 219 } }],
      errors: [{ instrumentId: "US.MSFT", message: "行情快照不可用" }],
    });
    await expect(listWatchlistSources()).resolves.toMatchObject([
      { id: "fallback", displayName: "Futu", available: true },
      { id: "bad", available: false, message: "权限不足" },
    ]);
    await expect(listWatchlistSourceGroups("fallback")).resolves.toEqual([
      { remoteGroupId: "system", name: "全部", type: "system", system: true, memberCount: 3 },
    ]);
    await expect(listWatchlistBindings()).resolves.toEqual([
      { id: "binding-1", sourceId: "fallback", remoteGroupId: "system", remoteGroupName: "全部", localGroupId: "g-1" },
    ]);
    await expect(previewWatchlistImport({
      sourceId: "fallback",
      remoteGroupId: "system",
      remoteGroupName: "全部",
      newGroupName: "新分组",
    })).resolves.toMatchObject({
      id: "preview-1",
      localGroupName: "新分组",
      localRevision: 8,
      added: [{ selected: true }],
    });
    await expect(listWatchlistImportRuns()).resolves.toMatchObject([
      { id: "run-1", deletedCount: 1, unchangedCount: 3 },
    ]);
    await expect(deleteWatchlistBinding("binding / 1")).resolves.toBeUndefined();
    expect(fetchMock.mock.calls.some((call) => String(call[0]).includes("bindingId=binding%20%2F%201"))).toBe(true);
  });
});
