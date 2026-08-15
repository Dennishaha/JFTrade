// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { nextTick } from "vue";
import { describe, expect, it } from "vitest";

import type { ADKRun, ADKTimelineEntry, ADKToolCall } from "../../../src/types";

import ADKTurnTrace from "../../../src/components/adk-page/ADKTurnTrace.vue";
import { createTimelineEntryState } from "@/composables/adk/adkTimeline";
import type { ADKTurnTraceBlock } from "@/composables/adk/adkTurnTraceGrouping";

function buildToolCall(
  overrides: Partial<ADKToolCall> & { id: string; toolName: string },
): ADKToolCall {
  return {
    runId: "run-1",
    permission: "read",
    status: "SUCCEEDED",
    requiresUser: false,
    createdAt: "2026-08-15T00:00:00Z",
    startedAt: "2026-08-15T00:00:00Z",
    updatedAt: "2026-08-15T00:00:04Z",
    completedAt: "2026-08-15T00:00:04Z",
    durationMs: 4000,
    ...overrides,
  };
}

function buildEntry(
  overrides: Partial<ADKTimelineEntry> & { id: string; kind: string },
) {
  return createTimelineEntryState({
    sessionId: "session-1",
    runId: "run-1",
    createdAt: "2026-08-15T00:00:00Z",
    sequence: 0,
    status: "final",
    ...overrides,
  } as ADKTimelineEntry);
}

function buildRun(overrides: Partial<ADKRun> = {}): ADKRun {
  return {
    id: "run-1",
    sessionId: "session-1",
    agentId: "agent-1",
    status: "COMPLETED",
    message: "done",
    toolCalls: [],
    pendingApprovals: [],
    createdAt: "2026-08-15T00:00:00Z",
    updatedAt: "2026-08-15T00:00:04Z",
    completedAt: "2026-08-15T00:00:04Z",
    usage: { durationMs: 4000 },
    ...overrides,
  };
}

function mountTrace(
  entries: ReturnType<typeof buildEntry>[],
  options: {
    props?: Record<string, unknown>;
    attachRun?: ADKRun;
    segmentPosition?: ADKTurnTraceBlock["segmentPosition"];
  } = {},
) {
  if (options.attachRun) {
    for (const entry of entries) entry.run = options.attachRun;
  }
  const block: ADKTurnTraceBlock = {
    type: "turn_trace",
    key: "turn-trace:run-1",
    runId: "run-1",
    segmentPosition: options.segmentPosition ?? "only",
    entries,
  };
  return mount(ADKTurnTrace, {
    props: {
      block,
      renderMarkdown: (content: string) => `<p>${content}</p>`,
      preview: (value: unknown) => JSON.stringify(value ?? {}),
      ...options.props,
    },
    global: {
      stubs: {
        "v-icon": { template: "<span class='v-icon-stub'><slot /></span>" },
        ADKToolVisualization: { template: "<div class='vis-stub' />" },
      },
    },
  });
}

describe("ADKTurnTrace", () => {
  it("collapses a finished run into a single worked-duration row and expands on click", async () => {
    const wrapper = mountTrace(
      [
        buildEntry({
          id: "tool-group-1",
          kind: "tool_group",
          toolCalls: [
            buildToolCall({
              id: "call-1",
              toolName: "market.candles",
              input: { symbol: "AAPL" },
              output: { candles: [{}, {}, {}] },
            }),
          ],
        }),
      ],
      {
        attachRun: buildRun(),
        props: {
          toolByName: (name: string) =>
            name === "market.candles"
              ? { name, displayName: "K 线查询" }
              : undefined,
        },
      },
    );

    const header = wrapper.get(".adk-turn-trace__header");
    expect(header.text()).toContain("已工作 4s");
    expect(wrapper.find(".adk-turn-trace__body").exists()).toBe(false);

    await header.trigger("click");
    await nextTick();

    expect(wrapper.find(".adk-trace-group__summary").text()).toContain(
      "已查询了 1 项",
    );
    const row = wrapper.get(".adk-trace-tool");
    expect(row.text()).toContain("K 线查询");
    expect(row.text()).toContain("AAPL");
    expect(row.text()).toContain("3 条");
    expect(wrapper.find(".adk-trace-tool__status.is-success").exists()).toBe(true);

    await row.trigger("click");
    await nextTick();
    expect(wrapper.find(".adk-trace-tool__detail").text()).toContain("AAPL");
  });

  it("stays expanded with a live label while the run is active", () => {
    const wrapper = mountTrace(
      [
        buildEntry({
          id: "tool-group-live",
          kind: "tool_group",
          status: "streaming",
          toolCalls: [
            buildToolCall({
              id: "call-live",
              toolName: "market.snapshot",
              status: "PENDING",
            }),
          ],
        }),
      ],
      {
        props: {
          activeRunId: "run-1",
          activeRunStatus: "RUNNING",
          hasBlockingRun: true,
        },
      },
    );

    expect(wrapper.find(".adk-turn-trace__body").exists()).toBe(true);
    expect(wrapper.get(".adk-turn-trace__header").text()).toContain("正在工作");
    expect(wrapper.find(".adk-turn-trace__status.is-running").exists()).toBe(true);
    expect(wrapper.find(".adk-trace-group__summary").text()).toContain("工具执行中");
    expect(wrapper.find(".adk-trace-tool__status.is-running").exists()).toBe(true);

    wrapper.unmount();
  });

  it("shows waiting labels for approval and pending runs", () => {
    const approvalTrace = mountTrace(
      [
        buildEntry({
          id: "tool-group-approval",
          kind: "tool_group",
          toolCalls: [
            buildToolCall({
              id: "call-approval",
              toolName: "execution.order_place",
              status: "PENDING_APPROVAL",
            }),
          ],
        }),
      ],
      {
        props: {
          activeRunId: "run-1",
          activeRunStatus: "PENDING_APPROVAL",
          hasBlockingRun: true,
        },
      },
    );

    expect(approvalTrace.get(".adk-turn-trace__header").text()).toContain("等待审批");
    expect(approvalTrace.find(".adk-trace-group__summary").text()).toContain(
      "等待审批",
    );
    expect(
      approvalTrace.find(".adk-trace-tool__status.is-warning").exists(),
    ).toBe(true);
    approvalTrace.unmount();

    const pendingTrace = mountTrace(
      [
        buildEntry({
          id: "tool-group-pending",
          kind: "tool_group",
          status: "streaming",
          toolCalls: [
            buildToolCall({
              id: "call-pending",
              toolName: "market.candles",
              status: "PENDING",
            }),
          ],
        }),
      ],
      {
        props: {
          activeRunId: "run-1",
          activeRunStatus: "PENDING",
          hasBlockingRun: true,
        },
      },
    );

    expect(pendingTrace.find(".adk-trace-group__summary").text()).toContain(
      "等待执行",
    );
    pendingTrace.unmount();
  });

  it("renders reasoning rows with a derived duration and toggles the body", async () => {
    const wrapper = mountTrace(
      [
        buildEntry({
          id: "reasoning-1",
          kind: "assistant_reasoning",
          text: "先查行情再校验脚本",
          createdAt: "2026-08-15T00:00:00Z",
        }),
        buildEntry({
          id: "tool-group-1",
          kind: "tool_group",
          createdAt: "2026-08-15T00:00:01Z",
          toolCalls: [
            buildToolCall({ id: "call-1", toolName: "market.snapshot" }),
          ],
        }),
      ],
      { attachRun: buildRun() },
    );

    await wrapper.get(".adk-turn-trace__header").trigger("click");
    await nextTick();

    const reasoningToggle = wrapper.get(".adk-trace-reasoning__toggle");
    expect(reasoningToggle.text()).toContain("思考过程 · 1s");
    expect(wrapper.find(".adk-trace-reasoning__body").exists()).toBe(false);

    await reasoningToggle.trigger("click");
    await nextTick();
    expect(wrapper.find(".adk-trace-reasoning__body").text()).toContain(
      "先查行情再校验脚本",
    );
  });

  it("shows failure meta and hint for failed runs", async () => {
    const wrapper = mountTrace(
      [
        buildEntry({
          id: "tool-group-failed",
          kind: "tool_group",
          toolCalls: [
            buildToolCall({
              id: "call-failed",
              toolName: "market.candles",
              status: "FAILED",
              error: "provider unavailable",
            }),
          ],
        }),
      ],
      {
        attachRun: buildRun({
          status: "FAILED",
          failureReason: "工具执行失败",
          usage: { durationMs: 2000 },
        }),
      },
    );

    const header = wrapper.get(".adk-turn-trace__header");
    expect(header.text()).toContain("已工作 2s");
    expect(header.text()).toContain("运行失败");
    expect(wrapper.find(".adk-turn-trace__status.is-error").exists()).toBe(true);

    await header.trigger("click");
    await nextTick();
    const row = wrapper.get(".adk-trace-tool");
    expect(row.text()).toContain("provider unavailable");
    expect(wrapper.find(".adk-trace-tool__status.is-error").exists()).toBe(true);
  });

  it("freezes an earlier segment as finished while the same run continues", () => {
    const wrapper = mountTrace(
      [
        buildEntry({
          id: "tool-group-done",
          kind: "tool_group",
          createdAt: "2026-08-15T00:00:00Z",
          updatedAt: "2026-08-15T00:00:04Z",
          toolCalls: [
            buildToolCall({ id: "call-done", toolName: "market.snapshot" }),
          ],
        }),
      ],
      {
        attachRun: buildRun({
          status: "RUNNING",
          startedAt: "2026-08-15T00:00:00Z",
          completedAt: undefined,
          usage: undefined,
        }),
        segmentPosition: "first",
        props: {
          activeRunId: "run-1",
          activeRunStatus: "RUNNING",
          hasBlockingRun: true,
        },
      },
    );

    const header = wrapper.get(".adk-turn-trace__header");
    expect(header.text()).toContain("已工作 4s");
    expect(header.text()).not.toContain("正在工作");
    expect(wrapper.find(".adk-turn-trace__status.is-running").exists()).toBe(false);
    expect(wrapper.find(".adk-turn-trace__body").exists()).toBe(false);
    expect(
      wrapper.find(".adk-trace-group__summary").exists(),
    ).toBe(false);
    wrapper.unmount();
  });

  it("caps long tool lists behind an explicit expander", async () => {
    const toolCalls = Array.from({ length: 23 }, (_, index) =>
      buildToolCall({
        id: `call-${index}`,
        toolName: "market.snapshot",
        input: { symbol: `S${index}` },
      }),
    );
    const wrapper = mountTrace(
      [buildEntry({ id: "tool-group-many", kind: "tool_group", toolCalls })],
      { attachRun: buildRun() },
    );

    await wrapper.get(".adk-turn-trace__header").trigger("click");
    await nextTick();

    expect(wrapper.findAll(".adk-trace-tool")).toHaveLength(20);
    const more = wrapper.get(".adk-trace-more");
    expect(more.text()).toContain("展开剩余 3 条");

    await more.trigger("click");
    await nextTick();
    expect(wrapper.findAll(".adk-trace-tool")).toHaveLength(23);
  });
});
