// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import ResearchDataTable from "../../src/components/research/ResearchDataTable.vue";
import { formatResearchCell } from "../../src/components/research/researchTable";

describe("research data table", () => {
  it("formats wire values without exposing unsupported objects", () => {
    expect(formatResearchCell(null)).toBe("--");
    expect(formatResearchCell("")).toBe("--");
    expect(formatResearchCell(1234.56789)).toBe("1,234.5679");
    expect(formatResearchCell(true)).toBe("是");
    expect(formatResearchCell(false)).toBe("否");
    expect(formatResearchCell("OPEN")).toBe("OPEN");
    expect(formatResearchCell({ raw: true })).toBe("--");
  });

  it("renders custom cells and emits mouse and keyboard row actions", async () => {
    const entries = [
      { id: "a", price: 12.5, status: "up" },
      { id: "b", price: null, status: "flat" },
    ];
    const wrapper = mount(ResearchDataTable, {
      props: {
        entries,
        columns: [
          {
            key: "price",
            label: "价格",
            align: "right",
            width: "100px",
            value: (entry) => entry.price,
            format: (value) => value == null ? "N/A" : `$${String(value)}`,
            className: (_value, entry) => entry.status === "up" ? "tv-up" : undefined,
          },
        ],
        rowKey: (entry) => String(entry.id),
        selectedKey: "a",
        compact: true,
      },
    });

    expect(wrapper.classes()).toContain("is-compact");
    expect(wrapper.get("th").classes()).toContain("is-right");
    expect(wrapper.get("th").attributes("style")).toContain("100px");
    const rows = wrapper.findAll("tbody tr");
    expect(rows[0]!.classes()).toContain("is-selected");
    expect(rows[0]!.get("td").classes()).toContain("tv-up");
    expect(rows[0]!.get("td").attributes("title")).toBe("$12.5");
    expect(rows[1]!.text()).toBe("N/A");

    await rows[0]!.trigger("click");
    await rows[0]!.trigger("dblclick");
    await rows[1]!.trigger("keydown.enter");
    expect(wrapper.emitted("select")).toEqual([[entries[0]], [entries[1]]]);
    expect(wrapper.emitted("open")).toEqual([[entries[0]]]);
  });

  it("uses the row index by default and renders an empty label", async () => {
    const wrapper = mount(ResearchDataTable, {
      props: {
        entries: [{ enabled: true }],
        columns: [{ key: "enabled", label: "启用", value: (entry) => entry.enabled }],
      },
    });
    expect(wrapper.get("tbody tr").text()).toBe("是");

    await wrapper.setProps({ entries: [], emptyLabel: "没有研究数据" });
    expect(wrapper.text()).toContain("没有研究数据");
  });
});
