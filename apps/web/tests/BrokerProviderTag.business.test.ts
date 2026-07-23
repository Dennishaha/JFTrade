// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";

const apiMocks = vi.hoisted(() => ({
  fetchEnvelope: vi.fn(),
}));

vi.mock("../src/composables/apiClient", () => ({
  fetchEnvelope: apiMocks.fetchEnvelope,
}));

import BrokerProviderTag from "../src/components/shared/BrokerProviderTag.vue";
import {
  brokerCapabilitySummary,
  brokerProviderOptions,
  brokerSupportedChartPeriods,
  configureBrokerProviderDefaults,
  logicalCapabilityMarkets,
  resetBrokerProviderSelectionForTests,
  useBrokerProviderSelection,
  withBrokerProvider,
} from "../src/composables/brokerProviderSelection";
import { flushPromises, productGlobalStubs } from "./productTestUtils";

const capabilities = {
  brokers: [
    {
      id: "futu",
      displayName: "Futu OpenAPI via OpenD",
      securityFirm: "Moomoo US",
      capabilities: [
        {
          market: "US",
          supportsQuote: true,
          supportsTrade: true,
          features: [
            {
              id: "research.news",
              markets: ["US"],
              state: "available",
            },
          ],
        },
      ],
    },
    {
      id: "alpha",
      displayName: "Alpha Broker",
      securityFirm: "Alpha Securities",
      capabilities: [
        {
          market: "US",
          supportsQuote: true,
          supportsTrade: false,
          features: [
            {
              id: "research.news",
              markets: ["US"],
              state: "degraded",
              reason: "延迟行情",
            },
          ],
        },
      ],
    },
    {
      id: "blocked",
      displayName: "Blocked Broker",
      capabilities: [
        {
          market: "US",
          supportsQuote: false,
          supportsTrade: false,
          features: [],
        },
      ],
    },
  ],
};

afterEach(() => {
  apiMocks.fetchEnvelope.mockReset();
  resetBrokerProviderSelectionForTests();
  vi.restoreAllMocks();
});

describe("broker provider tag", () => {
  it("keeps the toolbar compact and switches the shared persisted provider", async () => {
    apiMocks.fetchEnvelope.mockResolvedValue(capabilities);
    const selection = useBrokerProviderSelection();
    selection.selectBrokerProvider("futu");
    const wrapper = mount(BrokerProviderTag, {
      props: {
        market: "US",
        featureId: "research.news",
        menuLocation: "top end",
        provider: {
          brokerId: "futu",
          securityFirm: "Moomoo US",
          featureId: "research.news",
          capability: "available",
          selectionReason: "explicit",
          resolvedAt: "2026-07-17T00:00:00Z",
          asOf: "2026-07-17T00:00:00Z",
        },
      },
      global: { stubs: productGlobalStubs },
    });
    await flushPromises();

    expect(wrapper.getComponent({ name: "VMenu" }).props("location")).toBe(
      "top end",
    );

    const tag = wrapper.get(".broker-provider-tag");
    expect(tag.text()).toContain("Futu");
    expect(tag.text()).not.toContain("Moomoo US");
    expect(tag.classes()).toContain("is-available");

    await tag.trigger("click");
    const buttons = wrapper.findAll(
      '.broker-provider-tag__menu button[role="option"]',
    );
    expect(buttons).toHaveLength(3);
    expect(
      buttons
        .find((button) => button.text().includes("Blocked"))
        ?.attributes("disabled"),
    ).toBeDefined();

    await buttons
      .find((button) => button.text().includes("Alpha Broker"))!
      .trigger("click");
    expect(selection.selectedBrokerId.value).toBe("alpha");
    expect(window.localStorage.getItem("jftrade.market-provider.v1")).toBe(
      "alpha",
    );
    expect(wrapper.get(".broker-provider-tag").text()).toContain("Alpha");
    expect(wrapper.get(".broker-provider-tag").classes()).toContain(
      "is-degraded",
    );
  });

  it("shows runtime feed quality without changing static provider capabilities", async () => {
    apiMocks.fetchEnvelope.mockResolvedValue(capabilities);
    useBrokerProviderSelection().selectBrokerProvider("futu");
    const wrapper = mount(BrokerProviderTag, {
      props: {
        market: "US",
        featureId: "research.news",
        connectionState: "connected",
        transportMode: "push-stream",
        provider: {
          brokerId: "futu",
          securityFirm: "Moomoo US",
          featureId: "research.news",
          capability: "available",
          selectionReason: "explicit",
          resolvedAt: "2026-07-17T00:00:00Z",
          asOf: "2026-07-17T00:00:00Z",
        },
      },
      global: { stubs: productGlobalStubs },
    });
    await flushPromises();

    const tag = wrapper.get(".broker-provider-tag");
    expect(tag.classes()).toContain("is-available");
    expect(tag.attributes("data-quality")).toBe("healthy");
    expect(tag.attributes("title")).toContain("行情传输：实时推送正常");
    expect(tag.attributes("title")?.split("\n")).toEqual([
      "数据源：Futu OpenAPI via OpenD（Moomoo US）",
      "功能能力：可用",
      "行情传输：实时推送正常",
    ]);

    await wrapper.setProps({
      connectionState: "disconnected",
      transportMode: "snapshot-poll-fallback",
    });
    expect(tag.classes()).toContain("is-degraded");
    expect(tag.attributes("data-quality")).toBe("degraded");
    expect(tag.attributes("title")).toContain("行情传输：已降级到轮询");
    expect(tag.attributes("aria-label")).toContain("已降级到轮询");

    await wrapper.setProps({
      connectionState: "disconnected",
      transportMode: "push-stream",
    });
    expect(tag.attributes("title")).toContain("行情传输：实时连接已中断");
    expect(tag.attributes("aria-label")).toContain("实时连接已中断");

    await wrapper.setProps({ connectionState: "error", transportMode: null });
    expect(tag.classes()).toContain("is-unavailable");
    expect(tag.attributes("data-quality")).toBe("unavailable");
    expect(tag.attributes("title")).toContain("行情传输：数据源不可用");
  });

  it("keeps capability state and reason from the same runtime summary", async () => {
    apiMocks.fetchEnvelope.mockResolvedValue({
      brokers: [
        {
          id: "futu",
          displayName: "Futu OpenAPI via OpenD",
          securityFirm: "Futu/Moomoo via OpenD",
          capabilities: [
            {
              market: "US",
              supportsQuote: true,
              supportsTrade: false,
              features: [
                { id: "research.news", state: "available" },
              ],
            },
          ],
        },
      ],
      runtime: [
        {
          brokerId: "futu",
          market: "US",
          featureId: "research.news",
          capability: { id: "research.news", state: "available" },
          evaluation: {
            state: "degraded",
            code: "QUOTE_RIGHT_UNVERIFIED",
            reason:
              "OpenD has not reported quote entitlements for this session yet.",
          },
        },
      ],
    });
    useBrokerProviderSelection().selectBrokerProvider("futu");
    const wrapper = mount(BrokerProviderTag, {
      props: {
        market: "US",
        featureId: "research.news",
        connectionState: "connected",
        transportMode: "push-stream",
        provider: {
          brokerId: "futu",
          securityFirm: "Futu/Moomoo via OpenD",
          featureId: "research.news",
          capability: "available",
          selectionReason: "explicit",
          resolvedAt: "2026-07-17T00:00:00Z",
          asOf: "2026-07-17T00:00:00Z",
        },
      },
      global: { stubs: productGlobalStubs },
    });
    await flushPromises();

    const tag = wrapper.get(".broker-provider-tag");
    expect(tag.classes()).toContain("is-degraded");
    expect(tag.attributes("data-capability-state")).toBe("degraded");
    expect(tag.attributes("data-capability-reason")).toBe(
      "尚未完成当前 OpenD 行情权限核验",
    );
    expect(tag.attributes("title")?.split("\n")).toEqual([
      "数据源：Futu OpenAPI via OpenD（Futu/Moomoo via OpenD）",
      "功能能力：降级",
      "行情传输：实时推送正常",
      "原因：尚未完成当前 OpenD 行情权限核验",
    ]);
    expect(tag.attributes("title")).not.toContain("explicit");
  });

  it("adds or replaces brokerId without disturbing the existing query or hash", () => {
    expect(withBrokerProvider("/api/data?x=1#table", " Alpha ")).toBe(
      "/api/data?x=1&brokerId=alpha#table",
    );
    expect(withBrokerProvider("/api/data?brokerId=futu&x=1", "alpha")).toBe(
      "/api/data?brokerId=alpha&x=1",
    );
    expect(withBrokerProvider("/api/data", "")).toBe("/api/data");
  });

  it("uses account then server defaults only when no valid persisted choice exists", async () => {
    apiMocks.fetchEnvelope.mockResolvedValue(capabilities);
    configureBrokerProviderDefaults({
      accountBrokerId: "alpha",
      defaultBrokerId: "futu",
    });
    const selection = useBrokerProviderSelection();
    await selection.loadBrokerProviders();
    expect(selection.selectedBrokerId.value).toBe("alpha");

    selection.selectBrokerProvider("futu");
    configureBrokerProviderDefaults({
      accountBrokerId: "alpha",
      defaultBrokerId: "alpha",
    });
    expect(selection.selectedBrokerId.value).toBe("futu");
  });

  it("commits an available account default after descriptors are known", () => {
    const selection = useBrokerProviderSelection();
    selection.brokerDescriptors.value = capabilities.brokers;

    configureBrokerProviderDefaults({ accountBrokerId: "alpha" });

    expect(selection.selectedBrokerId.value).toBe("alpha");
    expect(window.localStorage.getItem("jftrade.market-provider.v1")).toBe(
      "alpha",
    );
  });

  it("derives compact availability tags from quote and feature capabilities", () => {
    const selection = useBrokerProviderSelection();
    selection.selectBrokerProvider("");
    expect(selection.selectedBrokerId.value).toBe("");
    selection.brokerDescriptors.value = [
      {
        id: "quote",
        displayName: "Quote Source",
        capabilities: [
          { market: "US", supportsQuote: true, supportsTrade: false },
        ],
      },
      {
        id: "feature",
        displayName: "Feature Source",
        capabilities: [
          {
            market: "US",
            supportsQuote: false,
            supportsTrade: false,
            features: [
              { id: "research.news", state: "available", markets: [] },
            ],
          },
        ],
      },
      {
        id: "degraded",
        displayName: "Degraded Source",
        capabilities: [
          {
            market: "US",
            supportsQuote: false,
            supportsTrade: false,
            features: [
              { id: "research.news", state: "degraded", reason: "" },
            ],
          },
        ],
      },
      {
        id: "blocked",
        displayName: "Blocked Source",
        capabilities: [
          {
            market: "US",
            supportsQuote: false,
            supportsTrade: false,
            features: [
              {
                id: "research.news",
                state: "unavailable",
                reason: "账户未开通",
              },
            ],
          },
        ],
      },
      { id: "", displayName: "", capabilities: [] },
    ];

    expect(brokerProviderOptions("", "US").map(({ state }) => state)).toEqual([
      "available",
      "available",
      "degraded",
      "unavailable",
      "unavailable",
    ]);
    expect(brokerProviderOptions("", "US")[2]?.reason).toBe(
      "部分行情或研究能力受限",
    );
    expect(brokerProviderOptions("research.news", "US")[2]?.reason).toBe(
      "此能力当前降级可用",
    );
    expect(brokerProviderOptions("research.news", "US")[3]?.reason).toBe(
      "账户未开通",
    );
    expect(brokerProviderOptions("research.macro", "US")[0]?.reason).toBe(
      "不支持 US 的此项能力",
    );
    expect(brokerProviderOptions("research.macro")[0]?.reason).toBe(
      "未声明此项能力",
    );
    expect(brokerProviderOptions()[4]).toMatchObject({
      label: "",
      shortLabel: "数据源",
    });
    expect(selection.options.value).toHaveLength(5);
  });

  it("prefers runtime evaluation, expands CN to SH and SZ, and aggregates features", async () => {
    apiMocks.fetchEnvelope.mockResolvedValue({
      brokers: [
        {
          id: "futu",
          displayName: "Futu OpenAPI via OpenD",
          capabilities: ["US", "SH", "SZ"].map((market) => ({
            market,
            supportsQuote: true,
            supportsTrade: false,
            features: [
              { id: "research.news", state: "available" },
              { id: "research.rankings", state: "available" },
              { id: "research.industry", state: "available" },
              { id: "research.calendar", state: "available" },
            ],
          })),
        },
      ],
      runtime: [
        {
          brokerId: "futu",
          market: "US",
          featureId: "research.news",
          capability: { id: "research.news", state: "available" },
          evaluation: {
            state: "degraded",
            code: "QUOTE_RIGHT_UNVERIFIED",
            reason: "OpenD 尚未报告美股行情权限",
          },
        },
        {
          brokerId: "futu",
          market: "SH",
          featureId: "research.rankings",
          capability: { id: "research.rankings", state: "available" },
          evaluation: { state: "available" },
        },
        {
          brokerId: "futu",
          market: "SZ",
          featureId: "research.rankings",
          capability: { id: "research.rankings", state: "available" },
          evaluation: {
            state: "unavailable",
            code: "QUOTE_RIGHT_DENIED",
            reason: "深市行情权限不可用",
          },
        },
        ...["SH", "SZ"].map((market) => ({
          brokerId: "futu",
          market,
          featureId: "research.industry",
          capability: { id: "research.industry", state: "available" },
          evaluation: { state: "available" },
        })),
      ],
    });
    const selection = useBrokerProviderSelection();
    await selection.loadBrokerProviders();

    expect(logicalCapabilityMarkets("CN")).toEqual(["SH", "SZ"]);
    expect(selection.brokerRuntimeCapabilities.value).toHaveLength(5);
    expect(
      brokerCapabilitySummary("futu", "research.news", "US"),
    ).toEqual({
      state: "degraded",
      reason: "OpenD 尚未报告美股行情权限",
    });
    expect(
      brokerCapabilitySummary("futu", "research.rankings", "CN"),
    ).toMatchObject({
      state: "degraded",
      reason: expect.stringContaining("SZ：深市行情权限不可用"),
    });
    expect(
      brokerCapabilitySummary(
        "futu",
        ["research.rankings", "research.industry"],
        "CN",
      ),
    ).toMatchObject({
      state: "degraded",
      reason: expect.stringContaining("research.rankings"),
    });
    expect(
      brokerCapabilitySummary("futu", "research.calendar", "CN"),
    ).toEqual({ state: "available", reason: "" });
    expect(
      brokerProviderOptions("research.rankings", "CN")[0],
    ).toMatchObject({ state: "degraded" });

    selection.brokerRuntimeCapabilities.value =
      selection.brokerRuntimeCapabilities.value.map((status) =>
        status.featureId === "research.rankings"
          ? {
              ...status,
              evaluation: {
                state: "unavailable" as const,
                reason: `${status.market} 排行能力不可用`,
              },
            }
          : status,
      );
    expect(
      brokerCapabilitySummary("futu", "research.rankings", "CN"),
    ).toMatchObject({ state: "unavailable" });
  });

  it("shows composite runtime reasons and keeps the single featureId API compatible", async () => {
    apiMocks.fetchEnvelope.mockResolvedValue({
      brokers: [
        {
          id: "futu",
          displayName: "Futu OpenAPI via OpenD",
          securityFirm: "Futu/Moomoo via OpenD",
          capabilities: [
            {
              market: "US",
              supportsQuote: true,
              supportsTrade: false,
              features: [
                { id: "research.news", state: "available" },
                { id: "research.macro", state: "available" },
              ],
            },
          ],
        },
      ],
      runtime: [
        {
          brokerId: "futu",
          market: "US",
          featureId: "research.news",
          capability: { id: "research.news", state: "available" },
          evaluation: {
            state: "unavailable",
            reason: "新闻权限关闭",
          },
        },
        {
          brokerId: "futu",
          market: "US",
          featureId: "research.macro",
          capability: { id: "research.macro", state: "available" },
          evaluation: { state: "available" },
        },
      ],
    });
    useBrokerProviderSelection().selectBrokerProvider("futu");
    const wrapper = mount(BrokerProviderTag, {
      props: {
        market: "US",
        featureId: "research.news",
        featureIds: ["research.news", "research.macro"],
      },
      global: { stubs: productGlobalStubs },
    });
    await flushPromises();

    const tag = wrapper.get(".broker-provider-tag");
    expect(tag.classes()).toContain("is-degraded");
    expect(tag.attributes("data-capability-state")).toBe("degraded");
    expect(tag.attributes("data-capability-reason")).toContain(
      "research.news：新闻权限关闭",
    );
    expect(tag.attributes("title")).toContain("新闻权限关闭");
    expect(tag.attributes("aria-label")).toContain("能力降级");

    await wrapper.setProps({ featureIds: [] });
    expect(tag.classes()).toContain("is-unavailable");
    expect(tag.attributes("data-capability-state")).toBe("unavailable");
    expect(tag.attributes("title")).toContain("新闻权限关闭");
    expect(tag.attributes("aria-label")).toContain("能力不可用");
    await tag.trigger("click");
    expect(
      wrapper.get('.broker-provider-tag__menu button[role="option"]')
        .attributes("disabled"),
    ).toBeDefined();
    expect(wrapper.get(".broker-provider-tag__menu").text()).toContain(
      "新闻权限关闭",
    );
  });

  it("deduplicates capability loads, caches success, and exposes both failure forms", async () => {
    let resolveCapabilities: ((value: typeof capabilities) => void) | undefined;
    apiMocks.fetchEnvelope.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveCapabilities = resolve;
        }),
    );
    const selection = useBrokerProviderSelection();
    const first = selection.loadBrokerProviders();
    const second = selection.loadBrokerProviders();
    expect(apiMocks.fetchEnvelope).toHaveBeenCalledTimes(1);
    resolveCapabilities!(capabilities);
    await Promise.all([first, second]);
    expect(selection.selectedBrokerId.value).toBe("alpha");

    await selection.loadBrokerProviders();
    expect(apiMocks.fetchEnvelope).toHaveBeenCalledTimes(1);

    apiMocks.fetchEnvelope
      .mockRejectedValueOnce(new Error("连接失败"))
      .mockRejectedValueOnce("权限失败");
    await selection.loadBrokerProviders(true);
    expect(selection.loadError.value).toBe("连接失败");
    await selection.loadBrokerProviders(true);
    expect(selection.loadError.value).toBe("权限失败");
    expect(selection.loading.value).toBe(false);
  });

  it("derives chart periods only from the selected provider and market", () => {
    const descriptors = [
      {
        id: "alpha",
        displayName: "Alpha",
        capabilities: [
          {
            market: "US",
            supportsQuote: true,
            supportsTrade: false,
            features: [
              {
                id: "market.candles",
                state: "degraded" as const,
                supportedPeriods: ["1m", "5m"],
              },
              { id: "market.ticks", state: "available" as const },
            ],
          },
          {
            market: "HK",
            supportsQuote: true,
            supportsTrade: false,
            features: [
              {
                id: "market.candles",
                state: "available" as const,
                supportedPeriods: ["1d"],
              },
              { id: "market.ticks", state: "unavailable" as const },
            ],
          },
        ],
      },
    ];

    expect(brokerSupportedChartPeriods("alpha", "US", descriptors)).toEqual([
      "1m",
      "5m",
      "tick",
    ]);
    expect(brokerSupportedChartPeriods("alpha", "HK", descriptors)).toEqual([
      "1d",
    ]);
    expect(brokerSupportedChartPeriods("missing", "US", descriptors)).toBeNull();
  });

  it("covers capability fallbacks without a market or selected descriptor", () => {
    const selection = useBrokerProviderSelection();
    selection.brokerDescriptors.value = [
      {
        id: "edge",
        displayName: "Edge / Provider",
        capabilities: [
          {
            market: "US",
            supportsQuote: false,
            supportsTrade: false,
            features: [
              {
                id: "research.edge",
                state: "degraded",
                reason: "",
              },
              {
                id: "market.other",
                state: "available",
              },
              {
                id: "market.candles",
                state: "available",
                supportedPeriods: [" ", "1D"],
              },
            ],
          },
        ],
      },
    ];
    selection.brokerRuntimeCapabilities.value = [
      {
        brokerId: "edge",
        market: "US",
        featureId: "research.edge",
        capability: {
          id: "research.edge",
          state: "invalid" as never,
        },
        evaluation: { state: "invalid" as never },
      },
    ];

    expect(brokerCapabilitySummary("edge", "research.edge")).toEqual({
      state: "degraded",
      reason: "此能力当前降级可用",
    });
    expect(brokerCapabilitySummary("missing")).toEqual({
      state: "unavailable",
      reason: "未找到券商 missing 的能力目录",
    });
    expect(brokerCapabilitySummary("")).toEqual({
      state: "unavailable",
      reason: "尚未选择行情提供者",
    });
    expect(brokerSupportedChartPeriods("", "US")).toEqual(["1d"]);
    expect(brokerSupportedChartPeriods("", "HK")).toEqual([]);
  });
});
