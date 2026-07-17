// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import ADKToolVisualization from "../src/components/shared/ADKToolVisualization.vue";

describe("ADKToolVisualization boundary states", () => {
  it("keeps optional content absent without hiding an otherwise valid tool result", () => {
    const summary = mount(ADKToolVisualization, {
      props: {
        visualization: {
          kind: "summary",
          title: "空摘要",
          cards: [{ label: "检查", value: "完成" }],
          rows: [],
        },
      },
    });
    expect(summary.text()).toContain("空摘要");
    expect(summary.find(".adk-tool-visual__subtitle").exists()).toBe(false);
    expect(summary.find(".adk-tool-visual__rows").exists()).toBe(false);
    expect(summary.find(".adk-tool-visual-card").classes()).toContain("is-muted");

    const table = mount(ADKToolVisualization, {
      props: {
        visualization: {
          kind: "table",
          title: "空表格",
          columns: [
            { key: "symbol", label: "标的" },
            { key: "status", label: "状态" },
          ],
          rows: [{ symbol: "US.AAPL" }],
        },
      },
    });
    expect(table.findAll("tbody tr")).toHaveLength(1);
    expect(table.text()).toContain("标的");
    expect(table.text()).toContain("US.AAPL");
    expect(table.text()).toContain("-");

    const depth = mount(ADKToolVisualization, {
      props: {
        visualization: {
          kind: "depth",
          title: "空盘口",
          bids: [{ price: "100", quantity: "5", percent: 40 }],
          asks: [{ price: "101", quantity: "8", percent: 80 }],
        },
      },
    });
    expect(depth.text()).toContain("Bids");
    expect(depth.findAll(".adk-tool-visual-depth-row")).toHaveLength(2);
    expect(depth.find(".is-bid i").attributes("style")).toContain("40%");
    expect(depth.find(".is-ask i").attributes("style")).toContain("80%");

    const timeline = mount(ADKToolVisualization, {
      props: {
        visualization: {
          kind: "timeline",
          title: "简要时间线",
          events: [
            { label: "开始" },
            { label: "完成", time: "10:00", detail: "已同步", tone: "ok" },
          ],
        },
      },
    });
    expect(timeline.findAll(".adk-tool-visual-timeline__item")).toHaveLength(2);
    expect(timeline.find(".adk-tool-visual-timeline__item").classes()).toContain("is-muted");
    expect(timeline.find(".adk-tool-visual-timeline__item.is-ok").exists()).toBe(true);
    expect(timeline.find("small").text()).toBe("10:00");
    expect(timeline.find(".adk-tool-visual-timeline__main span").text()).toBe("已同步");
  });
});
