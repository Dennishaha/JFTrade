import { describe, expect, it } from "vitest";

import {
  normalizeADKApprovalResolution,
  normalizeADKChatResponse,
  normalizeADKRun,
  normalizeADKRunList,
  normalizeADKTimelineEntries,
  normalizeADKTimelineEntry,
} from "../src/composables/adkNormalization";

describe("ADK normalization", () => {
  it("clones optional run data without retaining mutable API payload arrays", () => {
    const run = {
      id: "run-1",
      toolCalls: null,
      pendingApprovals: null,
      inputRequest: {
        questions: [{ id: "q-1", options: ["yes"] }],
        answers: ["original"],
      },
      inputRequests: [{ questions: [{ id: "q-2", options: ["no"] }], answers: ["pending"] }],
      childRunIds: null,
      workflowPlan: [{ id: "step-1", dependsOn: null, routes: null }],
    } as never;

    const normalized = normalizeADKRun(run);
    expect(normalized.toolCalls).toEqual([]);
    expect(normalized.pendingApprovals).toEqual([]);
    expect(normalized.childRunIds).toEqual([]);
    expect(normalized.inputRequest?.questions[0]?.options).toEqual(["yes"]);
    expect(normalized.inputRequests?.[0]?.answers).toEqual(["pending"]);
    expect(normalized.workflowPlan?.[0]?.dependsOn).toEqual([]);
    expect(normalized.workflowPlan?.[0]?.routes).toEqual([]);

    (run as { inputRequest: { answers: string[] } }).inputRequest.answers.push("mutated");
    expect(normalized.inputRequest?.answers).toEqual(["original"]);
  });

  it("normalizes nested timeline, response, resolution, and null list payloads", () => {
    const timeline = {
      id: "event-1",
      toolCalls: null,
      approvals: null,
      inputRequest: { questions: [{ options: null }], answers: null },
    } as never;
    const run = { id: "run-2", toolCalls: [], pendingApprovals: [] } as never;
    const entry = normalizeADKTimelineEntry(timeline);
    expect(entry.toolCalls).toEqual([]);
    expect(entry.approvals).toEqual([]);
    expect(entry.inputRequest?.questions[0]?.options).toEqual([]);
    expect(entry.inputRequest?.answers).toEqual([]);

    const response = normalizeADKChatResponse({
      run,
      pendingApprovals: null,
      timeline: [timeline],
    } as never);
    expect(response.pendingApprovals).toEqual([]);
    expect(response.timeline).toHaveLength(1);
    expect(normalizeADKRunList(null)).toEqual([]);
    expect(normalizeADKTimelineEntries(null)).toEqual([]);

    const resolution = normalizeADKApprovalResolution({ run, parentRun: run } as never);
    expect(resolution.run).not.toBe(run);
    expect(resolution.parentRun).not.toBe(run);
  });
});
