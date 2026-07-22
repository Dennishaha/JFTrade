import { describe, expect, it } from "vitest";

import {
  LEGACY_GO_PINE_RUNTIME,
  PINE_WORKER_RUNTIME,
  formatSourceFormat,
  formatStrategyEligibility,
  formatStrategyRuntime,
  isLegacyGoPineRuntime,
  isSupportedPineRuntime,
} from "../src/components/strategy-runtime/strategyRuntimeIdentity";
import type { StrategyInstanceItem } from "../src/contracts";

describe("strategy runtime identity", () => {
  it("keeps legacy Go Pine IDs as migration aliases instead of supported runtimes", () => {
    expect(formatStrategyRuntime(PINE_WORKER_RUNTIME)).toBe("PineTS worker");
    expect(formatStrategyRuntime(LEGACY_GO_PINE_RUNTIME)).toBe(
      "PineTS migration alias",
    );
    expect(isLegacyGoPineRuntime(LEGACY_GO_PINE_RUNTIME)).toBe(true);
    expect(isSupportedPineRuntime(LEGACY_GO_PINE_RUNTIME)).toBe(false);
    expect(formatSourceFormat("legacy")).toBe("未知 / 受限");
  });

  it("does not present legacy aliases as startable runtime support", () => {
    const strategy = {
      runtime: LEGACY_GO_PINE_RUNTIME,
      startable: false,
    } as StrategyInstanceItem;

    expect(formatStrategyEligibility(strategy)).toBe("受限");
  });
});
