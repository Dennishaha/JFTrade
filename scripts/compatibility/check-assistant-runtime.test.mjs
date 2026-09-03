import assert from "node:assert/strict";
import test from "node:test";

import { assertAssistantRuntimeEquivalent } from "./check-assistant-runtime.mjs";

const minimal = {
  rig: { recordTelemetryContent: false },
  approval: { replayResolutionChanged: false },
  input: { replayResolutionChanged: false },
  claims: { outcomeUnknownError: "TOOL_OUTCOME_UNKNOWN" },
  provider: { attempts: 2 },
};

test("Assistant runtime accepts an identical compatibility projection", () => {
  assert.doesNotThrow(() => assertAssistantRuntimeEquivalent(structuredClone(minimal), minimal));
});

test("Assistant runtime rejects a duplicated approval continuation", () => {
  const drifted = structuredClone(minimal);
  drifted.approval.replayResolutionChanged = true;
  assert.throws(() => assertAssistantRuntimeEquivalent(drifted, minimal));
});
