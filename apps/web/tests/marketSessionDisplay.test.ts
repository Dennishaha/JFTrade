import { describe, expect, it } from "vitest";

import { resolveMarketSnapshotDisplay } from "../src/composables/marketSessionDisplay";

describe("marketSessionDisplay", () => {
  it("shows only recent after-hours on closed sessions", () => {
    const display = resolveMarketSnapshotDisplay(
      {
        price: 190,
        previousClosePrice: 189.5,
        lastClosePrice: 188,
        session: "closed",
        extended: {
          preMarket: {
            price: 191,
            changeRate: 1.2,
            quoteTime: "2026-06-18T20:00:00.000Z",
          },
          afterMarket: {
            price: 192,
            changeRate: 1.8,
            quoteTime: "2026-06-18T20:00:00.000Z",
          },
        },
      },
      true,
    );

    expect(display.sessionLabel).toBe("休市");
    expect(display.mainPriceLabel).toBe("最近常规收盘");
    expect(display.extendedCards).toEqual([
      {
        key: "after",
        label: "最近盘后价格",
        price: 192,
        changeRate: 1.8,
        quoteTime: "2026-06-18T20:00:00.000Z",
      },
    ]);
  });

  it("hides closed-session after-hours cards without quote time", () => {
    const display = resolveMarketSnapshotDisplay(
      {
        price: 190,
        previousClosePrice: 189.5,
        lastClosePrice: 188,
        session: "closed",
        extended: {
          afterMarket: {
            price: 192,
            changeRate: 1.8,
          },
        },
      },
      true,
    );

    expect(display.extendedCards).toEqual([]);
  });

  it("keeps pre-market cards limited to active pre-market sessions", () => {
    const display = resolveMarketSnapshotDisplay(
      {
        price: 191,
        previousClosePrice: 190,
        lastClosePrice: 188,
        session: "pre",
        extended: {
          preMarket: {
            price: 191,
            changeRate: 0.5,
            quoteTime: "2026-06-22T08:15:00.000Z",
          },
          afterMarket: {
            price: 192,
            changeRate: 1.8,
            quoteTime: "2026-06-19T20:00:00.000Z",
          },
        },
      },
      true,
    );

    expect(display.extendedCards).toEqual([
      {
        key: "pre",
        label: "盘前价格",
        price: 191,
        changeRate: 0.5263157894736842,
        quoteTime: "2026-06-22T08:15:00.000Z",
      },
    ]);
  });

  it("shows active overnight pricing during overnight sessions", () => {
    const display = resolveMarketSnapshotDisplay(
      {
        price: 193,
        previousClosePrice: 190,
        lastClosePrice: 188,
        session: "overnight",
        extended: {
          afterMarket: {
            price: 192,
            changeRate: 1.8,
            quoteTime: "2026-06-23T20:00:00.000Z",
          },
          overnight: {
            price: 193,
            changeRate: 2.3,
            quoteTime: "2026-06-24T04:15:00.000Z",
          },
        },
      },
      true,
    );

    expect(display.sessionLabel).toBe("夜盘");
    expect(display.extendedCards).toEqual([
      {
        key: "after",
        label: "最近盘后价格",
        price: 192,
        changeRate: 1.8,
        quoteTime: "2026-06-23T20:00:00.000Z",
      },
      {
        key: "overnight",
        label: "夜盘价格",
        price: 193,
        changeRate: 1.5789473684210527,
        quoteTime: "2026-06-24T04:15:00.000Z",
      },
    ]);
  });
});
