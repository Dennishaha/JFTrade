import { describe, expect, it } from "vitest";

import {
  isADKAgent,
  isADKApproval,
  isADKInputRequest,
  isADKOptimizationTask,
  isADKSkill,
  isADKTimelineEntry,
  isADKToolCall,
  isADKToolDescriptor,
  isADKWorkflowDefinition,
  isADKWorkflowTriggerLog,
  isMCPSettingsSnapshot,
  normalizeTimelineWire,
} from "@/composables/adk/adkApiGuards";

describe("adk api guards wire shapes", () => {
  it("rejects non-record payloads for every ADK entity guard", () => {
    const invalid = [null, "agent", 42, ["tool"]];
    for (const value of invalid) {
      expect(isADKAgent(value)).toBe(false);
      expect(isADKToolDescriptor(value)).toBe(false);
      expect(isADKSkill(value)).toBe(false);
      expect(isADKToolCall(value)).toBe(false);
      expect(isADKApproval(value)).toBe(false);
      expect(isADKInputRequest(value)).toBe(false);
      expect(isADKTimelineEntry(value)).toBe(false);
      expect(isADKWorkflowDefinition(value)).toBe(false);
      expect(isADKWorkflowTriggerLog(value)).toBe(false);
    }
  });

  it("rejects nested payloads that miss their required record sections", () => {
    expect(isADKOptimizationTask({})).toBe(false);
    expect(isMCPSettingsSnapshot({ settings: {} })).toBe(false);
  });

  it("passes non-record values through timeline normalization", () => {
    expect(normalizeTimelineWire(null)).toBeNull();
    expect(normalizeTimelineWire("raw")).toBe("raw");
  });
});
