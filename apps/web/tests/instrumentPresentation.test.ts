// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";

import InstrumentIdentity from "../src/components/domain/market-data/InstrumentIdentity.vue";
import { formatMarketLabel } from "../src/composables/consoleDataFormatting";
import {
  bareInstrumentCode,
  categoryMarketForUser,
  formatInstrumentIdentityText,
  formatInstrumentExchangeTag,
  formatUserMarketLabel,
  parseInstrumentId,
  presentInstrument,
} from "../src/composables/instrumentPresentation";

describe("instrumentPresentation", () => {
  it("groups mainland exchange aliases under A shares without changing diagnostics", () => {
    for (const market of ["CN", "SH", "SZ", "CNSH", "CNSZ"]) {
      expect(categoryMarketForUser(market)).toBe("CN");
      expect(formatUserMarketLabel(market)).toBe("沪深");
    }
    expect(formatInstrumentExchangeTag("sh")).toBe("上证");
    expect(formatInstrumentExchangeTag("CNSZ")).toBe("深证");
    expect(formatInstrumentExchangeTag("HK")).toBeNull();

    expect(formatMarketLabel("SH")).toBe("上交所");
    expect(formatMarketLabel("SZ")).toBe("深交所");
  });

  it("parses canonical ids and builds display identities", () => {
    expect(parseInstrumentId(" sh:600519 ")).toEqual({
      market: "SH",
      code: "600519",
    });
    expect(parseInstrumentId("600519")).toBeNull();
    expect(bareInstrumentCode("SZ.000001")).toBe("000001");

    expect(
      presentInstrument({ market: "CN", instrumentId: "sh.600519" }),
    ).toEqual({
      market: "SH",
      categoryMarket: "CN",
      code: "600519",
      instrumentId: "SH.600519",
      displayCode: "600519",
      marketLabel: "沪深",
      exchangeTag: "上证",
    });
    expect(presentInstrument({ market: "SZ", instrumentId: "000001" })).toMatchObject({
      market: "SZ",
      code: "000001",
      instrumentId: "SZ.000001",
      displayCode: "000001",
      exchangeTag: "深证",
    });
    expect(presentInstrument({ market: "US", code: "AAPL" })).toMatchObject({
      instrumentId: "US.AAPL",
      displayCode: "US.AAPL",
      marketLabel: "美股",
      exchangeTag: null,
    });
    expect(
      formatInstrumentIdentityText({ instrumentId: "SH.600519" }),
    ).toBe("600519（上证）");
    expect(formatInstrumentIdentityText({ market: "US", code: "AAPL" })).toBe(
      "US.AAPL",
    );
  });
});

describe("InstrumentIdentity", () => {
  it("shows an A-share bare code and exchange tag while preserving the full id", () => {
    const wrapper = mount(InstrumentIdentity, {
      props: {
        market: "CN",
        instrumentId: "SH.600519",
        name: "贵州茅台",
      },
    });

    expect(wrapper.text()).toContain("贵州茅台");
    expect(wrapper.get(".instrument-identity__code").text()).toBe("600519");
    expect(wrapper.get(".instrument-identity__exchange-tag").text()).toBe("上证");
    expect(wrapper.attributes("title")).toBe("SH.600519");
    expect(wrapper.attributes("data-copy-value")).toBe("SH.600519");
    expect(wrapper.attributes("data-category-market")).toBe("CN");

    const setData = vi.fn();
    const copyEvent = new Event("copy", { bubbles: true, cancelable: true });
    Object.defineProperty(copyEvent, "clipboardData", {
      value: { setData },
    });
    wrapper.element.dispatchEvent(copyEvent);
    expect(setData).toHaveBeenCalledWith("text/plain", "SH.600519");
    expect(copyEvent.defaultPrevented).toBe(true);
  });

  it("keeps non-A-share canonical ids and omits the exchange tag", async () => {
    const wrapper = mount(InstrumentIdentity, {
      props: { market: "US", code: "AAPL", compact: true },
    });

    expect(wrapper.get(".instrument-identity__code").text()).toBe("US.AAPL");
    expect(wrapper.find(".instrument-identity__exchange-tag").exists()).toBe(false);
    expect(wrapper.classes()).toContain("instrument-identity--compact");

    await wrapper.setProps({ market: "CNSZ", code: "000001" });
    expect(wrapper.get(".instrument-identity__code").text()).toBe("000001");
    expect(wrapper.get(".instrument-identity__exchange-tag").text()).toBe("深证");
  });

  it("supports market-aware stacked identities without changing the canonical value", async () => {
    const wrapper = mount(InstrumentIdentity, {
      props: {
        market: "US",
        code: "AAPL",
        name: "Apple",
        layout: "stacked",
      },
    });

    expect(wrapper.get(".instrument-identity__primary").text()).toContain(
      "美股Apple",
    );
    expect(wrapper.get(".instrument-identity__secondary").text()).toBe(
      "AAPL",
    );
    expect(wrapper.classes()).toContain("instrument-identity--has-market-tag");
    expect(wrapper.attributes("data-market")).toBe("US");
    expect(wrapper.attributes("data-copy-value")).toBe("US.AAPL");

    await wrapper.setProps({ market: "HK", code: "00700", name: "腾讯控股" });
    expect(wrapper.get(".instrument-identity__exchange-tag").text()).toBe(
      "港股",
    );
    expect(wrapper.get(".instrument-identity__secondary").text()).toBe(
      "00700",
    );
    expect(wrapper.attributes("data-market")).toBe("HK");
  });
});
