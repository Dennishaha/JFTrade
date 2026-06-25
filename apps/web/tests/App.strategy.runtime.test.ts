// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest"

import {
  MockWebSocket,
  buildFetchMock,
  mountStrategyPage,
  resetStrategyPageTestState,
  settleStrategyWorkspace,
  waitForSelector,
} from "./strategyPageTestUtils"

afterEach(() => {
  vi.unstubAllGlobals()
  resetStrategyPageTestState()
})

describe("Strategy page unified Pine v6 launch wizard", () => {
  it("opens on the original strategy execution page", async () => {
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
            binding: {
              symbols: ["HK.00700"],
              interval: "5m",
              executionMode: "live",
            },
            params: {
              symbol: "HK.00700",
              interval: "5m",
              executionMode: "live",
            },
            status: "RUNNING",
            createdAt: "2026-05-16T00:00:00.000Z",
            logs: [],
          },
        ],
        logsById: {
          "instance-1": [
            "2026-05-16T00:00:00.000Z started strategy s-mean-revert",
            "2026-05-16T00:00:02.000Z tick QUOTE_SNAPSHOT HK.00700",
          ],
        },
        auditById: {
          "instance-1": [
            {
              instanceId: "instance-1",
              kind: "started",
              detail: "s-mean-revert",
              at: "2026-05-16T00:00:00.000Z",
            },
          ],
        },
      }),
    )
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket)

    const { router, wrapper } = await mountStrategyPage("/strategy")
    await waitForSelector(wrapper, '[data-testid="strategy-instance-1"]')

    expect(router.currentRoute.value.path).toBe("/strategy/runtime")
    expect(wrapper.text()).toContain("策略运行")
    expect(wrapper.find('[data-testid="strategy-workspace-tab-design"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="strategy-workspace-tab-runtime"]').exists()).toBe(false)
    expect(wrapper.text()).toContain("Mean Revert")
    expect(wrapper.text()).toContain("tick QUOTE_SNAPSHOT HK.00700")
    expect(wrapper.text()).toContain("运行审计")
    expect(wrapper.find('[data-testid="strategy-design-stage"]').exists()).toBe(false)

    wrapper.unmount()
  })

  it("analyzes, saves, syncs K-lines, instantiates, and starts from one entry", async () => {
    const fetchMock = buildFetchMock({ definitions: [], strategies: [] })
    vi.stubGlobal("fetch", fetchMock)
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket)

    const { router, wrapper } = await mountStrategyPage("/strategy/design")
    await settleStrategyWorkspace()
    expect(router.currentRoute.value.path).toBe("/strategy/design")

    await wrapper.get(".strategy-native-primary").trigger("click")
    await settleStrategyWorkspace()
    await settleStrategyWorkspace()

    const requestedUrls = fetchMock.mock.calls.map(([input]) => String(input))
    expect(requestedUrls.some((url) => url.endsWith("/api/v1/strategy-pine/analyze"))).toBe(true)
    expect(
      fetchMock.mock.calls.some(([input, init]) =>
        String(input).endsWith("/api/v1/strategy-definitions") && init?.method === "POST",
      ),
    ).toBe(true)
    expect(
      fetchMock.mock.calls.some(([input, init]) =>
        String(input).endsWith("/api/v1/backtests/sync") && init?.method === "POST",
      ),
    ).toBe(true)
    expect(requestedUrls.some((url) => /\/api\/v1\/strategy-definitions\/pine-strategy-1\/instantiate/.test(url))).toBe(true)
    expect(requestedUrls.some((url) => /\/api\/v1\/strategies\/pine-strategy-1-instance\/start/.test(url))).toBe(true)
    expect(router.currentRoute.value.path).toBe("/strategy/runtime")
    expect(router.currentRoute.value.query.definitionId).toBe("pine-strategy-1")
    expect(wrapper.text()).toContain("已启动策略实例")
    expect(wrapper.text()).toContain("策略运行")
    expect(wrapper.find('[data-testid="strategy-design-stage"]').exists()).toBe(false)

    wrapper.unmount()
  })

  it("does not update and start a selected RUNNING instance", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [
          {
            id: "pine-native",
            name: "Native EMA",
            version: "0.1.0",
            description: "pine workflow",
            runtime: "pine-go-plan",
            sourceFormat: "pine-v6",
            symbol: "00700",
            interval: "5m",
            script: "//@version=6\nstrategy(\"Native EMA\", overlay=true)\n",
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
        strategies: [
          {
            id: "pine-native-instance",
            definition: {
              strategyId: "pine-native",
              name: "Native EMA",
              version: "0.1.0",
            },
            binding: {
              symbols: ["HK.00700"],
              interval: "5m",
              executionMode: "live",
            },
            params: {
              symbol: "HK.00700",
              interval: "5m",
              executionMode: "live",
            },
            status: "RUNNING",
            createdAt: "2026-05-23T00:00:00.000Z",
            logs: [],
          },
        ],
      }),
    )
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket)

    const { wrapper } = await mountStrategyPage("/strategy/design")
    await settleStrategyWorkspace()

    await wrapper.get(".strategy-native-instance").trigger("click")
    await wrapper.get(".strategy-native-primary").trigger("click")
    await settleStrategyWorkspace()
    await settleStrategyWorkspace()

    expect(wrapper.text()).toContain("当前选中实例不是 STOPPED")
    expect(wrapper.text()).not.toContain("已启动策略实例")

    wrapper.unmount()
  })
})
