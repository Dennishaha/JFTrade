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
});
