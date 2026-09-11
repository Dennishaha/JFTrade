import { describe, expect, test } from "vitest";
import { incrementalResult, resultMarker } from "./pinetsResult";
import type { OrderIntentCapture } from "./pinetsOrderIntents";
import type { PineTSRunResult } from "./types";

const capture = (): OrderIntentCapture => ({
  supported: true,
  intents: [],
  previous: [],
});

function markerFor(result: PineTSRunResult) {
  return resultMarker(result, capture());
}

function deltaFor(result: PineTSRunResult, marker = markerFor(result)): PineTSRunResult {
  return incrementalResult(result, capture(), marker, false);
}

describe("PineTS drawing deltas", () => {
  test("emits a drawing appended after the live-session marker", () => {
    const first = { id: "entry", kind: "label", text: "Long" };
    const result: PineTSRunResult = { drawings: [first] };
    const marker = markerFor(result);
    result.drawings = [first, { id: "exit", kind: "label", text: "Flat" }];

    expect(deltaFor(result, marker).drawings).toEqual([
      { id: "exit", kind: "label", text: "Flat" },
    ]);
  });

  test("emits an in-place update when an identified drawing changes", () => {
    const drawing = { id: "entry", kind: "label", text: "Long", color: "green" };
    const result: PineTSRunResult = { drawings: [drawing] };
    const marker = markerFor(result);
    drawing.text = "Long (filled)";

    expect(deltaFor(result, marker).drawings).toEqual([
      { id: "entry", kind: "label", text: "Long (filled)", color: "green" },
    ]);
  });

  test("emits a replacement when a fixed-size drawing array changes", () => {
    const result: PineTSRunResult = {
      drawings: [{ id: "slot-0", value: "old" }],
    };
    const marker = markerFor(result);
    result.drawings = [{ id: "slot-0", value: "new" }];

    expect(deltaFor(result, marker).drawings).toEqual([
      { id: "slot-0", value: "new" },
    ]);
  });

  test("emits a tombstone when an identified drawing disappears", () => {
    const result: PineTSRunResult = {
      drawings: [{ id: "entry", name: "entry-label", kind: "label", text: "Long" }],
    };
    const marker = markerFor(result);
    result.drawings = [];

    expect(deltaFor(result, marker).drawings).toEqual([{
      id: "entry",
      name: "entry-label",
      kind: "label",
      deleted: true,
      tombstone: true,
      operation: "delete",
    }]);
  });

  test("compares nested and multibyte payloads structurally", () => {
    const drawing = { id: "note", payload: { text: "初始值", style: { size: 12 } } };
    const result: PineTSRunResult = { drawings: [drawing] };
    const marker = markerFor(result);
    drawing.payload.text = "更新后的中文内容";
    // Reordering object keys should not itself be treated as a mutation.
    drawing.payload = { style: { size: 12 }, text: "更新后的中文内容" };

    expect(deltaFor(result, marker).drawings).toEqual([drawing]);
  });

  test("keeps the append-only fallback for legacy markers without snapshots", () => {
    const marker = {
      intentCount: 0,
      plotLengths: {},
      alertCount: 0,
      visualCount: 0,
      drawingCount: 1,
      logCount: 0,
      warningCount: 0,
      diagnosticCount: 0,
    };
    const result: PineTSRunResult = {
      drawings: [{ id: "old" }, { id: "new" }],
    };

    expect(deltaFor(result, marker).drawings).toEqual([{ id: "new" }]);
  });
});
