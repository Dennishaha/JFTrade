import { describe, expect, it } from "vitest";

import type { ADKRun, ADKTimelineEntry } from "../src/contracts";
import {
  buildTimelineRun,
  replaceTimelineEntries,
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
});
