// @vitest-environment jsdom

import {
  DOMWrapper,
  flushPromises,
  mount,
  type VueWrapper,
} from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";

import InstrumentSearchBox from "../src/components/domain/market-data/InstrumentSearchBox.vue";
import type { InstrumentResolutionCandidate } from "../src/contracts";
import { createResponse } from "./helpers";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  document.body.replaceChildren();
});

function bodyGet(selector: string): DOMWrapper<Element> {
  const element = document.body.querySelector(selector);
  expect(element).not.toBeNull();
  return new DOMWrapper(element!);
}

function bodyFindAll(selector: string): DOMWrapper<Element>[] {
  return Array.from(document.body.querySelectorAll(selector)).map(
    (element) => new DOMWrapper(element),
  );
}

function candidate(
  market: string,
  code: string,
  securityType: string,
  patch: Partial<InstrumentResolutionCandidate> = {},
): InstrumentResolutionCandidate {
  return {
    market,
    resolvedMarket: market === "SH" || market === "SZ" ? "CN" : market,
    instrumentId: `${market}.${code}`,
    code,
    symbol: code,
    name: `${market} ${code}`,
    securityType,
    lotSize: 1,
    source: "test-search",
    isWatched: false,
    selectable: ["HK", "US", "SH", "SZ"].includes(market),
    unavailableReason: ["HK", "US", "SH", "SZ"].includes(market)
      ? null
      : `当前版本暂不支持 ${market} 市场`,
    ...patch,
  };
}

function mountSearch(): VueWrapper {
  let wrapper: VueWrapper;
  wrapper = mount(InstrumentSearchBox, {
    props: {
      modelValue: "",
      inputTestId: "search-input",
      submitTestId: "search-submit",
      rootTestId: "search-root",
      "onUpdate:modelValue": (value: string) =>
        wrapper.setProps({ modelValue: value }),
    },
  });
  return wrapper;
}

describe("InstrumentSearchBox", () => {
  it("queries only after confirmation and directly selects a unique result", async () => {
    const fetchMock = vi.fn(async () =>
      createResponse({
        requestedMarket: "",
        query: "Apple",
        resolutionStatus: "resolved",
        totalReturned: 1,
        entries: [candidate("US", "AAPL", "Eqty")],
        failures: [],
      }),
    );
    vi.stubGlobal("fetch", fetchMock);
    const wrapper = mountSearch();
    const input = wrapper.get('[data-testid="search-input"]');

    await input.setValue("Apple");
    await flushPromises();
    expect(fetchMock).not.toHaveBeenCalled();

    await input.trigger("keydown", { key: "Enter" });
    await flushPromises();

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/market-data/instruments?query=Apple&limit=20",
      expect.objectContaining({ method: "GET" }),
    );
    expect(wrapper.emitted("select")?.[0]?.[0]).toMatchObject({
      market: "US",
      instrumentId: "US.AAPL",
    });
    expect(document.body.querySelector('[role="listbox"]')).toBeNull();
  });

  it("filters multi-market results by market and type and navigates visible options", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        createResponse({
          requestedMarket: "",
          query: "bank",
          resolutionStatus: "ambiguous",
          totalReturned: 4,
          entries: [
            candidate("SH", "600000", "Eqty"),
            candidate("SZ", "159001", "Trust"),
            candidate("US", "BAC", "Eqty"),
            candidate("JP", "8306", "Future"),
          ],
          failures: [],
        }),
      ),
    );
    const wrapper = mountSearch();
    vi.stubGlobal("innerWidth", 1024);
    vi.stubGlobal("innerHeight", 768);
    const viewportWidth = vi
      .spyOn(document.documentElement, "clientWidth", "get")
      .mockReturnValue(1024);
    vi.spyOn(document.documentElement, "clientHeight", "get").mockReturnValue(768);
    vi.spyOn(wrapper.element as HTMLElement, "getBoundingClientRect").mockReturnValue({
      x: 900,
      y: 700,
      top: 700,
      right: 1100,
      bottom: 730,
      left: 900,
      width: 200,
      height: 30,
      toJSON: () => ({}),
    });
    const input = wrapper.get('[data-testid="search-input"]');
    await input.setValue("bank");
    await wrapper.get('[data-testid="search-submit"]').trigger("click");
    await flushPromises();

    expect(
      bodyGet('[data-testid="instrument-search-market-filters"]')
        .findAll("button")
        .map((button) => button.text()),
    ).toEqual(["全部", "沪深", "美股", "日本"]);
    expect(
      bodyGet('[data-testid="instrument-search-type-filters"]')
        .findAll("button")
        .map((button) => button.text()),
    ).toEqual(["全部", "股票", "基金/信托", "期货"]);
    const panel = bodyGet(".instrument-search-box__panel");
    const results = bodyGet('[data-testid="instrument-search-results"]');
    expect(panel.attributes("role")).toBeUndefined();
    expect((panel.element as HTMLElement).style.left).toBe("532px");
    expect((panel.element as HTMLElement).style.width).toBe("480px");
    expect((panel.element as HTMLElement).style.minWidth).toBe("480px");
    expect((panel.element as HTMLElement).style.maxHeight).toBe("560px");
    expect((panel.element as HTMLElement).style.top).toBe("auto");
    expect((panel.element as HTMLElement).style.bottom).toBe("74px");
    viewportWidth.mockReturnValue(360);
    window.dispatchEvent(new Event("resize"));
    await flushPromises();
    expect((panel.element as HTMLElement).style.left).toBe("12px");
    expect((panel.element as HTMLElement).style.width).toBe("336px");
    expect((panel.element as HTMLElement).style.minWidth).toBe("336px");
    expect(results.attributes("role")).toBe("listbox");
    expect(results.find('[data-testid="instrument-search-market-filters"]').exists()).toBe(false);
    expect(panel.get('[data-testid="instrument-search-market-filters"]').exists()).toBe(true);
    expect(results.find(".instrument-search-box__actions").exists()).toBe(false);
    expect(panel.get(".instrument-search-box__actions").exists()).toBe(true);
    expect(bodyFindAll('[role="option"]')).toHaveLength(4);

    await bodyGet('[data-filter-value="CN"]').trigger("click");
    await bodyGet('[data-filter-value="TRUST"]').trigger("click");
    const cnTrustOptions = bodyFindAll('[role="option"]');
    expect(cnTrustOptions).toHaveLength(1);
    expect(cnTrustOptions[0]?.text()).toContain("159001");
    expect(cnTrustOptions[0]?.text()).toContain("深证");

    await bodyGet('[data-testid="instrument-search-type-filters"] button').trigger("click");
    await bodyGet('[data-filter-value="US"]').trigger("click");
    await input.trigger("keydown", { key: "Enter" });
    expect(wrapper.emitted("select")?.[0]?.[0]).toMatchObject({
      market: "US",
      instrumentId: "US.BAC",
    });
  });

  it("keeps unavailable results visible and supports an explicit retry", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        createResponse({
          requestedMarket: "",
          query: "Toyota",
          resolutionStatus: "unavailable",
          totalReturned: 1,
          entries: [candidate("JP", "7203", "Eqty")],
          failures: [],
        }),
      )
      .mockResolvedValueOnce(
        createResponse({
          requestedMarket: "",
          query: "Toyota",
          resolutionStatus: "not_found",
          totalReturned: 0,
          entries: [],
          failures: [],
        }),
      );
    vi.stubGlobal("fetch", fetchMock);
    const wrapper = mountSearch();
    await wrapper.get('[data-testid="search-input"]').setValue("Toyota");
    await wrapper.get('[data-testid="search-submit"]').trigger("click");
    await flushPromises();

    const unavailable = bodyGet('[role="option"]');
    expect(unavailable.attributes("disabled")).toBeDefined();
    expect(unavailable.text()).toContain("当前版本暂不支持 JP 市场");
    expect(wrapper.emitted("select")).toBeUndefined();

    await wrapper.get('[data-testid="search-input"]').trigger("keydown", {
      key: "Enter",
    });
    await flushPromises();
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(document.body.textContent).toContain("未找到匹配标的");
    expect(document.body.textContent).toContain("重试");
  });
});
