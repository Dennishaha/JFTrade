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

import EarningsCalendarView from "../../src/components/research/EarningsCalendarView.vue";
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

afterEach(() => {
  mocks.fetch.mockReset();
});

describe("EarningsCalendarView", () => {
  it("renders 7 day columns with today highlighted and items placed by date", async () => {
    mocks.fetch.mockResolvedValue(
      featureResult([
        { instrumentId: "US.AAPL", market: "US", symbol: "AAPL", name: "Apple", productClass: "equity", eventDate: dayKeyOffset(0) },
        { instrumentId: "US.TSLA", market: "US", symbol: "TSLA", name: "特斯拉", productClass: "equity", eventDate: dayKeyOffset(1) },
        { name: "无日期公司" },
      ]),
    );
    const wrapper = mount(EarningsCalendarView);
    await flushPromises();

    const days = wrapper.findAll(".earnings-calendar-view__day");
    expect(days).toHaveLength(7);
    expect(days.filter((day) => day.classes().includes("is-today"))).toHaveLength(1);
    expect(wrapper.text()).toContain("Apple");
    expect(wrapper.text()).toContain("特斯拉");
    // 无日期字段的条目不归入任何格子
    expect(wrapper.text()).not.toContain("无日期公司");
    // 首字 avatar
    expect(wrapper.get(".earnings-calendar-view__avatar").text()).toBe("A");
  });

  it("opens a quoteable related security without making non-security events interactive", async () => {
    const entry = { instrumentId: "US.AAPL", market: "US", symbol: "AAPL", name: "Apple", productClass: "equity", eventDate: dayKeyOffset(0) };
    mocks.fetch.mockResolvedValue(featureResult([
      entry,
      { eventDate: dayKeyOffset(0) },
    ]));
    const wrapper = mount(EarningsCalendarView);
    await flushPromises();
    const items = wrapper.findAll(".earnings-calendar-view__item");
    expect(items[0]!.element.tagName).toBe("BUTTON");
    expect(items[1]!.element.tagName).toBe("DIV");
    expect(items[1]!.text()).toContain("--");
    await items[1]!.trigger("click");
    expect(wrapper.emitted("select")).toBeUndefined();
    await items[0]!.trigger("click");
    expect(wrapper.emitted("select")?.[0]?.[0]).toMatchObject({ instrumentId: "US.AAPL" });
  });

  it("shows empty state when nothing matches the current week, and week arrows work", async () => {
    mocks.fetch.mockResolvedValue(
      featureResult([{ instrumentId: "US.AAPL", symbol: "AAPL", name: "Apple", eventDate: dayKeyOffset(0) }]),
    );
    const wrapper = mount(EarningsCalendarView);
    await flushPromises();
    expect(wrapper.text()).toContain("Apple");

    // 翻到下一周后本周条目不可见 → 占位
    await wrapper.get('button[aria-label="下一周"]').trigger("click");
    await flushPromises();
    expect(wrapper.text()).not.toContain("Apple");
    expect(wrapper.text()).toContain("本周暂无财报数据");

    await wrapper.get('button[aria-label="上一周"]').trigger("click");
    await flushPromises();
    expect(wrapper.text()).toContain("Apple");
  });

  it("shows empty state without data", async () => {
    mocks.fetch.mockResolvedValue(featureResult([]));
    const wrapper = mount(EarningsCalendarView);
    await flushPromises();
    expect(wrapper.text()).toContain("本周暂无财报数据");
  });

  it("shows loading and request failures", async () => {
    let rejectRequest!: (reason: unknown) => void;
    mocks.fetch.mockImplementationOnce(
      () =>
        new Promise((_, reject) => {
          rejectRequest = reject;
        }),
    );
    const wrapper = mount(EarningsCalendarView);
    expect(wrapper.text()).toContain("加载中");
    rejectRequest(new Error("财报日历失败"));
    await flushPromises();
    expect(wrapper.text()).toContain("财报日历失败");
  });
});
