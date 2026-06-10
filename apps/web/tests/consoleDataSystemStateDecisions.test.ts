import { describe, expect, it } from "vitest";

import {
  emptyBrokerRuntime,
  emptyExecutionOrders,
  emptyFutuOpenDHealth,
} from "@/contracts";

import {
  resolveConsoleDataBrokerLiveSelection,
  resolveConsoleDataExecutionSelection,
} from "../src/composables/consoleDataSystemStateDecisions";

describe("consoleDataSystemStateDecisions", () => {
  it("uses the selected account broker and pauses futu reads when runtime is degraded", () => {
    expect(
      resolveConsoleDataBrokerLiveSelection({
        activeBrokerId: "futu",
        selectedAccount: {
          selectionKey: "ib|REAL|U123|US",
          source: "managed",
          brokerId: "ib",
          accountId: "U123",
          displayName: "US real",
          tradingEnvironment: "REAL",
          market: "US",
          securityFirm: null,
        },
        runtime: {
          ...emptyBrokerRuntime,
          descriptor: {
            ...emptyBrokerRuntime.descriptor,
            id: "futu",
          },
          session: {
            ...emptyBrokerRuntime.session,
            connectivity: "disconnected",
          },
        },
        opendHealth: {
          ...emptyFutuOpenDHealth,
          diagnosis: {
            ...emptyFutuOpenDHealth.diagnosis,
            manualRetryRequired: true,
          },
        },
      }),
    ).toEqual({
      nextSelectedBrokerAccountKey: "ib|REAL|U123|US",
      brokerIdForQueries: "ib",
      futuBrokerReadsPaused: true,
    });
  });

  it("keeps the current execution order when it still exists", () => {
    expect(
      resolveConsoleDataExecutionSelection({
        currentSelectedExecutionOrderId: "ord-2",
        executionOrders: {
          ...emptyExecutionOrders,
          orders: [
            { internalOrderId: "ord-1" },
            { internalOrderId: "ord-2" },
          ],
        },
      }),
    ).toEqual({
      nextSelectedExecutionOrderId: "ord-2",
      shouldResetExecutionDetails: false,
      shouldClearExecutionDetails: false,
    });
  });

  it("falls back to the first order and resets execution details when the selected order disappears", () => {
    expect(
      resolveConsoleDataExecutionSelection({
        currentSelectedExecutionOrderId: "missing-order",
        executionOrders: {
          ...emptyExecutionOrders,
          orders: [{ internalOrderId: "ord-1" }],
        },
      }),
    ).toEqual({
      nextSelectedExecutionOrderId: "ord-1",
      shouldResetExecutionDetails: true,
      shouldClearExecutionDetails: false,
    });
  });

  it("clears execution details when there are no orders", () => {
    expect(
      resolveConsoleDataExecutionSelection({
        currentSelectedExecutionOrderId: "ord-1",
        executionOrders: emptyExecutionOrders,
      }),
    ).toEqual({
      nextSelectedExecutionOrderId: "",
      shouldResetExecutionDetails: false,
      shouldClearExecutionDetails: true,
    });
  });
});