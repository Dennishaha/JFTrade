// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

import {
  emptyBrokerCashFlows,
  emptyBrokerFunds,
  emptyBrokerOrders,
  emptyBrokerPositions,
  emptyBrokerRuntime,
  emptyExecutionOrders,
  emptyMarketDataSubscriptions,
  emptyPortfolioCashBalances,
  emptyPortfolioCashReconciliation,
  emptyPortfolioPositions,
  emptyPortfolioReconciliation,
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
} from "@jftrade/ui-contracts";
import type {
  StrategyDefinitionDocument,
  SystemStatusResponse,
} from "@jftrade/ui-contracts";
import StrategyLogicFlowDesigner from "../src/components/StrategyLogicFlowDesigner.vue";

import {
  MockEventSource,
  createResponse,
  flushRequests,
  mountApp,
} from "./helpers";

afterEach(() => {
  vi.unstubAllGlobals();
  MockEventSource.instances = [];
});

async function openStrategyWorkspaceTab(
  wrapper: Awaited<ReturnType<typeof mountApp>>["wrapper"],
  tab: "runtime" | "design",
) {
  await wrapper
    .get(`[data-testid="strategy-workspace-tab-${tab}"]`)
    .trigger("click");
  await flushRequests();
}

async function openStrategyDesignWorkspace(
  wrapper: Awaited<ReturnType<typeof mountApp>>["wrapper"],
) {
  await openStrategyWorkspaceTab(wrapper, "design");
}

async function showStrategyCodeEditor(
  wrapper: Awaited<ReturnType<typeof mountApp>>["wrapper"],
  mode: "split" | "code" = "code",
) {
  await wrapper
    .get(`[data-testid="strategy-display-mode-${mode}"]`)
    .trigger("click");
  await flushRequests();
}

async function openNewStrategyFromRuntime(
  wrapper: Awaited<ReturnType<typeof mountApp>>["wrapper"],
) {
  await wrapper.get('[data-testid="strategy-new-definition"]').trigger("click");
  await flushRequests();
}

async function openStrategyTemplatesPanel(
  wrapper: Awaited<ReturnType<typeof mountApp>>["wrapper"],
) {
  if (wrapper.find('[data-testid="strategy-templates-section"]').exists()) {
    return;
  }
  await wrapper.get('[data-testid="toggle-strategy-templates-section"]').trigger("click");
  await flushRequests();
}

function buildFetchMock(options: {
  systemStatus?: SystemStatusResponse;
  definitions?: StrategyDefinitionDocument[];
  strategies?: Array<{
    id: string;
    definition: {
      strategyId: string;
      name: string;
      version: string;
    };
    params: Record<string, unknown>;
    status: "RUNNING" | "PAUSED" | "STOPPED";
    createdAt: string;
    logs: string[];
  }>;
  logsById?: Record<string, string[]>;
  auditById?: Record<
    string,
    Array<{
      instanceId: string;
      kind: string;
      detail?: string;
      at: string;
    }>
  >;
}) {
  const systemStatus = options.systemStatus ?? emptySystemStatus;
  const definitions = options.definitions ?? [];
  const strategies = options.strategies ?? [];
  const logsById = options.logsById ?? {};
  const auditById = options.auditById ?? {};

  const runtimeState = {
    strategies: strategies.map((strategy) => ({
      ...strategy,
      params: { ...strategy.params },
      definition: { ...strategy.definition },
      logs: [...strategy.logs],
    })),
    logsById: { ...logsById },
    auditById: { ...auditById },
  };

  return vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const request = input instanceof Request ? input : null;
    const url = String(input);
    const method = request?.method ?? init?.method ?? "GET";
    const logsMatch = url.match(/\/api\/v1\/strategies\/([^/]+)\/logs/);
    const auditMatch = url.match(/\/api\/v1\/strategies\/([^/]+)\/audit/);
    const instantiateMatch = url.match(/\/api\/v1\/strategy-definitions\/([^/]+)\/instantiate/);
    const lifecycleMatch = url.match(/\/api\/v1\/strategies\/([^/]+)\/(start|pause|stop)/);

    if (url.includes("/api/v1/market-data/subscriptions"))
      return createResponse(emptyMarketDataSubscriptions);
    if (url.includes("/api/v1/system/status"))
      return createResponse(systemStatus);
    if (url.includes("/api/v1/system/storage/overview"))
      return createResponse(emptyStorageOverview);
    if (url.includes("/api/v1/system/real-trade-approvals"))
      return createResponse(emptyRealTradeApprovals);
    if (url.includes("/api/v1/system/real-trade-hard-stops"))
      return createResponse(emptyRealTradeHardStops);
    if (url.includes("/api/v1/system/real-trade-hard-stop-events"))
      return createResponse(emptyRealTradeHardStopEvents);
    if (url.includes("/api/v1/system/real-trade-kill-switch-events"))
      return createResponse(emptyRealTradeKillSwitchEvents);
    if (url.includes("/api/v1/system/real-trade-kill-switch"))
      return createResponse(emptyRealTradeKillSwitchState);
    if (url.includes("/api/v1/system/real-trade-risk-events"))
      return createResponse(emptyRealTradeRiskEvents);
    if (url.includes("/api/v1/system/real-trade-risk-limits"))
      return createResponse(emptyRealTradeRiskState);
    if (url.includes("/api/v1/system/worker/broker-order-updates"))
      return createResponse(emptyWorkerBrokerOrderUpdates);
    if (url.includes("/api/v1/brokers/futu/runtime"))
      return createResponse(emptyBrokerRuntime);
    if (url.includes("/api/v1/brokers/futu/funds"))
      return createResponse(emptyBrokerFunds);
    if (url.includes("/api/v1/brokers/futu/positions"))
      return createResponse(emptyBrokerPositions);
    if (url.includes("/api/v1/brokers/futu/orders"))
      return createResponse(emptyBrokerOrders);
    if (url.includes("/api/v1/portfolio/futu/cash-balances"))
      return createResponse(emptyPortfolioCashBalances);
    if (url.includes("/api/v1/portfolio/futu/positions"))
      return createResponse(emptyPortfolioPositions);
    if (url.includes("/api/v1/portfolio/futu/cash-reconciliation"))
      return createResponse(emptyPortfolioCashReconciliation);
    if (url.includes("/api/v1/portfolio/futu/reconciliation"))
      return createResponse(emptyPortfolioReconciliation);
    if (url.includes("/api/v1/execution/orders"))
      return createResponse(emptyExecutionOrders);
    if (instantiateMatch && method === "POST") {
      const definitionId = decodeURIComponent(instantiateMatch[1]);
      const definition = definitions.find((item) => item.id === definitionId);
      if (definition === undefined) {
        throw new Error(`Unknown strategy definition: ${definitionId}`);
      }
      const instanceId = `${definitionId}-instance`;
      const instance = {
        id: instanceId,
        definition: {
          strategyId: definition.id,
          name: definition.name,
          version: definition.version,
        },
        params: {
          runtime: definition.runtime,
          definitionId: definition.id,
          symbol: definition.symbol,
          interval: definition.interval,
          script: definition.script,
        },
        status: "STOPPED" as const,
        createdAt: definition.updatedAt,
        logs: [],
      };
      runtimeState.strategies = [instance, ...runtimeState.strategies.filter((item) => item.id !== instanceId)];
      runtimeState.logsById[instanceId] = [
        `${definition.updatedAt} instantiated strategy from definition ${definition.id}`,
      ];
      runtimeState.auditById[instanceId] = [
        {
          instanceId,
          kind: "instantiated",
          detail: definition.id,
          at: definition.updatedAt,
        },
      ];
      return createResponse(instance);
    }
    if (lifecycleMatch && method === "POST") {
      const instanceId = decodeURIComponent(lifecycleMatch[1]);
      const action = lifecycleMatch[2];
      const instance = runtimeState.strategies.find((item) => item.id === instanceId);
      if (instance === undefined) {
        throw new Error(`Unknown strategy instance: ${instanceId}`);
      }
      const nextStatus = action === "start" ? "RUNNING" : action === "pause" ? "PAUSED" : "STOPPED";
      instance.status = nextStatus;
      runtimeState.logsById[instanceId] = [
        ...(runtimeState.logsById[instanceId] ?? []),
        `2026-05-23T00:00:00.000Z ${action}ed strategy ${instance.definition.strategyId}`,
      ];
      runtimeState.auditById[instanceId] = [
        ...(runtimeState.auditById[instanceId] ?? []),
        {
          instanceId,
          kind: action === "start" ? "started" : action === "pause" ? "paused" : "stopped",
          detail: action === "pause" ? "manual pause" : `manual ${action}`,
          at: "2026-05-23T00:00:00.000Z",
        },
      ];
      return createResponse(instance);
    }
    if (url.includes("/api/v1/strategy-definitions"))
      return createResponse(definitions);
    if (logsMatch) {
      const instanceId = decodeURIComponent(logsMatch[1]);
      return createResponse({
        instanceId,
        logs: runtimeState.logsById[instanceId] ?? [],
      });
    }
    if (auditMatch) {
      const instanceId = decodeURIComponent(auditMatch[1]);
      return createResponse({
        instanceId,
        entries: runtimeState.auditById[instanceId] ?? [],
      });
    }
    if (url.includes("/api/v1/strategies")) return createResponse(runtimeState.strategies);

    throw new Error(`Unexpected request: ${url}`);
  });
}

describe("Strategy page", () => {
  it("lists strategies and shows the selected strategy logs and audit", async () => {
    const strategies = [
      {
        id: "instance-1",
        definition: {
          strategyId: "s-mean-revert",
          name: "Mean Revert",
          version: "1.0.0",
        },
        params: {
          threshold: 10,
        },
        status: "RUNNING" as const,
        createdAt: "2026-05-16T00:00:00.000Z",
        logs: [],
      },
      {
        id: "instance-2",
        definition: {
          strategyId: "s-breakout",
          name: "Breakout",
          version: "1.0.0",
        },
        params: {
          window: 20,
        },
        status: "PAUSED" as const,
        createdAt: "2026-05-16T00:01:00.000Z",
        logs: [],
      },
    ];
    const systemStatus: SystemStatusResponse = {
      ...emptySystemStatus,
      defaultTradingEnvironment: "REAL",
      realTradingEnabled: true,
    };

    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        systemStatus,
        strategies,
        logsById: {
          "instance-1": [
            "2026-05-16T00:00:00.000Z started strategy s-mean-revert",
            "2026-05-16T00:00:02.000Z tick QUOTE_SNAPSHOT HK.00700",
          ],
          "instance-2": ["2026-05-16T00:01:00.000Z paused strategy execution"],
        },
        auditById: {
          "instance-1": [
            {
              instanceId: "instance-1",
              kind: "started",
              detail: "s-mean-revert",
              at: "2026-05-16T00:00:00.000Z",
            },
            {
              instanceId: "instance-1",
              kind: "tick",
              detail: "QUOTE_SNAPSHOT HK.00700",
              at: "2026-05-16T00:00:02.000Z",
            },
          ],
          "instance-2": [
            {
              instanceId: "instance-2",
              kind: "paused",
              at: "2026-05-16T00:01:10.000Z",
            },
          ],
        },
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/strategy");
    await openStrategyWorkspaceTab(wrapper, "runtime");

    expect(wrapper.text()).toContain("策略实例");
    expect(wrapper.text()).toContain("Mean Revert");
    expect(wrapper.text()).toContain("Breakout");
    expect(wrapper.text()).toContain("tick QUOTE_SNAPSHOT HK.00700");
    expect(wrapper.text()).toContain("运行审计");
    expect(wrapper.text()).toContain("QUOTE_SNAPSHOT HK.00700");
    expect(wrapper.text()).toContain("REAL");

    wrapper.unmount();
  });

  it("switches selected strategy and refreshes logs and audit", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        systemStatus: {
          ...emptySystemStatus,
          realTradingKillSwitch: {
            ...emptySystemStatus.realTradingKillSwitch,
            active: true,
          },
        },
        strategies: [
          {
            id: "instance-1",
            definition: {
              strategyId: "s-alpha",
              name: "Alpha",
              version: "1.0.0",
            },
            params: { fast: 5 },
            status: "RUNNING",
            createdAt: "2026-05-16T00:00:00.000Z",
            logs: [],
          },
          {
            id: "instance-2",
            definition: {
              strategyId: "s-beta",
              name: "Beta",
              version: "1.0.0",
            },
            params: { slow: 13 },
            status: "PAUSED",
            createdAt: "2026-05-16T00:02:00.000Z",
            logs: [],
          },
        ],
        logsById: {
          "instance-1": ["2026-05-16T00:00:00.000Z started strategy s-alpha"],
          "instance-2": ["2026-05-16T00:02:00.000Z paused strategy execution"],
        },
        auditById: {
          "instance-1": [
            {
              instanceId: "instance-1",
              kind: "started",
              detail: "s-alpha",
              at: "2026-05-16T00:00:00.000Z",
            },
          ],
          "instance-2": [
            {
              instanceId: "instance-2",
              kind: "paused",
              detail: "manual pause",
              at: "2026-05-16T00:02:10.000Z",
            },
          ],
        },
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/strategy");
    await openStrategyWorkspaceTab(wrapper, "runtime");

    await wrapper.get('[data-testid="strategy-instance-2"]').trigger("click");
    await flushRequests();

    expect(wrapper.text()).toContain("paused strategy execution");
    expect(wrapper.text()).toContain("manual pause");
    expect(wrapper.text()).toContain("已启用");

    wrapper.unmount();
  });

  it("shows the quickjs strategy design workspace", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [
          {
            id: "js-mean-revert",
            name: "JS Mean Revert",
            version: "0.1.0",
            description: "quickjs strategy",
            runtime: "quickjs-js",
            symbol: "00700",
            interval: "1m",
            script: "function onKLineClosed(ctx) { console.log(ctx.kline.close); }",
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/strategy");

    expect(wrapper.text()).toContain("策略运行");
    expect(wrapper.find('[data-testid="strategy-script-editor"]').exists()).toBe(false);

    await openStrategyDesignWorkspace(wrapper);

    expect(wrapper.text()).toContain("设计");
    expect(wrapper.text()).toContain("策略定义");
    expect(wrapper.text()).toContain("JS Mean Revert");
    expect(wrapper.text()).toContain("quickjs-js");
    expect(wrapper.find('[data-testid="strategy-logic-flow-canvas"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-visual-builder-section"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-logic-flow-zoom-fit"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-logic-flow-builder"]').exists()).toBe(true);
    expect(wrapper.get('[data-testid="strategy-logic-flow-builder-toggle"]').text()).toContain("展开创建器");
    expect(wrapper.find('[data-testid="toggle-strategy-visual-builder-section"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="toggle-strategy-metadata-section"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="toggle-strategy-block-inspector-section"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-definitions-panel"]').exists()).toBe(false);
    expect(wrapper.findAll('.strategy-stage__toolbar-card')).toHaveLength(1);
    expect(wrapper.find('[data-testid="sync-visual-script"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="reset-visual-model"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-visual-sync-status"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-code-editor-section"]').exists()).toBe(false);

    await showStrategyCodeEditor(wrapper, "code");

  expect(wrapper.text()).toContain("QuickJS 代码工作台");
    expect(wrapper.get('[data-testid="strategy-script-editor"]').element).toBeTruthy();
    expect(wrapper.html()).toContain("function onKLineClosed(ctx)");

    expect(wrapper.find('[data-testid="strategy-templates-section"]').exists()).toBe(false);
    await openStrategyTemplatesPanel(wrapper);
    expect(wrapper.text()).toContain("双均线系统");
    expect(wrapper.text()).toContain("MACD 动能交易");
    expect(wrapper.text()).toContain("布林带回归交易");

    wrapper.unmount();
  });

  it("supports searching inside the builder while keeping the close control at the launcher position", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [
          {
            id: "js-mean-revert",
            name: "JS Mean Revert",
            version: "0.1.0",
            description: "quickjs strategy",
            runtime: "quickjs-js",
            symbol: "00700",
            interval: "1m",
            script: "function onKLineClosed(ctx) { console.log(ctx.kline.close); }",
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/strategy");

    await openStrategyDesignWorkspace(wrapper);

    const toggle = wrapper.get('[data-testid="strategy-logic-flow-builder-toggle"]');
    const variablesToggle = wrapper.get('[data-testid="strategy-logic-flow-variables-toggle"]');
    expect(toggle.text()).toContain("展开创建器");
    expect(variablesToggle.text()).toContain("变量 0");
    expect(wrapper.find('.strategy-logic-flow-builder__grid').exists()).toBe(false);

    await toggle.trigger("click");
    await flushRequests();

    expect(wrapper.get('[data-testid="strategy-logic-flow-builder-toggle"]').text()).toContain("关闭创建器");
    expect(wrapper.find('.strategy-logic-flow-builder__grid').exists()).toBe(true);

    const initialLabels = wrapper.findAll('.strategy-logic-flow-builder__label').map((item) => item.text());
    expect(initialLabels).toContain("指标条件判断");
    expect(initialLabels).not.toContain("指标数据");
    expect(initialLabels).not.toContain("技术指标");

    await wrapper.get('[data-testid="strategy-logic-flow-builder-search"]').setValue("通知");
    await flushRequests();

    const filteredLabels = wrapper.findAll('.strategy-logic-flow-builder__label').map((item) => item.text());
    expect(filteredLabels).toContain("发送通知");
    expect(filteredLabels).not.toContain("输出日志");

    await wrapper.get('[data-testid="strategy-logic-flow-builder-search"]').setValue("不存在的图块");
    await flushRequests();

    expect(wrapper.text()).toContain("没有匹配的图块");

    await wrapper.get('[data-testid="strategy-logic-flow-builder-toggle"]').trigger("click");
    await flushRequests();

    expect(wrapper.get('[data-testid="strategy-logic-flow-builder-toggle"]').text()).toContain("展开创建器");
    expect(wrapper.find('.strategy-logic-flow-builder__grid').exists()).toBe(false);

    wrapper.unmount();
  });

  it("collapses the strategy definitions sidebar to free editing space", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [
          {
            id: "js-mean-revert",
            name: "JS Mean Revert",
            version: "0.1.0",
            description: "quickjs strategy",
            runtime: "quickjs-js",
            symbol: "00700",
            interval: "1m",
            script: "function onKLineClosed(ctx) { console.log(ctx.kline.close); }",
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/strategy");

    await openStrategyDesignWorkspace(wrapper);

    expect(wrapper.find('[data-testid="strategy-definitions-panel"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-definition-js-mean-revert"]').exists()).toBe(false);

    await wrapper.get('[data-testid="toggle-strategy-definitions-floating"]').trigger("click");
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-definitions-panel"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-definition-js-mean-revert"]').exists()).toBe(true);

    await wrapper.get('[data-testid="toggle-strategy-definitions"]').trigger("click");
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-definitions-panel"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-definition-js-mean-revert"]').exists()).toBe(false);

    await wrapper.get('[data-testid="toggle-strategy-definitions-floating"]').trigger("click");
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-definitions-panel"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-definition-js-mean-revert"]').exists()).toBe(true);

    wrapper.unmount();
  });

  it("switches the workspace display modes", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [
          {
            id: "js-mean-revert",
            name: "JS Mean Revert",
            version: "0.1.0",
            description: "quickjs strategy",
            runtime: "quickjs-js",
            symbol: "00700",
            interval: "1m",
            script: "function onKLineClosed(ctx) { console.log(ctx.kline.close); }",
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/strategy");

    await openStrategyDesignWorkspace(wrapper);

    expect(wrapper.find('[data-testid="strategy-logic-flow-canvas"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-visual-builder-section"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-logic-flow-zoom"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-code-editor-section"]').exists()).toBe(false);

    await wrapper.get('[data-testid="strategy-display-mode-code"]').trigger("click");
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-logic-flow-canvas"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-visual-builder-section"]').exists()).toBe(false);
    expect(wrapper.get('[data-testid="strategy-script-editor"]').element).toBeTruthy();

    await wrapper.get('[data-testid="strategy-display-mode-canvas"]').trigger("click");
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-logic-flow-canvas"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-code-editor-section"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-visual-builder-section"]').exists()).toBe(true);

    await wrapper.get('[data-testid="strategy-display-mode-split"]').trigger("click");
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-logic-flow-canvas"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-code-editor-section"]').exists()).toBe(true);

    wrapper.unmount();
  });

  it("opens and closes floating strategy editor panels from the toolbar", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [
          {
            id: "js-mean-revert",
            name: "JS Mean Revert",
            version: "0.1.0",
            description: "quickjs strategy",
            runtime: "quickjs-js",
            symbol: "00700",
            interval: "1m",
            script: "function onKLineClosed(ctx) { console.log(ctx.kline.close); }",
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/strategy");

    await openStrategyDesignWorkspace(wrapper);

    expect(wrapper.find('[data-testid="strategy-templates-section"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-basic-info-section"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-visual-builder-section"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-code-editor-section"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-metadata-section"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-definitions-panel"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="toggle-strategy-visual-builder-section"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="toggle-strategy-metadata-section"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="toggle-strategy-block-inspector-section"]').exists()).toBe(false);

    await wrapper.get('[data-testid="toggle-strategy-templates-section"]').trigger("click");
    await wrapper.get('[data-testid="toggle-strategy-basic-info-section"]').trigger("click");
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-templates-section"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-basic-info-section"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-metadata-section"]').exists()).toBe(false);
    expect(wrapper.get('[data-testid="strategy-basic-info-section"]').text()).toContain("元信息");

    await wrapper.get('[data-testid="strategy-display-mode-canvas"]').trigger("click");
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-code-editor-section"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-visual-builder-section"]').exists()).toBe(true);

    await wrapper.get('[data-testid="strategy-display-mode-code"]').trigger("click");
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-logic-flow-canvas"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-code-editor-section"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-templates-section"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-basic-info-section"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-metadata-section"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-visual-builder-section"]').exists()).toBe(false);

    await wrapper.get('[data-testid="strategy-display-mode-split"]').trigger("click");
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-logic-flow-canvas"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-visual-builder-section"]').exists()).toBe(true);

    wrapper.unmount();
  });

  it("auto opens block details when selecting a visual node and hides them when selection clears", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [
          {
            id: "js-rsi-visual",
            name: "RSI Visual",
            version: "0.2.0",
            description: "visual strategy",
            runtime: "quickjs-js",
            symbol: "00700",
            interval: "1m",
            script: [
              "/** @param {JFTradeInitContext} ctx */",
              "function onInit(ctx) {",
              "}",
              "",
              "/** @param {JFTradeKLineClosedContext} ctx */",
              "function onKLineClosed(ctx) {",
              "  const close = Number(ctx && ctx.kline ? ctx.kline.close : NaN);",
              "  if (!Number.isFinite(close)) {",
              '    console.log("skip candle because close is not a finite number");',
              "    return;",
              "  }",
              "",
              "  let latestRsi = null;",
              "",
              "  /**",
              "   * @jftradeFlowNodeId rsi-calc-node",
              "   * @jftradeFlowBlockKind technicalIndicator",
              "   * @jftradeFlowNodeText RSI 14 < 30",
              "   */",
              '  latestRsi = ctx.indicators["rsi:14"] ?? null;',
              "  if (latestRsi === null) {",
              '    console.log("waiting for indicator rsi:14");',
              "    return;",
              "  }",
              "  if (latestRsi < 30) {",
              "    /**",
              "     * @jftradeFlowNodeId notify-node",
              "     * @jftradeFlowBlockKind notify",
              "     * @jftradeFlowNodeText 发送通知",
              "     */",
              '    notify("RSI changed");',
              "  }",
              "}",
            ].join("\n"),
            visualModel: {
              engine: "logic-flow",
              version: 1,
              nodes: [
                {
                  id: "rsi-calc-node",
                  type: "rect",
                  x: 420,
                  y: 200,
                  text: "RSI 14 < 30",
                  properties: {
                    blockKind: "technicalIndicator",
                    indicatorType: "rsi",
                    conditionMode: "numeric",
                    operator: "<",
                    threshold: 30,
                    period: 14,
                  },
                },
                {
                  id: "on-kline-root",
                  type: "circle",
                  x: 160,
                  y: 200,
                  text: "K 线收盘",
                  properties: {
                    blockKind: "onKLineClosed",
                  },
                },
              ],
              edges: [
                {
                  id: "edge-1",
                  type: "polyline",
                  sourceNodeId: "on-kline-root",
                  targetNodeId: "rsi-calc-node",
                },
              ],
            },
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/strategy");

    await openStrategyDesignWorkspace(wrapper);

    expect(wrapper.find('[data-testid="strategy-block-inspector-card"]').exists()).toBe(false);

    wrapper.findComponent(StrategyLogicFlowDesigner).vm.$emit("select-node", "rsi-calc-node");
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-block-inspector-card"]').exists()).toBe(true);
    expect(wrapper.text()).toContain("图块详情");
    expect(wrapper.text()).toContain("RSI");

    wrapper.findComponent(StrategyLogicFlowDesigner).vm.$emit("select-node", null);
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-block-inspector-card"]').exists()).toBe(false);

    wrapper.unmount();
  });

  it("shows getter variable naming and condition input selectors in the block inspector", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [
          {
            id: "js-indicator-bindings",
            name: "JS Indicator Bindings",
            version: "0.2.0",
            description: "indicator binding inspector",
            runtime: "quickjs-js",
            symbol: "00700",
            interval: "1m",
            script: "function manualOnly() { return 'keep'; }",
            visualModel: {
              engine: "logic-flow",
              version: 1,
              nodes: [
                {
                  id: "on-kline-root",
                  type: "circle",
                  x: 160,
                  y: 200,
                  text: "K 线收盘",
                  properties: { blockKind: "onKLineClosed" },
                },
                {
                  id: "fast-ma",
                  type: "rect",
                  x: 380,
                  y: 150,
                  text: "获取 双均线 EMA 5",
                  properties: {
                    blockKind: "getTechnicalIndicator",
                    indicatorType: "movingAverage",
                    movingAverageType: "EMA",
                    windowSize: 5,
                    variableName: "EMA5",
                  },
                },
                {
                  id: "slow-ma",
                  type: "rect",
                  x: 380,
                  y: 260,
                  text: "获取 双均线 EMA 20",
                  properties: {
                    blockKind: "getTechnicalIndicator",
                    indicatorType: "movingAverage",
                    movingAverageType: "EMA",
                    windowSize: 20,
                    variableName: "EMA20",
                  },
                },
                {
                  id: "ma-condition",
                  type: "diamond",
                  x: 640,
                  y: 205,
                  text: "双均线金叉",
                  properties: {
                    blockKind: "technicalIndicatorCondition",
                    indicatorType: "movingAverage",
                    conditionMode: "pattern",
                    patternType: "goldenCross",
                    inputFastNodeId: "fast-ma",
                    inputSlowNodeId: "slow-ma",
                  },
                },
              ],
              edges: [
                {
                  id: "edge-root-fast",
                  type: "polyline",
                  sourceNodeId: "on-kline-root",
                  targetNodeId: "fast-ma",
                },
                {
                  id: "edge-fast-slow",
                  type: "polyline",
                  sourceNodeId: "fast-ma",
                  targetNodeId: "slow-ma",
                },
                {
                  id: "edge-slow-condition",
                  type: "polyline",
                  sourceNodeId: "slow-ma",
                  targetNodeId: "ma-condition",
                },
              ],
            },
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/strategy");

    await openStrategyDesignWorkspace(wrapper);

    const variablesToggle = wrapper.get('[data-testid="strategy-logic-flow-variables-toggle"]');
    expect(variablesToggle.text()).toContain("变量 2");

    await variablesToggle.trigger("click");
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-logic-flow-variables"]').exists()).toBe(true);
    expect(wrapper.text()).toContain("EMA5");
    expect(wrapper.text()).toContain("EMA20");

    wrapper.findComponent(StrategyLogicFlowDesigner).vm.$emit("select-node", "fast-ma");
    await flushRequests();

    const variableNameInput = wrapper.get('[data-testid="strategy-block-variable-name-input"]');
    expect(variableNameInput.element.getAttribute("placeholder")).toBe("EMA5");
    expect((variableNameInput.element as HTMLInputElement).value).toBe("EMA5");

    wrapper.findComponent(StrategyLogicFlowDesigner).vm.$emit("select-node", "ma-condition");
    await flushRequests();

    expect((wrapper.get('[data-testid="strategy-block-indicator-input-fast-select"]').element as HTMLSelectElement).value).toBe("fast-ma");
    expect((wrapper.get('[data-testid="strategy-block-indicator-input-slow-select"]').element as HTMLSelectElement).value).toBe("slow-ma");
    expect(wrapper.text()).toContain("EMA5 · 获取 均线 EMA 5日");
    expect(wrapper.text()).toContain("EMA20 · 获取 均线 EMA 20日");

    wrapper.unmount();
  });

  it("shows entry position policy only for opening order sides in the block inspector", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [
          {
            id: "js-place-order-policy",
            name: "JS Place Order Policy",
            version: "0.2.0",
            description: "place order policy inspector",
            runtime: "quickjs-js",
            symbol: "00700",
            interval: "1m",
            script: "function manualOnly() { return 'keep'; }",
            visualModel: {
              engine: "logic-flow",
              version: 1,
              nodes: [
                {
                  id: "on-kline-root",
                  type: "circle",
                  x: 160,
                  y: 200,
                  text: "K 线收盘",
                  properties: { blockKind: "onKLineClosed" },
                },
                {
                  id: "buy-node",
                  type: "rect",
                  x: 420,
                  y: 200,
                  text: "下单",
                  properties: {
                    blockKind: "placeOrder",
                    side: "BUY",
                    orderType: "MARKET",
                    quantityMode: "shares",
                    quantityValue: 100,
                  },
                },
              ],
              edges: [
                {
                  id: "edge-root-buy",
                  type: "polyline",
                  sourceNodeId: "on-kline-root",
                  targetNodeId: "buy-node",
                },
              ],
            },
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/strategy");

    await openStrategyDesignWorkspace(wrapper);

    wrapper.findComponent(StrategyLogicFlowDesigner).vm.$emit("select-node", "buy-node");
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-place-order-entry-position-policy"]').exists()).toBe(true);
    expect(wrapper.text()).toContain("账户仓位百分比");
    expect(wrapper.text()).toContain("当前标的仓位百分比");
    expect(wrapper.text()).toContain("融资可用百分比");
    expect(wrapper.text()).toContain("融券可用百分比");

    expect((wrapper.get('option[value="marginBuyingPowerPercent"]').element as HTMLOptionElement).disabled).toBe(false);
    expect((wrapper.get('option[value="shortSellingPowerPercent"]').element as HTMLOptionElement).disabled).toBe(true);

    await wrapper.find('[data-testid="strategy-place-order-side"]').setValue("SELL_SHORT");
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-place-order-entry-position-policy"]').exists()).toBe(true);
    expect((wrapper.get('option[value="marginBuyingPowerPercent"]').element as HTMLOptionElement).disabled).toBe(true);
    expect((wrapper.get('option[value="shortSellingPowerPercent"]').element as HTMLOptionElement).disabled).toBe(false);

    await wrapper.find('[data-testid="strategy-place-order-side"]').setValue("SELL");
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-place-order-entry-position-policy"]').exists()).toBe(false);
    expect((wrapper.get('option[value="marginBuyingPowerPercent"]').element as HTMLOptionElement).disabled).toBe(true);
    expect((wrapper.get('option[value="shortSellingPowerPercent"]').element as HTMLOptionElement).disabled).toBe(true);

    wrapper.unmount();
  });

  it("disconnects selected flow edges by menu action and keyboard delete", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [
          {
            id: "js-edge-disconnect",
            name: "JS Edge Disconnect",
            version: "0.2.0",
            description: "edge disconnect inspector",
            runtime: "quickjs-js",
            symbol: "00700",
            interval: "1m",
            script: "function manualOnly() { return 'keep'; }",
            visualModel: {
              engine: "logic-flow",
              version: 1,
              nodes: [
                {
                  id: "on-kline-root",
                  type: "circle",
                  x: 160,
                  y: 200,
                  text: "K 线收盘",
                  properties: { blockKind: "onKLineClosed" },
                },
                {
                  id: "notify-node",
                  type: "rect",
                  x: 420,
                  y: 200,
                  text: "发送通知",
                  properties: { blockKind: "notify", message: "edge one" },
                },
                {
                  id: "log-node",
                  type: "rect",
                  x: 700,
                  y: 200,
                  text: "输出日志",
                  properties: { blockKind: "log", message: "edge two" },
                },
              ],
              edges: [
                {
                  id: "edge-root-notify",
                  type: "polyline",
                  sourceNodeId: "on-kline-root",
                  targetNodeId: "notify-node",
                },
                {
                  id: "edge-notify-log",
                  type: "polyline",
                  sourceNodeId: "notify-node",
                  targetNodeId: "log-node",
                },
              ],
            },
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/strategy");

    await openStrategyDesignWorkspace(wrapper);

    const designer = wrapper.findComponent(StrategyLogicFlowDesigner);
    const designerVm = designer.vm as unknown as {
      selectEdgeById: (edgeId: string | null) => void;
    };

    designerVm.selectEdgeById("edge-root-notify");
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-logic-flow-edge-menu"]').exists()).toBe(true);
    expect(
      wrapper.get('[data-testid="strategy-logic-flow-canvas"]').element.contains(
        wrapper.get('[data-testid="strategy-logic-flow-edge-menu"]').element,
      ),
    ).toBe(false);

    await wrapper.get('[data-testid="strategy-logic-flow-edge-disconnect"]').trigger("click");
    await flushRequests();

    let visualModel = designer.props("modelValue") as NonNullable<StrategyDefinitionDocument["visualModel"]>;
    expect(visualModel.edges.map((edge) => edge.id)).toEqual(["edge-notify-log"]);

    designerVm.selectEdgeById("edge-notify-log");
    await flushRequests();
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Delete" }));
    await flushRequests();

    visualModel = designer.props("modelValue") as NonNullable<StrategyDefinitionDocument["visualModel"]>;
    expect(visualModel.edges).toHaveLength(0);
    expect(wrapper.find('[data-testid="strategy-logic-flow-edge-menu"]').exists()).toBe(false);

    wrapper.unmount();
  });

  it("auto syncs a saved logic flow model back into quickjs code", async () => {
    const visualModel = {
      engine: "logic-flow" as const,
      version: 1,
      nodes: [
        {
          id: "on-kline-root",
          type: "circle",
          x: 160,
          y: 200,
          text: "K 线收盘",
          properties: {
            blockKind: "onKLineClosed",
          },
        },
        {
          id: "notify-node",
          type: "rect",
          x: 380,
          y: 200,
          text: "发送通知",
          properties: {
            blockKind: "notify",
            message: "收盘价触发视觉策略",
          },
        },
      ],
      edges: [
        {
          id: "edge-1",
          type: "polyline",
          sourceNodeId: "on-kline-root",
          targetNodeId: "notify-node",
        },
      ],
    };

    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [
          {
            id: "js-logic-flow",
            name: "JS Logic Flow",
            version: "0.2.0",
            description: "visual strategy",
            runtime: "quickjs-js",
            symbol: "00700",
            interval: "1m",
            script: "function manualOnly() { return 'keep'; }",
            visualModel,
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/strategy");

    await openStrategyDesignWorkspace(wrapper);
    await showStrategyCodeEditor(wrapper, "split");

    wrapper.findComponent(StrategyLogicFlowDesigner).vm.$emit("select-node", "rsi-calc-node");
    await flushRequests();

    const scriptEditor = wrapper.get('[data-testid="strategy-script-editor"]')
      .element as HTMLTextAreaElement;
    expect(scriptEditor.value).toContain("manualOnly");

    wrapper.findComponent(StrategyLogicFlowDesigner).vm.$emit("update:modelValue", visualModel);
    await flushRequests();

    expect(scriptEditor.value).toContain(
      "/** @param {JFTradeKLineClosedContext} ctx */",
    );
    expect(scriptEditor.value).toContain("function onKLineClosed(ctx)");
    expect(scriptEditor.value).toContain('notify("收盘价触发视觉策略")');

    wrapper.unmount();
  });

  it("syncs handwritten quickjs back into flow on blur and preserves unsupported code as code blocks", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [
          {
            id: "js-handwritten",
            name: "JS Handwritten",
            version: "0.2.0",
            description: "code-first strategy",
            runtime: "quickjs-js",
            symbol: "00700",
            interval: "1m",
            script: "function onKLineClosed(ctx) { notify(`close=${ctx.kline.close}`); }",
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/strategy");

    await openStrategyDesignWorkspace(wrapper);
    await showStrategyCodeEditor(wrapper, "split");

    const nextScript = [
      "const helperFactor = 2;",
      "",
      "function onKLineClosed(ctx) {",
      "  notify(`close=${ctx.kline.close}`);",
      "  const doubled = ctx.kline.close * helperFactor;",
      "}",
    ].join("\n");

    await wrapper.get('[data-testid="strategy-script-editor"]').setValue(nextScript);
    await wrapper.get('[data-testid="strategy-script-editor"]').trigger("blur");
    await flushRequests();

    const visualModel = wrapper.findComponent(StrategyLogicFlowDesigner)
      .props("modelValue") as NonNullable<StrategyDefinitionDocument["visualModel"]>;

    expect(visualModel.nodes.some((node) => node.properties.blockKind === "notify")).toBe(true);
    expect(visualModel.nodes.some((node) => node.properties.blockKind === "codeBlock")).toBe(true);
    expect(wrapper.get('[data-testid="strategy-visual-sync-status"]').text()).toContain("代码块");

    const codeBlockNode = visualModel.nodes.find(
      (node) =>
        node.properties.blockKind === "codeBlock" &&
        node.properties.codeScope === "hook",
    );
    expect(codeBlockNode).toBeDefined();

    wrapper.findComponent(StrategyLogicFlowDesigner).vm.$emit("select-node", codeBlockNode!.id);
    await flushRequests();

    await wrapper
      .get('[placeholder="例如：const signal = ctx.kline.close > 520;"]')
      .setValue("const doubled = ctx.kline.close * 3;");
    await flushRequests();

    const scriptEditor = wrapper.get('[data-testid="strategy-script-editor"]')
      .element as HTMLTextAreaElement;
    expect(scriptEditor.value).toContain("const doubled = ctx.kline.close * 3;");

    wrapper.unmount();
  });

  it("rewrites quickjs code when a visual block parameter changes", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [
          {
            id: "js-rsi-visual",
            name: "RSI Visual",
            version: "0.2.0",
            description: "visual strategy",
            runtime: "quickjs-js",
            symbol: "00700",
            interval: "1m",
            script: [
              "/** @param {JFTradeInitContext} ctx */",
              "function onInit(ctx) {",
              "}",
              "",
              "/** @param {JFTradeKLineClosedContext} ctx */",
              "function onKLineClosed(ctx) {",
              "  const close = Number(ctx && ctx.kline ? ctx.kline.close : NaN);",
              "  if (!Number.isFinite(close)) {",
              '    console.log("skip candle because close is not a finite number");',
              "    return;",
              "  }",
              "",
              "  let latestRsi = null;",
              "",
              "  /**",
              "   * @jftradeFlowNodeId rsi-calc-node",
              "   * @jftradeFlowBlockKind technicalIndicator",
              "   * @jftradeFlowNodeText RSI 14 < 30",
              "   */",
              '  latestRsi = ctx.indicators["rsi:14"] ?? null;',
              "  if (latestRsi === null) {",
              '    console.log("waiting for indicator rsi:14");',
              "    return;",
              "  }",
              "  if (latestRsi < 30) {",
              "    /**",
              "     * @jftradeFlowNodeId notify-node",
              "     * @jftradeFlowBlockKind notify",
              "     * @jftradeFlowNodeText 发送通知",
              "     */",
              '    notify("RSI changed");',
              "  }",
              "}",
            ].join("\n"),
            visualModel: {
              engine: "logic-flow",
              version: 1,
              nodes: [
                {
                  id: "rsi-calc-node",
                  type: "rect",
                  x: 420,
                  y: 200,
                  text: "RSI 14 < 30",
                  properties: {
                    blockKind: "technicalIndicator",
                    indicatorType: "rsi",
                    conditionMode: "numeric",
                    operator: "<",
                    threshold: 30,
                    period: 14,
                  },
                },
                {
                  id: "on-kline-root",
                  type: "circle",
                  x: 160,
                  y: 200,
                  text: "K 线收盘",
                  properties: {
                    blockKind: "onKLineClosed",
                  },
                },
                {
                  id: "notify-node",
                  type: "rect",
                  x: 700,
                  y: 200,
                  text: "发送通知",
                  properties: {
                    blockKind: "notify",
                    message: "RSI changed",
                  },
                },
              ],
              edges: [
                {
                  id: "edge-1",
                  type: "polyline",
                  sourceNodeId: "on-kline-root",
                  targetNodeId: "rsi-calc-node",
                },
                {
                  id: "edge-2",
                  type: "polyline",
                  sourceNodeId: "rsi-calc-node",
                  targetNodeId: "notify-node",
                },
              ],
            },
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/strategy");

    await openStrategyDesignWorkspace(wrapper);
    await showStrategyCodeEditor(wrapper, "split");

    const designer = wrapper.findComponent(StrategyLogicFlowDesigner);
    const scriptEditor = wrapper.get('[data-testid="strategy-script-editor"]')
      .element as HTMLTextAreaElement;
    expect(scriptEditor.value).toContain('latestRsi = ctx.indicators["rsi:14"] ?? null;');

    const visualModel = JSON.parse(
      JSON.stringify(
        designer.props("modelValue") as NonNullable<StrategyDefinitionDocument["visualModel"]>,
      ),
    ) as NonNullable<StrategyDefinitionDocument["visualModel"]>;
    const indicatorNode = visualModel.nodes.find((node) => node.id === "rsi-calc-node");
    expect(indicatorNode).toBeDefined();
    if (indicatorNode === undefined) {
      return;
    }

    indicatorNode.properties = {
      ...indicatorNode.properties,
      period: 21,
    };
    designer.vm.$emit("update:modelValue", visualModel);
    await flushRequests();

    expect(scriptEditor.value).toContain('latestRsi = ctx.indicators["rsi:21"] ?? null;');

    wrapper.unmount();
  });

  it("rewrites risk block mode and window policy from the block inspector", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [
          {
            id: "js-risk-inspector",
            name: "Risk Inspector",
            version: "0.2.0",
            description: "risk block inspector",
            runtime: "quickjs-js",
            symbol: "US.AAPL",
            interval: "5m",
            script: "function manualOnly() { return 'keep'; }",
            visualModel: {
              engine: "logic-flow",
              version: 1,
              nodes: [
                {
                  id: "on-kline-root",
                  type: "circle",
                  x: 160,
                  y: 200,
                  text: "K 线收盘",
                  properties: {
                    blockKind: "onKLineClosed",
                  },
                },
                {
                  id: "risk-node",
                  type: "rect",
                  x: 420,
                  y: 200,
                  text: "自动止损 1日 2%",
                  properties: {
                    blockKind: "stopLoss",
                    mode: "stopLoss",
                    direction: "auto",
                    timeValue: 1,
                    timeUnit: "day",
                    percentage: 2,
                    windowPolicy: "continuous",
                  },
                },
              ],
              edges: [
                {
                  id: "edge-root-risk",
                  type: "polyline",
                  sourceNodeId: "on-kline-root",
                  targetNodeId: "risk-node",
                },
              ],
            },
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/strategy");

    await openStrategyDesignWorkspace(wrapper);
    await showStrategyCodeEditor(wrapper, "split");

    const designer = wrapper.findComponent(StrategyLogicFlowDesigner);
    designer.vm.$emit("select-node", "risk-node");
    await flushRequests();

    expect((wrapper.get('[data-testid="strategy-stop-loss-mode"]').element as HTMLSelectElement).value).toBe("stopLoss");
    expect((wrapper.get('[data-testid="strategy-stop-loss-window-policy"]').element as HTMLSelectElement).value).toBe("continuous");

    await wrapper.get('[data-testid="strategy-stop-loss-mode"]').setValue("trailingStop");
    await flushRequests();

    await wrapper.get('[data-testid="strategy-stop-loss-window-policy"]').setValue("session");
    await flushRequests();

    const updatedModel = designer.props("modelValue") as NonNullable<StrategyDefinitionDocument["visualModel"]>;
    const riskNode = updatedModel.nodes.find((node) => node.id === "risk-node");
    expect(riskNode).toBeDefined();
    expect(riskNode?.text).toBe("自动追踪止损 1日 2% 时段感知");
    expect(riskNode?.properties.mode).toBe("trailingStop");
    expect(riskNode?.properties.windowPolicy).toBe("session");

    const scriptEditor = wrapper.get('[data-testid="strategy-script-editor"]')
      .element as HTMLTextAreaElement;
    expect(scriptEditor.value).toContain('ctx.indicators["risk:trailingStop:auto:1:day:2:session"]');

    wrapper.unmount();
  });

  it("creates a new draft from the double moving average template", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/strategy");

    await openNewStrategyFromRuntime(wrapper);

    expect(wrapper.find('[data-testid="strategy-templates-section"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-code-editor-section"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-visual-builder-section"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-definitions-panel"]').exists()).toBe(false);

    await wrapper
      .get('[data-testid="strategy-template-double-moving-average"]')
      .trigger("click");
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-code-editor-section"]').exists()).toBe(false);

    await showStrategyCodeEditor(wrapper, "split");

    const scriptEditor = wrapper.get('[data-testid="strategy-script-editor"]')
      .element as HTMLTextAreaElement;
    expect(wrapper.find('[data-testid="strategy-templates-section"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-code-editor-section"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-visual-builder-section"]').exists()).toBe(true);
    expect(wrapper.text()).toContain("已基于「双均线系统」创建新草稿");
    expect(scriptEditor.value).toContain("const flow_dma_fast_ma = () => {");
    expect(scriptEditor.value).toContain('indicator_dma_fast_ma_snapshot = ctx.indicators["ma:MA:5:day"] ?? null;');
    expect(scriptEditor.value).toContain('indicator_dma_slow_ma_snapshot = ctx.indicators["ma:MA:20:day"] ?? null;');
    expect(scriptEditor.value).toContain("indicator_dma_fast_ma_previous <= indicator_dma_slow_ma_previous && indicator_dma_fast_ma_value > indicator_dma_slow_ma_value");
    expect(scriptEditor.value).toContain("金叉");

    wrapper.unmount();
  });

  it("allows dismissing strategy notices and errors", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/strategy");

    await openNewStrategyFromRuntime(wrapper);

    await wrapper
      .get('[data-testid="strategy-template-double-moving-average"]')
      .trigger("click");
    await flushRequests();

    expect(wrapper.text()).toContain("已基于「双均线系统」创建新草稿");

    await wrapper.get('[data-testid="dismiss-strategy-notice-banner"]').trigger("click");
    await flushRequests();

    expect(wrapper.text()).not.toContain("已基于「双均线系统」创建新草稿");
    expect(wrapper.find('[data-testid="dismiss-strategy-notice-banner"]').exists()).toBe(false);

    await wrapper.get('[data-testid="toggle-strategy-basic-info-section"]').trigger("click");
    await flushRequests();

    await wrapper
      .get('[data-testid="strategy-basic-info-section"] input[placeholder="00700"]')
      .setValue("");
    await wrapper.get('[data-testid="save-strategy-definition"]').trigger("click");
    await flushRequests();

    expect(wrapper.text()).toContain("标的不能为空。");

    await wrapper.get('[data-testid="dismiss-strategy-error-banner"]').trigger("click");
    await flushRequests();

    expect(wrapper.text()).not.toContain("标的不能为空。");
    expect(wrapper.find('[data-testid="dismiss-strategy-error-banner"]').exists()).toBe(false);

    wrapper.unmount();
  });

  it("prompts before leaving the editor for runtime when there are unsaved changes", async () => {
    const confirmMock = vi
      .fn()
      .mockReturnValueOnce(false)
      .mockReturnValueOnce(false)
      .mockReturnValueOnce(false)
      .mockReturnValueOnce(true);

    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );
    vi.stubGlobal("confirm", confirmMock);

    const { wrapper } = await mountApp("/strategy");

    await openNewStrategyFromRuntime(wrapper);
    await wrapper
      .get('[data-testid="strategy-template-double-moving-average"]')
      .trigger("click");
    await flushRequests();

    await wrapper.get('[data-testid="strategy-workspace-tab-runtime"]').trigger("click");
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-code-editor-section"]').exists()).toBe(false);
    expect(wrapper.text()).toContain("双均线系统");

    await wrapper.get('[data-testid="strategy-workspace-tab-runtime"]').trigger("click");
    await flushRequests();

    expect(wrapper.text()).toContain("策略实例");
    expect(confirmMock).toHaveBeenCalledTimes(4);

    wrapper.unmount();
  });

  it("prompts before route-leaving the editor when there are unsaved changes", async () => {
    const confirmMock = vi
      .fn()
      .mockReturnValueOnce(false)
      .mockReturnValueOnce(true);

    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );
    vi.stubGlobal("confirm", confirmMock);

    const { router, wrapper } = await mountApp("/strategy");

    await openNewStrategyFromRuntime(wrapper);
    await wrapper
      .get('[data-testid="strategy-template-double-moving-average"]')
      .trigger("click");
    await flushRequests();

    await router.push("/overview");
    await flushRequests();

    expect(router.currentRoute.value.path).toBe("/overview");
    expect(confirmMock).toHaveBeenCalledTimes(2);

    wrapper.unmount();
  });

  it("shows only templates when starting a new strategy and hides them after selection", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [
          {
            id: "js-existing",
            name: "Existing Strategy",
            version: "0.1.0",
            description: "existing quickjs strategy",
            runtime: "quickjs-js",
            symbol: "00700",
            interval: "1m",
            script: "function onKLineClosed(ctx) { console.log(ctx.kline.close); }",
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/strategy");

    await openStrategyDesignWorkspace(wrapper);

    await wrapper.get('[data-testid="toggle-strategy-basic-info-section"]').trigger("click");
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-basic-info-section"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-metadata-section"]').exists()).toBe(false);
    expect(wrapper.get('[data-testid="strategy-basic-info-section"]').text()).toContain("元信息");
    expect(wrapper.find('[data-testid="strategy-code-editor-section"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-visual-builder-section"]').exists()).toBe(true);

    await openStrategyWorkspaceTab(wrapper, "runtime");
    await openNewStrategyFromRuntime(wrapper);

    expect(wrapper.find('[data-testid="strategy-templates-section"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-definitions-panel"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-basic-info-section"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-metadata-section"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-code-editor-section"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-visual-builder-section"]').exists()).toBe(true);

    await wrapper
      .get('[data-testid="strategy-template-double-moving-average"]')
      .trigger("click");
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-templates-section"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-code-editor-section"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-visual-builder-section"]').exists()).toBe(true);

    wrapper.unmount();
  });

  it("creates a new draft from the rsi reversion template", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/strategy");

    await openNewStrategyFromRuntime(wrapper);
    await openStrategyTemplatesPanel(wrapper);
    await wrapper
      .get('[data-testid="strategy-template-rsi-reversion"]')
      .trigger("click");
    await flushRequests();

    await showStrategyCodeEditor(wrapper, "code");

    const scriptEditor = wrapper.get('[data-testid="strategy-script-editor"]')
      .element as HTMLTextAreaElement;
    expect(wrapper.text()).toContain("已基于「RSI 反转交易」创建新草稿");
    expect(scriptEditor.value).toContain("const flow_rsi_getter = () => {");
    expect(scriptEditor.value).toContain('indicator_rsi_getter_snapshot = ctx.indicators["rsi:14"] ?? null;');
    expect(scriptEditor.value).toContain('indicator_rsi_getter_value = indicator_rsi_getter_snapshot;');
    expect(scriptEditor.value).toContain("if (Number.isFinite(indicator_rsi_getter_value) && indicator_rsi_getter_value < 30)");
    expect(scriptEditor.value).toContain("if (Number.isFinite(indicator_rsi_getter_value) && indicator_rsi_getter_value > 70)");

    wrapper.unmount();
  });

  it("creates a new draft from the MACD momentum template", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/strategy");

    await openNewStrategyFromRuntime(wrapper);
    await openStrategyTemplatesPanel(wrapper);
    await wrapper
      .get('[data-testid="strategy-template-macd-momentum"]')
      .trigger("click");
    await flushRequests();

    await showStrategyCodeEditor(wrapper, "code");

    const scriptEditor = wrapper.get('[data-testid="strategy-script-editor"]')
      .element as HTMLTextAreaElement;
    expect(wrapper.text()).toContain("已基于「MACD 动能交易」创建新草稿");
    expect(scriptEditor.value).toContain("const flow_macd_getter = () => {");
    expect(scriptEditor.value).toContain('indicator_macd_getter = ctx.indicators["macd:12:26:9"] ?? null;');
    expect(scriptEditor.value).toContain("if (indicator_macd_getter_previous_diff !== null && indicator_macd_getter_previous_signal !== null && indicator_macd_getter_previous_diff <= indicator_macd_getter_previous_signal && indicator_macd_getter_diff > indicator_macd_getter_signal)");

    wrapper.unmount();
  });

  it("creates a new draft from the Bollinger reversion template", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/strategy");

    await openNewStrategyFromRuntime(wrapper);
    await openStrategyTemplatesPanel(wrapper);
    await wrapper
      .get('[data-testid="strategy-template-bollinger-reversion"]')
      .trigger("click");
    await flushRequests();

    await showStrategyCodeEditor(wrapper, "code");

    const scriptEditor = wrapper.get('[data-testid="strategy-script-editor"]')
      .element as HTMLTextAreaElement;
    expect(wrapper.text()).toContain("已基于「布林带回归交易」创建新草稿");
    expect(scriptEditor.value).toContain("const flow_boll_getter = () => {");
    expect(scriptEditor.value).toContain('indicator_boll_getter = ctx.indicators["bollinger:20:2"] ?? null;');
    expect(scriptEditor.value).toContain("if (ctx.kline.close > indicator_boll_getter_upper)");
    expect(scriptEditor.value).toContain("if (ctx.kline.close < indicator_boll_getter_lower)");

    wrapper.unmount();
  });

  it("instantiates a saved quickjs definition and drives runtime status actions", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [
          {
            id: "js-mean-revert",
            name: "JS Mean Revert",
            version: "0.1.0",
            description: "quickjs strategy",
            runtime: "quickjs-js",
            symbol: "00700",
            interval: "1m",
            script: "function onKLineClosed(ctx) { console.log(ctx.kline.close); }",
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/strategy");

    await openStrategyDesignWorkspace(wrapper);

    await wrapper.get('[data-testid="instantiate-strategy-definition"]').trigger("click");
    await flushRequests();

    expect(wrapper.text()).toContain("已创建运行实例");
    expect(wrapper.text()).toContain("js-mean-revert-instance");

    await wrapper.get('[data-testid="strategy-start"]').trigger("click");
    await flushRequests();
    expect(wrapper.text()).toContain("运行中");

    await wrapper.get('[data-testid="strategy-pause"]').trigger("click");
    await flushRequests();
    expect(wrapper.text()).toContain("manual pause");

    await wrapper.get('[data-testid="strategy-stop"]').trigger("click");
    await flushRequests();
    expect(wrapper.text()).toContain("已停止");

    wrapper.unmount();
  });
});
