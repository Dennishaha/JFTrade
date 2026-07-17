import { describe, expect, it } from "vitest";

import type { ADKApproval, ADKRun, ADKTimelineEntry } from "../src/contracts";
import {
  applyApprovalResolutions,
  buildTimelineRun,
  replaceTimelineEntries,
  sortTimelineEntries,
  upsertTimelineEntry,
} from "../src/composables/adkTimeline";

describe("adkTimeline", () => {
  it("uses session history run snapshots when rebuilding timeline runs", () => {
    const run: ADKRun = {
      id: "run-history",
      sessionId: "session-1",
      agentId: "agent-1",
      providerId: "provider-1",
      providerName: "Provider At Run Time",
      model: "model-at-run-time",
      status: "COMPLETED",
      message: "completed",
      toolCalls: [],
      pendingApprovals: [],
      createdAt: "2026-06-17T00:00:00Z",
      updatedAt: "2026-06-17T00:00:01Z",
    };
    const entries: ADKTimelineEntry[] = [
      {
        id: "tool-group-1",
        sessionId: "session-1",
        runId: "run-history",
        kind: "tool_group",
        createdAt: "2026-06-17T00:00:01Z",
        sequence: 1,
        toolCalls: [],
      },
    ];

    const timeline = replaceTimelineEntries(entries, [], new Map([[run.id, run]]));
    const rebuilt = buildTimelineRun(timeline[0]!);

    expect(rebuilt.providerName).toBe("Provider At Run Time");
    expect(rebuilt.model).toBe("model-at-run-time");
  });

  it("updates context notice entries by id", () => {
    const started: ADKTimelineEntry = {
      id: "notice-1",
      sessionId: "session-1",
      kind: "context_notice",
      createdAt: "2026-06-17T00:00:00Z",
      sequence: 1,
      status: "streaming",
      text: "正在压缩上下文...",
    };
    const completed: ADKTimelineEntry = {
      ...started,
      status: "final",
      text: "已压缩上下文，继续使用最新摘要。",
      updatedAt: "2026-06-17T00:00:01Z",
    };

    const timeline = upsertTimelineEntry(
      upsertTimelineEntry([], started),
      completed,
    );

    expect(timeline).toHaveLength(1);
    expect(timeline[0]?.kind).toBe("context_notice");
    expect(timeline[0]?.status).toBe("final");
    expect(timeline[0]?.text).toBe("已压缩上下文，继续使用最新摘要。");
  });

  it("keeps user prompt view on original when replay updates arrive", () => {
    const original: ADKTimelineEntry = {
      id: "entry-user-prompt",
      sessionId: "session-1",
      runId: "run-1",
      kind: "user_message",
      createdAt: "2026-06-18T00:00:00Z",
      sequence: 1,
      text: "编写个适合nvda的策略",
      originalText: "编写个适合nvda的策略",
      processedText: "请推进这个目标。\n总体目标：编写个适合nvda的策略\n用户请求：编写个适合nvda的策略",
    };
    const replayed: ADKTimelineEntry = {
      ...original,
      status: "final",
    };

    const timeline = upsertTimelineEntry(upsertTimelineEntry([], original), replayed);

    expect(timeline).toHaveLength(1);
    expect(timeline[0]?.text).toBe("编写个适合nvda的策略");
    expect(timeline[0]?.originalText).toBe("编写个适合nvda的策略");
    expect(timeline[0]?.processedText).toContain("请推进这个目标");
    expect(timeline[0]?.userPromptVariant).toBe("original");
  });

  it("uses stable fallback ordering and derives every visible tool-group terminal state", () => {
    const sorted = sortTimelineEntries([
      timelineEntry({ id: "later", createdAt: "z-not-a-date", sequence: 3 }),
      timelineEntry({ id: "earlier", createdAt: "a-not-a-date", sequence: 3 }),
    ]);
    expect(sorted.map((entry) => entry.id)).toEqual(["earlier", "later"]);

    const cases = [
      ["PENDING_APPROVAL", "PENDING_APPROVAL"],
      ["RUNNING", "RUNNING"],
      ["FAILED", "FAILED"],
      ["DENIED", "DENIED"],
      ["CANCELLED", "CANCELLED"],
    ] as const;
    for (const [toolStatus, expectedRunStatus] of cases) {
      const run = buildTimelineRun({
        ...timelineEntry({ id: `tool-${toolStatus}` }),
        kind: "tool_group",
        toolCalls: [toolCall(toolStatus)],
      });
      expect(run.status).toBe(expectedRunStatus);
      expect(run.toolCalls).toHaveLength(1);
    }
  });

  it("preserves historical snapshots on partial events and reconciles parent workflow approvals", () => {
    const persistedRun: ADKRun = {
      id: "parent-run",
      sessionId: "session-1",
      agentId: "agent-1",
      status: "RUNNING",
      message: "waiting",
      toolCalls: [toolCall("RUNNING")],
      pendingApprovals: [approval("approval-keep", "parent-run")],
      createdAt: "2026-06-18T00:00:00Z",
      updatedAt: "2026-06-18T00:00:01Z",
    };
    const base = {
      ...timelineEntry({ id: "tool-state", runId: persistedRun.id, kind: "tool_group" }),
      toolCalls: [toolCall("RUNNING")],
      approvals: [approval("approval-keep", persistedRun.id)],
    };
    const [history] = replaceTimelineEntries([base], [], new Map([[persistedRun.id, persistedRun]]));
    const merged = upsertTimelineEntry([history!], {
      ...timelineEntry({ id: "tool-state", runId: persistedRun.id, kind: "tool_group" }),
      status: "final",
    });
    expect(merged[0]?.run).toBe(persistedRun);
    expect(merged[0]?.toolCalls).toEqual(base.toolCalls);
    expect(merged[0]?.approvals).toEqual(base.approvals);

    const childApproval = approval("approval-child", "child-run");
    const blankIdApproval = approval("", "parent-run");
    const approvalEntries = [
      {
        ...timelineEntry({ id: "child-copy", runId: "child-run", kind: "approval_group" }),
        approvals: [childApproval],
      },
      {
        ...timelineEntry({ id: "parent-copy", runId: "parent-run", kind: "approval_group" }),
        approvals: [childApproval, blankIdApproval],
      },
    ];
    const deduped = replaceTimelineEntries(approvalEntries);
    expect(deduped.map((entry) => entry.id)).toEqual(["parent-copy"]);
    expect(deduped[0]?.approvals).toEqual([childApproval, blankIdApproval]);

    const unchanged = applyApprovalResolutions(deduped, []);
    expect(unchanged).toBe(deduped);
    const reconciled = applyApprovalResolutions(deduped, [
      {
        approval: childApproval,
        parentRun: {
          ...persistedRun,
          pendingApprovals: [blankIdApproval],
        },
      },
    ]);
    expect(reconciled).toHaveLength(1);
    expect(reconciled[0]?.approvals).toEqual([blankIdApproval]);
  });
});

function timelineEntry(overrides: Partial<ADKTimelineEntry> = {}): ADKTimelineEntry {
  return {
    id: "entry-1",
    sessionId: "session-1",
    kind: "assistant_message",
    createdAt: "2026-06-18T00:00:00Z",
    sequence: 1,
    ...overrides,
  };
}

function toolCall(status: string) {
  return {
    id: `tool-${status}`,
    runId: "run-1",
    toolName: "portfolio.summary",
    permission: "read",
    status,
    requiresUser: false,
    createdAt: "2026-06-18T00:00:00Z",
    updatedAt: "2026-06-18T00:00:00Z",
  };
}

function approval(id: string, runId: string): ADKApproval {
  return {
    id,
    runId,
    agentId: "agent-1",
    toolName: "strategy.save_definition",
    status: "PENDING",
    reason: "writes strategy state",
    createdAt: "2026-06-18T00:00:00Z",
    updatedAt: "2026-06-18T00:00:00Z",
  };
}
