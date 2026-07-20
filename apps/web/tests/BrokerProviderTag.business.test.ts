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
  brokerProviderOptions,
  brokerSupportedChartPeriods,
  configureBrokerProviderDefaults,
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
    expect(tag.attributes("title")).toContain("数据源质量：实时推送正常");

    await wrapper.setProps({
      connectionState: "disconnected",
      transportMode: "snapshot-poll-fallback",
    });
    expect(tag.classes()).toContain("is-degraded");
    expect(tag.attributes("data-quality")).toBe("degraded");
    expect(tag.attributes("title")).toContain("数据源质量：已降级到轮询");
    expect(tag.attributes("aria-label")).toContain("已降级到轮询");

    await wrapper.setProps({
      connectionState: "disconnected",
      transportMode: "push-stream",
    });
    expect(tag.attributes("title")).toContain("数据源质量：实时连接已中断");
    expect(tag.attributes("aria-label")).toContain("实时连接已中断");

    await wrapper.setProps({ connectionState: "error", transportMode: null });
    expect(tag.classes()).toContain("is-unavailable");
    expect(tag.attributes("data-quality")).toBe("unavailable");
    expect(tag.attributes("title")).toContain("数据源质量：数据源不可用");
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
});
