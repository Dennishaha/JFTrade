// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import QuoteSummaryCard from "../src/components/domain/market-data/QuoteSummaryCard.vue";

describe("QuoteSummaryCard", () => {
  it("renders the shared quote identity, price, favorite, and status layout", async () => {
    const wrapper = mount(QuoteSummaryCard, {
      props: {
        market: "HK",
        code: "08277",
        instrumentId: "HK.08277",
        name: "骏东控股",
        priceLabel: "最新价",
        price: 0.103,
        changeAmount: -0.018,
        changeRate: -14.88,
        showChangeAmount: true,
        sessionLabel: "交易中",
        sessionActive: true,
        statusText: "07/23 09:36:10",
        sourceText: "FUTU · OPEND.SNAPSHOT",
        favoriteVisible: true,
        favoriteActive: true,
        favoriteTestId: "shared-favorite",
      },
      slots: {
        badges: '<span data-testid="quote-badge">融资</span>',
      },
    });

    expect(wrapper.get('[data-testid="quote-summary-card"]').text()).toContain(
      "08277",
    );
    expect(wrapper.text()).toContain("骏东控股");
    expect(wrapper.get(".quote-summary__price").text()).toBe("0.103");
    expect(wrapper.get(".quote-summary__price").classes()).not.toContain(
      "tv-down",
    );
    expect(wrapper.get(".quote-summary__change.tv-down").text()).toBe("-0.02");
    expect(wrapper.findAll(".quote-summary__change.tv-down")[1]?.text()).toBe(
      "-14.88%",
    );
    expect(wrapper.get(".quote-summary__session").classes()).toContain(
      "is-active",
    );
    expect(wrapper.get('[data-testid="quote-badge"]').text()).toBe("融资");

    const favorite = wrapper.get('[data-testid="shared-favorite"]');
    expect(favorite.text()).toBe("★");
    expect(favorite.classes()).toContain("is-active");
    await favorite.trigger("click");
    expect(wrapper.emitted("favorite")).toHaveLength(1);
  });

  it("uses the homepage session-card palette for extended quotes", () => {
    const wrapper = mount(QuoteSummaryCard, {
      props: {
        market: "US",
        code: "AAPL",
        instrumentId: "US.AAPL",
        name: "Apple",
        price: 201.5,
        extendedCards: [
          {
            key: "pre",
            label: "盘前价格",
            price: 202,
            changeRate: 0.25,
            quoteTime: "2026-07-23T12:00:00Z",
          },
          {
            key: "after",
            label: "盘后价格",
            price: 200,
            changeRate: -0.75,
            quoteTime: "2026-07-23T21:00:00Z",
          },
          {
            key: "overnight",
            label: "夜盘价格",
            price: 203,
            changeRate: 0.75,
            quoteTime: "2026-07-24T01:00:00Z",
          },
        ],
      },
    });

    expect(wrapper.get(".quote-summary").classes()).toContain("quote-summary");
    expect(wrapper.get(".quote-summary__extended-card--pre").text()).toContain(
      "盘前价格",
    );
    expect(wrapper.get(".quote-summary__extended-card--after").text()).toContain(
      "盘后价格",
    );
    expect(
      wrapper.get(".quote-summary__extended-card--overnight").text(),
    ).toContain("夜盘价格");
  });

  it("hides favorite controls when the consumer does not expose them", () => {
    const wrapper = mount(QuoteSummaryCard, {
      props: {
        market: "HK",
        code: "BK1001",
        instrumentId: "HK.BK1001",
        name: "恒生科技",
        price: 3000,
      },
    });

    expect(wrapper.find(".watchlist-favorite-button").exists()).toBe(false);
  });
});
