import assert from "node:assert/strict";
import test from "node:test";

import { compareAssistantBudget } from "./check-assistant-budget.mjs";

test("assistant budget is a downward-only ratchet", () => {
  assert.deepEqual(compareAssistantBudget({
    engineProductionLines: 11,
    engineTestLines: 12,
    externalDependencyFiles: 13,
  }, {
    engineProductionLinesMax: 10,
    engineTestLinesMax: 11,
    externalDependencyFilesMax: 12,
  }), [
    "engine production lines 11 exceed budget 10",
    "engine test lines 12 exceed budget 11",
    "external engine dependency files 13 exceed budget 12",
  ]);
});

test("assistant budget rejects missing or invalid dimensions", () => {
  assert.deepEqual(compareAssistantBudget({ engineProductionLines: 0, engineTestLines: 0, externalDependencyFiles: 0 }, {}), [
    "engineProductionLinesMax must be a non-negative integer",
    "engineTestLinesMax must be a non-negative integer",
    "externalDependencyFilesMax must be a non-negative integer",
  ]);
});
