import type { ADKRun, ADKToolCall } from "@/types";

import type { ADKTimelineEntryState } from "@/composables/adk/adkTimeline";
import { parseTraceTime } from "@/composables/adk/adkToolTracePresentation";

export type ADKTurnTraceSegmentPosition = "only" | "first" | "middle" | "last";

export interface ADKTurnTraceBlock {
  type: "turn_trace";
  key: string;
  runId: string;
  /** Position of this block among the segments of the same run (a run's trace
   * is split when an input_request / approval_group entry interrupts it). */
  segmentPosition: ADKTurnTraceSegmentPosition;
  /** createdAt of the same-run entry that ended this segment (the posed
   * input request / approval group); marks when the segment's work stopped. */
  interruptedAt?: string | undefined;
  entries: ADKTimelineEntryState[];
}

export type ADKThreadItem =
  | { type: "entry"; entry: ADKTimelineEntryState }
  | ADKTurnTraceBlock;

const TURN_TRACE_CORE_KINDS = new Set(["assistant_reasoning", "tool_group"]);
const TURN_TRACE_SEGMENT_KINDS = new Set([
  "assistant_reasoning",
  "tool_group",
  "assistant_message",
]);

/**
 * Groups the consecutive reasoning / tool / intermediate-text entries of one
 * run into a single collapsible turn-trace block. The run's final reply (the
 * trailing non-empty assistant message of the segment) stays outside the
 * block so it keeps rendering as a normal chat bubble.
 */
export function groupTurnTraceEntries(
  entries: ADKTimelineEntryState[],
): ADKThreadItem[] {
  const items: ADKThreadItem[] = [];
  let index = 0;
  while (index < entries.length) {
    const entry = entries[index]!;
    const runId = String(entry.runId ?? "").trim();
    if (runId === "" || !TURN_TRACE_SEGMENT_KINDS.has(entry.kind)) {
      items.push({ type: "entry", entry });
      index += 1;
      continue;
    }
    const segment: ADKTimelineEntryState[] = [];
    while (index < entries.length) {
      const candidate = entries[index]!;
      if (
        String(candidate.runId ?? "").trim() !== runId ||
        !TURN_TRACE_SEGMENT_KINDS.has(candidate.kind)
      ) {
        break;
      }
      segment.push(candidate);
      index += 1;
    }
    // The entry that broke the segment marks when this segment's work stopped
    // (e.g. the input request was posed); only same-run breakers count.
    const breaker = entries[index];
    const interruptedAt =
      breaker && String(breaker.runId ?? "").trim() === runId
        ? breaker.createdAt
        : undefined;
    items.push(...buildRunSegmentItems(runId, segment, interruptedAt));
  }
  annotateSegmentPositions(items);
  return items;
}

function annotateSegmentPositions(items: ADKThreadItem[]): void {
  const blocksByRun = new Map<string, ADKTurnTraceBlock[]>();
  for (const item of items) {
    if (item.type !== "turn_trace") continue;
    const blocks = blocksByRun.get(item.runId) ?? [];
    blocks.push(item);
    blocksByRun.set(item.runId, blocks);
  }
  for (const blocks of blocksByRun.values()) {
    if (blocks.length === 1) {
      blocks[0]!.segmentPosition = "only";
      continue;
    }
    blocks.forEach((block, index) => {
      block.segmentPosition =
        index === 0 ? "first" : index === blocks.length - 1 ? "last" : "middle";
    });
  }
}

function buildRunSegmentItems(
  runId: string,
  segment: ADKTimelineEntryState[],
  interruptedAt?: string,
): ADKThreadItem[] {
  let replyIndex = -1;
  for (let position = segment.length - 1; position >= 0; position -= 1) {
    const entry = segment[position]!;
    if (TURN_TRACE_CORE_KINDS.has(entry.kind)) break;
    if (entry.kind === "assistant_message" && String(entry.text ?? "").trim() !== "") {
      replyIndex = position;
      break;
    }
  }
  const blockEntries = segment.filter((_, position) => position !== replyIndex);
  const hasTraceContent = blockEntries.some(
    (entry) =>
      TURN_TRACE_CORE_KINDS.has(entry.kind) ||
      String(entry.text ?? "").trim() !== "",
  );
  if (!hasTraceContent) {
    return segment.map((entry) => ({ type: "entry" as const, entry }));
  }
  const items: ADKThreadItem[] = [
    {
      type: "turn_trace",
      key: `turn-trace:${runId}:${blockEntries[0]!.id}`,
      runId,
      segmentPosition: "only",
      interruptedAt,
      entries: blockEntries,
    },
  ];
  if (replyIndex >= 0) {
    items.push({ type: "entry", entry: segment[replyIndex]! });
  }
  return items;
}

export function turnTraceBlockRun(block: ADKTurnTraceBlock): ADKRun | undefined {
  for (const entry of block.entries) {
    if (entry.run) return entry.run;
  }
  return undefined;
}

/**
 * Computes the elapsed time of a single turn-trace block. Each segment of a
 * run is timed on its own boundaries so that the time the run spent waiting
 * for user input / approval (which falls between segments) is not counted,
 * and split segments do not all report the whole run's duration.
 */
export function turnTraceElapsedMs(input: {
  block: ADKTurnTraceBlock;
  run?: ADKRun | undefined;
  nowMs: number;
  active: boolean;
}): number | undefined {
  const start = segmentStartMs(input.block, input.run);
  if (start == null) return undefined;
  const position = input.block.segmentPosition;
  if (input.active && (position === "only" || position === "last")) {
    return Math.max(0, input.nowMs - start);
  }
  if (position === "only") {
    const usageDuration = input.run?.usage?.durationMs;
    if (usageDuration != null && Number.isFinite(usageDuration)) return usageDuration;
    const runStart = parseTraceTime(input.run?.startedAt ?? input.run?.createdAt);
    const runEnd = parseTraceTime(input.run?.completedAt ?? input.run?.updatedAt);
    if (runStart != null && runEnd != null && runEnd >= runStart) {
      return runEnd - runStart;
    }
  }
  const endCandidates: number[] = [];
  if (position === "last") {
    const runEnd = parseTraceTime(input.run?.completedAt ?? input.run?.updatedAt);
    if (runEnd != null) endCandidates.push(runEnd);
  }
  const segmentEnd = segmentEndMs(input.block);
  if (segmentEnd != null) endCandidates.push(segmentEnd);
  if (endCandidates.length === 0) return undefined;
  const end = Math.max(...endCandidates);
  return end >= start ? end - start : undefined;
}

function segmentStartMs(block: ADKTurnTraceBlock, run?: ADKRun): number | null {
  const candidates = block.entries
    .map((entry) => parseTraceTime(entry.createdAt))
    .filter((value): value is number => value != null);
  if (block.segmentPosition === "only" || block.segmentPosition === "first") {
    const runStart = parseTraceTime(run?.startedAt ?? run?.createdAt);
    if (runStart != null) candidates.push(runStart);
  }
  return candidates.length > 0 ? Math.min(...candidates) : null;
}

/**
 * Latest timestamp known inside the segment. Entry.updatedAt is often just
 * the entry's creation time, so tool-call completion times and the timestamp
 * of the interrupting entry (when the question/approval was posed) must be
 * considered as well, otherwise the segment end is reported too early.
 */
function segmentEndMs(block: ADKTurnTraceBlock): number | null {
  const ends: number[] = [];
  const interruptedAt = parseTraceTime(block.interruptedAt);
  if (interruptedAt != null) ends.push(interruptedAt);
  for (const entry of block.entries) {
    const entryEnd = parseTraceTime(entry.updatedAt ?? entry.createdAt);
    if (entryEnd != null) ends.push(entryEnd);
    for (const toolCall of entry.toolCalls ?? []) {
      const toolEnd = toolCallEndMs(toolCall);
      if (toolEnd != null) ends.push(toolEnd);
    }
  }
  return ends.length > 0 ? Math.max(...ends) : null;
}

function toolCallEndMs(toolCall: ADKToolCall): number | null {
  const completedAt = parseTraceTime(toolCall.completedAt);
  if (completedAt != null) return completedAt;
  const startedAt = parseTraceTime(toolCall.startedAt);
  if (startedAt != null && toolCall.durationMs != null && Number.isFinite(toolCall.durationMs)) {
    return startedAt + toolCall.durationMs;
  }
  return parseTraceTime(toolCall.updatedAt ?? toolCall.createdAt);
}
