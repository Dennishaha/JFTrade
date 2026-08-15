import { describe, expect, it } from "vitest";

import type { ADKRun, ADKTimelineEntry } from "../../src/types";
import { createTimelineEntryState } from "@/composables/adk/adkTimeline";
import {
  groupTurnTraceEntries,
  turnTraceBlockRun,
  turnTraceElapsedMs,
  type ADKTurnTraceBlock,
} from "@/composables/adk/adkTurnTraceGrouping";

function entry(
  overrides: Partial<ADKTimelineEntry> & { id: string; kind: string },
) {
  return createTimelineEntryState({
    sessionId: "session-1",
    createdAt: "2026-08-15T00:00:00Z",
    sequence: 0,
    ...overrides,
  } as ADKTimelineEntry);
}

function run(overrides: Partial<ADKRun>): ADKRun {
  return {
    id: "run-1",
    sessionId: "session-1",
    agentId: "agent-1",
    status: "COMPLETED",
    message: "",
    toolCalls: [],
    pendingApprovals: [],
    createdAt: "2026-08-15T00:00:00Z",
    updatedAt: "2026-08-15T00:02:31Z",
    ...overrides,
  };
}

function turnTraceBlocks(items: ReturnType<typeof groupTurnTraceEntries>) {
  return items.filter(
    (item): item is ADKTurnTraceBlock => item.type === "turn_trace",
  );
}

describe("adkTurnTraceGrouping", () => {
  it("groups one run's reasoning, tools and intermediate text into a single block", () => {
    const items = groupTurnTraceEntries([
      entry({ id: "user-1", kind: "user_message", runId: "run-1", text: "查一下 AAPL" }),
      entry({ id: "reasoning-1", kind: "assistant_reasoning", runId: "run-1", text: "先看行情" }),
      entry({ id: "text-1", kind: "assistant_message", runId: "run-1", text: "我先查行情" }),
      entry({ id: "tool-1", kind: "tool_group", runId: "run-1" }),
      entry({ id: "reply-1", kind: "assistant_message", runId: "run-1", text: "AAPL 现价…" }),
    ]);

    expect(items).toHaveLength(3);
    expect(items[0]).toMatchObject({ type: "entry", entry: { id: "user-1" } });
    expect(items[1]).toMatchObject({
      type: "turn_trace",
      runId: "run-1",
      segmentPosition: "only",
    });
    const block = items[1];
    if (block?.type !== "turn_trace") throw new Error("expected turn trace block");
    expect(block.entries.map((item) => item.id)).toEqual([
      "reasoning-1",
      "text-1",
      "tool-1",
    ]);
    expect(items[2]).toMatchObject({ type: "entry", entry: { id: "reply-1" } });
  });

  it("keeps the whole segment inside the block while tools are still streaming", () => {
    const items = groupTurnTraceEntries([
      entry({ id: "reasoning-1", kind: "assistant_reasoning", runId: "run-1", text: "…" }),
      entry({ id: "tool-1", kind: "tool_group", runId: "run-1", status: "streaming" }),
    ]);

    expect(items).toHaveLength(1);
    expect(items[0]).toMatchObject({ type: "turn_trace", runId: "run-1" });
  });

  it("does not create a block for a bare assistant reply", () => {
    const items = groupTurnTraceEntries([
      entry({ id: "reply-1", kind: "assistant_message", runId: "run-1", text: "你好" }),
    ]);

    expect(items).toEqual([
      { type: "entry", entry: expect.objectContaining({ id: "reply-1" }) },
    ]);
  });

  it("splits blocks by run and leaves loose entries untouched", () => {
    const items = groupTurnTraceEntries([
      entry({ id: "loose-reasoning", kind: "assistant_reasoning", text: "无 runId" }),
      entry({ id: "tool-a", kind: "tool_group", runId: "run-a" }),
      entry({ id: "reply-a", kind: "assistant_message", runId: "run-a", text: "A 完成" }),
      entry({ id: "tool-b", kind: "tool_group", runId: "run-b" }),
      entry({ id: "reply-b", kind: "assistant_message", runId: "run-b", text: "B 完成" }),
    ]);

    expect(items.map((item) => item.type)).toEqual([
      "entry",
      "turn_trace",
      "entry",
      "turn_trace",
      "entry",
    ]);
    const [, first, , second] = items;
    if (first?.type !== "turn_trace" || second?.type !== "turn_trace") {
      throw new Error("expected two turn trace blocks");
    }
    expect(first.runId).toBe("run-a");
    expect(first.segmentPosition).toBe("only");
    expect(second.runId).toBe("run-b");
    expect(second.segmentPosition).toBe("only");
  });

  it("splits a run's block around interrupting non-trace entries to keep chronology", () => {
    const items = groupTurnTraceEntries([
      entry({ id: "tool-1", kind: "tool_group", runId: "run-1" }),
      entry({ id: "input-1", kind: "input_request", runId: "run-1" }),
      entry({ id: "tool-2", kind: "tool_group", runId: "run-1" }),
      entry({ id: "reply-1", kind: "assistant_message", runId: "run-1", text: "完成" }),
    ]);

    expect(items.map((item) => item.type)).toEqual([
      "turn_trace",
      "entry",
      "turn_trace",
      "entry",
    ]);
    const blocks = turnTraceBlocks(items);
    expect(blocks.map((block) => block.segmentPosition)).toEqual(["first", "last"]);
  });

  it("marks first, middle and last when a run is interrupted twice", () => {
    const items = groupTurnTraceEntries([
      entry({ id: "tool-1", kind: "tool_group", runId: "run-1" }),
      entry({ id: "input-1", kind: "input_request", runId: "run-1" }),
      entry({ id: "tool-2", kind: "tool_group", runId: "run-1" }),
      entry({ id: "approval-1", kind: "approval_group", runId: "run-1" }),
      entry({ id: "tool-3", kind: "tool_group", runId: "run-1" }),
      entry({ id: "reply-1", kind: "assistant_message", runId: "run-1", text: "完成" }),
    ]);

    const blocks = turnTraceBlocks(items);
    expect(blocks).toHaveLength(3);
    expect(blocks.map((block) => block.segmentPosition)).toEqual([
      "first",
      "middle",
      "last",
    ]);
  });

  it("exposes the attached run snapshot for block headers", () => {
    const toolEntry = entry({ id: "tool-1", kind: "tool_group", runId: "run-1" });
    toolEntry.run = {
      id: "run-1",
      sessionId: "session-1",
      agentId: "agent-1",
      status: "COMPLETED",
      message: "",
      toolCalls: [],
      pendingApprovals: [],
      createdAt: "2026-08-15T00:00:00Z",
      updatedAt: "2026-08-15T00:00:02Z",
    };
    const items = groupTurnTraceEntries([
      toolEntry,
      entry({ id: "reply-1", kind: "assistant_message", runId: "run-1", text: "完成" }),
    ]);

    const block = items[0];
    if (block?.type !== "turn_trace") throw new Error("expected turn trace block");
    expect(turnTraceBlockRun(block)?.status).toBe("COMPLETED");
  });

  it("times interrupted segments on their own boundaries, excluding the wait", () => {
    const items = groupTurnTraceEntries([
      entry({
        id: "reasoning-1",
        kind: "assistant_reasoning",
        runId: "run-1",
        text: "…",
        createdAt: "2026-08-15T00:00:01Z",
      }),
      entry({
        id: "tool-1",
        kind: "tool_group",
        runId: "run-1",
        createdAt: "2026-08-15T00:00:05Z",
        updatedAt: "2026-08-15T00:00:40Z",
      }),
      entry({
        id: "input-1",
        kind: "input_request",
        runId: "run-1",
        createdAt: "2026-08-15T00:00:41Z",
      }),
      entry({
        id: "tool-2",
        kind: "tool_group",
        runId: "run-1",
        createdAt: "2026-08-15T00:02:00Z",
        updatedAt: "2026-08-15T00:02:20Z",
      }),
      entry({
        id: "reply-1",
        kind: "assistant_message",
        runId: "run-1",
        text: "完成",
        createdAt: "2026-08-15T00:02:25Z",
      }),
    ]);
    const [first, last] = turnTraceBlocks(items);
    const snapshot = run({
      startedAt: "2026-08-15T00:00:00Z",
      completedAt: "2026-08-15T00:02:31Z",
    });

    // First segment: run start -> the posed input request, excluding the wait.
    // (Entry.updatedAt stops at 40s, but the interrupting entry marks 41s.)
    expect(
      turnTraceElapsedMs({ block: first!, run: snapshot, nowMs: 0, active: false }),
    ).toBe(41_000);
    // Last segment: its own first entry -> run completion.
    expect(
      turnTraceElapsedMs({ block: last!, run: snapshot, nowMs: 0, active: false }),
    ).toBe(31_000);
  });

  it("ticks only the tail segment while the run is active", () => {
    const items = groupTurnTraceEntries([
      entry({
        id: "tool-1",
        kind: "tool_group",
        runId: "run-1",
        createdAt: "2026-08-15T00:00:05Z",
        updatedAt: "2026-08-15T00:00:40Z",
      }),
      entry({ id: "input-1", kind: "input_request", runId: "run-1" }),
      entry({
        id: "tool-2",
        kind: "tool_group",
        runId: "run-1",
        createdAt: "2026-08-15T00:02:00Z",
      }),
    ]);
    const [first, last] = turnTraceBlocks(items);
    const snapshot = run({
      status: "RUNNING",
      startedAt: "2026-08-15T00:00:00Z",
    });
    const nowMs = Date.parse("2026-08-15T00:02:10Z");

    expect(
      turnTraceElapsedMs({ block: first!, run: snapshot, nowMs, active: true }),
    ).toBe(40_000);
    expect(
      turnTraceElapsedMs({ block: last!, run: snapshot, nowMs, active: true }),
    ).toBe(10_000);
  });

  it("counts tool-call completion times when entry timestamps stop early", () => {
    const items = groupTurnTraceEntries([
      entry({
        id: "tool-1",
        kind: "tool_group",
        runId: "run-1",
        createdAt: "2026-08-15T00:00:05Z",
        toolCalls: [
          {
            id: "call-1",
            runId: "run-1",
            toolName: "market.candles",
            permission: "read",
            status: "SUCCEEDED",
            requiresUser: false,
            createdAt: "2026-08-15T00:00:05Z",
            startedAt: "2026-08-15T00:00:05Z",
            updatedAt: "2026-08-15T00:00:05Z",
            completedAt: "2026-08-15T00:00:35Z",
            durationMs: 30_000,
          },
        ],
      }),
      entry({
        id: "input-1",
        kind: "input_request",
        runId: "run-1",
        createdAt: "2026-08-15T00:00:36Z",
      }),
      entry({
        id: "tool-2",
        kind: "tool_group",
        runId: "run-1",
        createdAt: "2026-08-15T00:02:00Z",
      }),
    ]);
    const [first] = turnTraceBlocks(items);
    const snapshot = run({ startedAt: "2026-08-15T00:00:00Z" });

    // Entry timestamps stop at 5s, the tool call completes at 35s and the
    // question is posed at 36s — the segment must not stop at 5s.
    expect(
      turnTraceElapsedMs({ block: first!, run: snapshot, nowMs: 0, active: false }),
    ).toBe(36_000);
  });

  it("keeps empty intermediate assistant text inside the block", () => {
    const items = groupTurnTraceEntries([
      entry({ id: "msg-leading", kind: "assistant_message", runId: "run-1" }),
      entry({ id: "tool-1", kind: "tool_group", runId: "run-1" }),
      entry({ id: "msg-trailing", kind: "assistant_message", runId: "run-1", text: "   " }),
    ]);

    const blocks = turnTraceBlocks(items);
    expect(blocks).toHaveLength(1);
    expect(blocks[0]!.entries.map((item) => item.id)).toEqual([
      "msg-leading",
      "tool-1",
      "msg-trailing",
    ]);
  });

  it("derives a tool call's end from its start and duration when it never completed", () => {
    const items = groupTurnTraceEntries([
      entry({
        id: "tool-1",
        kind: "tool_group",
        runId: "run-1",
        createdAt: "2026-08-15T00:00:05Z",
        updatedAt: "2026-08-15T00:00:05Z",
        toolCalls: [
          {
            id: "call-duration",
            runId: "run-1",
            toolName: "market.candles",
            permission: "read",
            status: "SUCCEEDED",
            requiresUser: false,
            createdAt: "2026-08-15T00:00:05Z",
            startedAt: "2026-08-15T00:00:05Z",
            updatedAt: "2026-08-15T00:00:05Z",
            durationMs: 30_000,
          },
          {
            id: "call-fallback-updated",
            runId: "run-1",
            toolName: "market.snapshot",
            permission: "read",
            status: "SUCCEEDED",
            requiresUser: false,
            createdAt: "2026-08-15T00:00:06Z",
            startedAt: "2026-08-15T00:00:06Z",
            updatedAt: "2026-08-15T00:00:10Z",
          },
          {
            id: "call-fallback-created",
            runId: "run-1",
            toolName: "market.snapshot",
            permission: "read",
            status: "SUCCEEDED",
            requiresUser: false,
            createdAt: "2026-08-15T00:00:07Z",
          },
        ],
      }),
      entry({
        id: "input-1",
        kind: "input_request",
        runId: "run-1",
        createdAt: "2026-08-15T00:00:20Z",
      }),
      entry({
        id: "tool-2",
        kind: "tool_group",
        runId: "run-1",
        createdAt: "2026-08-15T00:02:00Z",
      }),
    ]);
    const [first] = turnTraceBlocks(items);
    const snapshot = run({ startedAt: "2026-08-15T00:00:00Z" });

    // The uncompleted call ends at startedAt + durationMs (35s), later than
    // both the entry timestamps (5s) and the posed question (20s).
    expect(
      turnTraceElapsedMs({ block: first!, run: snapshot, nowMs: 0, active: false }),
    ).toBe(35_000);
  });

  it("reports no elapsed time when a segment has no usable timestamps", () => {
    const items = groupTurnTraceEntries([
      entry({ id: "tool-1", kind: "tool_group", runId: "run-1", createdAt: undefined }),
      entry({ id: "input-1", kind: "input_request", runId: "run-1" }),
      entry({ id: "tool-2", kind: "tool_group", runId: "run-1", createdAt: undefined }),
      entry({ id: "input-2", kind: "input_request", runId: "run-1" }),
      entry({ id: "tool-3", kind: "tool_group", runId: "run-1", createdAt: undefined }),
    ]);
    const [, middle] = turnTraceBlocks(items);

    expect(middle!.segmentPosition).toBe("middle");
    expect(
      turnTraceElapsedMs({ block: middle!, run: undefined, nowMs: 0, active: false }),
    ).toBeUndefined();
  });

  it("reports no elapsed time when neither run nor segment expose an end", () => {
    const items = groupTurnTraceEntries([
      entry({ id: "tool-1", kind: "tool_group", runId: "run-1" }),
      entry({ id: "input-1", kind: "input_request", runId: "run-1" }),
      entry({
        id: "tool-2",
        kind: "tool_group",
        runId: "run-1",
        createdAt: "2026-08-15T00:02:00Z",
        updatedAt: "corrupted-timestamp",
      }),
    ]);
    const [, last] = turnTraceBlocks(items);
    const snapshot = run({
      startedAt: "2026-08-15T00:00:00Z",
      completedAt: undefined,
      updatedAt: undefined,
    });

    expect(last!.segmentPosition).toBe("last");
    expect(
      turnTraceElapsedMs({ block: last!, run: snapshot, nowMs: 0, active: false }),
    ).toBeUndefined();
  });

  it("prefers the run usage duration for an uninterrupted run", () => {
    const items = groupTurnTraceEntries([
      entry({
        id: "tool-1",
        kind: "tool_group",
        runId: "run-1",
        createdAt: "2026-08-15T00:00:05Z",
        updatedAt: "2026-08-15T00:00:40Z",
      }),
    ]);
    const [block] = turnTraceBlocks(items);
    const snapshot = run({
      startedAt: "2026-08-15T00:00:00Z",
      completedAt: "2026-08-15T00:01:00Z",
      usage: { durationMs: 4200 },
    });

    expect(block!.segmentPosition).toBe("only");
    expect(
      turnTraceElapsedMs({ block: block!, run: snapshot, nowMs: 0, active: false }),
    ).toBe(4200);
  });
});
