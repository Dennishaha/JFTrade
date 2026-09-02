import assert from "node:assert/strict";
import test from "node:test";

import { assertStage6Equivalent } from "./check-stage6-differential.mjs";

const minimal = {
  rig: { recordTelemetryContent: false },
  approval: { replayResolutionChanged: false },
  input: { replayResolutionChanged: false },
  claims: { outcomeUnknownError: "TOOL_OUTCOME_UNKNOWN" },
  provider: { attempts: 2 },
};

test("Stage 6 comparison accepts an identical Assistant projection", () => {
  assert.doesNotThrow(() => assertStage6Equivalent(structuredClone(minimal), minimal));
});

test("Stage 6 comparison rejects a duplicated approval continuation", () => {
  const drifted = structuredClone(minimal);
  drifted.approval.replayResolutionChanged = true;
  assert.throws(() => assertStage6Equivalent(drifted, minimal));
});
