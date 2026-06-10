// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import type { ADKRun } from "@/contracts";

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

    expect(wrapper.text()).toContain("调用了 2 个工具");
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
    expect(wrapper.text()).not.toContain("调用了 1 个工具");
  });

  it("switches to completed summary as soon as tool calls finish even if llm is still busy", () => {
    const wrapper = mount(ADKRunTrace, {
      props: {
        run: buildRun(
          [
            buildToolCall("tool-1", "portfolio.summary"),
            buildToolCall("tool-2", "orders.latest"),
          ],
          "RUNNING",
        ),
        busy: true,
        toolProgress: "Calling portfolio.summary",
        summaryExpanded: false,
        expandedToolCallIds: [],
      },
    });

    expect(wrapper.text()).toContain("调用了 2 个工具");
    expect(wrapper.text()).toContain("已完成");
    expect(wrapper.text()).not.toContain("Calling portfolio.summary");
  });

  it("keeps the summary in running state while any tool call is still running", () => {
    const wrapper = mount(ADKRunTrace, {
      props: {
        run: buildRun(
          [
            buildToolCall("tool-1", "load_skill", "RUNNING"),
            buildToolCall("tool-2", "portfolio.summary"),
            buildToolCall("tool-3", "account.orders"),
          ],
          "COMPLETED",
        ),
        busy: false,
        summaryExpanded: false,
        expandedToolCallIds: [],
      },
    });

    expect(wrapper.text()).toContain("调用了 3 个工具");
    expect(wrapper.text()).toContain("运行中");
    expect(wrapper.text()).not.toContain("Collapse this tool trace");
  });

  it("expands the multi-tool summary to show individual calls", async () => {
    const wrapper = mount(ADKRunTrace, {
      props: {
        run: buildRun([
          buildToolCall("tool-1", "portfolio.summary"),
          buildToolCall("tool-2", "orders.latest"),
        ]),
        busy: false,
        summaryExpanded: false,
        expandedToolCallIds: [],
        "onUpdate:summaryExpanded": (value: boolean) => {
          void wrapper.setProps({ summaryExpanded: value });
        },
      },
    });

    expect(wrapper.text()).not.toContain("portfolio.summary");
    await wrapper.find(".adk-run-trace-card--summary").trigger("click");
    expect(wrapper.text()).toContain("portfolio.summary");
    expect(wrapper.text()).toContain("orders.latest");
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

    expect(wrapper.text()).toContain("组合摘要");
    expect(wrapper.text()).toContain("经纪通道");
    expect(wrapper.text()).toContain("已连接");
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

    expect(wrapper.text()).not.toContain("组合摘要");
    expect(wrapper.text()).toContain('"detail": "raw fallback"');
  });

  it("shows the failed tool and error summary without expanding the multi-tool trace", () => {
    const wrapper = mount(ADKRunTrace, {
      props: {
        run: buildRun(
          [
            buildToolCall("tool-1", "portfolio.summary"),
            buildToolCall(
              "tool-2",
              "strategy.save_draft",
              "FAILED",
              undefined,
              "disk full",
            ),
          ],
          "FAILED",
          "disk full",
        ),
        busy: false,
        summaryExpanded: false,
        expandedToolCallIds: [],
      },
    });

    expect(wrapper.text()).toContain("调用了 2 个工具");
    expect(wrapper.text()).toContain("strategy.save_draft: disk full");
    expect(wrapper.text()).not.toContain('"query": "strategy.save_draft"');
  });

  it("shows a collapsed single-tool error summary before expansion", () => {
    const wrapper = mount(ADKRunTrace, {
      props: {
        run: buildRun(
          [
            buildToolCall(
              "tool-1",
              "strategy.save_draft",
              "TIMED_OUT",
              undefined,
              "tool execution timed out: context deadline exceeded",
            ),
          ],
          "FAILED",
          "tool execution timed out: context deadline exceeded",
        ),
        busy: false,
        summaryExpanded: true,
        expandedToolCallIds: [],
      },
    });

    expect(wrapper.text()).toContain("strategy.save_draft");
    expect(wrapper.text()).toContain(
      "tool execution timed out: context deadline exceeded",
    );
    expect(wrapper.find(".adk-run-trace-detail").exists()).toBe(false);
  });
});

function buildRun(
  toolCalls: ADKRun["toolCalls"],
  status = "COMPLETED",
  failureReason = "",
): ADKRun {
  return {
    id: "run-1",
    sessionId: "session-1",
    agentId: "agent-1",
    status,
    message: status === "COMPLETED" ? "completed" : failureReason,
    failureReason,
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
  error?: string,
): ADKRun["toolCalls"][number] {
  return {
    id,
    runId: "run-1",
    toolName,
    permission: "read",
    status,
    input: { query: toolName },
    output,
    error,
    requiresUser: false,
    createdAt: "2026-06-06T00:00:00Z",
    updatedAt: "2026-06-06T00:00:00Z",
    durationMs: 120,
  };
}
