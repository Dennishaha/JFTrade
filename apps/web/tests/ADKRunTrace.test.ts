// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import type { ADKRun } from "@jftrade/ui-contracts";

import ADKRunTrace from "../src/components/shared/ADKRunTrace.vue";

describe("ADKRunTrace", () => {
  it("shows a collapsed summary for completed multi-tool runs", () => {
    const wrapper = mount(ADKRunTrace, {
      props: {
        run: buildRun([
          buildToolCall("tool-1", "portfolio.summary"),
          buildToolCall("tool-2", "orders.latest"),
        ]),
        busy: false,
        summaryExpanded: false,
        expandedToolCallIds: [],
      },
    });

    expect(wrapper.text()).toContain("已调用工具 2 个");
    expect(wrapper.text()).toContain("已完成");
    expect(wrapper.text()).not.toContain("portfolio.summary");
  });

  it("shows a direct tool summary for a completed single-tool run", () => {
    const wrapper = mount(ADKRunTrace, {
      props: {
        run: buildRun([buildToolCall("tool-1", "portfolio.summary")]),
        busy: false,
        summaryExpanded: true,
        expandedToolCallIds: [],
      },
    });

    expect(wrapper.text()).toContain("portfolio.summary");
    expect(wrapper.text()).toContain("已完成");
    expect(wrapper.text()).not.toContain("已调用工具 1 个");
  });

  it("switches to completed summary as soon as tool calls finish even if llm is still busy", () => {
    const wrapper = mount(ADKRunTrace, {
      props: {
        run: buildRun(
          [buildToolCall("tool-1", "portfolio.summary"), buildToolCall("tool-2", "orders.latest")],
          "RUNNING",
        ),
        busy: true,
        toolProgress: "正在调用 portfolio.summary",
        summaryExpanded: false,
        expandedToolCallIds: [],
      },
    });

    expect(wrapper.text()).toContain("已调用工具 2 个");
    expect(wrapper.text()).toContain("已完成");
    expect(wrapper.text()).not.toContain("正在调用 portfolio.summary");
  });

  it("keeps the summary in running state while any tool call is still running", () => {
    const wrapper = mount(ADKRunTrace, {
      props: {
        run: buildRun([
          buildToolCall("tool-1", "load_skill", "RUNNING"),
          buildToolCall("tool-2", "portfolio.summary"),
          buildToolCall("tool-3", "account.orders"),
        ], "COMPLETED"),
        busy: false,
        summaryExpanded: false,
        expandedToolCallIds: [],
      },
    });

    expect(wrapper.text()).toContain("已调用工具 3 个");
    expect(wrapper.text()).toContain("运行中");
    expect(wrapper.text()).not.toContain("已完成 展开查看本轮工具调用轨迹");
  });

  it("renders known tool output as an inline visualization and keeps raw JSON", () => {
    const wrapper = mount(ADKRunTrace, {
      props: {
        run: buildRun([
          buildToolCall("tool-1", "portfolio.summary", "SUCCEEDED", {
            brokerStatus: "connected",
            accountCount: 2,
            orderCount: 3,
          }),
        ]),
        busy: false,
        expandedToolCallIds: ["tool-1"],
      },
    });

    expect(wrapper.text()).toContain("Portfolio summary");
    expect(wrapper.text()).toContain("Broker");
    expect(wrapper.text()).toContain("connected");
    expect(wrapper.text()).toContain('"brokerStatus": "connected"');
  });

  it("falls back to raw JSON for unknown tool output", () => {
    const wrapper = mount(ADKRunTrace, {
      props: {
        run: buildRun([
          buildToolCall("tool-1", "unknown.tool", "SUCCEEDED", {
            ok: true,
            detail: "raw fallback",
          }),
        ]),
        busy: false,
        expandedToolCallIds: ["tool-1"],
      },
    });

    expect(wrapper.text()).not.toContain("Portfolio summary");
    expect(wrapper.text()).toContain('"detail": "raw fallback"');
  });
});

function buildRun(toolCalls: ADKRun["toolCalls"], status = "COMPLETED"): ADKRun {
  return {
    id: "run-1",
    sessionId: "session-1",
    agentId: "agent-1",
    status,
    message: "completed",
    toolCalls,
    pendingApprovals: [],
    createdAt: "2026-06-06T00:00:00Z",
    updatedAt: "2026-06-06T00:00:00Z",
  };
}

function buildToolCall(
  id: string,
  toolName: string,
  status = "SUCCEEDED",
  output: unknown = { ok: true },
): ADKRun["toolCalls"][number] {
  return {
    id,
    runId: "run-1",
    toolName,
    permission: "read",
    status,
    input: { query: toolName },
    output,
    requiresUser: false,
    createdAt: "2026-06-06T00:00:00Z",
    updatedAt: "2026-06-06T00:00:00Z",
    durationMs: 120,
  };
}
