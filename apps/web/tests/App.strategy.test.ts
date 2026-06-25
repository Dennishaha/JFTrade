// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest"

import { createDefaultPineV6Workflow } from "../src/features/pineV6Workflow"
import {
  MockWebSocket,
  buildFetchMock,
  flushRequests,
  mountStrategyPage,
  resetStrategyPageTestState,
  settleStrategyWorkspace,
} from "./strategyPageTestUtils"

afterEach(() => {
  vi.unstubAllGlobals()
  resetStrategyPageTestState()
})

describe("Strategy page Pine v6 workflow", () => {
  it("renders the native Pine v6 shortcuts workspace instead of the old LogicFlow canvas", async () => {
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
            visualModel: createDefaultPineV6Workflow("Native EMA"),
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
      }),
    )
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket)

    const { router, wrapper } = await mountStrategyPage("/strategy/design")
    await settleStrategyWorkspace()

    expect(router.currentRoute.value.path).toBe("/strategy/design")
    expect(wrapper.text()).toContain("策略快捷指令工作台")
    expect(wrapper.text()).toContain("收盘确认指令")
    expect(wrapper.text()).toContain("Pine v6 源码")
    expect(wrapper.text()).toContain("Native EMA")
    expect(wrapper.get('[data-testid="strategy-display-mode-instruction"]').text()).toBe("指令")
    expect(wrapper.get('[data-testid="strategy-display-mode-split"]').text()).toBe("双栏")
    expect(wrapper.get('[data-testid="strategy-display-mode-code"]').text()).toBe("代码")
    expect(wrapper.find(".strategy-native-shell.splitpanes").exists()).toBe(true)
    expect(wrapper.findAll(".strategy-native-shell > .splitpanes__pane")).toHaveLength(2)
    expect(wrapper.findAll(".strategy-native-shell > .splitpanes__splitter")).toHaveLength(1)
    expect(wrapper.findAll(".strategy-native-instruction > .splitpanes__pane")).toHaveLength(2)
    expect(wrapper.findAll(".strategy-native-instruction > .splitpanes__splitter")).toHaveLength(1)
    expect(wrapper.text()).not.toContain("K线准备")
    await wrapper.get('[data-testid="strategy-display-mode-instruction"]').trigger("click")
    expect(wrapper.findAll(".strategy-native-shell > .splitpanes__pane")).toHaveLength(1)
    expect(wrapper.findAll(".strategy-native-shell > .splitpanes__splitter")).toHaveLength(0)
    expect(wrapper.findAll(".strategy-native-instruction > .splitpanes__pane")).toHaveLength(2)
    expect(wrapper.findAll(".strategy-native-instruction > .splitpanes__splitter")).toHaveLength(1)
    expect(wrapper.find('[data-testid="strategy-script-editor"]').exists()).toBe(false)
    expect(wrapper.text()).toContain("快线上穿慢线")
    expect(wrapper.find('[data-testid="pine-v6-workflow-block-strategy_entry"]').exists()).toBe(false)

    await wrapper.get('[data-testid="strategy-display-mode-code"]').trigger("click")
    expect(wrapper.findAll(".strategy-native-shell > .splitpanes__pane")).toHaveLength(2)
    expect(wrapper.findAll(".strategy-native-shell > .splitpanes__splitter")).toHaveLength(1)
    expect(wrapper.findAll(".strategy-native-instruction > .splitpanes__pane")).toHaveLength(1)
    expect(wrapper.findAll(".strategy-native-instruction > .splitpanes__splitter")).toHaveLength(0)
    expect(wrapper.find('[data-testid="strategy-script-editor"]').exists()).toBe(true)
    expect(wrapper.text()).toContain("策略定义")
    expect(wrapper.find('[data-testid="pine-v6-workflow-block-strategy_entry"]').exists()).toBe(false)

    await wrapper.get('[data-testid="strategy-display-mode-split"]').trigger("click")
    expect(wrapper.findAll(".strategy-native-shell > .splitpanes__pane")).toHaveLength(2)
    expect(wrapper.findAll(".strategy-native-shell > .splitpanes__splitter")).toHaveLength(1)
    expect(wrapper.findAll(".strategy-native-instruction > .splitpanes__pane")).toHaveLength(2)
    expect(wrapper.findAll(".strategy-native-instruction > .splitpanes__splitter")).toHaveLength(1)
    expect(wrapper.find('[data-testid="strategy-instruction-scroll"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="strategy-variables-panel"]').classes()).not.toContain("strategy-native-variables--open")
    await wrapper.get(".strategy-native-variables__bar").trigger("click")
    expect(wrapper.get('[data-testid="strategy-variables-panel"]').classes()).toContain("strategy-native-variables--open")
    expect(wrapper.get('[data-testid="strategy-variables-body"]').text()).toContain("整数 int")
    const conditionBlockTitle = wrapper
      .findAll(".pine-block__summary")
      .find((item) => item.text().includes("快线上穿慢线"))
    expect(conditionBlockTitle).toBeDefined()
    await conditionBlockTitle?.trigger("click")
    await settleStrategyWorkspace()
    expect(wrapper.find('[data-testid="pine-v6-workflow-block-strategy_entry"]').exists()).toBe(true)
    const sourceEditor = wrapper.get('[data-testid="strategy-script-editor"]').element as HTMLTextAreaElement
    expect(sourceEditor.value).toContain("//@version=6")
    expect(sourceEditor.readOnly).toBe(true)
    expect(wrapper.find('[data-testid="strategy-logic-flow-canvas"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="strategy-logic-flow-builder"]').exists()).toBe(false)

    wrapper.unmount()
  })

  it("uses the Monaco Pine editor for advanced source editing", async () => {
    vi.stubGlobal("fetch", buildFetchMock({ definitions: [] }))
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket)

    const { wrapper } = await mountStrategyPage("/strategy/design")
    await settleStrategyWorkspace()

    await wrapper.get('[data-testid="strategy-source-override-toggle"]').setValue(true)
    await flushRequests()

    const sourceEditor = wrapper.get('[data-testid="strategy-script-editor"]')
    expect((sourceEditor.element as HTMLTextAreaElement).readOnly).toBe(false)

    await sourceEditor.setValue('//@version=6\nstrategy("Manual", overlay=true)\n')
    await flushRequests()

    expect((sourceEditor.element as HTMLTextAreaElement).value).toContain('strategy("Manual"')

    wrapper.unmount()
  })

  it("adds a Pine v6 order block and reflects it in the generated source preview", async () => {
    vi.stubGlobal("fetch", buildFetchMock({ definitions: [] }))
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket)

    const { wrapper } = await mountStrategyPage("/strategy/design")
    await settleStrategyWorkspace()

    const addKindSelect = wrapper.find(".pine-block-list__add select")
    await addKindSelect.setValue("strategy_order")
    await flushRequests()

    expect(wrapper.find('[data-testid="pine-v6-workflow-block-strategy_order"]').exists()).toBe(true)
    const sourceEditor = wrapper.get('[data-testid="strategy-script-editor"]').element as HTMLTextAreaElement
    expect(sourceEditor.value).toContain("strategy.order")
    expect(wrapper.text()).toContain("下一根 K 线成交")
    expect(wrapper.text()).toContain("OCA 当前为明确不支持边界")

    wrapper.unmount()
  })
})
