// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import PineSourceStructureBlockList from "../src/components/PineSourceStructureBlockList.vue";

function buildNode(overrides: Record<string, unknown>) {
  return {
    id: "node",
    kind: "raw",
    label: "Raw",
    raw: "",
    detail: "",
    depth: 0,
    lineRange: { start: 1, end: 1 },
    sourceRange: { start: 0, end: 0 },
    match: { type: "raw" },
    ...overrides,
  } as any;
}

describe("PineSourceStructureBlockList", () => {
  it("renders descriptions and emits block operations for editable and raw nodes", async () => {
    const strategyNode = buildNode({
      id: "strategy",
      kind: "strategy",
      label: "策略声明",
      raw: 'strategy("Demo", overlay=true)',
      lineRange: { start: 2, end: 2 },
      match: {
        type: "strategy",
        declaration: {
          title: "",
          initialCapital: 100000,
          pyramiding: 2,
        },
      },
    });
    const inputNode = buildNode({
      id: "input",
      kind: "input",
      label: "输入参数 len",
      raw: 'len = input.int(14, "Length")',
      depth: 1,
      lineRange: { start: 3, end: 3 },
      match: {
        type: "input",
        input: {
          name: "len",
          type: "int",
          title: "",
          defaultValue: 14,
        },
      },
    });
    const rawNode = buildNode({
      id: "library",
      kind: "library",
      label: "导入库 ta7",
      raw: "import TradingView/ta/7 as ta7",
      detail: "TradingView/ta/7",
      lineRange: { start: 4, end: 4 },
      match: { type: "raw" },
    });

    const wrapper = mount(PineSourceStructureBlockList, {
      props: {
        nodes: [strategyNode, inputNode, rawNode],
        selectedId: "library",
        expandedId: "input",
      },
    });

    expect(wrapper.text()).toContain("声明策略 Pine v6 策略");
    expect(wrapper.text()).toContain("初始资金 100000");
    expect(wrapper.text()).toContain("允许加仓 2 次");
    expect(wrapper.text()).toContain("输入参数 len 使用 int 类型，默认 14，标题 len");
    expect(wrapper.text()).toContain("导入库 ta7：TradingView/ta/7");
    expect(wrapper.find("[data-testid='pine-source-node-library']").classes()).toContain(
      "is-selected",
    );

    await wrapper
      .find("[data-testid='pine-source-node-library'] > button")
      .trigger("click");
    expect(wrapper.emitted("toggle-block")?.[0]).toEqual([rawNode]);

    const kindSelect = wrapper.find(".pine-block__kind");
    await kindSelect.setValue("strategy_entry");
    expect(wrapper.emitted("change-kind")?.[0]).toEqual([
      inputNode,
      "strategy_entry",
    ]);

    const moveButtons = wrapper.findAll(".pine-block__actions button");
    await moveButtons[0]!.trigger("click");
    await moveButtons[1]!.trigger("click");
    await moveButtons[2]!.trigger("click");
    await moveButtons[3]!.trigger("click");
    expect(wrapper.emitted("move-block")?.[0]).toEqual([inputNode, -1]);
    expect(wrapper.emitted("move-block")?.[1]).toEqual([inputNode, 1]);
    expect(wrapper.emitted("duplicate-block")?.[0]).toEqual([inputNode]);
    expect(wrapper.emitted("delete-block")?.[0]).toEqual([inputNode]);

    const editableInputs = wrapper.findAll(".pine-block__params input");
    const fieldInput = editableInputs[editableInputs.length - 1];
    expect(fieldInput).toBeDefined();
    await fieldInput!.setValue("21");
    expect(wrapper.emitted("update-field")?.[0]).toEqual([
      inputNode,
      "defaultValue",
      "21",
    ]);

    const addBlockSelect = wrapper.find(".pine-block-list__add select");
    await addBlockSelect.setValue("strategy_close_all");
    expect(wrapper.emitted("add-block")?.[0]).toEqual(["strategy_close_all"]);
  });

  it("describes instruction nodes with numeric and boolean params", () => {
    const instructionNode = buildNode({
      id: "log",
      kind: "instruction",
      label: "记录日志",
      raw: 'log.info("ready")',
      lineRange: { start: 8, end: 8 },
      match: {
        type: "instruction",
        block: {
          kind: "strategy_close_all",
          title: "",
          params: {
            immediately: true,
          },
        },
      },
    });
    const fallbackNode = buildNode({
      id: "fallback",
      kind: "instruction",
      label: "自定义块",
      raw: "custom.block()",
      lineRange: { start: 9, end: 9 },
      match: {
        type: "instruction",
        block: {
          kind: "custom_block",
          title: "自定义块标题",
          params: {
            retries: 3,
          },
        },
      },
    });

    const wrapper = mount(PineSourceStructureBlockList, {
      props: {
        nodes: [instructionNode, fallbackNode],
        selectedId: "",
        expandedId: null,
      },
    });

    expect(wrapper.text()).toContain("全部平仓，立即执行 true");
    expect(wrapper.text()).toContain("自定义块标题");
  });

  it("describes executable source blocks across series, orders, cancellation, and risk", () => {
    const instructions = [
      ["series", "series_assign", { name: "fast", expression: "ta.ema(close, 8)" }, "fast 设为 ta.ema(close, 8)"],
      ["state", "var_state", { name: "armed", initial: false }, "持久变量 armed 初始为 false"],
      ["if", "if", { condition: "close > fast" }, "当 close > fast 成立时执行分支"],
      ["security", "request_security", { symbol: "NASDAQ:AAPL", timeframe: "60", expression: "close" }, "读取 NASDAQ:AAPL 60 的 close"],
      ["array", "array_op", { name: "prices", mode: "push" }, "prices 执行 push"],
      ["entry", "strategy_entry", { direction: "strategy.long", id: "L" }, "按 strategy.long 开仓，订单 L"],
      ["order", "strategy_order", { direction: "strategy.short", id: "S" }, "提交 strategy.short 订单 S"],
      ["exit", "strategy_exit", { from_entry: "L", id: "XL" }, "从 L 退出，退出单 XL"],
      ["close", "strategy_close", { id: "L", when: "close < fast" }, "平仓 L，条件 close < fast"],
      ["cancel", "strategy_cancel", { id: "L" }, "撤销订单 L"],
      ["cancel-all", "strategy_cancel_all", {}, "撤销全部未成交订单"],
      ["allow-entry", "strategy_risk_allow_entry_in", { direction: "strategy.direction.long" }, "允许入场方向 strategy.direction.long"],
      ["drawdown", "strategy_risk_max_drawdown", { value: 12, type: "strategy.percent_of_equity" }, "最大回撤 12，类型 strategy.percent_of_equity"],
      ["intraday-loss", "strategy_risk_max_intraday_loss", { value: 5, type: "strategy.cash" }, "日内最大亏损 5，类型 strategy.cash"],
      ["filled-orders", "strategy_risk_max_intraday_filled_orders", { count: 4 }, "日内最多成交 4 笔"],
      ["risk", "strategy_risk_max_position_size", { contracts: 3 }, "最大持仓 3"],
      ["loss-days", "strategy_risk_max_cons_loss_days", { count: 2 }, "连续亏损 2 天后限制交易"],
      ["alert", "alertcondition", { condition: "close > fast" }, "当 close > fast 成立时触发提醒"],
      ["log", "log", { message: "risk guard armed" }, "记录日志：risk guard armed"],
    ] as const;
    const nodes = instructions.map(([id, kind, params], index) =>
      buildNode({
        id,
        kind: "instruction",
        label: id,
        lineRange: { start: index + 10, end: index + 10 },
        match: { type: "instruction", block: { kind, title: "", params } },
      }),
    );
    const wrapper = mount(PineSourceStructureBlockList, {
      props: { nodes, selectedId: "", expandedId: null },
    });

    for (const [, , , description] of instructions) {
      expect(wrapper.text()).toContain(description);
    }
  });
});
