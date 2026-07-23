// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import RankListPanel from "../../src/components/research/RankListPanel.vue";

const entries = [
  { instrumentId: "US.AAPL", market: "US", symbol: "AAPL", name: "Apple", productClass: "equity", changeRate: 1.5, lastPrice: 201.12 },
  { instrumentId: "US.TSLA", market: "US", symbol: "TSLA", name: "Tesla", productClass: "equity", changeRate: -2.3, lastPrice: 180.5 },
  { instrumentId: "HK.00700", market: "HK", symbol: "00700", name: "腾讯", productClass: "equity", changeRate: 3.1, lastPrice: 500 },
];

function mountPanel(props: Record<string, unknown> = {}) {
  return mount(RankListPanel, {
    props: { title: "涨幅榜", entries, ...props },
  });
}

describe("RankListPanel", () => {
  it("renders title, header labels and one row per entry", () => {
    const wrapper = mountPanel();
    expect(wrapper.text()).toContain("涨幅榜");
    expect(wrapper.text()).toContain("代码");
    expect(wrapper.text()).toContain("名称");
    expect(wrapper.text()).toContain("涨跌幅");
    expect(wrapper.text()).toContain("最新价");
    expect(wrapper.findAll("tbody tr")).toHaveLength(3);
    expect(wrapper.text()).toContain("00700");
  });

  it("sorts by value field descending by default and toggles on header click", async () => {
    const wrapper = mountPanel();
    const firstCode = () =>
      wrapper.get("tbody tr td").text();

    expect(firstCode()).toBe("00700"); // 3.1 最高
    const sortableHeader = wrapper.get("th.rank-list-panel__sortable");
    await sortableHeader.trigger("click");
    expect(firstCode()).toBe("TSLA"); // -2.3 最低
    await sortableHeader.trigger("click");
    expect(firstCode()).toBe("00700");
  });

  it("puts the largest decline first when configured for a losers list", () => {
    const wrapper = mountPanel({
      title: "领跌榜",
      defaultSortOrder: "asc",
      entries: [
        { instrumentId: "US.A", symbol: "A", name: "甲", changeRate: -3.2 },
        { instrumentId: "US.B", symbol: "B", name: "乙", changeRate: -12.6 },
        { instrumentId: "US.C", symbol: "C", name: "丙", changeRate: -7.4 },
      ],
    });
    expect(
      wrapper.findAll("tbody tr").map((row) => row.get("td").text()),
    ).toEqual(["B", "C", "A"]);
    expect(wrapper.get("th.rank-list-panel__sortable").attributes("aria-sort")).toBe(
      "ascending",
    );
  });

  it("supports a custom value field and label", () => {
    const wrapper = mountPanel({
      valueField: "dividendYieldTTM",
      valueLabel: "股息率",
      entries: [
        { instrumentId: "SH.A", symbol: "A", name: "甲", dividendYieldTTM: 5.2 },
        { instrumentId: "SH.B", symbol: "B", name: "乙", dividendYieldTTM: 2.1 },
      ],
    });
    expect(wrapper.text()).toContain("股息率");
    expect(wrapper.get("tbody tr td").text()).toBe("A");
  });

  it("colors values with tv-up / tv-down classes", () => {
    const wrapper = mountPanel();
    const values = wrapper.findAll("td.rank-list-panel__value");
    expect(values[0]!.classes()).toContain("tv-up");
    expect(wrapper.text()).toContain("+3.10%");
    expect(wrapper.text()).toContain("-2.30%");
    expect(values.some((cell) => cell.classes().includes("tv-down"))).toBe(true);
  });

  it("emits select with the clicked entry", async () => {
    const wrapper = mountPanel();
    await wrapper.findAll("tbody tr")[1]!.trigger("click");
    const events = wrapper.emitted("select");
    expect(events).toHaveLength(1);
    expect(events![0]![0]).toEqual(entries[0]); // 第二行是 AAPL（降序）
  });

  it("shows loading and empty states", () => {
    const loading = mountPanel({ loading: true });
    expect(loading.text()).toContain("加载中");
    const empty = mountPanel({ entries: [] });
    expect(empty.text()).toContain("暂无数据");
  });

  it("handles numeric identities, string values, missing values, and flat metrics", () => {
    const wrapper = mountPanel({
      valueField: "turnover",
      valueLabel: "成交额",
      entries: [
        { symbol: 700, name: 42, turnover: "12.5", price: "3.5" },
        { instrumentId: "US.NULL", turnover: null },
        { name: "右侧缺失", turnover: 5 },
        { name: "双侧缺失" },
      ],
    });

    const rows = wrapper.findAll("tbody tr");
    expect(rows[0]!.text()).toContain("700");
    expect(rows[0]!.text()).toContain("+12.50");
    expect(rows.some((row) => row.text().includes("US.NULL"))).toBe(true);
    expect(rows.some((row) => row.text().includes("--"))).toBe(true);
    expect(wrapper.text()).toContain("42");
  });

  it("updates the configured default order after mount", async () => {
    const wrapper = mountPanel({ defaultSortOrder: "desc" });
    await wrapper.setProps({ defaultSortOrder: "asc" });
    expect(
      wrapper.get("th.rank-list-panel__sortable").attributes("aria-sort"),
    ).toBe("ascending");
  });
});
