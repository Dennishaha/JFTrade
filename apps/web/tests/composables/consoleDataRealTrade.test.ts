import { describe, expect, it } from "vitest";

import { createConsoleDataRealTradeController } from "@/composables/trading/consoleDataRealTrade";
import {
  emptyRealTradeApprovals,
  emptyRealTradeHardStopEvents,
  emptyRealTradeHardStops,
  emptyRealTradeKillSwitchEvents,
  emptyRealTradeKillSwitchState,
  emptyRealTradeRiskEvents,
  emptyRealTradeRiskState,
} from "@/types";

describe("consoleDataRealTrade", () => {
  it("initializes every real-trade slice with its empty contract snapshot", () => {
    const controller = createConsoleDataRealTradeController();

    expect(controller.realTradeApprovals.value).toEqual(emptyRealTradeApprovals);
    expect(controller.realTradeHardStopEvents.value).toEqual(
      emptyRealTradeHardStopEvents,
    );
    expect(controller.realTradeHardStops.value).toEqual(emptyRealTradeHardStops);
    expect(controller.realTradeKillSwitchEvents.value).toEqual(
      emptyRealTradeKillSwitchEvents,
    );
    expect(controller.realTradeKillSwitchState.value).toEqual(
      emptyRealTradeKillSwitchState,
    );
    expect(controller.realTradeRiskEvents.value).toEqual(emptyRealTradeRiskEvents);
    expect(controller.realTradeRiskState.value).toEqual(emptyRealTradeRiskState);
  });

  it("keeps the remaining slices untouched when one slice is replaced", () => {
    const controller = createConsoleDataRealTradeController();

    controller.realTradeRiskState.value = {
      ...emptyRealTradeRiskState,
      realTradingEnabled: true,
      riskEnabled: true,
      effectiveMaxOrderQuantity: 500,
      effectiveMaxOrderNotional: 250_000,
    };
    controller.realTradeKillSwitchState.value = {
      ...emptyRealTradeKillSwitchState,
      realTradingEnabled: true,
      killSwitchActive: true,
      killSwitchSource: "runtime",
    };

    expect(controller.realTradeRiskState.value.effectiveMaxOrderQuantity).toBe(500);
    expect(controller.realTradeKillSwitchState.value.killSwitchActive).toBe(true);
    expect(controller.realTradeApprovals.value).toEqual(emptyRealTradeApprovals);
    expect(controller.realTradeHardStopEvents.value).toEqual(
      emptyRealTradeHardStopEvents,
    );
    expect(controller.realTradeHardStops.value).toEqual(emptyRealTradeHardStops);
    expect(controller.realTradeKillSwitchEvents.value).toEqual(
      emptyRealTradeKillSwitchEvents,
    );
    expect(controller.realTradeRiskEvents.value).toEqual(emptyRealTradeRiskEvents);
  });

  it("does not share live state between controller instances", () => {
    const first = createConsoleDataRealTradeController();
    const second = createConsoleDataRealTradeController();

    first.realTradeApprovals.value = {
      ...emptyRealTradeApprovals,
      realTradingEnabled: true,
      requiredConfirmationText: "I_ACCEPT_REAL_RISK",
    };

    expect(first.realTradeApprovals.value.requiredConfirmationText).toBe(
      "I_ACCEPT_REAL_RISK",
    );
    expect(second.realTradeApprovals.value).toEqual(emptyRealTradeApprovals);
    expect(second.realTradeApprovals.value.realTradingEnabled).toBe(false);
  });
});
