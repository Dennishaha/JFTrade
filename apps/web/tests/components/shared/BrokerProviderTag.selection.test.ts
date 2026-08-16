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

import BrokerProviderTag from "../../../src/components/shared/BrokerProviderTag.vue";
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
import { flushPromises, productGlobalStubs } from "../../productTestUtils";

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

  it("offers embedded providers on the news surface served by the backend facade", async () => {
    apiMocks.fetchEnvelope.mockImplementation((url: string) =>
      url.includes("/api/v1/settings/market-data-provider")
        ? Promise.resolve({ activeProvider: "futu" })
        : Promise.resolve(capabilities),
    );
    const wrapper = mount(BrokerProviderTag, {
      props: {
        market: "US",
        featureId: "research.news",
        featureIds: ["research.news"],
        enableEmbeddedMarketDataProvider: true,
      },
      global: { stubs: productGlobalStubs },
    });
    await flushPromises();

    await wrapper.get(".broker-provider-tag").trigger("click");
    const options = wrapper.findAll(
      '.broker-provider-tag__menu button[role="option"]',
    );
    expect(options.some((button) => button.text().includes("Yahoo"))).toBe(
      true,
    );
    expect(options.some((button) => button.text().includes("AKShare"))).toBe(
      true,
    );
    wrapper.unmount();
  });

  it("keeps embedded providers off Futu-only research surfaces", async () => {
    apiMocks.fetchEnvelope.mockImplementation((url: string) =>
      url.includes("/api/v1/settings/market-data-provider")
        ? Promise.resolve({ activeProvider: "futu" })
        : Promise.resolve(capabilities),
    );
    const wrapper = mount(BrokerProviderTag, {
      props: {
        market: "US",
        featureId: "research.instrument",
        featureIds: ["research.instrument"],
        enableEmbeddedMarketDataProvider: true,
      },
      global: { stubs: productGlobalStubs },
    });
    await flushPromises();

    await wrapper.get(".broker-provider-tag").trigger("click");
    const options = wrapper.findAll(
      '.broker-provider-tag__menu button[role="option"]',
    );
    expect(options.some((button) => button.text().includes("Yahoo"))).toBe(
      false,
    );
    wrapper.unmount();
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
            readiness: "ready",
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

  it("shows Yahoo warming until the background runtime is ready", async () => {
    vi.useFakeTimers();
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
            activeCount: 0,
            readiness: "warming",
          },
          runtime: {},
          subscriptions: {},
          checkedAt: "2026-08-02T00:00:00Z",
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
    expect(tag.text()).toContain("Yahoo 预热中");
    expect(tag.classes()).toContain("is-degraded");
    expect(tag.attributes("title")).toContain("后台预热");
    wrapper.unmount();
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
});
