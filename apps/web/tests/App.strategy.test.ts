// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest"

import { PINE_V6_BLOCK_KINDS, buildPineV6WorkflowScript, createDefaultPineV6Workflow } from "../src/features/pineV6Workflow"
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
    const nativeWorkflow = createDefaultPineV6Workflow("Native EMA")
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
            script: `${buildPineV6WorkflowScript(nativeWorkflow)}\nimport TradingView/ta/7 as ta7\n`,
            visualModel: nativeWorkflow,
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
          {
            id: "other-instance",
            definition: {
              strategyId: "other-definition",
              name: "Other Strategy",
              version: "0.1.0",
            },
            binding: {
              symbols: ["US.AAPL"],
              interval: "1d",
              executionMode: "live",
            },
            params: {
              symbol: "US.AAPL",
              interval: "1d",
              executionMode: "live",
            },
            status: "STOPPED",
            createdAt: "2026-05-23T00:00:00.000Z",
            logs: [],
          },
        ],
      }),
    )
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket)

    const { router, wrapper } = await mountStrategyPage("/strategy/design")
    await settleStrategyWorkspace()

    expect(router.currentRoute.value.path).toBe("/strategy/design")
    expect(wrapper.text()).toContain("策略快捷指令工作台")
    expect(wrapper.text()).toContain("结构指令")
    expect(wrapper.text()).toContain("Pine v6 源码")
    expect(wrapper.text()).toContain("Native EMA")
    expect(wrapper.find(".strategy-native-banner--ok").exists()).toBe(false)
    expect(wrapper.text()).not.toContain("已加载 Native EMA")
    expect(wrapper.get('[data-testid="strategy-display-mode-instruction"]').text()).toBe("指令")
    expect(wrapper.get('[data-testid="strategy-display-mode-split"]').text()).toBe("双栏")
    expect(wrapper.get('[data-testid="strategy-display-mode-code"]').text()).toBe("代码")
    expect(wrapper.find(".strategy-native-shell.splitpanes").exists()).toBe(true)
    expect(wrapper.findAll(".strategy-native-shell > .splitpanes__pane")).toHaveLength(2)
    expect(wrapper.findAll(".strategy-native-shell > .splitpanes__splitter")).toHaveLength(1)
    expect(wrapper.findAll(".strategy-native-instruction > .splitpanes__pane")).toHaveLength(2)
    expect(wrapper.findAll(".strategy-native-instruction > .splitpanes__splitter")).toHaveLength(1)
    expect(wrapper.find('[data-testid="pine-source-structure-index"]').exists()).toBe(true)
    expect(wrapper.text()).not.toContain("源码结构块")
    expect(wrapper.text()).not.toContain("K线准备")
    expect(wrapper.text()).not.toContain("运行向导")
    expect(wrapper.text()).not.toContain("保存并启动")
    expect(wrapper.text()).not.toContain("仅允许平仓")
    expect(wrapper.text()).not.toContain("最大下单数量")
    expect(wrapper.text()).toContain("策略实例")
    expect(wrapper.text()).toContain("Native EMA")
    expect(wrapper.text()).not.toContain("Other Strategy")
    const readonlyInstance = wrapper.get(".strategy-native-instance")
    expect(readonlyInstance.element.tagName).toBe("SECTION")
    expect(readonlyInstance.findAll("button")).toHaveLength(0)
    await wrapper.get('[data-testid="strategy-display-mode-instruction"]').trigger("click")
    expect(wrapper.findAll(".strategy-native-shell > .splitpanes__pane")).toHaveLength(1)
    expect(wrapper.findAll(".strategy-native-shell > .splitpanes__splitter")).toHaveLength(0)
    expect(wrapper.findAll(".strategy-native-instruction > .splitpanes__pane")).toHaveLength(2)
    expect(wrapper.findAll(".strategy-native-instruction > .splitpanes__splitter")).toHaveLength(1)
    expect(wrapper.find('[data-testid="strategy-script-editor"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="pine-source-structure-index"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="pine-source-node-order"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="pine-v6-workflow-block-strategy_entry"]').exists()).toBe(false)

    await wrapper.get('[data-testid="strategy-display-mode-code"]').trigger("click")
    expect(wrapper.findAll(".strategy-native-shell > .splitpanes__pane")).toHaveLength(2)
    expect(wrapper.findAll(".strategy-native-shell > .splitpanes__splitter")).toHaveLength(1)
    expect(wrapper.findAll(".strategy-native-instruction > .splitpanes__pane")).toHaveLength(1)
    expect(wrapper.findAll(".strategy-native-instruction > .splitpanes__splitter")).toHaveLength(0)
    expect(wrapper.find('[data-testid="strategy-script-editor"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="pine-source-structure-index"]').exists()).toBe(false)
    expect(wrapper.text()).toContain("策略定义")
    expect(wrapper.find('[data-testid="pine-v6-workflow-block-strategy_entry"]').exists()).toBe(false)

    await wrapper.get('[data-testid="strategy-display-mode-split"]').trigger("click")
    expect(wrapper.findAll(".strategy-native-shell > .splitpanes__pane")).toHaveLength(2)
    expect(wrapper.findAll(".strategy-native-shell > .splitpanes__splitter")).toHaveLength(1)
    expect(wrapper.findAll(".strategy-native-instruction > .splitpanes__pane")).toHaveLength(2)
    expect(wrapper.findAll(".strategy-native-instruction > .splitpanes__splitter")).toHaveLength(1)
    expect(wrapper.find('[data-testid="strategy-instruction-scroll"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="strategy-variables-panel"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="pine-source-node-order"]').text()).toContain("指令")
    const sourceEditor = wrapper.get('[data-testid="strategy-script-editor"]').element as HTMLTextAreaElement
    expect(sourceEditor.value).toContain("//@version=6")
    expect(sourceEditor.value).toContain("import TradingView/ta/7 as ta7")
    expect(sourceEditor.readOnly).toBe(true)
    await wrapper.get('[data-testid="strategy-declaration-title"]').setValue("Native Edited")
    await flushRequests()
    expect((wrapper.get('[data-testid="strategy-script-editor"]').element as HTMLTextAreaElement).value).toContain('strategy("Native Edited"')
    const scriptBeforeSourceEdit = sourceEditor.value
    await wrapper.get('[data-testid="strategy-source-override-toggle"]').setValue(true)
    await flushRequests()
    const editableSourceEditor = wrapper.get('[data-testid="strategy-script-editor"]').element as HTMLTextAreaElement
    expect(editableSourceEditor.readOnly).toBe(false)
    expect(editableSourceEditor.value).toBe(scriptBeforeSourceEdit)
    expect(wrapper.find('[data-testid="strategy-logic-flow-canvas"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="strategy-logic-flow-builder"]').exists()).toBe(false)

    wrapper.unmount()
  })

  it("uses the Monaco Pine editor for advanced source editing", async () => {
    const savedPayloads: Array<Record<string, unknown>> = []
    const fetchMock = buildFetchMock({ definitions: [] })
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.endsWith("/api/v1/strategy-definitions") && init?.method === "POST" && typeof init.body === "string") {
        savedPayloads.push(JSON.parse(init.body) as Record<string, unknown>)
      }
      return fetchMock(input, init)
    }))
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket)

    const { wrapper } = await mountStrategyPage("/strategy/design")
    await settleStrategyWorkspace()

    expect(wrapper.text()).toContain("源码是运行权威")
    expect(wrapper.text()).not.toContain("源码为运行权威；匹配项可编辑，Raw Pine 保留原文。")
    expect(wrapper.text()).not.toContain("源码结构块")
    expect(wrapper.text()).toContain("图块生成")
    expect(wrapper.find('[data-testid="pine-source-node-strategy"]').exists()).toBe(true)

    await wrapper.get('[data-testid="strategy-source-override-toggle"]').setValue(true)
    await flushRequests()

    const sourceEditor = wrapper.get('[data-testid="strategy-script-editor"]')
    expect((sourceEditor.element as HTMLTextAreaElement).readOnly).toBe(false)

    const manualSource = '//@version=6\nstrategy("Manual", overlay=true)\nif close > open\n    strategy.entry("Long", strategy.long)\nimport TradingView/ta/7 as ta7\n'
    await sourceEditor.setValue(manualSource)
    await flushRequests()

    expect((sourceEditor.element as HTMLTextAreaElement).value).toContain('strategy("Manual"')
    expect(wrapper.text()).toContain("源码覆盖")
    let orderSourceNode = wrapper.get('[data-testid="pine-source-node-order"]')
    expect(orderSourceNode.text()).toContain("指令")
    expect(orderSourceNode.text()).toContain("按 strategy.long 开仓，订单 Long")
    expect(orderSourceNode.find("select").exists()).toBe(false)
    await orderSourceNode.get("button").trigger("click")
    await flushRequests()
    orderSourceNode = wrapper.get('[data-testid="pine-source-node-order"]')
    expect(orderSourceNode.findAll("select")[0]?.text()).toContain("通用订单")
    expect(orderSourceNode.findAll(".pine-block__actions button")).toHaveLength(4)
    await orderSourceNode.findAll("select")[1]!.setValue("strategy.short")
    await flushRequests()
    orderSourceNode = wrapper.get('[data-testid="pine-source-node-order"]')
    const qtyInput = orderSourceNode.findAll("input")[1]
    expect(qtyInput).toBeDefined()
    await qtyInput!.setValue("25")
    await flushRequests()
    expect((sourceEditor.element as HTMLTextAreaElement).value).toContain('strategy.entry("Long", strategy.short, qty=25)')
    const rawSourceNode = wrapper.get('[data-testid="pine-source-node-library"]')
    expect(rawSourceNode.text()).toContain("导入库 ta7")
    expect(rawSourceNode.text()).toContain("导入库 ta7：TradingView/ta/7")
    await rawSourceNode.get("button").trigger("click")
    expect((sourceEditor.element as HTMLTextAreaElement).selectionStart).toBe((sourceEditor.element as HTMLTextAreaElement).value.indexOf("import TradingView"))

    await wrapper.get('[data-testid="strategy-source-override-toggle"]').setValue(false)
    await flushRequests()
    expect((sourceEditor.element as HTMLTextAreaElement).readOnly).toBe(true)
    expect((sourceEditor.element as HTMLTextAreaElement).value).toContain("import TradingView/ta/7 as ta7")

    const saveButton = wrapper.findAll("button").find((button) => button.text() === "保存")
    expect(saveButton).toBeDefined()
    await saveButton!.trigger("click")
    await flushRequests()

    expect(savedPayloads).toHaveLength(1)
    expect(savedPayloads[0].script).toContain('strategy("Manual"')
    expect(savedPayloads[0].script).toContain("import TradingView/ta/7 as ta7")
    expect(wrapper.text()).toContain("已保存")
    expect(savedPayloads[0].visualModel).toMatchObject({
      engine: "pine-v6-workflow",
      blocks: expect.arrayContaining([
        expect.objectContaining({
          kind: "if",
          thenBlocks: expect.arrayContaining([
            expect.objectContaining({
              kind: "strategy_entry",
              params: expect.objectContaining({ direction: "strategy.short", qty: "25" }),
            }),
          ]),
        }),
      ]),
    })

    wrapper.unmount()
  })

  it("edits a structure instruction block and reflects it in the source pane", async () => {
    vi.stubGlobal("fetch", buildFetchMock({ definitions: [] }))
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket)

    const { wrapper } = await mountStrategyPage("/strategy/design")
    await settleStrategyWorkspace()

    let orderSourceNode = wrapper.get('[data-testid="pine-source-node-order"]')
    expect(orderSourceNode.text()).toContain("指令")
    expect(orderSourceNode.text()).toContain("按 strategy.long 开仓，订单 Long")
    expect(orderSourceNode.find("select").exists()).toBe(false)
    await orderSourceNode.get("button").trigger("click")
    await flushRequests()
    orderSourceNode = wrapper.get('[data-testid="pine-source-node-order"]')
    expect(orderSourceNode.findAll("select")[0]?.text()).toContain("通用订单")
    expect(orderSourceNode.findAll(".pine-block__actions button")).toHaveLength(4)
    await orderSourceNode.findAll("select")[1]!.setValue("strategy.short")
    await flushRequests()
    orderSourceNode = wrapper.get('[data-testid="pine-source-node-order"]')
    const qtyInput = orderSourceNode.findAll("input")[1]
    expect(qtyInput).toBeDefined()
    await qtyInput!.setValue("10")
    await flushRequests()

    const sourceEditor = wrapper.get('[data-testid="strategy-script-editor"]').element as HTMLTextAreaElement
    expect(sourceEditor.value).toContain('strategy.entry("Long", strategy.short, qty=10)')
    expect(sourceEditor.readOnly).toBe(false)
    expect(wrapper.text()).toContain("下一根 K 线成交")
    expect(wrapper.text()).toContain("L14 入场订单")
    expect(wrapper.find('[data-testid="pine-v6-workflow-block-strategy_order"]').exists()).toBe(false)

    wrapper.unmount()
  })

  it("creates and performs source-backed structure block actions", async () => {
    vi.stubGlobal("fetch", buildFetchMock({ definitions: [] }))
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket)

    const { wrapper } = await mountStrategyPage("/strategy/design")
    await settleStrategyWorkspace()

    const creator = wrapper.get(".pine-block-list__add select")
    expect(creator.findAll("option")).toHaveLength(PINE_V6_BLOCK_KINDS.length + 1)
    expect((wrapper.get('[data-testid="strategy-source-undo"]').element as HTMLButtonElement).disabled).toBe(true)
    expect((wrapper.get('[data-testid="strategy-source-redo"]').element as HTMLButtonElement).disabled).toBe(true)
    await creator.setValue("strategy_order")
    await flushRequests()

    let sourceEditor = wrapper.get('[data-testid="strategy-script-editor"]').element as HTMLTextAreaElement
    expect(sourceEditor.value).toContain('strategy.order("Order", strategy.long)')
    expect(sourceEditor.readOnly).toBe(false)
    expect((wrapper.get('[data-testid="strategy-source-undo"]').element as HTMLButtonElement).disabled).toBe(false)
    expect((wrapper.get('[data-testid="strategy-source-redo"]').element as HTMLButtonElement).disabled).toBe(true)
    await wrapper.get('[data-testid="strategy-source-undo"]').trigger("click")
    await flushRequests()
    sourceEditor = wrapper.get('[data-testid="strategy-script-editor"]').element as HTMLTextAreaElement
    expect(sourceEditor.value).not.toContain('strategy.order("Order", strategy.long)')
    expect((wrapper.get('[data-testid="strategy-source-redo"]').element as HTMLButtonElement).disabled).toBe(false)
    await wrapper.get('[data-testid="strategy-source-redo"]').trigger("click")
    await flushRequests()
    sourceEditor = wrapper.get('[data-testid="strategy-script-editor"]').element as HTMLTextAreaElement
    expect(sourceEditor.value).toContain('strategy.order("Order", strategy.long)')

    let orderNodes = wrapper.findAll('[data-testid="pine-source-node-order"]')
    const orderNode = orderNodes[orderNodes.length - 1]!
    await orderNode.get("button").trigger("click")
    await flushRequests()
    await wrapper.findAll('[data-testid="pine-source-node-order"]').at(-1)!.findAll("select")[0]!.setValue("plot")
    await flushRequests()
    sourceEditor = wrapper.get('[data-testid="strategy-script-editor"]').element as HTMLTextAreaElement
    expect(sourceEditor.value).toContain('plot(close, title="Close", color=color.blue)')

    const rawSource = '//@version=6\nstrategy("Raw Ops", overlay=true)\nimport TradingView/ta/7 as ta7\n'
    await wrapper.get('[data-testid="strategy-script-editor"]').setValue(rawSource)
    await flushRequests()
    let rawNode = wrapper.get('[data-testid="pine-source-node-library"]')
    await rawNode.get("button").trigger("click")
    await flushRequests()
    rawNode = wrapper.get('[data-testid="pine-source-node-library"]')
    await rawNode.findAll(".pine-block__actions button")[2]!.trigger("click")
    await flushRequests()
    sourceEditor = wrapper.get('[data-testid="strategy-script-editor"]').element as HTMLTextAreaElement
    expect(sourceEditor.value.match(/import TradingView\/ta\/7 as ta7/g)).toHaveLength(2)
    rawNode = wrapper.get('[data-testid="pine-source-node-library"]')
    await rawNode.get("button").trigger("click")
    await flushRequests()
    await wrapper.get('[data-testid="pine-source-node-library"]').findAll(".pine-block__actions button")[3]!.trigger("click")
    await flushRequests()
    sourceEditor = wrapper.get('[data-testid="strategy-script-editor"]').element as HTMLTextAreaElement
    expect(sourceEditor.value.match(/import TradingView\/ta\/7 as ta7/g)).toHaveLength(1)

    wrapper.unmount()
  })
})
