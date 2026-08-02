// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import { nextTick } from "vue";

const apiMocks = vi.hoisted(() => ({
  fetchEnvelope: vi.fn(),
  putEnvelope: vi.fn(),
}));

vi.mock("@/composables/shared/apiClient", () => ({
  apiGet: apiMocks.fetchEnvelope,
  apiPut: apiMocks.putEnvelope,
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
} from "@/composables/trading/brokerProviderSelection";
import { flushPromises, productGlobalStubs } from "./productTestUtils";

const capabilities = {
  brokers: [
    {
      id: "futu",
      displayName: "Futu",
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

function futuOpenDHealth(healthy: boolean) {
  return {
    checkedAt: "2026-08-01T00:00:00Z",
    status: healthy ? "healthy" : "offline",
    runtime: {
      connectivity: healthy ? "connected" : "disconnected",
      host: "127.0.0.1",
      apiPort: 11110,
      websocketPort: 11111,
      useEncryption: false,
      websocketKeyConfigured: true,
      marketDataTransport: "bbgo-opend-tcp-api",
      quoteLoggedIn: healthy,
      tradeLoggedIn: healthy,
      programStatus: healthy ? "Ready" : null,
      serverVersion: healthy ? "10.9.6908" : null,
      minimumVersion: "10.9.6908",
      lastError: healthy ? null : "connection refused",
    },
    diagnosis: {
      code: healthy ? "NONE" : "OPEND_API_CONNECTIVITY",
      summary: healthy ? null : "connection refused",
      manualRetryRequired: !healthy,
      restartOpenDRecommended: !healthy,
    },
    localSocketDiagnostics: {
      configuredOpenDWebSocketLimit: 20,
      configuredOpenDWebSocketLimitActive: false,
      configuredOpenDWebSocketLimitScope: "diagnostic",
      websocketEstablishedConnections: 0,
      jftradeLiveWebSocketLimit: 20,
      jftradeLiveWebSocketAtLimit: false,
      likelyConnectionSaturation: false,
      openDWebSocketPoolLikelySaturation: false,
      liveQuoteBackoffActive: false,
      liveQuoteRetryAfter: null,
      liveQuoteFailureCount: 0,
      liveQuoteLastError: null,
      liveStreamBackoffActive: false,
      liveStreamRetryAfter: null,
      liveStreamFailureCount: 0,
      liveStreamLastError: null,
      transportMode: "bbgo-opend-tcp-api",
      topClientProcesses: [],
    },
    localInstallation: {
      platform: "darwin",
      installed: true,
      version: "10.9.6908",
      installPath: "/Applications/Futu_OpenD.app",
      guiDetected: true,
      process: { running: true, pid: 100, executablePath: "/Futu_OpenD" },
    },
    latestVersion: {
      value: null,
      sourceUrl: null,
      checkedAt: null,
      status: "unknown",
      error: null,
    },
    recommendations: [],
  };
}

afterEach(() => {
  apiMocks.fetchEnvelope.mockReset();
  apiMocks.putEnvelope.mockReset();
  resetBrokerProviderSelectionForTests();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("broker provider tag", () => {
  it("shows the embedded Yahoo provider on market-data surfaces and switches it through settings", async () => {
    apiMocks.fetchEnvelope.mockImplementation((url: string) =>
      url.includes("/api/v1/settings/market-data-provider")
        ? Promise.resolve({ activeProvider: "futu" })
        : Promise.resolve(capabilities),
    );
    apiMocks.putEnvelope.mockResolvedValue({ activeProvider: "yfinance" });
    const wrapper = mount(BrokerProviderTag, {
      props: {
        market: "US",
        featureId: "market.candles",
        featureIds: ["market.candles", "research.rankings"],
        enableEmbeddedMarketDataProvider: true,
      },
      global: { stubs: productGlobalStubs },
    });
    await flushPromises();

    expect(wrapper.get(".broker-provider-tag").text()).toContain("Futu");
    await wrapper.get(".broker-provider-tag").trigger("click");
    const yfinance = wrapper
      .findAll('.broker-provider-tag__menu button[role="option"]')
      .find((button) => button.text().includes("Yahoo"));
    expect(yfinance).toBeDefined();
    await yfinance!.trigger("click");
    expect(apiMocks.putEnvelope).toHaveBeenCalledWith(
      "/api/v1/settings/market-data-provider",
      { activeProvider: "yfinance" },
    );
  });

  it("renders Yahoo's delayed HTTP polling as a normal green provider state", async () => {
    apiMocks.fetchEnvelope.mockImplementation((url: string) => {
      if (url.includes("/api/v1/settings/market-data-provider")) {
        return Promise.resolve({ activeProvider: "yfinance" });
      }
      if (url.includes("/api/v1/market-data/provider")) {
        return Promise.resolve({
          descriptor: {
            providerId: "yahoo-finance",
            brokerId: "yfinance",
            displayName: "Yahoo",
          },
          health: {
            connected: true,
            streamMode: "snapshot-poll-delayed",
            activeCount: 1,
          },
          runtime: {},
          subscriptions: {},
          checkedAt: "2026-07-31T00:00:00Z",
        });
      }
      return Promise.resolve(capabilities);
    });
    const wrapper = mount(BrokerProviderTag, {
      props: {
        market: "US",
        featureId: "market.candles",
        enableEmbeddedMarketDataProvider: true,
      },
      global: { stubs: productGlobalStubs },
    });
    await flushPromises();
    await flushPromises();

    const tag = wrapper.get(".broker-provider-tag");
    expect(tag.text()).toContain("Yahoo");
    expect(tag.classes()).toContain("is-available");
    expect(tag.attributes("data-capability-state")).toBe("degraded");
    expect(tag.attributes("data-display-state")).toBe("available");
    expect(tag.attributes("data-quality")).toBe("degraded");
    expect(tag.attributes("title")?.split("\n")).toEqual([
      "供应商：Yahoo",
      "连接方式：HTTP 定时查询",
      "数据质量：非实时快照，时效以供应商返回为准",
    ]);
    expect(tag.attributes("title")).not.toContain("降级");
  });

  it("keeps the selected provider usable when the status read fails", async () => {
    apiMocks.fetchEnvelope.mockImplementation((url: string) => {
      if (url.includes("/api/v1/settings/market-data-provider")) {
        return Promise.resolve({ activeProvider: "yfinance" });
      }
      if (url.includes("/api/v1/market-data/provider")) {
        return Promise.reject(new Error("provider status unavailable"));
      }
      return Promise.resolve(capabilities);
    });
    const wrapper = mount(BrokerProviderTag, {
      props: {
        market: "US",
        featureId: "market.candles",
        enableEmbeddedMarketDataProvider: true,
      },
      global: { stubs: productGlobalStubs },
    });
    await flushPromises();
    await flushPromises();

    const tag = wrapper.get(".broker-provider-tag");
    expect(tag.text()).toContain("Yahoo");
    expect(tag.classes()).toContain("is-available");
    expect(tag.classes()).not.toContain("is-unavailable");
    expect(tag.attributes("title")).toContain(
      "状态详情：provider status unavailable",
    );
  });

  it("marks disconnected Futu red and disables switching from Yahoo", async () => {
    apiMocks.fetchEnvelope.mockImplementation((url: string) => {
      if (url.includes("/api/v1/settings/market-data-provider")) {
        return Promise.resolve({ activeProvider: "yfinance" });
      }
      if (url.includes("/api/v1/market-data/provider")) {
        return Promise.resolve({
          descriptor: { providerId: "yahoo-finance", brokerId: "yfinance" },
          health: {
            connected: true,
            streamMode: "snapshot-poll-delayed",
            activeCount: 0,
          },
          runtime: {},
          subscriptions: {},
          checkedAt: "2026-08-01T00:00:00Z",
        });
      }
      if (url.includes("/api/v1/system/futu-opend")) {
        return Promise.resolve(futuOpenDHealth(false));
      }
      return Promise.resolve(capabilities);
    });

    const wrapper = mount(BrokerProviderTag, {
      props: {
        market: "US",
        featureId: "market.candles",
        enableEmbeddedMarketDataProvider: true,
      },
      global: { stubs: productGlobalStubs },
    });
    await flushPromises();
    await wrapper.get(".broker-provider-tag").trigger("click");
    await flushPromises();

    const futu = wrapper
      .findAll('.broker-provider-tag__menu button[role="option"]')
      .find((button) => button.text().includes("Futu"))!;
    expect(futu.classes()).toContain("is-unavailable");
    expect(futu.attributes("disabled")).toBeDefined();
    expect(futu.text()).toContain("当前无法连接 OpenD");
    await futu.trigger("click");
    expect(apiMocks.putEnvelope).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it("uses OpenD health instead of the browser socket for active Futu", async () => {
    apiMocks.fetchEnvelope.mockImplementation((url: string) => {
      if (url.includes("/api/v1/settings/market-data-provider")) {
        return Promise.resolve({ activeProvider: "futu" });
      }
      if (url.includes("/api/v1/system/futu-opend")) {
        return Promise.resolve(futuOpenDHealth(false));
      }
      return Promise.resolve(capabilities);
    });

    const wrapper = mount(BrokerProviderTag, {
      props: {
        market: "US",
        featureId: "market.candles",
        enableEmbeddedMarketDataProvider: true,
        connectionState: "connected",
        transportMode: "push-stream",
      },
      global: { stubs: productGlobalStubs },
    });
    await flushPromises();
    await flushPromises();

    const tag = wrapper.get(".broker-provider-tag");
    expect(tag.text()).toContain("Futu");
    expect(tag.classes()).toContain("is-unavailable");
    expect(tag.attributes("data-quality")).toBe("unavailable");
    expect(tag.attributes("title")).toContain("当前无法连接 OpenD");
  });

  it("distinguishes logged-out and unknown OpenD quote sessions", async () => {
    const cases = [
      { quoteLoggedIn: false, reason: "OpenD 行情会话尚未登录" },
      { quoteLoggedIn: null, reason: "OpenD 行情会话状态不可用" },
    ];

    for (const test of cases) {
      const health = futuOpenDHealth(false);
      health.runtime.connectivity = "connected";
      (health.runtime as { quoteLoggedIn: boolean | null }).quoteLoggedIn =
        test.quoteLoggedIn;
      apiMocks.fetchEnvelope.mockImplementation((url: string) => {
        if (url.includes("/api/v1/settings/market-data-provider")) {
          return Promise.resolve({ activeProvider: "futu" });
        }
        if (url.includes("/api/v1/system/futu-opend")) {
          return Promise.resolve(health);
        }
        return Promise.resolve(capabilities);
      });

      const wrapper = mount(BrokerProviderTag, {
        props: {
          market: "US",
          featureId: "market.candles",
          enableEmbeddedMarketDataProvider: true,
        },
        global: { stubs: productGlobalStubs },
      });
      await flushPromises();
      await flushPromises();

      expect(wrapper.get(".broker-provider-tag").attributes("title")).toContain(
        test.reason,
      );
      wrapper.unmount();
      resetBrokerProviderSelectionForTests();
      apiMocks.fetchEnvelope.mockReset();
    }
  });

  it("polls only while visible and open, then enables Futu after recovery", async () => {
    vi.useFakeTimers();
    const recoverableCapabilities = {
      ...capabilities,
      brokers: capabilities.brokers.map((broker) =>
        broker.id !== "futu"
          ? broker
          : {
              ...broker,
              capabilities: broker.capabilities.map((capability) => ({
                ...capability,
                features: [
                  ...capability.features,
                  { id: "market.candles", markets: ["US"], state: "available" },
                ],
              })),
            },
      ),
    };
    const visibility = vi
      .spyOn(document, "visibilityState", "get")
      .mockReturnValue("visible");
    let healthy = false;
    let healthCalls = 0;
    apiMocks.putEnvelope.mockResolvedValue({ activeProvider: "yfinance" });
    apiMocks.fetchEnvelope.mockImplementation((url: string) => {
      if (url.includes("/api/v1/settings/market-data-provider")) {
        return Promise.resolve({ activeProvider: "yfinance" });
      }
      if (url.includes("/api/v1/market-data/provider")) {
        return Promise.resolve({
          descriptor: { providerId: "yahoo-finance", brokerId: "yfinance" },
          health: {
            connected: true,
            streamMode: "snapshot-poll-delayed",
            activeCount: 0,
          },
          runtime: {},
          subscriptions: {},
          checkedAt: "2026-08-01T00:00:00Z",
        });
      }
      if (url.includes("/api/v1/system/futu-opend")) {
        healthCalls += 1;
        return Promise.resolve(futuOpenDHealth(healthy));
      }
      return Promise.resolve(recoverableCapabilities);
    });

    const wrapper = mount(BrokerProviderTag, {
      props: {
        market: "US",
        featureId: "market.candles",
        enableEmbeddedMarketDataProvider: true,
      },
      global: { stubs: productGlobalStubs },
    });
    await flushPromises();
    const tag = wrapper.get(".broker-provider-tag");
    await wrapper.get(".broker-provider-tag").trigger("click");
    await flushPromises();
    expect(healthCalls).toBe(1);

    await vi.advanceTimersByTimeAsync(10_000);
    await flushPromises();
    expect(healthCalls).toBe(2);

    visibility.mockReturnValue("hidden");
    document.dispatchEvent(new Event("visibilitychange"));
    await vi.advanceTimersByTimeAsync(20_000);
    expect(healthCalls).toBe(2);

    healthy = true;
    visibility.mockReturnValue("visible");
    document.dispatchEvent(new Event("visibilitychange"));
    await flushPromises();
    await flushPromises();
    expect(healthCalls).toBeGreaterThanOrEqual(3);
    for (let index = 0; index < 6; index += 1) await flushPromises();
    const futu = wrapper
      .findAll('.broker-provider-tag__menu button[role="option"]')
      .find((button) => button.text().includes("Futu"))!;
    expect(futu.classes()).toContain("is-available");
    expect(futu.attributes("disabled")).toBeUndefined();
    await wrapper
      .findAll('.broker-provider-tag__menu button[role="option"]')
      .find((button) => button.text().includes("Yahoo"))!
      .trigger("click");
    await flushPromises();
    expect(wrapper.find(".broker-provider-tag__menu").exists()).toBe(false);
    const recoveredHealthCalls = healthCalls;
    await vi.advanceTimersByTimeAsync(20_000);
    expect(healthCalls).toBe(recoveredHealthCalls);
  });

  it("ignores an older OpenD health response after switching", async () => {
    const healthResolvers: Array<(value: unknown) => void> = [];
    const statusResolvers: Array<(value: unknown) => void> = [];
    const status = (brokerId: string, streamMode: string) => ({
      descriptor: {
        providerId: brokerId === "yfinance" ? "yahoo-finance" : brokerId,
        brokerId,
        displayName: brokerId === "yfinance" ? "Yahoo" : "Futu OpenD",
      },
      health: { connected: true, streamMode, activeCount: 1 },
      runtime: {},
      subscriptions: {},
      checkedAt: "2026-07-31T00:00:00Z",
    });
    apiMocks.fetchEnvelope.mockImplementation((url: string) => {
      if (url.includes("/api/v1/settings/market-data-provider")) {
        return Promise.resolve({ activeProvider: "futu" });
      }
      if (url.includes("/api/v1/market-data/provider")) {
        return new Promise((resolve) => statusResolvers.push(resolve));
      }
      if (url.includes("/api/v1/system/futu-opend")) {
        return new Promise((resolve) => healthResolvers.push(resolve));
      }
      return Promise.resolve(capabilities);
    });
    apiMocks.putEnvelope.mockResolvedValue({ activeProvider: "yfinance" });
    const wrapper = mount(BrokerProviderTag, {
      props: {
        market: "US",
        featureId: "market.candles",
        enableEmbeddedMarketDataProvider: true,
      },
      global: { stubs: productGlobalStubs },
    });
    await flushPromises();
    await flushPromises();
    expect(healthResolvers).toHaveLength(1);

    await wrapper.get(".broker-provider-tag").trigger("click");
    await wrapper
      .findAll('.broker-provider-tag__menu button[role="option"]')
      .find((button) => button.text().includes("Yahoo"))!
      .trigger("click");
    await flushPromises();
    await flushPromises();
    expect(statusResolvers).toHaveLength(0);

    healthResolvers[0]!(futuOpenDHealth(true));
    await flushPromises();
    await flushPromises();
    expect(statusResolvers).toHaveLength(1);

    statusResolvers[0]!(status("yfinance", "snapshot-poll-delayed"));
    await flushPromises();
    await flushPromises();
    const tag = wrapper.get(".broker-provider-tag");
    expect(tag.text()).toContain("Yahoo");
    expect(tag.attributes("title")).toContain("HTTP 定时查询");
  });

  it("preserves the embedded Yahoo provider when the broker catalog finishes later", async () => {
    let resolveCapabilities: (value: typeof capabilities) => void = () => {};
    const pendingCapabilities = new Promise<typeof capabilities>((resolve) => {
      resolveCapabilities = resolve;
    });
    apiMocks.fetchEnvelope.mockImplementation((url: string) =>
      url.includes("/api/v1/settings/market-data-provider")
        ? Promise.resolve({ activeProvider: "yfinance" })
        : pendingCapabilities,
    );
    const selection = useBrokerProviderSelection();
    const wrapper = mount(BrokerProviderTag, {
      props: {
        market: "US",
        featureId: "market.candles",
        enableEmbeddedMarketDataProvider: true,
      },
      global: { stubs: productGlobalStubs },
    });

    await flushPromises();
    expect(selection.selectedBrokerId.value).toBe("yfinance");
    expect(wrapper.get(".broker-provider-tag").text()).toContain("Yahoo");

    resolveCapabilities(capabilities);
    await flushPromises();
    expect(selection.selectedBrokerId.value).toBe("yfinance");
    expect(wrapper.get(".broker-provider-tag").text()).toContain("Yahoo");
  });

  it("keeps the current provider selected while a switch is pending", async () => {
    apiMocks.fetchEnvelope.mockImplementation((url: string) =>
      url.includes("/api/v1/settings/market-data-provider")
        ? Promise.resolve({ activeProvider: "futu" })
        : Promise.resolve(capabilities),
    );
    let resolvePut: (value: { activeProvider: "yfinance" }) => void = () => {};
    apiMocks.putEnvelope.mockReturnValue(
      new Promise<{ activeProvider: "yfinance" }>((resolve) => {
        resolvePut = resolve;
      }),
    );
    const wrapper = mount(BrokerProviderTag, {
      props: {
        market: "US",
        featureId: "market.candles",
        enableEmbeddedMarketDataProvider: true,
      },
      global: { stubs: productGlobalStubs },
    });
    await flushPromises();

    const tag = wrapper.get(".broker-provider-tag");
    await tag.trigger("click");
    const options = wrapper.findAll(
      '.broker-provider-tag__menu button[role="option"]',
    );
    await options.find((button) => button.text().includes("Yahoo"))!.trigger("click");
    await nextTick();

    expect(tag.text()).toContain("启动中");
    expect(
      options.every((button) => button.attributes("disabled") !== undefined),
    ).toBe(true);
    const futu = options.find((button) => button.text().includes("Futu"));
    expect(futu?.attributes("aria-selected")).toBe("true");

    resolvePut({ activeProvider: "yfinance" });
    await flushPromises();
    expect(tag.text()).toContain("Yahoo");
    await tag.trigger("click");
    expect(
      wrapper
        .findAll('.broker-provider-tag__menu button[role="option"]')
        .find((button) => button.text().includes("Yahoo"))
        ?.attributes("aria-selected"),
    ).toBe("true");
  });

  it("ignores an older initial provider read after a newer switch succeeds", async () => {
    let resolveInitialRead: (value: { activeProvider: "futu" }) => void = () => {};
    apiMocks.fetchEnvelope.mockImplementation((url: string) => {
      if (url.includes("/api/v1/settings/market-data-provider")) {
        return new Promise<{ activeProvider: "futu" }>((resolve) => {
          resolveInitialRead = resolve;
        });
      }
      return Promise.resolve(capabilities);
    });
    apiMocks.putEnvelope.mockResolvedValue({ activeProvider: "yfinance" });

    const wrapper = mount(BrokerProviderTag, {
      props: {
        market: "US",
        featureId: "market.candles",
        enableEmbeddedMarketDataProvider: true,
      },
      global: { stubs: productGlobalStubs },
    });
    await flushPromises();

    const tag = wrapper.get(".broker-provider-tag");
    await tag.trigger("click");
    await wrapper
      .findAll('.broker-provider-tag__menu button[role="option"]')
      .find((button) => button.text().includes("Yahoo"))!
      .trigger("click");
    await flushPromises();
    expect(tag.text()).toContain("Yahoo");

    resolveInitialRead({ activeProvider: "futu" });
    await flushPromises();
    expect(tag.text()).toContain("Yahoo");
  });

  it("fails closed when the persisted embedded provider is unknown", async () => {
    apiMocks.fetchEnvelope.mockImplementation((url: string) =>
      url.includes("/api/v1/settings/market-data-provider")
        ? Promise.resolve({ activeProvider: "unsupported" })
        : Promise.resolve(capabilities),
    );

    const wrapper = mount(BrokerProviderTag, {
      props: {
        market: "US",
        featureId: "market.candles",
        enableEmbeddedMarketDataProvider: true,
      },
      global: { stubs: productGlobalStubs },
    });
    await flushPromises();

    const tag = wrapper.get(".broker-provider-tag");
    expect(tag.classes()).toContain("is-unavailable");
    expect(tag.text()).toContain("不可用");
    expect(tag.text()).not.toContain("Futu");
    expect(tag.attributes("title")).toContain("不支持的行情提供者");
  });

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
      "is-available",
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
    expect(tag.attributes("title")).toContain("连接方式：实时推送");
    expect(tag.attributes("title")).toContain("数据质量：实时推送正常");
    expect(tag.attributes("title")?.split("\n")).toEqual([
      "供应商：Futu",
      "连接方式：实时推送",
      "数据质量：实时推送正常",
    ]);

    await wrapper.setProps({
      connectionState: "disconnected",
      transportMode: "snapshot-poll-fallback",
    });
    expect(tag.classes()).toContain("is-degraded");
    expect(tag.attributes("data-quality")).toBe("degraded");
    expect(tag.attributes("title")).toContain("数据质量：快照轮询（推送回退）");
    expect(tag.attributes("aria-label")).toContain("快照轮询（推送回退）");

    await wrapper.setProps({
      connectionState: "disconnected",
      transportMode: "push-stream",
    });
    expect(tag.attributes("title")).toContain("数据质量：实时连接已中断");
    expect(tag.attributes("aria-label")).toContain("实时连接已中断");

    await wrapper.setProps({ connectionState: "error", transportMode: null });
    expect(tag.classes()).toContain("is-unavailable");
    expect(tag.attributes("data-quality")).toBe("unavailable");
    expect(tag.attributes("title")).toContain("数据质量：数据源不可用");
  });

  it("keeps capability state and reason from the same runtime summary", async () => {
    apiMocks.fetchEnvelope.mockResolvedValue({
      brokers: [
        {
          id: "futu",
          displayName: "Futu",
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
      "供应商：Futu",
      "连接方式：实时推送",
      "数据质量：实时推送正常",
      "功能范围：当前功能受限",
      "说明：尚未完成当前 OpenD 行情权限核验",
    ]);
    expect(tag.attributes("title")).not.toContain("explicit");
  });

  it("normalizes optional and forward-compatible generated capability fields", async () => {
    apiMocks.fetchEnvelope.mockResolvedValueOnce({
      brokers: [
        {
          id: "future",
          displayName: "Future Broker",
          securityFirm: "Future Securities",
          capabilityVersion: " 2026-07 ",
          capabilities: [
            {
              market: "US",
              supportsQuote: true,
              supportsTrade: false,
            },
            {
              market: "HK",
              supportsQuote: true,
              supportsTrade: false,
              features: [
                {
                  id: "research.news",
                  state: "future-state",
                  markets: ["HK"],
                  supportedPeriods: ["1d"],
                  reasonCode: "FUTURE_STATE",
                  reason: "等待适配",
                },
              ],
            },
          ],
        },
        {
          id: "sparse",
          displayName: "Sparse Broker",
        },
      ],
      runtime: [
        {
          brokerId: "future",
          securityFirm: "Future Securities",
          market: "HK",
          featureId: "research.news",
          capability: {
            id: "research.news",
            state: "future-state",
            reasonCode: "RUNTIME_FUTURE",
          },
          evaluation: {
            state: "future-state",
            checkedAt: " 2026-07-26T00:00:00Z ",
            code: "UNKNOWN_STATE",
            reason: "尚未识别",
          },
        },
      ],
    });

    const selection = useBrokerProviderSelection();
    await selection.loadBrokerProviders();

    expect(selection.brokerDescriptors.value).toEqual([
      expect.objectContaining({
        id: "future",
        capabilityVersion: "2026-07",
        capabilities: [
          expect.objectContaining({ market: "US" }),
          expect.objectContaining({
            market: "HK",
            features: [
              expect.objectContaining({
                state: "unavailable",
                reasonCode: "FUTURE_STATE",
                supportedPeriods: ["1d"],
              }),
            ],
          }),
        ],
      }),
      expect.objectContaining({ id: "sparse", capabilities: [] }),
    ]);
    expect(selection.brokerRuntimeCapabilities.value[0]).toMatchObject({
      securityFirm: "Future Securities",
      capability: {
        state: "unavailable",
        reasonCode: "RUNTIME_FUTURE",
      },
      evaluation: {
        state: "unavailable",
        checkedAt: "2026-07-26T00:00:00Z",
        code: "UNKNOWN_STATE",
      },
    });

    resetBrokerProviderSelectionForTests();
    apiMocks.fetchEnvelope.mockResolvedValueOnce({ runtime: [] });
    const empty = useBrokerProviderSelection();
    await empty.loadBrokerProviders();
    expect(empty.brokerDescriptors.value).toEqual([]);
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
          displayName: "Futu",
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
          displayName: "Futu",
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
    expect(tag.attributes("aria-label")).toContain("当前功能受限");

    await wrapper.setProps({ featureIds: [] });
    expect(tag.classes()).toContain("is-unavailable");
    expect(tag.attributes("data-capability-state")).toBe("unavailable");
    expect(tag.attributes("title")).toContain("新闻权限关闭");
    expect(tag.attributes("aria-label")).toContain("当前功能不可用");
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
    expect(brokerSupportedChartPeriods("yfinance", "US", descriptors)).toEqual([
      "1m",
      "5m",
      "15m",
      "30m",
      "1h",
      "1d",
      "1w",
      "1mo",
    ]);
    expect(brokerSupportedChartPeriods("yfinance", "HK", descriptors)).toEqual([
      "1m",
      "5m",
      "15m",
      "30m",
      "1h",
      "1d",
      "1w",
      "1mo",
    ]);
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
