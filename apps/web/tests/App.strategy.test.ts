// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";
import type { StrategyDefinitionDocument } from "@/contracts";
import StrategyLogicFlowDesigner from "../src/components/StrategyLogicFlowDesigner.vue";
import {
  MockWebSocket,
  buildDslScript,
  buildFetchMock,
  flushRequests,
  mountStrategyPage,
  openNewStrategyFromRuntime,
  openStrategyDesignWorkspace,
  openStrategyTemplatesPanel,
  openStrategyWorkspaceTab,
  resetStrategyPageTestState,
  settleStrategyWorkspace,
  showStrategyCodeEditor,
} from "./strategyPageTestUtils";

afterEach(() => {
  vi.unstubAllGlobals();
  resetStrategyPageTestState();
});

describe("Strategy page", () => {
  it("shows the DSL strategy design workspace", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [
          {
            id: "dsl-mean-revert",
            name: "DSL Mean Revert",
            version: "0.1.0",
            description: "dsl strategy",
            runtime: "dsl-go-plan",
            symbol: "00700",
            interval: "1m",
            script: buildDslScript("DSL Mean Revert"),
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
      }),
    );
    vi.stubGlobal(
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const { wrapper } = await mountStrategyPage("/strategy");

    expect(wrapper.text()).toContain("策略运行");
    expect(wrapper.find('[data-testid="strategy-script-editor"]').exists()).toBe(false);

    await openStrategyDesignWorkspace(wrapper);

    expect(wrapper.text()).toContain("设计");
    expect(wrapper.text()).toContain("策略定义");
    expect(wrapper.text()).toContain("DSL Mean Revert");
    expect(wrapper.text()).toContain("dsl-go-plan");
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

    expect(wrapper.text()).toContain("DSL 策略工作台");
    expect(wrapper.get('[data-testid="strategy-script-editor"]').element).toBeTruthy();
    expect(wrapper.html()).toContain("on kline_close:");

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
            id: "dsl-mean-revert",
            name: "DSL Mean Revert",
            version: "0.1.0",
            description: "dsl strategy",
            runtime: "dsl-go-plan",
            symbol: "00700",
            interval: "1m",
            script: buildDslScript("DSL Mean Revert"),
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
      }),
    );
    vi.stubGlobal(
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const { wrapper } = await mountStrategyPage("/strategy");

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
            id: "dsl-mean-revert",
            name: "DSL Mean Revert",
            version: "0.1.0",
            description: "dsl strategy",
            runtime: "dsl-go-plan",
            symbol: "00700",
            interval: "1m",
            script: buildDslScript("DSL Mean Revert"),
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
      }),
    );
    vi.stubGlobal(
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const { wrapper } = await mountStrategyPage("/strategy");

    await openStrategyDesignWorkspace(wrapper);

    expect(wrapper.find('[data-testid="strategy-definitions-panel"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-definition-dsl-mean-revert"]').exists()).toBe(false);

    await wrapper.get('[data-testid="toggle-strategy-definitions-floating"]').trigger("click");
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-definitions-panel"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-definition-dsl-mean-revert"]').exists()).toBe(true);

    await wrapper.get('[data-testid="toggle-strategy-definitions"]').trigger("click");
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-definitions-panel"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-definition-dsl-mean-revert"]').exists()).toBe(false);

    await wrapper.get('[data-testid="toggle-strategy-definitions-floating"]').trigger("click");
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-definitions-panel"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-definition-dsl-mean-revert"]').exists()).toBe(true);

    wrapper.unmount();
  });

  it("deletes an unused strategy definition from the design sidebar", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [
          {
            id: "dsl-mean-revert",
            name: "DSL Mean Revert",
            version: "0.1.0",
            description: "dsl strategy",
            runtime: "dsl-go-plan",
            script: buildDslScript("DSL Mean Revert"),
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
          {
            id: "dsl-breakout",
            name: "DSL Breakout",
            version: "0.1.0",
            description: "second dsl strategy",
            runtime: "dsl-go-plan",
            script: buildDslScript("DSL Breakout"),
            createdAt: "2026-05-23T00:01:00.000Z",
            updatedAt: "2026-05-23T00:01:00.000Z",
          },
        ],
      }),
    );
    vi.stubGlobal(
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const { wrapper } = await mountStrategyPage("/strategy");

    await openStrategyDesignWorkspace(wrapper);
    await wrapper.get('[data-testid="toggle-strategy-definitions-floating"]').trigger("click");
    await settleStrategyWorkspace();

    await wrapper.get('[data-testid="delete-strategy-definition-dsl-mean-revert"]').trigger("click");
    await settleStrategyWorkspace();

    expect(wrapper.find('[data-testid="strategy-delete-definition-dialog"]').exists()).toBe(true);
    expect(wrapper.get('[data-testid="strategy-delete-definition-summary"]').text()).toContain("DSL Mean Revert");
    expect(wrapper.find('[data-testid="confirm-delete-strategy-definition"]').exists()).toBe(true);

    await wrapper.get('[data-testid="confirm-delete-strategy-definition"]').trigger("click");
    await settleStrategyWorkspace();

    expect(wrapper.text()).toContain("策略已删除：DSL Mean Revert。");
    expect(wrapper.find('[data-testid="strategy-definition-dsl-mean-revert"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-definition-dsl-breakout"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-delete-definition-dialog"]').exists()).toBe(false);

    wrapper.unmount();
  });

  it("blocks deleting a strategy definition when linked instances still exist", async () => {
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
              executionMode: "notify_only",
            },
            params: {
              definitionId: "dsl-breakout",
              script: buildDslScript("DSL Breakout"),
            },
            status: "STOPPED",
            createdAt: "2026-05-23T00:01:00.000Z",
            logs: [],
          },
        ],
      }),
    );
    vi.stubGlobal(
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const { wrapper } = await mountStrategyPage("/strategy");

    await openStrategyDesignWorkspace(wrapper);
    await wrapper.get('[data-testid="toggle-strategy-definitions-floating"]').trigger("click");
    await settleStrategyWorkspace();

    await wrapper.get('[data-testid="delete-strategy-definition-dsl-breakout"]').trigger("click");
    await settleStrategyWorkspace();

    expect(wrapper.find('[data-testid="strategy-delete-definition-dialog"]').exists()).toBe(true);
    expect(wrapper.get('[data-testid="strategy-delete-definition-summary"]').text()).toContain("1 个实例引用");
    expect(wrapper.find('[data-testid="strategy-delete-linked-instance-dsl-breakout-instance"]').exists()).toBe(true);
    expect(wrapper.get('[data-testid="strategy-delete-linked-instance-dsl-breakout-instance"]').text()).toContain("STOPPED · US.AAPL");
    expect(wrapper.find('[data-testid="confirm-delete-strategy-definition"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="jump-to-runtime-for-delete-linked-instances"]').exists()).toBe(true);

    await wrapper.get('[data-testid="jump-to-runtime-for-delete-linked-instances"]').trigger("click");
    await settleStrategyWorkspace();

    expect(wrapper.find('[data-testid="strategy-delete-definition-dialog"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-definitions-panel"]').exists()).toBe(false);
    expect(wrapper.text()).toContain("已切换到运行面板，请先删除策略「DSL Breakout」关联的 1 个实例");
    expect(wrapper.find('[data-testid="strategy-create-instance-panel"]').exists()).toBe(true);

    wrapper.unmount();
  });

  it("switches the workspace display modes", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [
          {
            id: "dsl-mean-revert",
            name: "DSL Mean Revert",
            version: "0.1.0",
            description: "dsl strategy",
            runtime: "dsl-go-plan",
            symbol: "00700",
            interval: "1m",
            script: buildDslScript("DSL Mean Revert"),
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
      }),
    );
    vi.stubGlobal(
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const { wrapper } = await mountStrategyPage("/strategy");

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
            id: "dsl-mean-revert",
            name: "DSL Mean Revert",
            version: "0.1.0",
            description: "dsl strategy",
            runtime: "dsl-go-plan",
            symbol: "00700",
            interval: "1m",
            script: buildDslScript("DSL Mean Revert"),
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
      }),
    );
    vi.stubGlobal(
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const { wrapper } = await mountStrategyPage("/strategy");

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
            id: "dsl-rsi-visual",
            name: "RSI Visual",
            version: "0.2.0",
            description: "visual strategy",
            runtime: "dsl-go-plan",
            symbol: "00700",
            interval: "1m",
            script: buildDslScript(
              "RSI Visual",
              [
                "let rsi_calc_node = rsi(14)",
                "if rsi_calc_node < 30:",
                '  notify "RSI changed"',
              ],
              { version: "0.2.0" },
            ),
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
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const { wrapper } = await mountStrategyPage("/strategy");

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
            id: "dsl-indicator-bindings",
            name: "DSL Indicator Bindings",
            version: "0.2.0",
            description: "indicator binding inspector",
            runtime: "dsl-go-plan",
            symbol: "00700",
            interval: "1m",
            script: buildDslScript("DSL Indicator Bindings", ['log "seed"'], { version: "0.2.0" }),
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
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const { wrapper } = await mountStrategyPage("/strategy");

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
            id: "dsl-place-order-policy",
            name: "DSL Place Order Policy",
            version: "0.2.0",
            description: "place order policy inspector",
            runtime: "dsl-go-plan",
            symbol: "00700",
            interval: "1m",
            script: buildDslScript("DSL Place Order Policy", ['log "seed"'], { version: "0.2.0" }),
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
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const { wrapper } = await mountStrategyPage("/strategy");

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
            id: "dsl-edge-disconnect",
            name: "DSL Edge Disconnect",
            version: "0.2.0",
            description: "edge disconnect inspector",
            runtime: "dsl-go-plan",
            symbol: "00700",
            interval: "1m",
            script: buildDslScript("DSL Edge Disconnect", ['log "seed"'], { version: "0.2.0" }),
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
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const { wrapper } = await mountStrategyPage("/strategy");

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

  it("auto syncs a saved logic flow model back into DSL", async () => {
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
            id: "dsl-logic-flow",
            name: "DSL Logic Flow",
            version: "0.2.0",
            description: "visual strategy",
            runtime: "dsl-go-plan",
            symbol: "00700",
            interval: "1m",
            script: buildDslScript("DSL Logic Flow", ['log "seed"'], { version: "0.2.0" }),
            visualModel,
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
      }),
    );
    vi.stubGlobal(
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const { wrapper } = await mountStrategyPage("/strategy");

    await openStrategyDesignWorkspace(wrapper);
    await showStrategyCodeEditor(wrapper, "split");

    wrapper.findComponent(StrategyLogicFlowDesigner).vm.$emit("select-node", "rsi-calc-node");
    await flushRequests();

    const scriptEditor = wrapper.get('[data-testid="strategy-script-editor"]')
      .element as HTMLTextAreaElement;
    expect(scriptEditor.value).toContain("strategy DSL Logic Flow");
    expect(scriptEditor.value).not.toContain("manualOnly");

    wrapper.findComponent(StrategyLogicFlowDesigner).vm.$emit("update:modelValue", visualModel);
    await flushRequests();

    expect(scriptEditor.value).toContain("on kline_close:");
    expect(scriptEditor.value).toContain('notify "收盘价触发视觉策略"');

    wrapper.unmount();
  });

  it("syncs handwritten DSL back into flow on blur", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [
          {
            id: "dsl-handwritten",
            name: "DSL Handwritten",
            version: "0.2.0",
            description: "code-first strategy",
            runtime: "dsl-go-plan",
            symbol: "00700",
            interval: "1m",
            script: buildDslScript("DSL Handwritten", ['notify "close seed"'], { version: "0.2.0" }),
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
      }),
    );
    vi.stubGlobal(
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const { wrapper } = await mountStrategyPage("/strategy");

    await openStrategyDesignWorkspace(wrapper);
    await showStrategyCodeEditor(wrapper, "split");

    const nextScript = [
      "strategy DSL Handwritten",
      "version 0.2.0",
      "on kline_close:",
      "  notify \"close signal\"",
      "  let rsi14 = rsi(14)",
      "  if rsi14 < 30:",
      "    buy shares 100 policy same_direction type MARKET",
    ].join("\n");

    await wrapper.get('[data-testid="strategy-script-editor"]').setValue(nextScript);
    await wrapper.get('[data-testid="strategy-script-editor"]').trigger("blur");
    await flushRequests();

    const visualModel = wrapper.findComponent(StrategyLogicFlowDesigner)
      .props("modelValue") as NonNullable<StrategyDefinitionDocument["visualModel"]>;

    expect(visualModel.nodes.some((node) => node.properties.blockKind === "notify")).toBe(true);
    expect(visualModel.nodes.some((node) => node.properties.blockKind === "getTechnicalIndicator")).toBe(true);
    expect(visualModel.nodes.some((node) => node.properties.blockKind === "technicalIndicatorCondition")).toBe(true);
    expect(visualModel.nodes.some((node) => node.properties.blockKind === "placeOrder")).toBe(true);
    expect(wrapper.get('[data-testid="strategy-visual-sync-status"]').text()).toContain("DSL 已同步");

    wrapper.unmount();
  });

  it("rewrites DSL when a visual block parameter changes", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [
          {
            id: "dsl-rsi-visual",
            name: "RSI Visual",
            version: "0.2.0",
            description: "visual strategy",
            runtime: "dsl-go-plan",
            symbol: "00700",
            interval: "1m",
            script: buildDslScript(
              "RSI Visual",
              [
                "let rsi_calc_node = rsi(14)",
                "if rsi_calc_node < 30:",
                '  notify "RSI changed"',
              ],
              { version: "0.2.0" },
            ),
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
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const { wrapper } = await mountStrategyPage("/strategy");

    await openStrategyDesignWorkspace(wrapper);
    await showStrategyCodeEditor(wrapper, "split");

    const designer = wrapper.findComponent(StrategyLogicFlowDesigner);
    const scriptEditor = wrapper.get('[data-testid="strategy-script-editor"]')
      .element as HTMLTextAreaElement;
    expect(scriptEditor.value).toContain("let rsi_calc_node = rsi(14)");

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

    expect(scriptEditor.value).toContain("let rsi_calc_node = rsi(21)");

    wrapper.unmount();
  });

  it("rewrites risk block mode and window policy from the block inspector", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [
          {
            id: "dsl-risk-inspector",
            name: "Risk Inspector",
            version: "0.2.0",
            description: "risk block inspector",
            runtime: "dsl-go-plan",
            symbol: "US.AAPL",
            interval: "5m",
            script: buildDslScript("Risk Inspector", ['log "seed"'], { version: "0.2.0", symbol: "US.AAPL", interval: "5m" }),
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
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const { wrapper } = await mountStrategyPage("/strategy");

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
    expect(scriptEditor.value).toContain("protect auto trailingStop 1 day 2 window session");

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
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const { wrapper } = await mountStrategyPage("/strategy");

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
    expect(scriptEditor.value).toContain("let dma_fast_ma = ma(MA, 5, day)");
    expect(scriptEditor.value).toContain("let dma_slow_ma = ma(MA, 20, day)");
    expect(scriptEditor.value).toContain("if cross_over(dma_fast_ma, dma_slow_ma):");
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
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const { wrapper } = await mountStrategyPage("/strategy");

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
      .get('[data-testid="strategy-basic-info-section"] input[placeholder="例如：双均线观察策略"]')
      .setValue("");
    await wrapper.get('[data-testid="save-strategy-definition"]').trigger("click");
    await flushRequests();

    expect(wrapper.text()).toContain("策略名称不能为空。");

    await wrapper.get('[data-testid="dismiss-strategy-error-banner"]').trigger("click");
    await flushRequests();

    expect(wrapper.text()).not.toContain("策略名称不能为空。");
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
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );
    vi.stubGlobal("confirm", confirmMock);

    const { wrapper } = await mountStrategyPage("/strategy");

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
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );
    vi.stubGlobal("confirm", confirmMock);

    const { router, wrapper } = await mountStrategyPage("/strategy");

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

  it("prompts to save only or update linked instances when saving a definition", async () => {
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
              script: buildDslScript("DSL Breakout"),
            },
            status: "STOPPED",
            createdAt: "2026-05-23T00:01:00.000Z",
            logs: [],
          },
        ],
      }),
    );
    vi.stubGlobal(
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const { wrapper } = await mountStrategyPage("/strategy");
    await openStrategyDesignWorkspace(wrapper);
    await wrapper.get('[data-testid="toggle-strategy-basic-info-section"]').trigger("click");
    await settleStrategyWorkspace();

    const description = wrapper.get('[data-testid="strategy-basic-info-section"] textarea');
    await description.setValue("save and apply latest instance snapshot");
    await wrapper.get('[data-testid="save-strategy-definition"]').trigger("click");
    await settleStrategyWorkspace();

    expect(wrapper.find('[data-testid="strategy-save-linked-dialog"]').exists()).toBe(true);
    expect(wrapper.get('[data-testid="strategy-save-linked-summary"]').text()).toContain("当前共有 1 个实例应用了这份策略");
    expect(wrapper.find('[data-testid="strategy-save-definition-only"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-save-definition-and-apply"]').exists()).toBe(true);

    await wrapper.get('[data-testid="strategy-save-definition-and-apply"]').trigger("click");
    await settleStrategyWorkspace();

    expect(wrapper.text()).toContain("策略已保存。 已同步 1 个关联实例");

    wrapper.unmount();
  });

  it("shows only templates when starting a new strategy and hides them after selection", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [
          {
            id: "dsl-existing",
            name: "Existing Strategy",
            version: "0.1.0",
            description: "existing dsl strategy",
            runtime: "dsl-go-plan",
            symbol: "00700",
            interval: "1m",
            script: buildDslScript("Existing Strategy"),
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
      }),
    );
    vi.stubGlobal(
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const { wrapper } = await mountStrategyPage("/strategy");

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
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const { wrapper } = await mountStrategyPage("/strategy");

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
    expect(scriptEditor.value).toContain("let rsi_getter = rsi(14)");
    expect(scriptEditor.value).toContain("if rsi_getter < 30:");
    expect(scriptEditor.value).toContain("if rsi_getter > 70:");

    wrapper.unmount();
  });

});
