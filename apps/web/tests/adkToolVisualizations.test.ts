import { describe, expect, it } from "vitest";

import { buildADKToolVisualization } from "../src/composables/adkToolVisualizations";

describe("buildADKToolVisualization", () => {
  it("builds portfolio summary cards from known fields", () => {
    const visualization = buildADKToolVisualization("portfolio.summary", {
      brokerStatus: "connected",
      accountCount: 2,
      orderCount: 3,
      checkedAt: "2026-06-08T10:00:00Z",
    });

    expect(visualization?.kind).toBe("summary");
    if (visualization?.kind !== "summary") return;
    expect(visualization.title).toBe("Portfolio summary");
    expect(visualization.cards.map((card) => card.label)).toEqual(["Broker", "Accounts", "Orders"]);
    expect(visualization.rows?.[0]).toEqual({ label: "Checked at", value: "2026-06-08T10:00:00Z" });
  });

  it("builds broker order tables and keeps only available preferred columns", () => {
    const visualization = buildADKToolVisualization("broker.orders", {
      orders: [
        {
          symbol: "HK.00700",
          side: "BUY",
          status: "SUBMITTED",
          quantity: 100,
          price: 388.12345,
          ignored: "not shown",
        },
      ],
    });

    expect(visualization?.kind).toBe("table");
    if (visualization?.kind !== "table") return;
    expect(visualization.title).toBe("Broker orders");
    expect(visualization.columns.map((column) => column.key)).toEqual(["symbol", "side", "status", "quantity", "price"]);
    expect(visualization.rows[0]).toMatchObject({
      symbol: "HK.00700",
      side: "BUY",
      status: "SUBMITTED",
      quantity: "100",
      price: "388.1235",
    });
  });

  it("builds market depth rows with relative bar percentages", () => {
    const visualization = buildADKToolVisualization("market.depth", {
      symbol: "HK.00700",
      bids: [[388, 100], [387.8, 50]],
      asks: [{ price: 388.2, quantity: 200 }],
    });

    expect(visualization?.kind).toBe("depth");
    if (visualization?.kind !== "depth") return;
    expect(visualization.title).toBe("Market depth");
    expect(visualization.bids[0]).toMatchObject({ price: "388", quantity: "100", percent: 50 });
    expect(visualization.asks[0]).toMatchObject({ price: "388.2", quantity: "200", percent: 100 });
  });

  it("builds risk state summary with kill switch tone", () => {
    const visualization = buildADKToolVisualization("risk.state", {
      killSwitch: true,
      realTradingEnabled: false,
      riskConfigSource: "workspace",
    });

    expect(visualization?.kind).toBe("summary");
    if (visualization?.kind !== "summary") return;
    expect(visualization.title).toBe("Risk state");
    expect(visualization.cards.find((card) => card.label === "Kill switch")).toMatchObject({
      value: "Yes",
      tone: "danger",
    });
  });

  it("builds execution timelines from event arrays", () => {
    const visualization = buildADKToolVisualization("execution.order_events", {
      events: [
        { type: "submitted", createdAt: "2026-06-08T10:00:00Z", symbol: "HK.00700" },
        { type: "failed", reason: "rejected" },
      ],
    });

    expect(visualization?.kind).toBe("timeline");
    if (visualization?.kind !== "timeline") return;
    expect(visualization.events).toHaveLength(2);
    expect(visualization.events[1]).toMatchObject({ label: "failed", detail: "rejected", tone: "danger" });
  });

  it("returns null for unknown tools and malformed known outputs", () => {
    expect(buildADKToolVisualization("unknown.tool", { ok: true })).toBeNull();
    expect(buildADKToolVisualization("broker.orders", { orders: "not an array" })).toBeNull();
    expect(buildADKToolVisualization("portfolio.summary", {})).toBeNull();
    expect(buildADKToolVisualization("market.depth", { bids: [{ price: 1 }], asks: [] })).toBeNull();
  });
});
