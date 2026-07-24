// @vitest-environment jsdom

import { flushPromises, mount } from "@vue/test-utils";
import { ref } from "vue";
import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ useResearchFeature: vi.fn() }));

vi.mock("../../src/composables/useResearchFeature", () => ({
  useResearchFeature: mocks.useResearchFeature,
}));

import MacroResearchView from "../../src/components/research/MacroResearchView.vue";

function state(entries: Record<string, unknown>[] = []) {
  return {
    entries: ref(entries),
    metadata: ref<Record<string, unknown>>({}),
    loading: ref(false),
    error: ref(""),
    asOf: ref(""),
    refresh: vi.fn(),
  };
}

function mountMacro(
  states: [ReturnType<typeof state>, ReturnType<typeof state>, ReturnType<typeof state>],
  props: Record<string, unknown> = {},
) {
  for (const value of states) {
    mocks.useResearchFeature.mockImplementationOnce((source: unknown) => {
      if (typeof source === "function") source();
      return value;
    });
  }
  return mount(MacroResearchView, {
    props,
    global: { stubs: { SparklineChart: { props: ["points"], template: "<div class='sparkline-stub'>{{ points.length }}</div>" } } },
  });
}

beforeEach(() => mocks.useResearchFeature.mockReset());

describe("macro research protocol views", () => {
  it("normalizes grouped and flat indicators with unit-aware history", async () => {
    const list = state([
      {
        categoryName: "就业",
        indicatorList: [
          { indicatorId: 101, name: "失业率" },
          { id: "", name: "无标识" },
          null,
        ],
      },
      { indicatorId: "CPI", indicatorName: "消费者价格", categoryName: "通胀" },
      { id: "GDP", name: "" },
      { name: "无指标" },
    ]);
    const history = state([
      { dataTime: "2026-07", releaseTime: "08:30", value: 4.5, predictValue: 4.4, previousValue: 4.3, unitType: 1 },
      { date: "2026-06", value: 102.3, forecastValue: 102, previousValue: null, unitType: 3 },
      { eventDate: "2026-05", value: 10, unit: "亿美元" },
      { dataTime: "2026-04", value: null },
    ]);
    history.asOf.value = "2026-07-24";
    const fed = state();
    const wrapper = mountMacro([list, history, fed], { operation: "indicators" });
    await flushPromises();

    expect(wrapper.text()).toContain("失业率");
    expect(wrapper.text()).toContain("消费者价格");
    expect(wrapper.text()).toContain("GDP");
    expect(wrapper.text()).toContain("4.5%");
    expect(wrapper.text()).toContain("102.3（指数）");
    expect(wrapper.text()).toContain("10亿美元");
    expect(wrapper.get(".sparkline-stub").text()).toBe("3");
    expect(wrapper.emitted("update:indicatorId")?.[0]).toEqual(["101"]);

    const buttons = wrapper.findAll(".macro-research__catalog nav button");
    await buttons[1]!.trigger("click");
    expect(wrapper.emitted("update:indicatorId")?.at(-1)).toEqual(["CPI"]);
    await wrapper.setProps({ indicatorId: "GDP" });
    await flushPromises();
    expect(buttons[2]!.classes()).toContain("is-active");
    await wrapper.get(".macro-research__detail-head button").trigger("click");
    expect(history.refresh).toHaveBeenCalledOnce();
  });

  it("surfaces list/history status and repairs a stale indicator selection", async () => {
    const list = state([{ categoryName: "其他", indicators: [{ id: "ONLY" }] }]);
    const history = state();
    const fed = state();
    list.loading.value = true;
    const wrapper = mountMacro([list, history, fed], { indicatorId: "STALE" });
    expect(wrapper.text()).toContain("加载中");

    list.loading.value = false;
    list.error.value = "指标目录失败";
    await flushPromises();
    expect(wrapper.text()).toContain("指标目录失败");
    list.error.value = "";
    history.loading.value = true;
    await flushPromises();
    expect(wrapper.text()).toContain("加载中");
    history.loading.value = false;
    history.error.value = "历史数据失败";
    await flushPromises();
    expect(wrapper.text()).toContain("历史数据失败");
    history.error.value = "";
    await flushPromises();
    expect(wrapper.emitted("update:indicatorId")?.at(-1)).toEqual(["ONLY"]);
  });

  it("flattens FedWatch meetings and clamps visual probabilities", async () => {
    const fed = state([
      {
        meetingDate: "2026-09-16",
        targetRateList: [
          { targetRange: "4.00-4.25", probability: -10 },
          { targetRange: "4.25-4.50", probability: 120 },
        ],
      },
      { date: "2026-11-04", rates: [{ rateRange: "3.75-4.00", probability: null }] },
      { date: "invalid", rates: "bad" },
    ]);
    fed.asOf.value = "2026-07-24";
    const wrapper = mountMacro([state(), state(), fed], { operation: "fed_target_rate" });

    expect(wrapper.text()).toContain("FedWatch 目标利率概率");
    expect(wrapper.findAll(".macro-research__probability-row")).toHaveLength(3);
    const bars = wrapper.findAll(".macro-research__probability-row i");
    expect(bars[0]!.attributes("style")).toContain("0%");
    expect(bars[1]!.attributes("style")).toContain("100%");
    expect(wrapper.text()).toContain("3.75-4.00");
    await wrapper.get(".macro-research__toolbar button").trigger("click");
    expect(fed.refresh).toHaveBeenCalledOnce();

    fed.loading.value = true;
    await flushPromises();
    expect(wrapper.text()).toContain("加载中");
    fed.loading.value = false;
    fed.error.value = "利率概率失败";
    await flushPromises();
    expect(wrapper.text()).toContain("利率概率失败");
  });

  it("renders dot-plot year fallbacks and non-negative integral votes", async () => {
    const fed = state([
      {
        year: "2027",
        medianRate: 3.75,
        dotList: [
          { rate: 3.5, voteCount: 2.9 },
          { rate: 3.75, voteCount: -2 },
        ],
      },
      { year: 2028, dots: [{ rate: 3.25, voteCount: null }, "bad"] },
      { dots: "invalid" },
    ]);
    fed.metadata.value = { currentRate: "4.25-4.50%" };
    const wrapper = mountMacro([state(), state(), fed], { operation: "fed_dot_plot" });

    expect(wrapper.text()).toContain("美联储点阵图");
    expect(wrapper.text()).toContain("当前利率 4.25-4.50%");
    expect(wrapper.text()).toContain("2027");
    expect(wrapper.text()).toContain("2028");
    expect(wrapper.text()).toContain("--");
    expect(wrapper.findAll(".macro-research__dot-row i")).toHaveLength(2);
    await wrapper.get(".macro-research__toolbar button").trigger("click");
    expect(fed.refresh).toHaveBeenCalledOnce();
  });
});
