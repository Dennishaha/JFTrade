// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";

import SettingsExchangeCalendarSection from "../src/components/SettingsExchangeCalendarSection.vue";
import { createResponse, flushRequests } from "./helpers";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("SettingsExchangeCalendarSection", () => {
  it("shows market usage first, drills into a selected source, and preserves settings on toggle", async () => {
    const fetchMock = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);

      if (url.includes("/api/v1/settings/exchange-calendars") && init?.method === "PUT") {
        const body = JSON.parse(String(init.body));
        expect(body.exchangeCalendars).toMatchObject({
          autoRefreshEnabled: true,
          errorNotificationsEnabled: false,
          refreshIntervalHours: 24,
          warmupMarkets: ["US", "HK"],
          sourcePolicies: [
            {
              market: "US",
              enabledSourceIds: ["nyse_official"],
              fallbackToBuiltin: true,
            },
          ],
          manualOverrides: [
            {
              market: "US",
              date: "2026-01-02",
              status: "closed",
            },
          ],
        });
        return createResponse({
          exchangeCalendars: {
            ...body.exchangeCalendars,
            errorNotificationsEnabled: false,
          },
        });
      }

      if (url.includes("/api/v1/settings/exchange-calendars")) {
        return createResponse({ exchangeCalendars: buildSettings() });
      }

      if (url.includes("/api/v1/system/exchange-calendars/probe")) {
        expect(url).toContain("/api/v1/system/exchange-calendars/probe/US");
        return createResponse({
          accepted: true,
          checkedAt: "2026-06-24T10:00:00Z",
          healthy: 1,
          failures: 0,
          probeScope: ["US"],
          results: [
            {
              sourceId: "nyse_official",
              market: "US",
              status: "healthy",
              fetchedAt: "2026-06-24T09:59:00Z",
              validUntil: "2026-06-30T00:00:00Z",
              schedulesParsed: 128,
              checksum: "ok-checksum",
            },
          ],
        });
      }

      if (url.includes("/api/v1/system/exchange-calendars/refresh")) {
        expect(url).toContain("/api/v1/system/exchange-calendars/refresh/US");
        return createResponse({
          accepted: true,
          market: "US",
          updated: 1,
          failures: 0,
          warmupMarkets: ["US"],
        });
      }

      if (url.includes("/api/v1/system/exchange-calendars/status")) {
        return createResponse(buildStatus());
      }

      return createResponse({});
    });
    vi.stubGlobal("fetch", fetchMock);

    const wrapper = mount(SettingsExchangeCalendarSection);
    await flushRequests();

    expect(wrapper.get("[data-testid='calendar-market-US']").text()).toContain("使用外部日历");
    expect(wrapper.get("[data-testid='calendar-market-US']").text()).toContain("nyse_official");
    expect(wrapper.get("[data-testid='calendar-market-HK']").text()).toContain("内置兜底");
    expect(wrapper.get("[data-testid='calendar-market-HK']").text()).toContain("正在使用内置规则");

    const hkSource = wrapper.get("[data-testid='calendar-source-nav-hk_gov_1823_ical']");
    expect(hkSource.text()).toContain("异常");
    expect(hkSource.text()).toContain("timeout");
    expect(hkSource.text()).toContain("当前未被市场使用");
    expect(wrapper.text()).toContain("当前未使用外部日历，正在使用内置规则");

    await wrapper.get("[data-testid='calendar-source-nav-nyse_official']").trigger("click");
    await flushRequests();
    expect(wrapper.text()).toContain("正在服务 US");
    expect(wrapper.text()).not.toContain("old timeout");
    expect(wrapper.text()).toContain("2026-01-19 closed / holiday");
    expect(wrapper.text()).not.toContain("2026-05-01 closed / labor day");

    await wrapper.get("[data-testid='calendar-error-notifications-toggle']").setValue(false);
    await flushRequests();
    expect(fetchMock.mock.calls.some((call) => call[1]?.method === "PUT")).toBe(true);
    expect(wrapper.text()).toContain("错误推送已关闭");

    await wrapper.get("[data-testid='calendar-probe']").trigger("click");
    await flushRequests();
    expect(wrapper.text()).toContain("在线探测结果");
    expect(wrapper.text()).toContain("128 条");

    await wrapper.get("[data-testid='calendar-refresh']").trigger("click");
    await flushRequests();
    expect(wrapper.text()).toContain("US：更新 1 个源，失败 0 个源");
  });
});

function buildSettings() {
  return {
    autoRefreshEnabled: true,
    errorNotificationsEnabled: true,
    refreshIntervalHours: 24,
    warmupMarkets: ["US", "HK"],
    sourcePolicies: [
      {
        market: "US",
        enabledSourceIds: ["nyse_official"],
        fallbackToBuiltin: true,
      },
    ],
    manualOverrides: [
      {
        market: "US",
        date: "2026-01-02",
        status: "closed",
      },
    ],
  };
}

function buildStatus() {
  return {
    autoRefreshEnabled: true,
    refreshIntervalHours: 24,
    warmupMarkets: ["US", "HK"],
    markets: [
      {
        market: "US",
        effectiveSource: "nyse_official",
        effectiveMode: "remote_covered_day",
        effectiveReason: "remote_covered_day",
        fallbackChain: ["manual_override", "nyse_official", "builtin_rules"],
        checkedAt: "2026-06-24T10:00:00Z",
      },
      {
        market: "HK",
        effectiveSource: "builtin_rules",
        effectiveMode: "builtin_fallback",
        effectiveReason: "no fresh source covers HK",
        fallbackChain: ["manual_override", "hk_gov_1823_ical", "builtin_rules"],
        checkedAt: "2026-06-24T10:00:00Z",
      },
    ],
    sources: [
      {
        id: "nyse_official",
        kind: "official_html",
        authority: "NYSE",
        markets: ["US"],
        enabled: true,
        healthState: "healthy",
        lastSuccessAt: "2026-06-24T09:00:00Z",
        lastFailureAt: "2026-06-24T08:00:00Z",
        lastError: "old timeout",
        lastSnapshotFetchedAt: "2026-06-24T09:00:00Z",
      },
      {
        id: "hk_gov_1823_ical",
        kind: "official_ical",
        authority: "HK GOV",
        markets: ["HK"],
        enabled: true,
        healthState: "unhealthy",
        lastFailureAt: "2026-06-24T09:30:00Z",
        lastError: "fetch failed: timeout",
        lastProbeStatus: "unhealthy",
        lastProbeError: "timeout",
      },
      {
        id: "builtin_rules",
        kind: "builtin_rules",
        authority: "builtin",
        markets: ["US", "HK"],
        enabled: true,
        healthState: "healthy",
      },
    ],
    snapshots: [
      {
        market: "US",
        sourceId: "nyse_official",
        from: "2026-01-01T00:00:00Z",
        to: "2027-12-31T23:59:59Z",
        fetchedAt: "2026-06-24T09:00:00Z",
        validUntil: "2026-06-30T00:00:00Z",
        schedulesParsed: 128,
        checksum: "checksum-1",
        sampleSchedules: [
          {
            market: "US",
            date: "2026-01-19",
            status: "closed",
            reason: "holiday",
            sourceId: "nyse_official",
          },
        ],
      },
      {
        market: "HK",
        sourceId: "hk_gov_1823_ical",
        from: "2026-01-01T00:00:00Z",
        to: "2027-12-31T23:59:59Z",
        fetchedAt: "2026-06-20T09:00:00Z",
        validUntil: "2026-06-21T00:00:00Z",
        schedulesParsed: 88,
        checksum: "hk-checksum",
        sampleSchedules: [
          {
            market: "HK",
            date: "2026-05-01",
            status: "closed",
            reason: "labor day",
            sourceId: "hk_gov_1823_ical",
          },
        ],
      },
    ],
  };
}
