// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ fetch: vi.fn() }));

vi.mock("../../src/composables/productFeatures", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("../../src/composables/productFeatures")>();
  return { ...actual, fetchProductFeature: mocks.fetch };
});

import DividendCalendarView from "../../src/components/research/DividendCalendarView.vue";
import { flushPromises } from "../productTestUtils";

function result(entries: Record<string, unknown>[]) {
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

afterEach(() => {
  mocks.fetch.mockReset();
});

describe("DividendCalendarView", () => {
  it("requests a concrete date and advances it through the date toolbar", async () => {
    mocks.fetch.mockResolvedValue(result([]));
    const wrapper = mount(DividendCalendarView, {
      props: { market: "HK", brokerId: "futu" },
    });
    await flushPromises();

    const firstPath = String(mocks.fetch.mock.calls[0]?.[0]);
    const firstDate = new URLSearchParams(firstPath.split("?")[1]).get("date");
    expect(firstPath).toContain("/api/v1/research/calendars?");
    expect(firstPath).toContain("market=HK");
    expect(firstPath).toContain("operation=dividends");
    expect(firstPath).toContain("brokerId=futu");
    expect(firstDate).toMatch(/^\d{4}-\d{2}-\d{2}$/);

    await wrapper.get('button[aria-label="后一天"]').trigger("click");
    await flushPromises();
    const nextPath = String(mocks.fetch.mock.calls.at(-1)?.[0]);
    const nextDate = new URLSearchParams(nextPath.split("?")[1]).get("date");
    const expected = new Date(`${firstDate}T12:00:00`);
    expected.setDate(expected.getDate() + 1);
    expect(nextDate).toBe(expected.toISOString().slice(0, 10));
  });

  it("uses table selection for preview and double click for opening", async () => {
    mocks.fetch.mockResolvedValue(
      result([
        {
          instrumentId: "HK.00700",
          symbol: "00700",
          name: "腾讯控股",
          statement: "末期股息",
          exDate: "2026-07-24",
        },
      ]),
    );
    const wrapper = mount(DividendCalendarView);
    await flushPromises();

    const row = wrapper.get("tbody tr");
    await row.trigger("click");
    await row.trigger("dblclick");
    expect(wrapper.emitted("select")?.[0]?.[0]).toMatchObject({
      instrumentId: "HK.00700",
    });
    expect(wrapper.emitted("open")?.[0]?.[0]).toMatchObject({
      instrumentId: "HK.00700",
    });
  });

  it("shows an upstream error and retries the same dividend request", async () => {
    mocks.fetch.mockRejectedValueOnce(new Error("派息上游失败"));
    const wrapper = mount(DividendCalendarView);
    await flushPromises();

    expect(wrapper.get(".dividend-calendar__status").text()).toContain("派息上游失败");

    mocks.fetch.mockResolvedValueOnce(result([]));
    await wrapper.findAll("button").at(-1)!.trigger("click");
    await flushPromises();
    expect(wrapper.find(".dividend-calendar__status").exists()).toBe(false);
  });
});
