// @vitest-environment jsdom

import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { defineComponent, ref } from "vue";

import {
  emptyMarketDataSubscriptions,
  emptyRealTradeApprovals,
  emptyRealTradeHardStopEvents,
  emptyRealTradeHardStops,
  emptyRealTradeKillSwitchEvents,
  emptyRealTradeKillSwitchState,
  emptyRealTradeRiskEvents,
  emptyRealTradeRiskState,
  emptyStorageOverview,
  emptySystemStatus,
  emptyWorkerBrokerOrderUpdates,
} from "@/contracts";

import {
  emptyStub,
  tabStub,
  tabsStub,
  windowItemStub,
  windowStub,
} from "./helpers";

const testState = vi.hoisted(() => ({
  store: null as null | Record<string, any>,
  fetchEnvelopeMock: vi.fn(),
  fetchEnvelopeWithInitMock: vi.fn(),
}));

vi.mock("../src/composables/useConsoleData", () => ({
  useConsoleData: () => testState.store,
}));

vi.mock("../src/composables/apiClient", () => ({
  fetchEnvelope: (...args: unknown[]) => testState.fetchEnvelopeMock(...args),
  fetchEnvelopeWithInit: (...args: unknown[]) =>
    testState.fetchEnvelopeWithInitMock(...args),
}));

import SystemPage from "../src/pages/SystemPage.vue";

const cardStub = defineComponent({
  template: "<section><slot /></section>",
});

const cardTextStub = defineComponent({
  template: "<div><slot /></div>",
});

const chipStub = defineComponent({
  props: ["color", "variant", "size"],
  template: "<span><slot /></span>",
});

const buttonStub = defineComponent({
  props: ["loading", "variant", "color", "size", "to"],
  emits: ["click"],
  template: "<button type='button' @click='$emit(\"click\", $event)'><slot /></button>",
});

const alertStub = defineComponent({
  props: ["title", "type", "closable", "variant", "density"],
  template:
    "<div><div v-if='title'>{{ title }}</div><div><slot /></div></div>",
});

const wrappers: VueWrapper[] = [];

function createStrategyInstance(overrides: Record<string, unknown> = {}) {
  return {
    id: "instance-1",
    status: "PAUSED",
    definition: {
      strategyId: "mean-revert",
      name: "Mean Revert",
      version: "1.0.0",
    },
    binding: {
      runtimeRisk: {
        mode: "monitor",
        closeOnly: true,
        maxOrderQuantity: 5,
        maxOrderNotional: 1000,
        dailyMaxOrders: 12,
        pauseOnReject: true,
      },
    },
    ...overrides,
  } as any;
}

function createSystemStore() {
  const loadSystemState = vi.fn(async () => undefined);
  const loadMarketDataSubscriptions = vi.fn(async () => undefined);

  return {
    loadError: ref("system api degraded"),
    loadSystemState,
    realTradeApprovals: ref({
      ...emptyRealTradeApprovals,
      realTradingEnabled: true,
      approvalWorkflowAvailable: false,
      approvalWorkflowStatus: "not_implemented",
      approvalWorkflowMessage:
        "large-order approval threshold is configured but approval workflow is not implemented yet.",
      approvalPolicy: {
        ...emptyRealTradeApprovals.approvalPolicy,
        largeOrderNotional: 1000,
        approvalWorkflowAvailable: false,
        approvalMode: "blocking_threshold_without_workflow",
      },
      entries: [
        {
          id: "approval-1",
          operation: "PLACE",
          brokerId: "futu",
          tradingEnvironment: "REAL",
          accountId: "REAL-001",
          market: "US",
          operatorId: "ops-a",
          ticketId: "ticket-1",
          decision: "approved",
        },
      ],
    } as any),
    realTradeHardStopEvents: ref({
      ...emptyRealTradeHardStopEvents,
      entries: [],
    } as any),
    realTradeHardStops: ref({
      ...emptyRealTradeHardStops,
      entries: [
        {
          id: "hard-stop-1",
          brokerId: "futu",
          accountId: "REAL-001",
          tradingEnvironment: "REAL",
          market: "US",
          operatorId: "ops-a",
          reason: "manual freeze",
        },
      ],
    } as any),
    realTradeKillSwitchEvents: ref({
      ...emptyRealTradeKillSwitchEvents,
      realTradingEnabled: true,
      killSwitchActive: true,
      entries: [
        {
          id: "ks-1",
          eventType: "activated",
          brokerId: "futu",
          createdAt: "2026-07-01T00:00:00.000Z",
        },
      ],
    } as any),
    realTradeKillSwitchState: ref({
      ...emptyRealTradeKillSwitchState,
      realTradingEnabled: true,
      killSwitchActive: true,
      killSwitchSource: "ENV",
      envConfiguredActive: true,
      blockedOperations: ["PLACE", "MODIFY"],
      allowsCancel: true,
    } as any),
    realTradeRiskEvents: ref({
      ...emptyRealTradeRiskEvents,
      realTradingEnabled: true,
      riskEnabled: true,
      riskConfigSource: "ENV",
      effectiveMaxOrderQuantity: 100,
      effectiveMaxOrderNotional: 50000,
      entries: [
        {
          id: "risk-1",
          eventType: "rejected",
          brokerId: "futu",
          reason: "quantity over limit",
        },
      ],
    } as any),
    realTradeRiskState: ref({
      ...emptyRealTradeRiskState,
      realTradingEnabled: true,
      riskEnabled: true,
      riskConfigSource: "ENV",
      effectiveMaxOrderQuantity: 100,
      effectiveMaxOrderNotional: 50000,
      entry: {
        effectiveMaxOrderQuantity: 100,
        effectiveMaxOrderNotional: 50000,
      },
    } as any),
    storageOverview: ref({
      ...emptyStorageOverview,
      recentAuditLogs: [
        {
          id: "audit-1",
          action: "system.refresh",
          targetType: "service",
          targetId: "api",
        },
      ],
      recentExecutionCommands: [
        {
          id: "cmd-1",
          operation: "PLACE",
          brokerId: "futu",
          completedAt: null,
          idempotencyKey: "idem-1",
          actorType: "operator",
          actorId: "ops-a",
          internalOrderId: "ord-1",
        },
      ],
      pendingOutbox: [{ id: "outbox-1" }],
      recentJobs: [{ id: "job-1" }],
    } as any),
    systemStatus: ref({
      ...emptySystemStatus,
      apiPort: 8080,
      realTradingEnabled: true,
      realTradingRisk: {
        enabled: true,
        maxOrderQuantity: 100,
        maxOrderNotional: 50000,
        riskConfigSource: "ENV",
      },
      realTradingKillSwitch: {
        active: true,
      },
      persistence: {
        ...emptySystemStatus.persistence,
        status: "warn",
        pendingMigrations: ["003_runtime_risk"],
        tables: ["orders", "fills"],
        databasePath: "/tmp/jftrade.sqlite",
      },
      broker: {
        ...emptySystemStatus.broker,
        displayName: "Futu Securities",
        capabilities: [{ market: "US" }],
      },
      strategyRuntime: {
        ...emptySystemStatus.strategyRuntime,
        activeStrategies: 2,
        activeInstances: [
          {
            instanceId: "runtime-paused",
            definitionName: "Runtime Paused",
            actualStatus: "PAUSED",
            activeSymbols: null,
            lastClosedKlineAt: null,
            lastSignalAt: null,
            lastOrderAt: null,
            lastErrorAt: null,
            lastError: "",
            updatedAt: "2026-07-01T00:00:00.000Z",
          },
          {
            instanceId: "runtime-stopped",
            definitionName: "Runtime Stopped",
            actualStatus: "STOPPED",
            activeSymbols: ["US.AAPL"],
            lastClosedKlineAt: "2026-07-01T00:00:00.000Z",
            lastSignalAt: "2026-07-01T00:01:00.000Z",
            lastOrderAt: "2026-07-01T00:02:00.000Z",
            lastErrorAt: "2026-07-01T00:03:00.000Z",
            lastError: "stopped by operator",
            updatedAt: "2026-07-01T00:03:00.000Z",
          },
        ],
      },
      runtimeResources: {
        checkedAt: "2026-07-04T00:00:00.000Z",
        count: 2,
        items: [
          {
            id: "execution-orders-db",
            owner: "trading",
            kind: "sqlite",
            path: "/tmp/execution-orders.db",
            initializedBy: "trading module",
            schemaOwner: "execution order store",
            closeOwner: "trading module",
            healthProvider: "data-migration/execution",
            environmentOverride: "JFTRADE_EXECUTION_ORDER_DB",
            critical: true,
          },
          {
            id: "adk-session-db",
            owner: "assistant/runtime",
            kind: "sqlite",
            path: "/tmp/adk-session.db",
            initializedBy: "assistant runtime",
            schemaOwner: "adk session store",
            closeOwner: "assistant runtime",
            healthProvider: "system.runtime-dependencies/adk",
            environmentOverride: "JFTRADE_ADK_SESSION_DB",
            critical: false,
          },
        ],
      },
      observability: {
        requests: {
          slowThresholdMs: 800,
          minimumImportance: "low",
          recentErrors: [
            {
              at: "2026-07-01T00:00:00.000Z",
              importance: "critical",
              level: "error",
              message: "agent run failed",
              sessionId: "session-1",
              providerId: "openai",
              source: "adk",
            },
            {
              at: "2026-07-01T00:05:00.000Z",
              importance: "high",
              level: "error",
              message: "backtest sync failed",
              runId: "bt-1",
              taskId: "sync-1",
              instrumentId: "US.AAPL",
              source: "backtest",
            },
            {
              at: "2026-07-01T00:06:00.000Z",
              importance: "urgent",
              level: "warn",
              message: "future category",
              source: "api",
            },
          ],
          recentSlowRequests: [
            {
              at: "2026-07-01T00:10:00.000Z",
              importance: "normal",
              level: "info",
              message: "slow request",
              method: "GET",
              path: "/api/v1/system/status",
              latencyMs: 900,
              requestId: "req-1",
            },
          ],
          openD: {
            totalCalls: 3,
            failedCalls: 1,
            lastOperation: "proto_3006",
          },
        },
      },
    } as any),
    workerBrokerOrderUpdates: ref({
      ...emptyWorkerBrokerOrderUpdates,
      subscriptions: [
        {
          subscriptionKey: "sub-1",
          brokerId: "futu",
          tradingEnvironment: "REAL",
          accountId: "REAL-001",
          market: "US",
          status: "retrying",
          lastAction: "subscribe_failed",
          lastActionAt: "2026-07-01T00:00:00.000Z",
          lastError: "network down",
          lastErrorContext: {
            summary: "network down",
            rawMessage: "code=1006",
          },
          retryDelayMs: 4000,
          backoffUntil: "2026-07-01T00:04:00.000Z",
        },
      ],
      recentInvalidations: [],
      brokers: [
        {
          brokerId: "ib",
          lastAction: "retrying",
          lastActionAt: "2026-07-01T00:02:00.000Z",
          connectivity: "degraded",
          lastError: "gateway busy",
          accountsDiscovered: 2,
          activeSubscriptions: 1,
          retryingSubscriptions: 1,
          inactiveSubscriptions: 0,
          backoffSubscriptions: 1,
          topBackoffHotspots: [
            {
              subscriptionKey: "ib:REAL:acct-2:US",
              source: "ERROR",
              remainingMs: 3000,
              backoffUntil: "2026-07-01T00:05:00.000Z",
              lastActionAt: "2026-07-01T00:02:00.000Z",
              tradingEnvironment: "REAL",
              accountId: "acct-2",
              market: "US",
              reason: "gateway busy",
              reasonContext: {
                summary: "gateway busy",
                rawMessage: "busy",
              },
            },
          ],
          layeredBackoffSummaries: [],
        },
        {
          brokerId: "futu",
          lastAction: "invalidated",
          lastActionAt: "2026-07-01T00:01:00.000Z",
          connectivity: "degraded",
          lastError: "network down",
          accountsDiscovered: 1,
          activeSubscriptions: 0,
          retryingSubscriptions: 1,
          inactiveSubscriptions: 0,
          backoffSubscriptions: 1,
          topBackoffHotspots: [
            {
              subscriptionKey: "futu:REAL:REAL-001:US",
              source: "DISCONNECTED",
              remainingMs: 9000,
              backoffUntil: "2026-07-01T00:09:00.000Z",
              lastActionAt: "2026-07-01T00:01:00.000Z",
              tradingEnvironment: "REAL",
              accountId: "REAL-001",
              market: "US",
              reason: "network down",
              reasonContext: {
                summary: "network down",
                rawMessage: "network down",
              },
            },
          ],
          layeredBackoffSummaries: [
            {
              tradingEnvironment: "REAL",
              accountId: "REAL-001",
              activeSubscriptions: 0,
              retryingSubscriptions: 1,
              inactiveSubscriptions: 0,
              backoffSubscriptions: 1,
              dominantBackoffSource: "DISCONNECTED",
              topBackoffMarket: "US",
            },
          ],
        },
      ],
    } as any),
    isLoading: ref(false),
    isLoadingMarketData: ref(false),
    loadMarketDataSubscriptions,
    marketDataSubscriptions: ref({
      ...emptyMarketDataSubscriptions,
      totalActiveSubscriptions: 2,
      quota: {
        totalUsed: 2,
        totalLimit: 10,
        totalRemaining: 8,
        byMarket: [
          {
            market: "US",
            used: 2,
            limit: 5,
            remaining: 3,
          },
        ],
      },
      entries: [
        {
          key: "US.AAPL|quote",
          instrumentId: "US.AAPL",
          channel: "quote",
          interval: "",
          refCount: 2,
          market: "US",
          createdAt: "2026-07-01T00:00:00.000Z",
        },
      ],
    } as any),
  };
}

function mountSystemPage() {
  const wrapper = mount(SystemPage, {
    global: {
      stubs: {
        "v-alert": alertStub,
        "v-btn": buttonStub,
        "v-card": cardStub,
        "v-card-text": cardTextStub,
        "v-chip": chipStub,
        "v-empty-state": emptyStub,
        "v-tab": tabStub,
        "v-tabs": tabsStub,
        "v-window": windowStub,
        "v-window-item": windowItemStub,
      },
    },
  });
  wrappers.push(wrapper);
  return wrapper;
}

beforeEach(() => {
  testState.store = createSystemStore();

  const baseInstances = [
    createStrategyInstance(),
    createStrategyInstance({
      id: "legacy",
      status: "",
      definition: {
        strategyId: "legacy",
        name: "Legacy",
        version: "0.9.0",
      },
      binding: {
        runtimeRisk: undefined,
      },
    }),
  ];

  let updateAttempt = 0;
  testState.fetchEnvelopeMock = vi.fn(async (path: string) => {
    if (path === "/api/v1/strategies") {
      return baseInstances;
    }
    throw new Error(`Unexpected fetchEnvelope path: ${path}`);
  });
  testState.fetchEnvelopeWithInitMock = vi.fn(
    async (path: string, init: RequestInit) => {
      if (path === "/api/v1/system/real-trade-kill-switch/activate") {
        testState.store!.realTradeKillSwitchState.value = {
          ...testState.store!.realTradeKillSwitchState.value,
          killSwitchActive: true,
          controlPlaneActive: true,
          killSwitchSource: "CONTROL_PLANE",
        };
        return testState.store!.realTradeKillSwitchState.value;
      }
      if (path === "/api/v1/system/real-trade-kill-switch/release") {
        testState.store!.realTradeKillSwitchState.value = {
          ...testState.store!.realTradeKillSwitchState.value,
          killSwitchActive: false,
          controlPlaneActive: false,
          killSwitchSource: null,
        };
        return testState.store!.realTradeKillSwitchState.value;
      }
      if (path === "/api/v1/system/real-trade-hard-stops") {
        const payload = JSON.parse(String(init.body));
        testState.store!.realTradeHardStops.value = {
          ...testState.store!.realTradeHardStops.value,
          entries: [
            ...testState.store!.realTradeHardStops.value.entries,
            {
              id: "hard-stop-created",
              brokerId: payload.brokerId,
              accountId: payload.accountId,
              tradingEnvironment: payload.tradingEnvironment,
              market: payload.market,
              symbol: payload.symbol,
              operatorId: "local",
              reason: payload.reason,
            },
          ],
        };
        return testState.store!.realTradeHardStops.value;
      }
      if (path === "/api/v1/system/real-trade-hard-stops/hard-stop-1/release") {
        testState.store!.realTradeHardStops.value = {
          ...testState.store!.realTradeHardStops.value,
          entries: testState.store!.realTradeHardStops.value.entries.filter(
            (entry: any) => entry.id !== "hard-stop-1",
          ),
        };
        return testState.store!.realTradeHardStops.value;
      }
      if (!path.endsWith("/runtime-risk")) {
        throw new Error(`Unexpected fetchEnvelopeWithInit path: ${path}`);
      }
      updateAttempt += 1;
      if (updateAttempt === 1) {
        throw new Error("risk update failed");
      }
      const payload = JSON.parse(String(init.body));
      return createStrategyInstance({
        id: "instance-1",
        status: "STOPPED",
        binding: {
          runtimeRisk: {
            mode: payload.mode,
            closeOnly: true,
            maxOrderQuantity: 5,
            maxOrderNotional: 1000,
            dailyMaxOrders: 12,
            pauseOnReject: true,
          },
        },
      });
    },
  );
});

afterEach(() => {
  for (const wrapper of wrappers.splice(0)) {
    wrapper.unmount();
  }
  vi.clearAllMocks();
});

describe("SystemPage business flows", () => {
  it("treats nullable real-trade arrays as empty lists while rendering", async () => {
    testState.store!.realTradeApprovals.value = {
      ...testState.store!.realTradeApprovals.value,
      entries: null,
    };
    testState.store!.realTradeHardStops.value = {
      ...testState.store!.realTradeHardStops.value,
      blockedOperations: null,
      entries: null,
    };
    testState.store!.realTradeKillSwitchEvents.value = {
      ...testState.store!.realTradeKillSwitchEvents.value,
      blockedOperations: null,
      entries: null,
    };
    testState.store!.realTradeKillSwitchState.value = {
      ...testState.store!.realTradeKillSwitchState.value,
      blockedOperations: null,
    };
    testState.store!.realTradeRiskEvents.value = {
      ...testState.store!.realTradeRiskEvents.value,
      entries: null,
    };

    const wrapper = mountSystemPage();

    await flushPromises();

    expect(wrapper.text()).toContain("无活跃实盘硬停止。");
    expect(wrapper.text()).toContain("实盘熔断开关已激活");
  });

  it("renders rich system summaries and updates strategy runtime risk modes", async () => {
    const wrapper = mountSystemPage();

    await flushPromises();

    expect(testState.store?.loadMarketDataSubscriptions).toHaveBeenCalledTimes(1);
    expect(wrapper.text()).toContain("system api degraded");
    expect(wrapper.text()).toContain("ADK 运行");
    expect(wrapper.text()).toContain("回测任务");
    expect(wrapper.text()).toContain("urgent");
    expect(wrapper.text()).toContain("proto_3006");
    expect(wrapper.text()).toContain("实盘审批");
    expect(wrapper.text()).toContain("approval workflow is not implemented");
    expect(wrapper.text()).toContain("ops-a");
    expect(wrapper.text()).toContain("manual freeze");
    expect(wrapper.text()).toContain("quantity over limit");
    expect(wrapper.text()).toContain("US.AAPL");
    expect(wrapper.text()).toContain("Runtime Paused");
    expect(wrapper.text()).toContain("暂无");
    expect(wrapper.text()).toContain("execution-orders-db");
    expect(wrapper.text()).toContain("trading");
    expect(wrapper.text()).toContain("/tmp/execution-orders.db");
    expect(wrapper.text()).toContain("adk-session-db");
    expect(wrapper.text()).toContain("assistant/runtime");

    await wrapper
      .findAll("button")
      .find((button) => button.text() === "激活控制面熔断")!
      .trigger("click");
    await flushPromises();
    expect(testState.fetchEnvelopeWithInitMock).toHaveBeenCalledWith(
      "/api/v1/system/real-trade-kill-switch/activate",
      expect.objectContaining({ method: "POST" }),
    );

    await wrapper
      .findAll("button")
      .find((button) => button.text() === "解除控制面熔断")!
      .trigger("click");
    await flushPromises();
    expect(testState.fetchEnvelopeWithInitMock).toHaveBeenCalledWith(
      "/api/v1/system/real-trade-kill-switch/release",
      expect.objectContaining({ method: "POST" }),
    );

    await wrapper.get('input[aria-label="硬停止账户 ID"]').setValue("ACC-NEW");
    await wrapper.get('select[aria-label="硬停止范围"]').setValue("SYMBOL");
    await wrapper.get('input[aria-label="硬停止市场"]').setValue("us");
    await wrapper.get('input[aria-label="硬停止标的"]').setValue("msft");
    await wrapper.get('input[aria-label="硬停止原因"]').setValue("manual test");
    await wrapper
      .findAll("button")
      .find((button) => button.text() === "创建硬停止")!
      .trigger("click");
    await flushPromises();
    const hardStopCreateCall = testState.fetchEnvelopeWithInitMock.mock.calls.find(
      ([path]: [string]) => path === "/api/v1/system/real-trade-hard-stops",
    );
    expect(hardStopCreateCall).toBeTruthy();
    expect(JSON.parse(String((hardStopCreateCall![1] as RequestInit).body))).toEqual(
      expect.objectContaining({
        accountId: "ACC-NEW",
        hardStopScope: "SYMBOL",
        market: "US",
        symbol: "MSFT",
        reason: "manual test",
      }),
    );

    await wrapper
      .findAll("button")
      .find((button) => button.text() === "解除硬停止")!
      .trigger("click");
    await flushPromises();
    expect(testState.fetchEnvelopeWithInitMock).toHaveBeenCalledWith(
      "/api/v1/system/real-trade-hard-stops/hard-stop-1/release",
      expect.objectContaining({ method: "POST" }),
    );

    const refreshButtons = wrapper.findAll("button").filter((candidate) =>
      candidate.text() === "刷新",
    );
    expect(refreshButtons.length).toBeGreaterThanOrEqual(3);
    for (const button of refreshButtons) {
      await button.trigger("click");
    }
    await flushPromises();

    expect(testState.store?.loadSystemState).toHaveBeenCalled();
    expect(testState.store?.loadMarketDataSubscriptions).toHaveBeenCalledTimes(2);
    expect(
      testState.fetchEnvelopeMock.mock.calls.filter(
        ([path]: [string]) => path === "/api/v1/strategies",
      ).length,
    ).toBeGreaterThanOrEqual(2);

    const legacyModeSelect = wrapper.get('select[aria-label="Legacy 动态风控模式"]');
    await legacyModeSelect.setValue("monitor");
    await flushPromises();
    expect(wrapper.text()).toContain("risk update failed");

    const primaryModeSelect = wrapper.get(
      'select[aria-label="Mean Revert 动态风控模式"]',
    );
    await primaryModeSelect.setValue("enforce");
    await flushPromises();

    expect(testState.fetchEnvelopeWithInitMock).toHaveBeenCalledWith(
      "/api/v1/strategies/instance-1/runtime-risk",
      expect.objectContaining({
        method: "PUT",
        headers: { "Content-Type": "application/json" },
      }),
    );
    const latestInit =
      testState.fetchEnvelopeWithInitMock.mock.calls[
        testState.fetchEnvelopeWithInitMock.mock.calls.length - 1
      ]?.[1] as RequestInit;
    expect(JSON.parse(String(latestInit.body))).toEqual(
      expect.objectContaining({
        mode: "enforce",
        closeOnly: true,
        pauseOnReject: true,
      }),
    );
    expect(testState.store?.loadSystemState).toHaveBeenCalledWith({
      bypassCooldown: true,
    });
    expect(primaryModeSelect.element.value).toBe("enforce");
    expect(wrapper.text()).toContain("Runtime Stopped");
  });
});
