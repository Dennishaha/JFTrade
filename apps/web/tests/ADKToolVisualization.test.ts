// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import ADKToolVisualization from "../src/components/shared/ADKToolVisualization.vue";

describe("ADKToolVisualization", () => {
  it("renders summary cards, subtitles, and detail rows", () => {
    const wrapper = mount(ADKToolVisualization, {
      props: {
        visualization: {
          kind: "summary",
          title: "组合摘要",
          subtitle: "最近一次检查",
          cards: [
            { label: "状态", value: "已连接", tone: "ok" },
            { label: "账户数", value: "2" },
          ],
          rows: [{ label: "检查时间", value: "2026-07-03T12:00:00Z" }],
        },
      },
    });

    expect(wrapper.text()).toContain("组合摘要");
    expect(wrapper.text()).toContain("最近一次检查");
    expect(wrapper.find(".adk-tool-visual-card.is-ok").exists()).toBe(true);
    expect(wrapper.find(".adk-tool-visual-card.is-muted").exists()).toBe(true);
    expect(wrapper.text()).toContain("检查时间");
  });

  it("renders table cells and uses a dash for missing values", () => {
    const wrapper = mount(ADKToolVisualization, {
      props: {
        visualization: {
          kind: "table",
          title: "订单列表",
          columns: [
            { key: "symbol", label: "标的" },
            { key: "status", label: "状态" },
          ],
          rows: [{ symbol: "US.AAPL" }],
        },
      },
    });

    expect(wrapper.text()).toContain("订单列表");
    expect(wrapper.text()).toContain("US.AAPL");
    expect(wrapper.text()).toContain("-");
  });

  it("renders depth ladders for both bid and ask sides", () => {
    const wrapper = mount(ADKToolVisualization, {
      props: {
        visualization: {
          kind: "depth",
          title: "盘口深度",
          bids: [{ price: "100", quantity: "5", percent: 40 }],
          asks: [{ price: "101", quantity: "8", percent: 80 }],
        },
      },
    });

    expect(wrapper.text()).toContain("Bids");
    expect(wrapper.text()).toContain("Asks");
    expect(wrapper.find(".adk-tool-visual-depth-row.is-bid i").attributes("style")).toContain("40%");
    expect(wrapper.find(".adk-tool-visual-depth-row.is-ask i").attributes("style")).toContain("80%");
  });

  it("renders timeline events with tone, time, and optional detail", () => {
    const wrapper = mount(ADKToolVisualization, {
      props: {
        visualization: {
          kind: "timeline",
          title: "执行时间线",
          events: [
            { label: "已提交", time: "10:00", tone: "ok" },
            { label: "失败", detail: "已拒绝", tone: "danger" },
          ],
        },
      },
    });

    expect(wrapper.text()).toContain("执行时间线");
    expect(wrapper.findAll(".adk-tool-visual-timeline__item")).toHaveLength(2);
    expect(wrapper.find(".adk-tool-visual-timeline__item.is-ok").exists()).toBe(true);
    expect(wrapper.find(".adk-tool-visual-timeline__item.is-danger").exists()).toBe(true);
    expect(wrapper.text()).toContain("10:00");
    expect(wrapper.text()).toContain("已拒绝");
  });
});
