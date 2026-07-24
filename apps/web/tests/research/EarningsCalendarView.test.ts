// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import {
  afterEach,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from "vitest";

const mocks = vi.hoisted(() => ({
  fetch: vi.fn(),
}));

vi.mock("../../src/composables/productFeatures", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("../../src/composables/productFeatures")>();
  return { ...actual, fetchProductFeature: mocks.fetch };
});

import EarningsCalendarView from "../../src/components/research/EarningsCalendarView.vue";
import { flushPromises } from "../productTestUtils";

function featureResult(entries: Record<string, unknown>[]) {
  return {
    provider: {
      brokerId: "futu",
      featureId: "research.calendar",
      capability: "available" as const,
      selectionReason: "explicit",
      resolvedAt: "2026-07-23T00:00:00Z",
      asOf: "2026-07-23T00:00:00Z",
    },
    asOf: "2026-07-23T00:00:00Z",
    entries,
  };
}

function earningsEntry(
  instrumentId: string,
  name: string,
  eventDate = "2026-07-23",
): Record<string, unknown> {
  const [, symbol = instrumentId] = instrumentId.split(".");
  return {
    instrumentId,
    market: "US",
    symbol,
    name,
    productClass: "equity",
    eventDate,
    periodText: "2026Q2",
    marketCap: 3_000_000_000_000,
    price: 213.75,
    optionVolume: 25_000,
    iv: 28.4,
    ivRank: 44,
    ivPercentile: 71,
  };
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date(2026, 6, 23, 12));
  mocks.fetch.mockResolvedValue(featureResult([]));
});

afterEach(() => {
  vi.useRealTimers();
  mocks.fetch.mockReset();
});

describe("EarningsCalendarView", () => {
  it("defaults to a complete Sunday-first month and keeps quote selection", async () => {
    const apple = earningsEntry("US.AAPL", "Apple");
    mocks.fetch.mockResolvedValue(
      featureResult([
        apple,
        earningsEntry("US.TSLA", "特斯拉", "2026-07-24"),
        { name: "无日期公司" },
      ]),
    );

    const wrapper = mount(EarningsCalendarView);
    await flushPromises();

    const tabs = wrapper.findAll('[role="tab"]');
    expect(tabs).toHaveLength(3);
    expect(tabs[2]!.attributes("aria-selected")).toBe("true");
    expect(wrapper.text()).toContain("2026/07");
    expect(wrapper.findAll(".earnings-calendar-view__day")).toHaveLength(35);
    expect(wrapper.findAll(".earnings-calendar-view__day.is-today")).toHaveLength(1);
    expect(wrapper.text()).toContain("Apple");
    expect(wrapper.text()).toContain("特斯拉");
    expect(wrapper.text()).not.toContain("无日期公司");
    expect(wrapper.get(".earnings-calendar-view__avatar").text()).toBe("A");
    expect(mocks.fetch.mock.calls.at(-1)?.[0]).toContain("beginDate=2026-06-28");
    expect(mocks.fetch.mock.calls.at(-1)?.[0]).toContain("endDate=2026-08-01");

    await wrapper.get("button.earnings-calendar-view__item").trigger("click");
    expect(wrapper.emitted("select")?.[0]?.[0]).toEqual(apple);
  });

  it("switches between day, week, and month with view-specific structures", async () => {
    mocks.fetch.mockResolvedValue(
      featureResult([earningsEntry("US.AAPL", "Apple")]),
    );
    const wrapper = mount(EarningsCalendarView);
    await flushPromises();

    await wrapper.findAll('[role="tab"]')[0]!.trigger("click");
    await flushPromises();
    expect(wrapper.get(".earnings-calendar-view__table").text()).toContain("2026Q2");
    expect(wrapper.text()).toContain("3.00万亿");
    expect(wrapper.text()).toContain("28.4%");
    expect(mocks.fetch.mock.calls.at(-1)?.[0]).toContain("beginDate=2026-07-23");
    expect(mocks.fetch.mock.calls.at(-1)?.[0]).toContain("endDate=2026-07-23");

    await wrapper.findAll('[role="tab"]')[1]!.trigger("click");
    await flushPromises();
    expect(wrapper.findAll(".earnings-calendar-view__day")).toHaveLength(7);
    expect(wrapper.get(".earnings-calendar-view__week-scroll").attributes("aria-label")).toBe(
      "本周财报日历",
    );
    expect(mocks.fetch.mock.calls.at(-1)?.[0]).toContain("beginDate=2026-07-19");
    expect(mocks.fetch.mock.calls.at(-1)?.[0]).toContain("endDate=2026-07-25");
  });

  it("navigates periods and accepts native date or month selection", async () => {
    const wrapper = mount(EarningsCalendarView);
    await flushPromises();

    await wrapper.get('button[aria-label="下一月"]').trigger("click");
    await flushPromises();
    expect(wrapper.text()).toContain("2026/08");
    expect(wrapper.findAll(".earnings-calendar-view__day")).toHaveLength(42);
    expect(mocks.fetch.mock.calls.at(-1)?.[0]).toContain("beginDate=2026-07-26");
    expect(mocks.fetch.mock.calls.at(-1)?.[0]).toContain("endDate=2026-09-05");

    const monthInput = wrapper.get('input[aria-label="选择月份"]');
    const showPicker = vi.fn();
    Object.defineProperty(monthInput.element, "showPicker", {
      configurable: true,
      value: showPicker,
    });
    await wrapper.get('button[aria-label="打开月份选择器"]').trigger("click");
    expect(showPicker).toHaveBeenCalledOnce();

    await monthInput.setValue("2024-02");
    await monthInput.trigger("change");
    await flushPromises();
    expect(wrapper.text()).toContain("2024/02");
    expect(mocks.fetch.mock.calls.at(-1)?.[0]).toContain("beginDate=2024-01-28");
    expect(mocks.fetch.mock.calls.at(-1)?.[0]).toContain("endDate=2024-03-02");
  });

  it("shows per-cell empty states and month overflow without replacing the calendar", async () => {
    mocks.fetch.mockResolvedValue(
      featureResult(
        ["一", "二", "三", "四", "五"].map((name, index) =>
          earningsEntry(`US.TEST${index}`, `公司${name}`),
        ),
      ),
    );
    const wrapper = mount(EarningsCalendarView);
    await flushPromises();

    expect(wrapper.findAll(".earnings-calendar-view__day")).toHaveLength(35);
    expect(wrapper.text()).toContain("暂无数据");
    expect(wrapper.text()).toContain("另 1 家");
    expect(wrapper.findAll(".earnings-calendar-view__item")).toHaveLength(4);
  });

  it("uses real sort and filter query parameters with display-unit conversion", async () => {
    const wrapper = mount(EarningsCalendarView);
    await flushPromises();

    await wrapper.get(".earnings-calendar-view__sort-trigger").trigger("click");
    const sortItems = wrapper.findAll('[role="menuitemradio"]');
    expect(sortItems.map((item) => item.text())).toContain("隐含波动率百分位数");
    await sortItems.find((item) => item.text().includes("期权成交量"))!.trigger("click");
    await flushPromises();
    expect(mocks.fetch.mock.calls.at(-1)?.[0]).toContain("sort=option_volume");

    await wrapper.get('button[aria-label="筛选"]').trigger("click");
    await wrapper.get('select').setValue("watchlist");
    await wrapper.get('input[aria-label="市值下限，单位亿"]').setValue("12.5");
    await wrapper.get('input[aria-label="期权成交量上限，单位万"]').setValue("3");
    await wrapper.get('input[aria-label="隐含波动率下限"]').setValue("20");
    await wrapper
      .findAll("button")
      .find((button) => button.text().trim() === "应用")!
      .trigger("click");
    await flushPromises();

    const path = String(mocks.fetch.mock.calls.at(-1)?.[0]);
    expect(path).toContain("stockScope=watchlist");
    expect(path).toContain("marketCapMin=1250000000");
    expect(path).toContain("optionVolumeMax=30000");
    expect(path).toContain("ivMin=20");
    expect(wrapper.get(".earnings-calendar-view__filter-button").text()).toContain("4");
  });

  it("keeps invalid ranges as drafts and supports reset before apply", async () => {
    const wrapper = mount(EarningsCalendarView);
    await flushPromises();
    const requestCount = mocks.fetch.mock.calls.length;

    await wrapper.get('button[aria-label="筛选"]').trigger("click");
    await wrapper.get('input[aria-label="市值下限，单位亿"]').setValue("10");
    await wrapper.get('input[aria-label="市值上限，单位亿"]').setValue("5");
    await wrapper
      .findAll("button")
      .find((button) => button.text().trim() === "应用")!
      .trigger("click");
    await flushPromises();
    expect(wrapper.text()).toContain("市值上限不能小于下限");
    expect(wrapper.find('[role="dialog"]').exists()).toBe(true);
    expect(mocks.fetch).toHaveBeenCalledTimes(requestCount);

    await wrapper
      .findAll("button")
      .find((button) => button.text().trim() === "重置")!
      .trigger("click");
    expect(
      (wrapper.get('input[aria-label="市值下限，单位亿"]').element as HTMLInputElement)
        .value,
    ).toBe("");
  });

  it("removes option sort and filters when switching to the mainland market", async () => {
    const wrapper = mount(EarningsCalendarView, { props: { market: "US" } });
    await flushPromises();

    await wrapper.get(".earnings-calendar-view__sort-trigger").trigger("click");
    await wrapper
      .findAll('[role="menuitemradio"]')
      .find((item) => item.text().includes("隐含波动率等级"))!
      .trigger("click");
    await wrapper.get('button[aria-label="筛选"]').trigger("click");
    await wrapper.get('input[aria-label="隐含波动率下限"]').setValue("10");
    await wrapper
      .findAll("button")
      .find((button) => button.text().trim() === "应用")!
      .trigger("click");
    await flushPromises();

    await wrapper.setProps({ market: "CN" });
    await flushPromises();
    const path = String(mocks.fetch.mock.calls.at(-1)?.[0]);
    expect(path).toContain("market=SZ");
    expect(path).toContain("sort=hot");
    expect(path).not.toContain("ivMin");

    await wrapper.get(".earnings-calendar-view__sort-trigger").trigger("click");
    expect(wrapper.findAll('[role="menuitemradio"]').map((item) => item.text())).toEqual([
      "热门",
      "市值",
    ]);
    await wrapper.get(".earnings-calendar-view__filter-button").trigger("click");
    expect(wrapper.find('input[aria-label="隐含波动率下限"]').exists()).toBe(false);
  });

  it("preserves the selected view on failures and retries the same request", async () => {
    mocks.fetch.mockRejectedValueOnce(new Error("财报日历失败"));
    const wrapper = mount(EarningsCalendarView);
    await flushPromises();
    expect(wrapper.get('[role="alert"]').text()).toContain("财报日历失败");

    mocks.fetch.mockResolvedValueOnce(featureResult([]));
    await wrapper
      .findAll("button")
      .find((button) => button.text().trim() === "重试")!
      .trigger("click");
    await flushPromises();
    expect(wrapper.find('[role="alert"]').exists()).toBe(false);
    expect(wrapper.findAll(".earnings-calendar-view__day")).toHaveLength(35);
    expect(wrapper.text()).toContain("暂无数据");
  });

  it("closes menus and drawer with Escape", async () => {
    const wrapper = mount(EarningsCalendarView);
    await flushPromises();
    await wrapper.get(".earnings-calendar-view__sort-trigger").trigger("click");
    expect(wrapper.find('[role="menu"]').exists()).toBe(true);
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    await flushPromises();
    expect(wrapper.find('[role="menu"]').exists()).toBe(false);

    await wrapper.get(".earnings-calendar-view__filter-button").trigger("click");
    expect(wrapper.find('[role="dialog"]').exists()).toBe(true);
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    await flushPromises();
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false);
  });
});
