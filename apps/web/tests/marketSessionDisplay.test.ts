import { describe, expect, it } from "vitest";

import { resolveMarketSnapshotDisplay } from "../src/composables/marketSessionDisplay";

describe("marketSessionDisplay", () => {
  it("keeps BABA Thursday regular close when the session advances to after-hours", () => {
    const regularDisplay = resolveMarketSnapshotDisplay(
      {
        price: 111.14,
        previousClosePrice: 108.98,
        lastClosePrice: 108.98,
        session: "regular",
      },
      true,
    );

    expect(regularDisplay.mainPriceLabel).toBe("最新价");
    expect(regularDisplay.mainDisplayPrice).toBe(111.14);

    const afterHoursDisplay = resolveMarketSnapshotDisplay(
      {
        // Broker snapshots can still expose the regular close at the top level.
        // The active after-hours block remains authoritative for its card.
        price: 111.14,
        previousClosePrice: 111.14,
        lastClosePrice: 108.98,
        session: "after",
        extended: {
          afterMarket: {
            price: 111.81,
            changeRate: 0.602,
            quoteTime: "2026-07-09T23:36:39.917Z",
          },
        },
      },
      true,
    );

    expect(afterHoursDisplay.sessionLabel).toBe("盘后");
    expect(afterHoursDisplay.mainPriceLabel).toBe("最近常规收盘");
    expect(afterHoursDisplay.mainDisplayPrice).toBe(111.14);
    expect(afterHoursDisplay.mainChangePercent).toBeCloseTo(
      ((111.14 - 108.98) / 108.98) * 100,
      10,
    );
    expect(afterHoursDisplay.extendedCards).toHaveLength(1);
    expect(afterHoursDisplay.extendedCards[0]).toMatchObject({
      key: "after",
      label: "盘后价格",
      price: 111.81,
      quoteTime: "2026-07-09T23:36:39.917Z",
    });
    expect(afterHoursDisplay.extendedCards[0]?.changeRate).toBeCloseTo(
      ((111.81 - 111.14) / 111.14) * 100,
      10,
    );
  });

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
        price: 190,
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
    const initialDisplay = resolveMarketSnapshotDisplay(
      {
        price: 114.97,
        previousClosePrice: 114.97,
        lastClosePrice: 117.49,
        session: "overnight",
        extended: {
          afterMarket: {
            price: 115.1768,
            changeRate: 0.179,
            quoteTime: "2026-06-23T20:00:00.000Z",
          },
          overnight: {
            price: 119.1,
            changeRate: 3.592,
            quoteTime: "2026-06-24T04:15:00.000Z",
          },
        },
      },
      true,
    );

    expect(initialDisplay.sessionLabel).toBe("夜盘");
    expect(initialDisplay.extendedCards).toEqual([
      {
        key: "after",
        label: "最近盘后价格",
        price: 115.1768,
        changeRate: 0.179,
        quoteTime: "2026-06-23T20:00:00.000Z",
      },
      {
        key: "overnight",
        label: "夜盘价格",
        price: 119.1,
        changeRate: ((119.1 - 114.97) / 114.97) * 100,
        quoteTime: "2026-06-24T04:15:00.000Z",
      },
    ]);

    const liveDisplay = resolveMarketSnapshotDisplay(
      {
        price: 119.1,
        previousClosePrice: 114.97,
        lastClosePrice: 117.49,
        session: "overnight",
        extended: {
          overnight: {
            price: 119.1,
            quoteTime: "2026-06-24T04:15:01.000Z",
          },
        },
      },
      true,
    );
    expect(liveDisplay.extendedCards[0]?.price).toBe(
      initialDisplay.extendedCards[1]?.price,
    );
  });

  it("keeps incomplete market snapshots from showing a fabricated percentage", () => {
    expect(resolveMarketSnapshotDisplay(null, true)).toMatchObject({
      mainDisplayPrice: null,
      mainChangePercent: null,
      extendedCards: [],
    });

    const afterHoursWithoutComparableCloses = resolveMarketSnapshotDisplay(
      {
        price: 201,
        previousClosePrice: 200,
        lastClosePrice: 200,
        session: "after",
      },
      true,
    );
    expect(afterHoursWithoutComparableCloses.mainChangePercent).toBeNull();

    const regularWithMissingReference = resolveMarketSnapshotDisplay(
      {
        price: 201,
        previousClosePrice: 0,
        session: "regular",
      },
      false,
    );
    expect(regularWithMissingReference.mainChangePercent).toBeNull();
  });
});
