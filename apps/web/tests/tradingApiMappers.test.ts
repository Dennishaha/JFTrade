import { describe, expect, it } from "vitest";

import type { ExecutionOrderDto } from "@/contracts";
import {
  mapExecutionOrderDetails,
  mapExecutionOrderEvents,
  mapExecutionOrders,
} from "@/composables/trading/tradingApiMappers";

type ExecutionOrderWire = ExecutionOrderDto;

function wireOrder(
  overrides: Partial<ExecutionOrderWire> = {},
): ExecutionOrderWire {
  return {
    accountId: "SIM-1",
    brokerId: "futu",
    brokerOrderId: null,
    brokerOrderIdEx: null,
    clientOrderId: null,
    createdAt: "2026-07-26T00:00:00Z",
    fees: null,
    filledAveragePrice: null,
    filledQuantity: null,
    internalOrderId: "order-1",
    lastError: null,
    lastErrorCode: null,
    lastErrorSource: null,
    market: "HK",
    orderKind: "single",
    orderType: "LIMIT",
    payout: null,
    previewId: null,
    productClass: "equity",
    quantityMode: "units",
    rawBrokerStatus: null,
    remark: null,
    requestedAmount: null,
    requestedPrice: 320,
    requestedQuantity: 100,
    side: "BUY",
    source: "system",
    sourceDetail: "command.place",
    status: "SUBMITTED",
    submittedAt: "2026-07-26T00:00:00Z",
    symbol: "HK.00700",
    tradingEnvironment: "SIMULATE",
    updatedAt: "2026-07-26T00:00:00Z",
    ...overrides,
  };
}

describe("trading API mappers", () => {
  it("normalizes source casing and preserves newer source-detail values", () => {
    const mapped = mapExecutionOrders({
      orders: [wireOrder({ source: "BROKER", sourceDetail: "broker.cache" })],
    });

    expect(mapped.orders[0]?.source).toBe("broker");
    expect(mapped.orders[0]?.sourceDetail).toBe("broker.cache");
  });

  it("supplies stable source defaults when an older response omits them", () => {
    const partialWire = {
      ...wireOrder(),
      source: undefined,
      sourceDetail: undefined,
    } as unknown as ExecutionOrderWire;

    const mapped = mapExecutionOrders({ orders: [partialWire] });

    expect(mapped.orders[0]?.source).toBe("system");
    expect(mapped.orders[0]?.sourceDetail).toBe("command.place");
  });

  it("uses broker defaults for empty broker details and trims error provenance", () => {
    const [brokerOrder, systemOrder] = mapExecutionOrders({
      orders: [
        wireOrder({
          source: "broker",
          sourceDetail: " ",
          lastErrorSource: " broker.reject ",
        }),
        wireOrder({
          source: "system",
          sourceDetail: "",
          lastErrorSource: " ",
        }),
      ],
    }).orders;

    expect(brokerOrder).toMatchObject({
      source: "broker",
      sourceDetail: "broker.current",
      lastErrorSource: "broker.reject",
    });
    expect(systemOrder).toMatchObject({
      source: "system",
      sourceDetail: "command.place",
      lastErrorSource: null,
    });
  });

  it("maps details and event arrays without sharing mutable arrays", () => {
    const order = wireOrder();
    const event = {
      createdAt: "2026-07-26T00:00:01Z",
      eventType: "SUBMITTED",
      id: "event-1",
      internalOrderId: order.internalOrderId,
      nextStatus: "SUBMITTED",
      payloadJson: "{}",
      previousStatus: null,
    };
    const detailsWire = {
      checkedAt: "2026-07-26T00:00:02Z",
      order,
      recentEvents: [event],
    };
    const eventsWire = {
      events: [event],
      internalOrderId: order.internalOrderId,
    };

    const details = mapExecutionOrderDetails(detailsWire);
    const events = mapExecutionOrderEvents(eventsWire);

    expect(details.order.internalOrderId).toBe("order-1");
    expect(details.recentEvents).not.toBe(detailsWire.recentEvents);
    expect(events.events).not.toBe(eventsWire.events);
  });

  it("clones multi-leg payloads and normalizes absent response arrays", () => {
    const legs = [{
      legId: "leg-1",
      market: "US",
      symbol: "AAPL",
      side: "BUY",
      quantity: 1,
    }] as never;
    const wire = wireOrder({ legs });
    const mapped = mapExecutionOrders({ orders: [wire] });

    expect(mapped.orders[0]?.legs).toEqual(legs);
    expect(mapped.orders[0]?.legs).not.toBe(legs);
    expect(mapped.orders[0]?.legs?.[0]).not.toBe(legs[0]);

    expect(mapExecutionOrders({ orders: undefined } as never)).toEqual({
      orders: [],
    });
    expect(mapExecutionOrderDetails({
      checkedAt: "2026-07-26T00:00:02Z",
      order: wireOrder(),
      recentEvents: undefined,
    } as never).recentEvents).toEqual([]);
    expect(mapExecutionOrderEvents({
      internalOrderId: "order-1",
      events: undefined,
    } as never).events).toEqual([]);
  });

  it("rejects an unknown top-level order source", () => {
    expect(() =>
      mapExecutionOrders({ orders: [wireOrder({ source: "import" })] }),
    ).toThrow("Unsupported execution order source: import");
  });
});
