// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest"

import {
  MockWebSocket,
  buildFetchMock,
  mountStrategyPage,
  resetStrategyPageTestState,
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

})
