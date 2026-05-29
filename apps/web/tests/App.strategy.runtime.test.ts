// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest"

import {
  emptyBrokerSettings,
  emptyBrokerRuntime,
  emptySystemStatus,
} from "@jftrade/ui-contracts"
import type { SystemStatusResponse } from "@jftrade/ui-contracts"

import {
  MockEventSource,
  appendInstrumentTags,
  buildDslScript,
  buildFetchMock,
  buildRuntimeAccount,
  flushRequests,
  getCurrentConsoleDataStore,
  mountStrategyPage,
  openCreateInstancePanel,
  openStrategyWorkspaceTab,
  resetStrategyPageTestState,
  settleStrategyWorkspace,
  waitForSelector,
} from "./strategyPageTestUtils"
import { createResponse } from "./helpers"

afterEach(() => {
  vi.unstubAllGlobals()
  resetStrategyPageTestState()
})

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
        binding: {
          symbols: ["HK.00700"],
          interval: "5m",
          executionMode: "live" as const,
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
        binding: {
          symbols: ["US.AAPL"],
          interval: "15m",
          executionMode: "notify_only" as const,
        },
        params: {
          window: 20,
        },
        status: "PAUSED" as const,
        createdAt: "2026-05-16T00:01:00.000Z",
        logs: [],
      },
    ]
    const systemStatus: SystemStatusResponse = {
      ...emptySystemStatus,
      defaultTradingEnvironment: "REAL",
      realTradingEnabled: true,
    }

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
    )
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    )

    const { wrapper } = await mountStrategyPage("/strategy")
    await openStrategyWorkspaceTab(wrapper, "runtime")
    await waitForSelector(wrapper, '[data-testid="strategy-instance-1"]')

    expect(wrapper.text()).toContain("策略实例")
    expect(wrapper.text()).toContain("Mean Revert")
    expect(wrapper.text()).toContain("Breakout")
    expect(wrapper.text()).toContain("tick QUOTE_SNAPSHOT HK.00700")
    expect(wrapper.text()).toContain("运行审计")
    expect(wrapper.text()).toContain("QUOTE_SNAPSHOT HK.00700")
    expect(wrapper.text()).toContain("REAL")
    expect(wrapper.text()).toContain("仅通知")

    const createdAt = wrapper.get('[data-testid="strategy-instance-1"]').find('.strategy-time-display')
    expect(createdAt.text()).not.toContain("T")
    expect(createdAt.attributes("title")).toContain("UTC")

    expect(wrapper.get('[data-testid="strategy-status-instance-1"]').classes()).toContain("strategy-status-badge--running")
    expect(wrapper.get('[data-testid="strategy-status-instance-2"]').classes()).toContain("strategy-status-badge--paused")

    wrapper.unmount()
  })

  it("shows activity tabs, importance filters, and params dialog", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        strategies: [
          {
            id: "instance-1",
            definition: {
              strategyId: "s-mean-revert",
              name: "Mean Revert",
              version: "1.0.0",
            },
            params: {
              window: 20,
              threshold: 1.8,
            },
            status: "RUNNING",
            createdAt: "2026-05-16T00:00:00.000Z",
            logs: [],
          },
        ],
        logsById: {
          "instance-1": [
            "2026-05-16T00:00:00.000Z ERROR order rejected for HK.00700",
            "2026-05-16T00:00:02.000Z paused strategy execution",
            "2026-05-16T00:00:03.000Z tick QUOTE_SNAPSHOT HK.00700",
          ],
        },
        auditById: {
          "instance-1": [
            {
              instanceId: "instance-1",
              kind: "failed",
              detail: "order rejected for HK.00700",
              at: "2026-05-16T00:00:05.000Z",
            },
            {
              instanceId: "instance-1",
              kind: "paused",
              detail: "manual guardrail pause",
              at: "2026-05-16T00:00:06.000Z",
            },
            {
              instanceId: "instance-1",
              kind: "started",
              detail: "runtime ready",
              at: "2026-05-16T00:00:07.000Z",
            },
          ],
        },
      }),
    )
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    )

    const { wrapper } = await mountStrategyPage("/strategy")
    await openStrategyWorkspaceTab(wrapper, "runtime")
    await waitForSelector(wrapper, '[data-testid="strategy-instance-1"]')

    expect(wrapper.findAll('[data-testid^="strategy-log-entry-"]')).toHaveLength(3)
    expect(wrapper.get('[data-testid="strategy-log-entry-0"]').text()).toContain("tick QUOTE_SNAPSHOT HK.00700")
    expect(wrapper.get('[data-testid="strategy-log-entry-1"]').text()).toContain("paused strategy execution")
    expect(wrapper.get('[data-testid="strategy-log-entry-2"]').text()).toContain("ERROR order rejected for HK.00700")

    const logTime = wrapper.get('[data-testid="strategy-log-entry-0"]').find('.strategy-time-display')
    expect(logTime.text()).not.toContain("T")
    expect(logTime.attributes("title")).toContain("UTC")

    await wrapper.get('[data-testid="strategy-log-detail-trigger-0"]').trigger("click")
    await settleStrategyWorkspace()

    expect(wrapper.find('[data-testid="strategy-activity-detail-dialog"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="strategy-activity-detail-dialog"]').text()).toContain("tick QUOTE_SNAPSHOT HK.00700")

    await wrapper.get('[data-testid="strategy-close-activity-detail-dialog"]').trigger("click")
    await settleStrategyWorkspace()

    await wrapper.get('[data-testid="strategy-activity-filter-error"]').trigger("click")
    await settleStrategyWorkspace()

    expect(wrapper.findAll('[data-testid^="strategy-log-entry-"]')).toHaveLength(1)
    expect(wrapper.get('[data-testid="strategy-log-list"]').text()).toContain("ERROR order rejected for HK.00700")

    await wrapper.get('[data-testid="strategy-activity-tab-audit"]').trigger("click")
    await settleStrategyWorkspace()

    await wrapper.get('[data-testid="strategy-activity-filter-all"]').trigger("click")
    await settleStrategyWorkspace()

    expect(wrapper.findAll('[data-testid^="strategy-audit-entry-"]')).toHaveLength(3)
    expect(wrapper.get('[data-testid="strategy-audit-entry-0"]').text()).toContain("runtime ready")
    expect(wrapper.get('[data-testid="strategy-audit-entry-1"]').text()).toContain("manual guardrail pause")
    expect(wrapper.get('[data-testid="strategy-audit-entry-2"]').text()).toContain("order rejected for HK.00700")
    expect(wrapper.get('[data-testid="strategy-audit-entry-0"]').find('.strategy-time-display').attributes("title")).toContain("UTC")

    await wrapper.get('[data-testid="strategy-audit-detail-trigger-0"]').trigger("click")
    await settleStrategyWorkspace()

    expect(wrapper.get('[data-testid="strategy-activity-detail-dialog"]').text()).toContain("runtime ready")
    await wrapper.get('[data-testid="strategy-close-activity-detail-dialog"]').trigger("click")
    await settleStrategyWorkspace()

    await wrapper.get('[data-testid="strategy-open-params-dialog"]').trigger("click")
    await settleStrategyWorkspace()

    expect(wrapper.find('[data-testid="strategy-params-dialog"]').exists()).toBe(true)
    const paramsEditor = wrapper.get('[data-testid="strategy-params-editor"]')
    expect((paramsEditor.element as HTMLTextAreaElement).value).toContain('"window": 20')
    expect((paramsEditor.element as HTMLTextAreaElement).value).toContain('"threshold": 1.8')
    expect((paramsEditor.element as HTMLTextAreaElement).readOnly).toBe(true)

    wrapper.unmount()
  })

  it("creates, updates, and deletes a strategy instance with bindings", async () => {
    const fetchMock = buildFetchMock({
      brokerRuntime: buildRuntimeAccount({
        descriptor: {
          ...emptyBrokerRuntime.descriptor,
          id: "futu",
        },
        accounts: [
          {
            accountId: "123456",
            tradingEnvironment: "SIMULATE",
            marketAuthorities: ["US"],
            securityFirm: "futu-securities",
          },
        ],
      }),
      definitions: [
        {
          id: "dsl-breakout",
          name: "DSL Breakout",
          version: "0.1.0",
          description: "dsl strategy",
          runtime: "dsl-go-plan",
          script: buildDslScript("DSL Breakout"),
          createdAt: "2026-05-23T00:00:00.000Z",
          updatedAt: "2026-05-23T00:00:00.000Z",
        },
      ],
      strategies: [],
    })
    vi.stubGlobal("fetch", fetchMock)
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    )
    vi.stubGlobal("confirm", vi.fn().mockReturnValue(true))

    const { wrapper } = await mountStrategyPage("/strategy")
    await openStrategyWorkspaceTab(wrapper, "runtime")

    expect(wrapper.find('[data-testid="strategy-create-instance-panel"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="strategy-current-binding-summary"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain("运行控制")

    await openCreateInstancePanel(wrapper)

    await appendInstrumentTags(
      wrapper,
      {
        market: '[data-testid="strategy-instance-symbol-market"]',
        code: '[data-testid="strategy-instance-symbols"]',
      },
      [
        { market: "US", code: "aapl" },
        { market: "HK", code: "00700" },
      ],
    )
    await wrapper.get('[data-testid="strategy-instance-interval"]').setValue("15m")
    await wrapper.get('[data-testid="strategy-instance-execution-mode"]').setValue("notify_only")
    await wrapper.get('[data-testid="strategy-create-instance"]').trigger("click")
    await settleStrategyWorkspace()

    const instantiateCall = fetchMock.mock.calls.find(([input, init]) => (
      String(input).includes("/instantiate")
      && init?.method === "POST"
    ))
    expect(instantiateCall).toBeDefined()
    const instantiatePayload = JSON.parse(String(instantiateCall?.[1]?.body ?? "{}"))
    expect(instantiatePayload.instruments).toEqual([
      { market: "US", code: "AAPL" },
      { market: "HK", code: "00700" },
    ])
    expect(instantiatePayload.symbols).toEqual(["US.AAPL", "HK.00700"])

    expect(wrapper.text()).toContain("DSL Breakout")
    expect(wrapper.text()).toContain("US.AAPL, HK.00700")
    expect(wrapper.text()).toContain("15m")
    expect(wrapper.text()).toContain("仅通知")

    expect(wrapper.find('[data-testid="strategy-edit-instance-panel"]').exists()).toBe(false)
    await wrapper.get('[data-testid="strategy-current-binding-summary"]').trigger("click")
    await settleStrategyWorkspace()

    expect(wrapper.find('[data-testid="strategy-edit-instance-panel"]').exists()).toBe(true)

    await appendInstrumentTags(
      wrapper,
      {
        market: '[data-testid="strategy-edit-symbol-market"]',
        code: '[data-testid="strategy-edit-symbols"]',
      },
      [{ market: "US", code: "msft" }],
    )
    await wrapper.get('[data-testid="strategy-edit-interval"]').setValue("30m")
    await wrapper.get('[data-testid="strategy-edit-execution-mode"]').setValue("live")
    await wrapper.get('[data-testid="strategy-update-binding"]').trigger("click")
    await settleStrategyWorkspace()

    const updateCall = fetchMock.mock.calls.find(([input, init]) => (
      /\/api\/v1\/strategies\/[^/]+$/.test(String(input))
      && init?.method === "PUT"
    ))
    expect(updateCall).toBeDefined()
    const updatePayload = JSON.parse(String(updateCall?.[1]?.body ?? "{}"))
    expect(updatePayload.instruments).toEqual([
      { market: "US", code: "AAPL" },
      { market: "HK", code: "00700" },
      { market: "US", code: "MSFT" },
    ])
    expect(updatePayload.symbols).toEqual(["US.AAPL", "HK.00700", "US.MSFT"])

    expect(wrapper.text()).toContain("US.MSFT")
    expect(wrapper.text()).toContain("30m")
    expect(wrapper.text()).toContain("已更新实例绑定")

    await wrapper.get('[data-testid="strategy-current-binding-summary"]').trigger("click")
    await settleStrategyWorkspace()

    await wrapper.get('[data-testid="strategy-delete-instance"]').trigger("click")
    await settleStrategyWorkspace()

    expect(wrapper.text()).toContain("暂无策略实例")

    wrapper.unmount()
  })

  it("shows the add menu and expands the instance composer only on demand", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [
          {
            id: "dsl-breakout",
            name: "DSL Breakout",
            version: "0.1.0",
            description: "dsl strategy",
            runtime: "dsl-go-plan",
            script: buildDslScript("DSL Breakout"),
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
        strategies: [],
      }),
    )
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    )

    const { wrapper } = await mountStrategyPage("/strategy")
    await openStrategyWorkspaceTab(wrapper, "runtime")

    expect(wrapper.find('[data-testid="strategy-create-instance-panel"]').exists()).toBe(false)

    await wrapper.get('[data-testid="strategy-create-menu-toggle"]').trigger("click")
    await settleStrategyWorkspace()

    expect(wrapper.text()).toContain("新增策略")
    expect(wrapper.text()).toContain("新增实例")

    await wrapper.get('[data-testid="strategy-new-instance"]').trigger("click")
    await settleStrategyWorkspace()

    expect(wrapper.find('[data-testid="strategy-create-instance-panel"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="strategy-instance-dialog"]').exists()).toBe(true)

    await wrapper.get('[data-testid="strategy-create-instance-close"]').trigger("click")
    await settleStrategyWorkspace()

    expect(wrapper.find('[data-testid="strategy-create-instance-panel"]').exists()).toBe(false)

    wrapper.unmount()
  })

  it("tokenizes pasted symbols into tags in the instance composer", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [
          {
            id: "dsl-breakout",
            name: "DSL Breakout",
            version: "0.1.0",
            description: "dsl strategy",
            runtime: "dsl-go-plan",
            script: buildDslScript("DSL Breakout"),
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
        strategies: [],
      }),
    )
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    )

    const { wrapper } = await mountStrategyPage("/strategy")
    await openStrategyWorkspaceTab(wrapper, "runtime")
    await openCreateInstancePanel(wrapper)

    await wrapper.get('[data-testid="strategy-instance-symbols"]').trigger("paste", {
      clipboardData: {
        getData: () => "us:tme\nhk:00700",
      },
    })
    await settleStrategyWorkspace()

    expect(wrapper.text()).toContain("US.TME")
    expect(wrapper.text()).toContain("HK.00700")

    wrapper.unmount()
  })

  it("filters broker accounts in the searchable selector", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        brokerRuntime: buildRuntimeAccount({
          descriptor: {
            ...emptyBrokerRuntime.descriptor,
            id: "futu",
          },
          accounts: [
            {
              accountId: "123456",
              tradingEnvironment: "SIMULATE",
              marketAuthorities: ["US"],
              securityFirm: "futu-securities",
            },
            {
              accountId: "654321",
              tradingEnvironment: "REAL",
              marketAuthorities: ["HK"],
              securityFirm: "futu-securities",
            },
          ],
        }),
        definitions: [
          {
            id: "dsl-breakout",
            name: "DSL Breakout",
            version: "0.1.0",
            description: "dsl strategy",
            runtime: "dsl-go-plan",
            script: buildDslScript("DSL Breakout"),
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
        strategies: [],
      }),
    )
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    )

    const { wrapper } = await mountStrategyPage("/strategy")
    const consoleDataStore = getCurrentConsoleDataStore()
    expect(consoleDataStore).not.toBeNull()
    if (consoleDataStore == null) {
      throw new Error("console data store not initialized")
    }
    consoleDataStore.brokerSettings.value = {
      ...emptyBrokerSettings,
      accounts: [
        {
          id: "managed-1",
          brokerId: "futu",
          accountId: "123456",
          displayName: "模拟 US 123456",
          tradingEnvironment: "SIMULATE",
          market: "US",
          securityFirm: "futu-securities",
          enabled: true,
          createdAt: "2026-05-23T00:00:00.000Z",
          updatedAt: "2026-05-23T00:00:00.000Z",
        },
        {
          id: "managed-2",
          brokerId: "futu",
          accountId: "654321",
          displayName: "实盘 HK 654321",
          tradingEnvironment: "REAL",
          market: "HK",
          securityFirm: "futu-securities",
          enabled: true,
          createdAt: "2026-05-23T00:00:00.000Z",
          updatedAt: "2026-05-23T00:00:00.000Z",
        },
      ],
    }
    consoleDataStore.brokerRuntime.value = buildRuntimeAccount({
      descriptor: {
        ...emptyBrokerRuntime.descriptor,
        id: "futu",
      },
      accounts: [
        {
          accountId: "123456",
          tradingEnvironment: "SIMULATE",
          marketAuthorities: ["US"],
          securityFirm: "futu-securities",
        },
        {
          accountId: "654321",
          tradingEnvironment: "REAL",
          marketAuthorities: ["HK"],
          securityFirm: "futu-securities",
        },
      ],
    })
    await settleStrategyWorkspace()
    await openStrategyWorkspaceTab(wrapper, "runtime")
    await openCreateInstancePanel(wrapper)

    await wrapper.get('[data-testid="strategy-instance-account"]').trigger("click")
    await settleStrategyWorkspace()
    await wrapper.get('[data-testid="strategy-instance-account-search"]').setValue("654321")
    await settleStrategyWorkspace()

    expect(wrapper.find('[data-testid="strategy-instance-account-option-654321"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="strategy-instance-account-option-123456"]').exists()).toBe(false)

    await wrapper.get('[data-testid="strategy-instance-account-option-654321"]').trigger("click")
    await settleStrategyWorkspace()

    expect(wrapper.get('[data-testid="strategy-instance-account"]').text()).toContain("654321")

    wrapper.unmount()
  })

  it("clears invalid symbols on blur and blocks instance creation", async () => {
    const fetchMock = buildFetchMock({
      definitions: [
        {
          id: "dsl-breakout",
          name: "DSL Breakout",
          version: "0.1.0",
          description: "dsl strategy",
          runtime: "dsl-go-plan",
          script: buildDslScript("DSL Breakout"),
          createdAt: "2026-05-23T00:00:00.000Z",
          updatedAt: "2026-05-23T00:00:00.000Z",
        },
      ],
      strategies: [],
    })
    vi.stubGlobal("fetch", fetchMock)
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    )

    const { wrapper } = await mountStrategyPage("/strategy")
    await openStrategyWorkspaceTab(wrapper, "runtime")
    await openCreateInstancePanel(wrapper)

    const symbolInput = wrapper.get('[data-testid="strategy-instance-symbols"]')
  await wrapper.get('[data-testid="strategy-instance-symbol-market"]').setValue("US")
  await symbolInput.setValue("bad input")
    await symbolInput.trigger("blur")
    await settleStrategyWorkspace()

    expect((wrapper.get('[data-testid="strategy-instance-symbols"]').element as HTMLInputElement).value).toBe("")
  expect(wrapper.get('[data-testid="strategy-instance-symbols-validation"]').text()).toContain("请选择市场后输入代码")

    await wrapper.get('[data-testid="strategy-create-instance"]').trigger("click")
    await settleStrategyWorkspace()

    expect(
      fetchMock.mock.calls.some(([input, init]) => (
        String(input).includes("/instantiate")
        && init?.method === "POST"
      )),
    ).toBe(false)
    expect(wrapper.text()).toContain("已忽略无效交易代码")

    wrapper.unmount()
  })

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
    )
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    )

    const { wrapper } = await mountStrategyPage("/strategy")
    await openStrategyWorkspaceTab(wrapper, "runtime")

    await wrapper.get('[data-testid="strategy-instance-2"]').trigger("click")
    await flushRequests()

    expect(wrapper.text()).toContain("paused strategy execution")

    await wrapper.get('[data-testid="strategy-activity-tab-audit"]').trigger("click")
    await settleStrategyWorkspace()

    expect(wrapper.text()).toContain("manual pause")
    expect(wrapper.text()).toContain("已启用")

    wrapper.unmount()
  })

  it("shows runtime observation details for the selected strategy instance", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
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
            runtimeObservation: {
              actualStatus: "RUNNING",
              activeSymbols: ["US.AAPL", "US.MSFT"],
              lastClosedKlineAt: "2026-05-16T00:03:00.000Z",
              lastSignalAt: "2026-05-16T00:03:05.000Z",
              lastOrderAt: "2026-05-16T00:03:06.000Z",
              lastErrorAt: "2026-05-16T00:02:59.000Z",
              lastError: "network glitch",
              updatedAt: "2026-05-16T00:03:06.000Z",
            },
          },
        ],
      }),
    )
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    )

    const { wrapper } = await mountStrategyPage("/strategy")
    await openStrategyWorkspaceTab(wrapper, "runtime")

    await wrapper.get('[data-testid="strategy-instance-1"]').trigger("click")
    await flushRequests()

    expect(wrapper.find('[data-testid="strategy-runtime-observation"]').exists()).toBe(true)
    expect(wrapper.text()).toContain("实际运行态")
    expect(wrapper.text()).toContain("US.AAPL, US.MSFT")
    const runtimeTimes = wrapper.get('[data-testid="strategy-runtime-observation"]').findAll('.strategy-time-display')
    expect(runtimeTimes.length).toBeGreaterThan(0)
    expect(runtimeTimes[0].text()).not.toContain("T")
    expect(runtimeTimes[0].attributes("title")).toContain("UTC")
    expect(wrapper.text()).toContain("network glitch")

    wrapper.unmount()
  })

  it("refreshes selected strategy runtime content on demand", async () => {
    const initialStrategy = {
      id: "instance-1",
      definition: {
        strategyId: "s-alpha",
        name: "Alpha",
        version: "1.0.0",
      },
      runtime: "dsl-go-plan",
      sourceFormat: "dsl-v1" as const,
      startable: true,
      binding: {
        symbols: ["US.TME"],
        interval: "1m",
        executionMode: "live" as const,
      },
      params: { fast: 5 },
      status: "RUNNING" as const,
      createdAt: "2026-05-16T00:00:00.000Z",
      logs: [],
    }
    const refreshedStrategy = {
      ...initialStrategy,
      runtimeObservation: {
        actualStatus: "RUNNING" as const,
        activeSymbols: ["US.TME"],
        lastClosedKlineAt: "2026-05-16T00:03:00.000Z",
        lastSignalAt: "2026-05-16T00:03:05.000Z",
        lastOrderAt: "2026-05-16T00:03:06.000Z",
        updatedAt: "2026-05-16T00:03:06.000Z",
      },
    }
    const baseFetch = buildFetchMock({
      strategies: [initialStrategy],
      logsById: {
        "instance-1": ["2026-05-16T00:00:00.000Z started strategy s-alpha"],
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
      },
    })
    let refreshed = false
    const dynamicFetch = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const url = String(input)
      if (refreshed && /\/api\/v1\/strategies(?:\?|$)/.test(url)) {
        return createResponse([refreshedStrategy])
      }
      if (refreshed && url.includes("/api/v1/strategies/instance-1/logs")) {
        return createResponse({
          instanceId: "instance-1",
          logs: ["2026-05-16T00:03:05.000Z signal emitted for US.TME"],
          page: {
            limit: 500,
            offset: 0,
            total: 1,
            returned: 1,
            hasMore: false,
          },
        })
      }
      if (refreshed && url.includes("/api/v1/strategies/instance-1/audit")) {
        return createResponse({
          instanceId: "instance-1",
          entries: [
            {
              instanceId: "instance-1",
              kind: "signal.emitted",
              detail: "US.TME",
              at: "2026-05-16T00:03:05.000Z",
            },
          ],
          page: {
            limit: 500,
            offset: 0,
            total: 1,
            returned: 1,
            hasMore: false,
          },
        })
      }
      return baseFetch(input, init)
    })

    vi.stubGlobal("fetch", dynamicFetch)
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    )

    const { wrapper } = await mountStrategyPage("/strategy")
    await openStrategyWorkspaceTab(wrapper, "runtime")

    expect(wrapper.text()).toContain("started strategy s-alpha")
    expect(wrapper.find('[data-testid="strategy-runtime-observation"]').exists()).toBe(false)

    refreshed = true
    await wrapper.get('[data-testid="strategy-refresh-content"]').trigger("click")
    await settleStrategyWorkspace()

    expect(wrapper.find('[data-testid="strategy-runtime-observation"]').exists()).toBe(true)
    expect(wrapper.text()).toContain("US.TME")
    expect(wrapper.text()).toContain("signal emitted for US.TME")

    wrapper.unmount()
  })

  it("counts only actual running runtime observations in the runtime header", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        strategies: [
          {
            id: "instance-running",
            definition: {
              strategyId: "s-alpha",
              name: "Alpha",
              version: "1.0.0",
            },
            params: { fast: 5 },
            status: "RUNNING",
            createdAt: "2026-05-16T00:00:00.000Z",
            logs: [],
            runtimeObservation: {
              actualStatus: "RUNNING",
              activeSymbols: ["US.AAPL"],
            },
          },
          {
            id: "instance-stale",
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
      }),
    )
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    )

    const { wrapper } = await mountStrategyPage("/strategy")
    await openStrategyWorkspaceTab(wrapper, "runtime")

    expect(wrapper.text()).toContain("1 个活跃实例")
    expect(wrapper.text()).toContain("1 个运行中")

    wrapper.unmount()
  })

  it("shows stale instance strategy status and refreshes to latest definition", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [
          {
            id: "dsl-breakout",
            name: "DSL Breakout",
            version: "0.1.1",
            description: "latest dsl strategy",
            runtime: "dsl-go-plan",
            script: buildDslScript("DSL Breakout", ['log "latest"'], { version: "0.1.1" }),
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:05:00.000Z",
          },
        ],
        strategies: [
          {
            id: "dsl-breakout-instance",
            definition: {
              strategyId: "dsl-breakout",
              name: "DSL Breakout",
              version: "0.1.0",
            },
            binding: {
              symbols: ["US.AAPL"],
              interval: "5m",
              executionMode: "live",
            },
            params: {
              definitionId: "dsl-breakout",
              script: buildDslScript("DSL Breakout", ['log "old"'], { version: "0.1.0" }),
            },
            status: "STOPPED",
            createdAt: "2026-05-23T00:01:00.000Z",
            logs: [],
          },
        ],
      }),
    )
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    )

    const { wrapper } = await mountStrategyPage("/strategy")
    await openStrategyWorkspaceTab(wrapper, "runtime")
    await waitForSelector(wrapper, '[data-testid="strategy-dsl-breakout-instance"]')

    expect(wrapper.find('[data-testid="strategy-definition-stale-dsl-breakout-instance"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="strategy-definition-sync-badge"]').text()).toContain("待刷新 v0.1.0 -> v0.1.1")
    expect(wrapper.get('[data-testid="strategy-refresh-definition"]').attributes("disabled")).toBeUndefined()

    await wrapper.get('[data-testid="strategy-refresh-definition"]').trigger("click")
    await settleStrategyWorkspace()

    expect(wrapper.text()).toContain("已刷新实例策略到最新版本：DSL Breakout / v0.1.1")
    expect(wrapper.get('[data-testid="strategy-definition-sync-badge"]').text()).toContain("已同步至 v0.1.1")
    expect(wrapper.find('[data-testid="strategy-definition-stale-dsl-breakout-instance"]').exists()).toBe(false)

    wrapper.unmount()
  })
})