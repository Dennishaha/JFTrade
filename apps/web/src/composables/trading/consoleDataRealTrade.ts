import { ref } from "vue";

import {
  emptyRealTradeApprovals,
  emptyRealTradeHardStopEvents,
  emptyRealTradeHardStops,
  emptyRealTradeKillSwitchEvents,
  emptyRealTradeKillSwitchState,
  emptyRealTradeRiskEvents,
  emptyRealTradeRiskState,
} from "@/types";
import type {
  RealTradeApprovalsResponse,
  RealTradeHardStopEventsResponse,
  RealTradeHardStopsResponse,
  RealTradeKillSwitchEventsResponse,
  RealTradeKillSwitchStateResponse,
  RealTradeRiskEventsResponse,
  RealTradeRiskStateResponse,
} from "@/contracts";

export function createConsoleDataRealTradeController() {
  const realTradeApprovals = ref<RealTradeApprovalsResponse>(
    emptyRealTradeApprovals,
  );
  const realTradeHardStopEvents = ref<RealTradeHardStopEventsResponse>(
    emptyRealTradeHardStopEvents,
  );
  const realTradeHardStops = ref<RealTradeHardStopsResponse>(
    emptyRealTradeHardStops,
  );
  const realTradeKillSwitchState = ref<RealTradeKillSwitchStateResponse>(
    emptyRealTradeKillSwitchState,
  );
  const realTradeKillSwitchEvents = ref<RealTradeKillSwitchEventsResponse>(
    emptyRealTradeKillSwitchEvents,
  );
  const realTradeRiskState = ref<RealTradeRiskStateResponse>(
    emptyRealTradeRiskState,
  );
  const realTradeRiskEvents = ref<RealTradeRiskEventsResponse>(
    emptyRealTradeRiskEvents,
  );

  return {
    realTradeApprovals,
    realTradeHardStopEvents,
    realTradeHardStops,
    realTradeKillSwitchEvents,
    realTradeKillSwitchState,
    realTradeRiskEvents,
    realTradeRiskState,
  };
}
