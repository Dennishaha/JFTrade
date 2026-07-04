// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

import type { StrategyDefinitionDocument } from "@/contracts";
import StrategyRuntimePanel from "../src/components/StrategyRuntimePanel.vue";
import { PINE_WORKER_RUNTIME } from "../src/components/strategy-runtime/strategyRuntimeIdentity";
import {
  MockWebSocket,
  buildFetchMock,
  flushRequests,
  mountStrategyPage,
  openCreateInstancePanel,
  resetStrategyPageTestState,
  settleStrategyWorkspace,
  waitForSelector,
} from "./strategyPageTestUtils";

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  resetStrategyPageTestState();
});

describe("StrategyRuntimePanel business workflows", () => {
  it("creates, edits, risk-controls and deletes a stopped strategy instance", async () => {
    const fetchMock = buildFetchMock({ definitions: [buildDefinition()] });
    const fetchSpy = vi.fn(fetchMock);
    vi.stubGlobal("fetch", fetchSpy);
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket);
    vi.stubGlobal("confirm", vi.fn(() => true));

    const { wrapper } = await mountStrategyPage("/strategy/runtime");
    await openCreateInstancePanel(wrapper);

    await wrapper.get('[data-testid="strategy-instance-definition"]').setValue("mean-revert");
    await wrapper.get('[data-testid="strategy-instance-symbol-market"]').setValue("HK");
    await wrapper.get('[data-testid="strategy-instance-symbols"]').setValue("00700");
    await wrapper.get('[data-testid="strategy-instance-symbols"]').trigger("keydown", { key: "Enter" });
    await settleStrategyWorkspace();
    await wrapper.get('[data-testid="strategy-instance-interval"]').setValue("15m");
    await wrapper.get('[data-testid="strategy-instance-execution-mode"]').setValue("notify_only");
    await wrapper.get('[data-testid="strategy-runtime-risk-mode"]').setValue("enforce");
    await wrapper.get('[data-testid="strategy-runtime-risk-close-only"]').setValue(true);
    await wrapper.get('[data-testid="strategy-runtime-risk-pause-on-reject"]').setValue(true);
    await wrapper.get('[data-testid="strategy-runtime-risk-max-quantity"]').setValue("100");
    await wrapper.get('[data-testid="strategy-runtime-risk-max-notional"]').setValue("25000");
    await wrapper.get('[data-testid="strategy-runtime-risk-daily-max-orders"]').setValue("12");
    await wrapper.get('[data-testid="strategy-create-instance"]').trigger("click");
    await waitForSelector(wrapper, '[data-testid="strategy-mean-revert-instance"]');
    await settleStrategyWorkspace();

    expect(wrapper.text()).toContain("已创建实例：Mean Revert");
    expect(wrapper.text()).toContain("HK.00700");
    expect(wrapper.get('[data-testid="strategy-runtime-start-hint"]').text()).toContain("仅通知模式");
    const instantiateCall = findFetchCall(fetchSpy, "/strategy-definitions/mean-revert/instantiate", "POST");
    expect(readBody(instantiateCall)).toMatchObject({
      instruments: [{ market: "HK", code: "00700" }],
      symbols: ["HK.00700"],
      interval: "15m",
      executionMode: "notify_only",
      runtimeRisk: {
        mode: "enforce",
        closeOnly: true,
        pauseOnReject: true,
        maxOrderQuantity: 100,
        maxOrderNotional: 25000,
        dailyMaxOrders: 12,
      },
    });

    await wrapper.get('[data-testid="strategy-current-binding-summary"]').trigger("click");
    await waitForSelector(wrapper, '[data-testid="strategy-edit-instance-panel"]');
    await wrapper.get('[data-testid="strategy-edit-interval"]').setValue("30m");
    await wrapper.get('[data-testid="strategy-edit-execution-mode"]').setValue("live");
    await wrapper.get('[data-testid="strategy-update-binding"]').trigger("click");
    await settleStrategyWorkspace();

    expect(wrapper.text()).toContain("已更新实例绑定：Mean Revert");
    const updateCall = findFetchCall(fetchSpy, "/strategies/mean-revert-instance", "PUT");
    expect(readBody(updateCall)).toMatchObject({
      symbols: ["HK.00700"],
      interval: "30m",
      executionMode: "live",
    });

    await wrapper.get('[data-testid="strategy-runtime-risk-quick-mode"]').setValue("monitor");
    await settleStrategyWorkspace();
    expect(wrapper.text()).toContain("已更新动态风控：观察");
    const riskCall = findFetchCall(fetchSpy, "/strategies/mean-revert-instance/runtime-risk", "PUT");
    expect(readBody(riskCall)).toMatchObject({ mode: "monitor" });

    await wrapper.get('[data-testid="strategy-runtime-risk-quick-close-only"]').setValue(true);
    await settleStrategyWorkspace();
    expect(fetchSpy.mock.calls.filter(([input, init]) =>
      String(input).includes("/runtime-risk") && requestMethod(input, init) === "PUT",
    )).toHaveLength(2);

    await wrapper.get('[data-testid="strategy-current-binding-summary"]').trigger("click");
    await waitForSelector(wrapper, '[data-testid="strategy-delete-instance"]');
    await wrapper.get('[data-testid="strategy-delete-instance"]').trigger("click");
    await settleStrategyWorkspace();

    expect(window.confirm).toHaveBeenCalledWith("确认删除策略实例「Mean Revert」吗？");
    expect(wrapper.text()).toContain("已删除实例：Mean Revert");
    expect(wrapper.find('[data-testid="strategy-mean-revert-instance"]').exists()).toBe(false);
    expect(findFetchCall(fetchSpy, "/strategies/mean-revert-instance", "DELETE")).toBeTruthy();

    wrapper.unmount();
  });

  it("runs the complete start, pause and stop lifecycle and refreshes activity data", async () => {
    const fetchMock = buildFetchMock({
      definitions: [buildDefinition()],
      strategies: [buildStrategy("STOPPED")],
      logsById: {
        "instance-1": ["2026-06-01T01:00:00.000Z instantiated strategy"],
      },
      auditById: {
        "instance-1": [
          { instanceId: "instance-1", kind: "instantiated", detail: "mean-revert", at: "2026-06-01T01:00:00.000Z" },
        ],
      },
    });
    const fetchSpy = vi.fn(fetchMock);
    vi.stubGlobal("fetch", fetchSpy);
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket);

    const { wrapper } = await mountStrategyPage("/strategy/runtime");
    await waitForSelector(wrapper, '[data-testid="strategy-start"]');

    await wrapper.get('[data-testid="strategy-start"]').trigger("click");
    await settleStrategyWorkspace();
    expect(wrapper.text()).toContain("运行中");
    expect(wrapper.text()).toContain("started strategy mean-revert");

    await wrapper.get('[data-testid="strategy-pause"]').trigger("click");
    await settleStrategyWorkspace();
    expect(wrapper.text()).toContain("已暂停");
    expect(wrapper.text()).toContain("pauseed strategy mean-revert");

    await wrapper.get('[data-testid="strategy-stop"]').trigger("click");
    await settleStrategyWorkspace();
    expect(wrapper.text()).toContain("已停止");
    expect(wrapper.text()).toContain("stoped strategy mean-revert");

    for (const action of ["start", "pause", "stop"]) {
      expect(findFetchCall(fetchSpy, `/strategies/instance-1/${action}`, "POST")).toBeTruthy();
    }

    wrapper.unmount();
  });

  it("blocks live startup when the selected strategy contains unsupported live semantics", async () => {
    const definition: StrategyDefinitionDocument = {
      ...buildDefinition(),
      script: [
        '//@version=6',
        'strategy("Percent Cancel", default_qty_type=strategy.percent_of_equity)',
        'strategy.entry("Long", strategy.long, qty_percent=50)',
        'strategy.cancel_all()',
      ].join("\n"),
    };
    const fetchSpy = vi.fn(buildFetchMock({
      definitions: [definition],
      strategies: [buildStrategy("STOPPED")],
    }));
    vi.stubGlobal("fetch", fetchSpy);
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket);

    const { wrapper } = await mountStrategyPage("/strategy/runtime");
    await waitForSelector(wrapper, '[data-testid="strategy-live-limitations"]');

    expect(wrapper.get('[data-testid="strategy-live-limitations"]').text()).toContain("QuantityPct");
    expect(wrapper.get('[data-testid="strategy-live-limitations"]').text()).toContain("strategy.cancel");
    expect(wrapper.get('[data-testid="strategy-runtime-start-hint"]').text()).toContain("live 暂不支持语义");
    expect(wrapper.get('[data-testid="strategy-start"]').attributes("disabled")).toBeDefined();
    expect(readSetupArray<string>(wrapper.getComponent(StrategyRuntimePanel).vm.$.setupState.selectedStrategyLiveLimitations)).toHaveLength(2);

    const setup = wrapper.getComponent(StrategyRuntimePanel).vm.$.setupState as Record<string, unknown>;
    await (setup.changeStrategyStatus as (action: "start") => Promise<void>)("start");
    expect(readSetupText(setup.detailsError)).toContain("启动前检查未通过");
    expect(fetchSpy.mock.calls.some(([input, init]) =>
      String(input).includes("/strategies/instance-1/start") && requestMethod(input, init) === "POST",
    )).toBe(false);

    wrapper.unmount();
  });

  it("shows runtime observations, stale-definition boundaries and applies the latest stopped version", async () => {
    const definition = buildDefinition("0.2.0");
    const strategy = {
      ...buildStrategy("STOPPED"),
      definition: { strategyId: "mean-revert", name: "Mean Revert", version: "0.1.0" },
      params: {
        ...buildStrategy("STOPPED").params,
        compiledHooks: ["on_init", "on_kline_close"],
        compiledRequirements: { indicators: [{ key: "ema:5" }, { key: "atr:14" }] },
      },
      runtimeObservation: {
        actualStatus: "RUNNING" as const,
        activeSymbols: ["HK.00700", "US.AAPL"],
        lastClosedKlineAt: "",
        lastSignalAt: "invalid-time",
        lastOrderAt: "2026-06-01T02:00:00.000Z",
        lastErrorAt: "2026-06-01T02:01:00.000Z",
        lastError: "broker rejected order",
        updatedAt: null,
      },
    };
    const fetchMock = buildFetchMock({ definitions: [definition], strategies: [strategy] });
    const fetchSpy = vi.fn(fetchMock);
    vi.stubGlobal("fetch", fetchSpy);
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket);

    const { wrapper } = await mountStrategyPage("/strategy/runtime");
    await waitForSelector(wrapper, '[data-testid="strategy-runtime-observation"]');

    expect(wrapper.text()).toContain("2 个 hook / 2 项依赖");
    expect(wrapper.text()).toContain("HK.00700, US.AAPL");
    expect(wrapper.text()).toContain("invalid-time");
    expect(wrapper.text()).toContain("broker rejected order");
    expect(wrapper.get('[data-testid="strategy-definition-sync-badge"]').text()).toContain("待刷新 v0.1.0 -> v0.2.0");

    await wrapper.get('[data-testid="strategy-refresh-definition"]').trigger("click");
    await settleStrategyWorkspace();

    expect(wrapper.text()).toContain("已刷新实例策略到最新版本：Mean Revert / v0.2.0");
    expect(wrapper.get('[data-testid="strategy-definition-sync-badge"]').text()).toContain("已同步至 v0.2.0");
    expect(findFetchCall(fetchSpy, "/strategies/instance-1/refresh-definition", "POST")).toBeTruthy();

    wrapper.unmount();
  });

  it("stops polling while hidden and immediately refreshes when the page becomes visible", async () => {
    let visibilityState: DocumentVisibilityState = "visible";
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      get: () => visibilityState,
    });
    const fetchMock = buildFetchMock({ definitions: [buildDefinition()], strategies: [buildStrategy("STOPPED")] });
    const fetchSpy = vi.fn(fetchMock);
    vi.stubGlobal("fetch", fetchSpy);
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket);

    const { wrapper } = await mountStrategyPage("/strategy/runtime");
    const countStrategyLists = () => fetchSpy.mock.calls.filter(([input, init]) =>
      String(input).endsWith("/api/v1/strategies") && requestMethod(input, init) === "GET",
    ).length;
    const beforeHide = countStrategyLists();

    visibilityState = "hidden";
    document.dispatchEvent(new Event("visibilitychange"));
    await flushRequests();
    expect(countStrategyLists()).toBe(beforeHide);

    visibilityState = "visible";
    document.dispatchEvent(new Event("visibilitychange"));
    await settleStrategyWorkspace();
    expect(countStrategyLists()).toBeGreaterThan(beforeHide);

    wrapper.unmount();
  });

  it("surfaces definition, instance-list and activity loading failures without stale content", async () => {
    const baseFetch = buildFetchMock({ definitions: [buildDefinition()], strategies: [buildStrategy("STOPPED")] });
    vi.stubGlobal("fetch", vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      const method = requestMethod(input, init);
      if (url.endsWith("/api/v1/strategy-definitions") && method === "GET") {
        return errorResponse("策略定义服务暂不可用", 503);
      }
      if (url.endsWith("/api/v1/strategies") && method === "GET") {
        return errorResponse("策略实例服务暂不可用", 503);
      }
      return baseFetch(input, init);
    }));
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket);

    const failedLists = await mountStrategyPage("/strategy/runtime");
    await settleStrategyWorkspace();
    expect(failedLists.wrapper.text()).toContain("策略实例服务暂不可用");
    await failedLists.wrapper.get('[data-testid="strategy-create-menu-toggle"]').trigger("click");
    await failedLists.wrapper.get('[data-testid="strategy-new-instance"]').trigger("click");
    await settleStrategyWorkspace();
    const refreshDefinitionsButton = failedLists.wrapper
      .get('[data-testid="strategy-instance-dialog"]')
      .findAll("button")[0];
    expect(refreshDefinitionsButton).toBeDefined();
    await refreshDefinitionsButton!.trigger("click");
    await settleStrategyWorkspace();
    expect(failedLists.wrapper.text()).toContain("策略实例服务暂不可用");
    failedLists.wrapper.unmount();
    resetStrategyPageTestState();

    const detailsBaseFetch = buildFetchMock({
      definitions: [buildDefinition()],
      strategies: [buildStrategy("STOPPED")],
    });
    vi.stubGlobal("fetch", vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      if (String(input).includes("/strategies/instance-1/logs")) {
        return errorResponse("运行日志读取失败", 502);
      }
      return detailsBaseFetch(input, init);
    }));
    const failedDetails = await mountStrategyPage("/strategy/runtime");
    await settleStrategyWorkspace();
    expect(failedDetails.wrapper.text()).toContain("运行日志读取失败");
    expect(failedDetails.wrapper.text()).not.toContain("stale log");
    failedDetails.wrapper.unmount();
  });

  it("keeps mutation dialogs recoverable when create, update, risk, delete or refresh APIs fail", async () => {
    let failedOperation = "instantiate";
    const baseFetch = buildFetchMock({
      definitions: [buildDefinition("0.2.0")],
      strategies: [
        {
          ...buildStrategy("STOPPED"),
          definition: { strategyId: "mean-revert", name: "Mean Revert", version: "0.1.0" },
        },
      ],
    });
    const fetchSpy = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      const method = requestMethod(input, init);
      const shouldFail =
        (failedOperation === "instantiate" && url.includes("/instantiate") && method === "POST")
        || (failedOperation === "binding" && url.endsWith("/strategies/instance-1") && method === "PUT")
        || (failedOperation === "risk" && url.endsWith("/strategies/instance-1/runtime-risk") && method === "PUT")
        || (failedOperation === "delete" && url.endsWith("/strategies/instance-1") && method === "DELETE")
        || (failedOperation === "refresh" && url.endsWith("/strategies/instance-1/refresh-definition") && method === "POST");
      if (shouldFail) {
        return errorResponse(`${failedOperation} rejected`, 409);
      }
      return baseFetch(input, init);
    });
    vi.stubGlobal("fetch", fetchSpy);
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket);
    vi.stubGlobal("confirm", vi.fn(() => true));

    const { wrapper } = await mountStrategyPage("/strategy/runtime");
    await openCreateInstancePanel(wrapper);
    await wrapper.get('[data-testid="strategy-instance-definition"]').setValue("mean-revert");
    await wrapper.get('[data-testid="strategy-create-instance"]').trigger("click");
    await settleStrategyWorkspace();
    expect(wrapper.text()).toContain("instantiate rejected");
    expect(wrapper.find('[data-testid="strategy-create-instance-panel"]').exists()).toBe(true);
    await wrapper.get('[data-testid="strategy-create-instance-close"]').trigger("click");

    failedOperation = "binding";
    await wrapper.get('[data-testid="strategy-current-binding-summary"]').trigger("click");
    await wrapper.get('[data-testid="strategy-update-binding"]').trigger("click");
    await settleStrategyWorkspace();
    expect(wrapper.text()).toContain("binding rejected");
    await wrapper.get('[data-testid="strategy-edit-instance-close"]').trigger("click");

    failedOperation = "risk";
    await wrapper.get('[data-testid="strategy-runtime-risk-quick-mode"]').setValue("monitor");
    await settleStrategyWorkspace();
    expect(wrapper.text()).toContain("risk rejected");

    failedOperation = "delete";
    await wrapper.get('[data-testid="strategy-current-binding-summary"]').trigger("click");
    await wrapper.get('[data-testid="strategy-delete-instance"]').trigger("click");
    await settleStrategyWorkspace();
    expect(wrapper.text()).toContain("delete rejected");
    expect(wrapper.find('[data-testid="strategy-instance-1"]').exists()).toBe(true);
    await wrapper.get('[data-testid="strategy-edit-instance-close"]').trigger("click");

    failedOperation = "refresh";
    await wrapper.get('[data-testid="strategy-refresh-definition"]').trigger("click");
    await settleStrategyWorkspace();
    expect(wrapper.text()).toContain("refresh rejected");
    expect(wrapper.get('[data-testid="strategy-definition-sync-badge"]').text()).toContain("待刷新 v0.1.0 -> v0.2.0");

    wrapper.unmount();
  });

  it("handles stale UI events after selection disappears and preserves explicit business errors", async () => {
    vi.stubGlobal("fetch", buildFetchMock({ definitions: [buildDefinition()], strategies: [] }));
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket);
    const { wrapper } = await mountStrategyPage("/strategy/runtime");
    const panel = wrapper.getComponent(StrategyRuntimePanel);
    const setup = panel.vm.$.setupState as Record<string, unknown>;

    expect(readSetupText(setup.selectedStrategyParamsJson)).toBe("");
    expect(readSetupText(setup.selectedStrategyRuntimeLabel)).toBe("暂无");
    expect(readSetupText(setup.selectedStrategySourceFormatLabel)).toBe("暂无");
    expect(readSetupText(setup.selectedStrategyStartHint)).toBe("请选择策略实例。");

    writeSetupValue(setup, "createDefinitionId", "");
    await (setup.createStrategyInstance as () => Promise<void>)();
    expect(readSetupText(setup.instanceMutationError)).toBe("请先选择已保存的策略定义。");
    writeSetupValue(setup, "createSymbolValidationMessage", "交易代码格式无效。");
    await (setup.createStrategyInstance as () => Promise<void>)();
    expect(readSetupText(setup.instanceMutationError)).toBe("交易代码格式无效。");
    writeSetupValue(setup, "createSymbolValidationMessage", "");

    writeSetupValue(setup, "editSymbolValidationMessage", "绑定代码不可用。");
    await (setup.updateSelectedStrategyBinding as () => Promise<void>)();
    expect(readSetupText(setup.instanceMutationError)).toBe("绑定代码不可用。");
    writeSetupValue(setup, "editSymbolValidationMessage", "");

    (setup.openEditInstanceForm as () => void)();
    await (setup.updateSelectedStrategyBinding as () => Promise<void>)();
    expect(readSetupText(setup.instanceMutationError)).toBe("请先选择策略实例。");

    await (setup.updateSelectedStrategyRuntimeRisk as (patch: Record<string, unknown>) => Promise<void>)({ mode: "monitor" });
    expect(readSetupText(setup.instanceMutationError)).toBe("请先选择策略实例。");

    await (setup.deleteSelectedStrategy as () => Promise<void>)();
    expect(readSetupText(setup.instanceMutationError)).toBe("请先选择策略实例。");

    await (setup.refreshSelectedStrategyDefinition as () => Promise<void>)();
    expect(readSetupText(setup.instanceMutationError)).toBe("请先选择策略实例。");

    await (setup.changeStrategyStatus as (action: "start") => Promise<void>)("start");
    expect(readSetupText(setup.detailsError)).toBe("请先选择策略实例。");

    wrapper.unmount();
  });

  it("rejects stopped-only actions for a running stale instance, handles cancel and latest-version no-op", async () => {
    vi.stubGlobal("fetch", buildFetchMock({
      definitions: [buildDefinition("0.2.0")],
      strategies: [
        {
          ...buildStrategy("RUNNING"),
          definition: { strategyId: "mean-revert", name: "Mean Revert", version: "0.1.0" },
        },
      ],
    }));
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket);
    vi.stubGlobal("confirm", vi.fn(() => false));
    const { wrapper } = await mountStrategyPage("/strategy/runtime");
    const setup = wrapper.getComponent(StrategyRuntimePanel).vm.$.setupState as Record<string, unknown>;

    await (setup.updateSelectedStrategyBinding as () => Promise<void>)();
    expect(readSetupText(setup.instanceMutationError)).toBe("仅已停止的实例允许修改绑定。");
    await (setup.deleteSelectedStrategy as () => Promise<void>)();
    expect(readSetupText(setup.instanceMutationError)).toBe("仅已停止的实例允许删除。");
    await (setup.refreshSelectedStrategyDefinition as () => Promise<void>)();
    expect(readSetupText(setup.instanceMutationError)).toContain("先停止后才能刷新");

    const strategies = readSetupArray<{ status: string; definitionSync?: { isLatest: boolean } }>(setup.strategies);
    strategies[0]!.status = "STOPPED";
    await (setup.deleteSelectedStrategy as () => Promise<void>)();
    expect(window.confirm).toHaveBeenCalled();
    expect(wrapper.find('[data-testid="strategy-instance-1"]').exists()).toBe(true);

    strategies[0]!.definitionSync!.isLatest = true;
    await (setup.refreshSelectedStrategyDefinition as () => Promise<void>)();
    expect(readSetupText(setup.instanceMutationNotice)).toBe("当前实例已经是最新策略版本。");

    wrapper.unmount();
  });

  it("maps non-Error lifecycle failures to each action-specific fallback", async () => {
    const baseFetch = buildFetchMock({ definitions: [buildDefinition()], strategies: [buildStrategy("STOPPED")] });
    vi.stubGlobal("fetch", vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      if (/\/strategies\/instance-1\/(start|pause|stop|restart)$/.test(String(input))) {
        throw "transport closed";
      }
      return baseFetch(input, init);
    }));
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket);
    const { wrapper } = await mountStrategyPage("/strategy/runtime");
    const setup = wrapper.getComponent(StrategyRuntimePanel).vm.$.setupState as Record<string, unknown>;
    const changeStatus = setup.changeStrategyStatus as (action: string) => Promise<void>;

    for (const [action, expected] of [
      ["start", "执行启动失败。"],
      ["pause", "执行暂停失败。"],
      ["stop", "执行停止失败。"],
      ["restart", "执行restart失败。"],
    ] as const) {
      await changeStatus(action);
      expect(readSetupText(setup.detailsError)).toBe(expected);
    }

    wrapper.unmount();
  });

  it("opens the create editor for a definition carried from the design workflow", async () => {
    vi.stubGlobal("fetch", buildFetchMock({ definitions: [buildDefinition()], strategies: [] }));
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket);
    const { wrapper } = await mountStrategyPage("/strategy/runtime?definitionId=mean-revert");
    await waitForSelector(wrapper, '[data-testid="strategy-create-instance-panel"]');
    expect((wrapper.get('[data-testid="strategy-instance-definition"]').element as HTMLSelectElement).value).toBe("mean-revert");
    expect(wrapper.find('[data-testid="strategy-create-menu"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("explains non-startable runtimes, selects another instance and routes new-definition intent", async () => {
    const strategies = [
      { ...buildStrategy("STOPPED"), id: "pine-planned", startable: false },
      {
        ...buildStrategy("STOPPED"),
        id: "legacy-runtime",
        runtime: "legacy-js",
        sourceFormat: undefined,
        startable: false,
        definition: { strategyId: "legacy", name: "Legacy", version: "1.0.0" },
      },
    ];
    vi.stubGlobal("fetch", buildFetchMock({ definitions: [buildDefinition()], strategies }));
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket);
    const { router, wrapper } = await mountStrategyPage("/strategy/runtime");

    expect(wrapper.get('[data-testid="strategy-runtime-start-hint"]').text()).toContain("已完成 Pine 编译与 requirements 规划");
    await wrapper.get('[data-testid="strategy-legacy-runtime"]').trigger("click");
    await settleStrategyWorkspace();
    expect(wrapper.get('[data-testid="strategy-runtime-start-hint"]').text()).toBe("当前实例暂不可启动。");

    await wrapper.get('[data-testid="strategy-create-menu-toggle"]').trigger("click");
    await wrapper.get('[data-testid="strategy-new-definition"]').trigger("click");
    await settleStrategyWorkspace();
    expect(router.currentRoute.value.path).toBe("/strategy/design");
    expect(router.currentRoute.value.query.mode).toBe("new");

    wrapper.unmount();
  });
});

function buildDefinition(version = "0.1.0"): StrategyDefinitionDocument {
  return {
    id: "mean-revert",
    name: "Mean Revert",
    version,
    description: "EMA mean reversion",
    runtime: PINE_WORKER_RUNTIME,
    sourceFormat: "pine-v6",
    symbol: "HK.00700",
    interval: "5m",
    script: '//@version=6\nstrategy("Mean Revert")\n',
    visualModel: null,
    createdAt: "2026-06-01T00:00:00.000Z",
    updatedAt: "2026-06-01T00:00:00.000Z",
  };
}

function buildStrategy(status: "RUNNING" | "PAUSED" | "STOPPED") {
  return {
    id: "instance-1",
    definition: { strategyId: "mean-revert", name: "Mean Revert", version: "0.1.0" },
    runtime: PINE_WORKER_RUNTIME,
    sourceFormat: "pine-v6" as const,
    startable: true,
    binding: {
      symbols: ["HK.00700"],
      interval: "5m",
      executionMode: "live" as const,
    },
    params: {
      definitionId: "mean-revert",
      symbol: "HK.00700",
      symbols: ["HK.00700"],
      interval: "5m",
      executionMode: "live",
    },
    status,
    createdAt: "2026-06-01T00:00:00.000Z",
    logs: [],
  };
}

function requestMethod(input: string | URL | Request, init?: RequestInit): string {
  return input instanceof Request ? input.method : (init?.method ?? "GET");
}

function findFetchCall(
  fetchSpy: ReturnType<typeof vi.fn>,
  path: string,
  method: string,
): [string | URL | Request, RequestInit | undefined] {
  const call = fetchSpy.mock.calls.find(([input, init]) =>
    String(input).includes(path) && requestMethod(input, init) === method,
  );
  expect(call, `${method} ${path}`).toBeDefined();
  return call as [string | URL | Request, RequestInit | undefined];
}

function readBody(call: [string | URL | Request, RequestInit | undefined]): Record<string, unknown> {
  const [input, init] = call;
  if (input instanceof Request) {
    throw new Error("Request body inspection is not supported synchronously in this helper");
  }
  return JSON.parse(String(init?.body ?? "{}")) as Record<string, unknown>;
}

function errorResponse(message: string, status: number): Response {
  return {
    ok: false,
    status,
    json: async () => ({
      ok: false,
      error: { code: status === 409 ? "CONFLICT" : "UNAVAILABLE", message },
      timestamp: "2026-06-01T00:00:00.000Z",
    }),
  } as Response;
}

function readSetupText(value: unknown): string {
  if (value !== null && typeof value === "object" && "value" in value) {
    return String((value as { value: unknown }).value);
  }
  return String(value ?? "");
}

function writeSetupValue(setup: Record<string, unknown>, key: string, value: unknown): void {
  const current = setup[key];
  if (current !== null && typeof current === "object" && "value" in current) {
    (current as { value: unknown }).value = value;
    return;
  }
  setup[key] = value;
}

function readSetupArray<T>(value: unknown): T[] {
  if (value !== null && typeof value === "object" && "value" in value) {
    return (value as { value: T[] }).value;
  }
  return value as T[];
}
