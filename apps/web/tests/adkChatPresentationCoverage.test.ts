import { describe, expect, it } from "vitest";

import type { ADKRun } from "../src/contracts";
import {
  firstFailedToolCall,
  runErrorSummary,
  toolCallErrorSummary,
} from "../src/composables/adkChatPresentation";

describe("adkChatPresentation failure presentation", () => {
  it("keeps tool-level timeout, cancellation, and generic failures distinguishable", () => {
    expect(firstFailedToolCall(undefined)).toBeUndefined();
    expect(toolCallErrorSummary(toolCall("TIMED_OUT"))).toBe("Tool execution timed out");
    expect(toolCallErrorSummary(toolCall("CANCELLED"))).toBe("Tool execution was cancelled");
    expect(toolCallErrorSummary(toolCall("FAILED"))).toBe("Tool execution failed");
  });

  it("preserves an unknown operational failure without inventing a misleading cause", () => {
    const summary = runErrorSummary(buildRun({
      status: "SUSPENDED_BY_PROVIDER",
      errorCode: "UNCLASSIFIED_PROVIDER_STATE",
      message: "",
    }));

    expect(summary).toMatchObject({
      title: "运行异常",
      detail: "",
      tone: "muted",
      code: "UNCLASSIFIED_PROVIDER_STATE",
    });
  });

  it("truncates oversized provider diagnostics before they can overwhelm the run trace", () => {
    const detail = `provider diagnostic ${"x".repeat(420)}`;
    const summary = runErrorSummary(buildRun({
      status: "FAILED",
      failureReason: detail,
    }));

    expect(summary?.detail).toHaveLength(360);
    expect(summary?.detail.endsWith("...")).toBe(true);
  });
});

function toolCall(status: string) {
  return {
    id: `tool-${status}`,
    runId: "run-1",
    toolName: "strategy.save_definition",
    permission: "write",
    status,
    requiresUser: false,
    createdAt: "2026-07-16T00:00:00Z",
    updatedAt: "2026-07-16T00:00:00Z",
  };
}

function buildRun(overrides: Partial<ADKRun> = {}): ADKRun {
  return {
    id: "run-1",
    sessionId: "session-1",
    agentId: "agent-1",
    status: "FAILED",
    message: "",
    toolCalls: [],
    pendingApprovals: [],
    createdAt: "2026-07-16T00:00:00Z",
    updatedAt: "2026-07-16T00:00:01Z",
    ...overrides,
  };
}
