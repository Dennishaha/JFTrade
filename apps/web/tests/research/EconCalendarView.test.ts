// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  fetch: vi.fn(),
}));

vi.mock("../../src/composables/productFeatures", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("../../src/composables/productFeatures")>();
  return { ...actual, fetchProductFeature: mocks.fetch };
});

import EconCalendarView from "../../src/components/research/EconCalendarView.vue";
import { dayKeyOf } from "../../src/components/research/researchEntry";
import { flushPromises } from "../productTestUtils";

function featureResult(entries: Record<string, unknown>[]) {
  return {
    provider: {
      brokerId: "futu",
      featureId: "research.test",
      capability: "available" as const,
      selectionReason: "explicit",
      resolvedAt: "2026-07-17T00:00:00Z",
      asOf: "2026-07-17T00:00:00Z",
    },
    asOf: "2026-07-17T00:00:00Z",
    entries,
  };
}

function dayKeyOffset(days: number): string {
  const date = new Date();
  date.setDate(date.getDate() + days);
  return dayKeyOf(date);
}

const entries = [
  {
    title: "CPI 月率",
    region: "美国",
    eventDate: dayKeyOffset(0),
    eventTimestamp: Date.parse(`${dayKeyOffset(0)}T00:30:00Z`) / 1_000,
    eventTime: `${dayKeyOffset(0)}T00:30:00Z`,
    previousValue: "0.2%",
    forecastValue: "0.3%",
    actualValue: "0.4%",
    importance: 3,
  },
  {
    title: "美联储讲话",
    region: "美国",
    eventDate: dayKeyOffset(1),
    eventTime: "22:00",
    importance: 2,
  },
  {
    title: "某新股上市",
    region: "中国",
    eventDate: dayKeyOffset(2),
  },
];

afterEach(() => {
  mocks.fetch.mockReset();
});

async function mountView() {
  mocks.fetch.mockResolvedValue(featureResult(entries));
  const wrapper = mount(EconCalendarView);
  await flushPromises();
  return wrapper;
}

describe("EconCalendarView", () => {
  it("groups items by date with values, stars and region tags", async () => {
    const wrapper = await mountView();
    expect(wrapper.findAll(".econ-calendar-view__group")).toHaveLength(3);
    expect(wrapper.text()).toContain("共 3 条");
    expect(wrapper.text()).toContain("前值 0.2% · 预测 0.3% · 公布 0.4%");
    expect(wrapper.text()).toContain("★★★");
    expect(wrapper.text()).toContain("美国");
    const firstHeadline = wrapper.findAll(".econ-calendar-view__headline")[0]!;
    expect(firstHeadline.get(".econ-calendar-view__title").text()).toBe(
      "CPI 月率",
    );
    expect(firstHeadline.get(".econ-calendar-view__stars").text()).toBe("★★★");
    expect(firstHeadline.get(".econ-calendar-view__region-tag").text()).toBe(
      "美国",
    );
    // 缺值字段显示 --
    expect(wrapper.text()).toContain("前值 -- · 预测 -- · 公布 --");
    // 地区下拉去重
    const options = wrapper
      .get("select.econ-calendar-view__region")
      .findAll("option")
      .map((option) => option.text());
    expect(options).toEqual(["全部地区", "美国", "中国"]);
  });

  it("does not invent event-type filters and renders timestamps in Asia/Shanghai", async () => {
    const wrapper = await mountView();
    expect(
      wrapper.findAll(".tv-seg button").map((button) => button.text()),
    ).toEqual(["今天往后", "本周", "本月"]);
    expect(wrapper.findAll(".econ-calendar-view__time")[0]!.text()).toBe("08:30");
  });

  it("filters by region dropdown", async () => {
    const wrapper = await mountView();
    await wrapper.get("select.econ-calendar-view__region").setValue("中国");
    expect(wrapper.text()).toContain("共 1 条");
    expect(wrapper.text()).toContain("某新股上市");
    expect(wrapper.text()).not.toContain("CPI 月率");
  });

  it("switches week and month bounds and refreshes the query", async () => {
    const wrapper = await mountView();
    const buttons = wrapper.findAll(".tv-seg button");
    await buttons.find((button) => button.text() === "本周")!.trigger("click");
    await flushPromises();
    await buttons.find((button) => button.text() === "本月")!.trigger("click");
    await flushPromises();
    expect(mocks.fetch.mock.calls.some(([path]) => String(path).includes("beginDate="))).toBe(true);
    expect(mocks.fetch).toHaveBeenCalledTimes(3);
  });

  it("normalizes alternate times and ignores invalid or duplicate rows", async () => {
    const today = dayKeyOffset(0);
    mocks.fetch.mockResolvedValue(
      featureResult([
        {
          eventId: "one",
          eventTime: `${today}T09:15:00+08:00`,
          title: "",
          region: "",
          importance: -2,
        },
        {
          eventId: "one",
          eventDate: today,
          title: "重复",
        },
        {
          eventDate: today,
          eventTime: "09:30 later",
          title: "原始时间",
          importance: 9,
        },
        {
          eventDate: dayKeyOffset(-2),
          title: "已过期",
        },
        {
          eventTime: "invalid",
          title: "无日期",
        },
      ]),
    );
    const wrapper = mount(EconCalendarView);
    await flushPromises();
    expect(wrapper.text()).toContain("共 2 条");
    expect(wrapper.text()).toContain("--");
    expect(wrapper.text()).toContain("09:30");
    expect(wrapper.text()).toContain("★");
    expect(wrapper.text()).toContain("★★★");
    expect(wrapper.text()).not.toContain("重复");
    expect(wrapper.text()).not.toContain("已过期");
    expect(wrapper.text()).not.toContain("无日期");
    expect(
      wrapper.get("select.econ-calendar-view__region").findAll("option"),
    ).toHaveLength(1);
  });

  it("does not emit quote selection for economic events", async () => {
    const wrapper = await mountView();
    expect(wrapper.emitted("select")).toBeUndefined();
  });

  it("queries logical CN through one CNSH branch and removes duplicate events", async () => {
    mocks.fetch.mockResolvedValue(featureResult([entries[0]!, { ...entries[0]! }]));
    const wrapper = mount(EconCalendarView, {
      props: { market: "CN", brokerId: "futu" },
    });
    await flushPromises();

    expect(mocks.fetch).toHaveBeenCalledTimes(1);
    const path = String(mocks.fetch.mock.calls[0]?.[0]);
    expect(path).toContain("market=SH");
    expect(path).toContain("brokerId=futu");
    expect(path).not.toContain("market=CN");
    expect(path).not.toContain("market=SZ");
    expect(wrapper.text()).toContain("共 1 条");
    expect(wrapper.findAll(".econ-calendar-view__item")).toHaveLength(1);
  });

  it("shows empty state without data", async () => {
    mocks.fetch.mockResolvedValue(featureResult([]));
    const wrapper = mount(EconCalendarView);
    await flushPromises();
    expect(wrapper.text()).toContain("暂无数据");
  });

  it("shows loading and upstream failures", async () => {
    let rejectRequest!: (reason: unknown) => void;
    mocks.fetch.mockImplementationOnce(
      () =>
        new Promise((_, reject) => {
          rejectRequest = reject;
        }),
    );
    const wrapper = mount(EconCalendarView);
    expect(wrapper.text()).toContain("加载中");
    rejectRequest(new Error("经济日历失败"));
    await flushPromises();
    expect(wrapper.text()).toContain("经济日历失败");
  });
});
