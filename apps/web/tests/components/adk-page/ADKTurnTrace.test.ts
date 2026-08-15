// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { nextTick } from "vue";
import { describe, expect, it, vi } from "vitest";

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

  it("opens external links from markdown and reuses the rendered cache", async () => {
    const openSpy = vi.spyOn(window, "open").mockReturnValue(null);
    const wrapper = mountTrace(
      [
        buildEntry({
          id: "msg-link",
          kind: "assistant_message",
          text: "详见报告",
        }),
      ],
      {
        attachRun: buildRun(),
        props: {
          renderMarkdown: () =>
            `<p><a href="https://example.com/report">完整报告</a></p>`,
        },
      },
    );

    await wrapper.get(".adk-turn-trace__header").trigger("click");
    await nextTick();

    const text = wrapper.get(".adk-turn-trace__text");
    await text.trigger("click");
    expect(openSpy).not.toHaveBeenCalled();

    await text.get("a").trigger("click");
    expect(openSpy).toHaveBeenCalledWith(
      "https://example.com/report",
      "_blank",
      "noopener,noreferrer",
    );

    // Collapsing and re-expanding re-renders from the markdown cache.
    await wrapper.get(".adk-turn-trace__header").trigger("click");
    await nextTick();
    await wrapper.get(".adk-turn-trace__header").trigger("click");
    await nextTick();
    expect(wrapper.get(".adk-turn-trace__text a").attributes("href")).toBe(
      "https://example.com/report",
    );
    openSpy.mockRestore();
  });

  it("collapses an expanded tool row on a second click", async () => {
    const wrapper = mountTrace(
      [
        buildEntry({
          id: "tool-group-toggle",
          kind: "tool_group",
          toolCalls: [
            buildToolCall({ id: "call-toggle", toolName: "market.snapshot" }),
          ],
        }),
      ],
      { attachRun: buildRun() },
    );

    await wrapper.get(".adk-turn-trace__header").trigger("click");
    await nextTick();
    const row = wrapper.get(".adk-trace-tool");
    await row.trigger("click");
    await nextTick();
    expect(wrapper.find(".adk-trace-tool__detail").exists()).toBe(true);

    await row.trigger("click");
    await nextTick();
    expect(wrapper.find(".adk-trace-tool__detail").exists()).toBe(false);
  });

  it("stops the live ticker when the run is no longer the blocking active run", async () => {
    const wrapper = mountTrace(
      [
        buildEntry({
          id: "tool-group-active",
          kind: "tool_group",
          toolCalls: [
            buildToolCall({
              id: "call-active",
              toolName: "market.snapshot",
              status: "PENDING",
            }),
            buildToolCall({
              id: "call-finished",
              toolName: "market.candles",
              status: "SUCCEEDED",
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
    expect(wrapper.get(".adk-turn-trace__header").text()).toContain("正在工作");
    // The pending call is presented as running while the finished call keeps
    // its own success tone.
    expect(wrapper.find(".adk-trace-tool__status.is-running").exists()).toBe(true);
    expect(wrapper.find(".adk-trace-tool__status.is-success").exists()).toBe(true);

    await wrapper.setProps({
      activeRunId: "",
      activeRunStatus: "",
      hasBlockingRun: false,
    });
    await nextTick();

    expect(wrapper.get(".adk-turn-trace__header").text()).not.toContain(
      "正在工作",
    );
    expect(wrapper.find(".adk-turn-trace__status.is-running").exists()).toBe(false);
    wrapper.unmount();
  });

  it("shows a waiting header for approval- and input-blocked runs", () => {
    const expectations: Array<{ status: string; label: string }> = [
      { status: "PENDING_APPROVAL", label: "等待审批" },
      { status: "PENDING_INPUT", label: "等待回答" },
    ];
    for (const { status, label } of expectations) {
      const wrapper = mountTrace(
        [
          buildEntry({
            id: `tool-group-${status}`,
            kind: "tool_group",
            toolCalls: [
              buildToolCall({
                id: `call-${status}`,
                toolName: "interaction.request_user",
              }),
            ],
          }),
        ],
        { attachRun: buildRun({ status }) },
      );

      expect(wrapper.get(".adk-turn-trace__label").text()).toContain(label);
      expect(wrapper.find(".adk-turn-trace__status.is-warning").exists()).toBe(true);
      wrapper.unmount();
    }
  });

  it("tones cancelled and denied runs as muted without a failure hint", () => {
    for (const status of ["CANCELLED", "DENIED"]) {
      const wrapper = mountTrace(
        [
          buildEntry({
            id: `tool-group-${status}`,
            kind: "tool_group",
            toolCalls: [
              buildToolCall({
                id: `call-${status}`,
                toolName: "execution.order_place",
                status,
              }),
            ],
          }),
        ],
        { attachRun: buildRun({ status }) },
      );

      expect(wrapper.find(".adk-turn-trace__status.is-muted").exists()).toBe(true);
      expect(wrapper.find(".adk-turn-trace__hint").exists()).toBe(false);
      wrapper.unmount();
    }
  });

  it("tones denied tool groups and rows as muted", async () => {
    const wrapper = mountTrace(
      [
        buildEntry({
          id: "tool-group-denied",
          kind: "tool_group",
          toolCalls: [
            buildToolCall({
              id: "call-denied",
              toolName: "execution.order_place",
              status: "DENIED",
              error: "user rejected",
            }),
            buildToolCall({
              id: "call-unknown",
              toolName: "market.snapshot",
              status: undefined,
            }),
          ],
        }),
      ],
      { attachRun: buildRun() },
    );

    await wrapper.get(".adk-turn-trace__header").trigger("click");
    await nextTick();

    expect(
      wrapper.find(".adk-trace-group .adk-turn-trace__status.is-muted").exists(),
    ).toBe(true);
    expect(wrapper.find(".adk-trace-tool__status.is-muted").exists()).toBe(true);
  });

  it("shows an error tone without a hint when the run snapshot is missing", () => {
    const wrapper = mountTrace([
      buildEntry({
        id: "tool-group-orphan-failure",
        kind: "tool_group",
        toolCalls: [
          buildToolCall({
            id: "call-orphan-failure",
            toolName: "market.candles",
            status: "FAILED",
            error: "provider unavailable",
          }),
        ],
      }),
    ]);

    expect(wrapper.find(".adk-turn-trace__status.is-error").exists()).toBe(true);
    expect(wrapper.find(".adk-turn-trace__hint").exists()).toBe(false);
  });

  it("toggles tools on entries that never initialized expansion state", async () => {
    const rawEntry = {
      id: "tool-group-raw",
      kind: "tool_group",
      sessionId: "session-1",
      runId: "run-1",
      createdAt: "2026-08-15T00:00:00Z",
      updatedAt: "2026-08-15T00:00:04Z",
      sequence: 0,
      status: "final",
      toolCalls: [buildToolCall({ id: "call-raw", toolName: "market.snapshot" })],
    } as unknown as ReturnType<typeof buildEntry>;
    const wrapper = mountTrace([rawEntry], { attachRun: buildRun() });

    await wrapper.get(".adk-turn-trace__header").trigger("click");
    await nextTick();
    const row = wrapper.get(".adk-trace-tool");
    expect(row.attributes("aria-expanded")).toBe("false");

    await row.trigger("click");
    await nextTick();
    expect(wrapper.find(".adk-trace-tool__detail").exists()).toBe(true);
  });

  it("skips empty assistant text inside an expanded block", async () => {
    const wrapper = mountTrace(
      [
        buildEntry({
          id: "tool-group-before-gap",
          kind: "tool_group",
          toolCalls: [
            buildToolCall({ id: "call-before-gap", toolName: "market.snapshot" }),
          ],
        }),
        buildEntry({ id: "msg-empty", kind: "assistant_message" }),
      ],
      { attachRun: buildRun() },
    );

    await wrapper.get(".adk-turn-trace__header").trigger("click");
    await nextTick();

    expect(wrapper.find(".adk-turn-trace__text").exists()).toBe(false);
    expect(wrapper.find(".adk-trace-group__summary").exists()).toBe(true);
  });

  it("hides the duration of a trailing reasoning entry that never updated", async () => {
    const wrapper = mountTrace(
      [
        buildEntry({
          id: "reasoning-stale",
          kind: "assistant_reasoning",
          text: "没有后续",
          createdAt: "2026-08-15T00:00:03Z",
          updatedAt: "2026-08-15T00:00:03Z",
        }),
      ],
      { attachRun: buildRun() },
    );

    await wrapper.get(".adk-turn-trace__header").trigger("click");
    await nextTick();

    expect(wrapper.get(".adk-trace-reasoning__toggle").text()).not.toContain("·");
  });

  it("shows streaming progress for a group that streams outside the active run", async () => {
    const wrapper = mountTrace(
      [
        buildEntry({
          id: "tool-group-streaming",
          kind: "tool_group",
          status: "streaming",
          toolCalls: [
            buildToolCall({ id: "call-stream", toolName: "market.candles" }),
          ],
        }),
      ],
      { attachRun: buildRun() },
    );

    await wrapper.get(".adk-turn-trace__header").trigger("click");
    await nextTick();

    expect(wrapper.get(".adk-trace-group__summary").text()).toContain(
      "工具执行中",
    );
  });

  it("derives reasoning durations from neighboring entries and its own update time", async () => {
    const wrapper = mountTrace(
      [
        buildEntry({ id: "reasoning-notext", kind: "assistant_reasoning" }),
        buildEntry({
          id: "reasoning-undated",
          kind: "assistant_reasoning",
          text: "无时间戳",
          createdAt: undefined,
        }),
        buildEntry({
          id: "reasoning-same",
          kind: "assistant_reasoning",
          text: "同时",
        }),
        buildEntry({
          id: "reasoning-next",
          kind: "assistant_reasoning",
          text: "有后继",
        }),
        buildEntry({
          id: "tool-mid",
          kind: "tool_group",
          createdAt: "2026-08-15T00:00:02Z",
          toolCalls: [
            buildToolCall({ id: "call-mid", toolName: "market.snapshot" }),
          ],
        }),
        buildEntry({
          id: "reasoning-tail",
          kind: "assistant_reasoning",
          text: "收尾",
          createdAt: "2026-08-15T00:00:03Z",
          updatedAt: "2026-08-15T00:00:07Z",
        }),
      ],
      { attachRun: buildRun() },
    );

    await wrapper.get(".adk-turn-trace__header").trigger("click");
    await nextTick();

    const toggles = wrapper.findAll(".adk-trace-reasoning__toggle");
    expect(toggles).toHaveLength(5);
    // The next entry has no parseable timestamp, so no duration is shown.
    expect(toggles[0]!.text()).not.toContain("·");
    // Without a createdAt there is no duration either.
    expect(toggles[1]!.text()).not.toContain("·");
    // Same-timestamp successor: zero-length spans are hidden.
    expect(toggles[2]!.text()).not.toContain("·");
    expect(toggles[3]!.text()).toContain("思考过程 · 2s");

    // Expanding a reasoning row without text renders an empty body.
    await toggles[0]!.trigger("click");
    await nextTick();
    expect(wrapper.get(".adk-trace-reasoning__body").text()).toBe("");

    // The trailing reasoning falls back to its own updatedAt.
    expect(toggles[4]!.text()).toContain("思考过程 · 4s");
  });

  it("ticks a trailing reasoning duration while the run is active", () => {
    const wrapper = mountTrace(
      [
        buildEntry({
          id: "reasoning-live",
          kind: "assistant_reasoning",
          text: "思考中",
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

    expect(wrapper.get(".adk-trace-reasoning__toggle").text()).toContain(
      "思考过程 ·",
    );
    wrapper.unmount();
  });

  it("prunes cached markdown and visualizations when block entries change", async () => {
    const wrapper = mountTrace(
      [
        buildEntry({ id: "msg-old", kind: "assistant_message", text: "旧文本" }),
        buildEntry({ id: "msg-kept", kind: "assistant_message", text: "保留文本" }),
        buildEntry({
          id: "tools",
          kind: "tool_group",
          toolCalls: [
            buildToolCall({
              id: "call-drop",
              toolName: "market.candles",
              output: { candles: [{}] },
            }),
            buildToolCall({
              id: "call-kept",
              toolName: "market.candles",
              output: { candles: [{}] },
            }),
          ],
        }),
      ],
      { attachRun: buildRun() },
    );

    await wrapper.get(".adk-turn-trace__header").trigger("click");
    await nextTick();
    const rows = wrapper.findAll(".adk-trace-tool");
    await rows[0]!.trigger("click");
    await rows[1]!.trigger("click");
    await nextTick();
    expect(wrapper.findAll(".adk-trace-tool__detail")).toHaveLength(2);

    const nextEntries = [
      buildEntry({ id: "msg-kept", kind: "assistant_message", text: "保留文本" }),
      buildEntry({
        id: "tools",
        kind: "tool_group",
        toolCalls: [
          buildToolCall({
            id: "call-kept",
            toolName: "market.candles",
            output: { candles: [{}] },
          }),
        ],
      }),
      buildEntry({ id: "tools-empty", kind: "tool_group" }),
    ];
    await wrapper.setProps({
      block: {
        type: "turn_trace",
        key: "turn-trace:run-1",
        runId: "run-1",
        segmentPosition: "only",
        entries: nextEntries,
      },
    });
    await nextTick();

    // The replacement block starts collapsed; expanding it re-renders from
    // the retained caches while dropped entries stay gone.
    await wrapper.get(".adk-turn-trace__header").trigger("click");
    await nextTick();

    expect(wrapper.text()).toContain("保留文本");
    expect(wrapper.text()).not.toContain("旧文本");
    expect(wrapper.findAll(".adk-trace-tool")).toHaveLength(1);
    expect(wrapper.findAll(".adk-trace-group__summary")).toHaveLength(2);
  });

  it("renders tool details without output and surfaces tool errors", async () => {
    const wrapper = mountTrace(
      [
        buildEntry({
          id: "tool-group-detail",
          kind: "tool_group",
          toolCalls: [
            buildToolCall({
              id: "call-no-output",
              toolName: "market.snapshot",
              input: undefined,
              output: undefined,
              durationMs: undefined,
            }),
            buildToolCall({
              id: "call-error",
              toolName: "execution.order_place",
              status: "FAILED",
              error: "insufficient buying power",
              output: undefined,
            }),
          ],
        }),
      ],
      {
        attachRun: buildRun(),
        props: {
          toolByName: (name: string) =>
            name === "market.snapshot"
              ? ({ name, displayName: "快照" } as never)
              : undefined,
        },
      },
    );

    await wrapper.get(".adk-turn-trace__header").trigger("click");
    await nextTick();

    const rows = wrapper.findAll(".adk-trace-tool");
    expect(rows[0]!.text()).toContain("快照");
    expect(rows[0]!.find(".adk-trace-tool__arg").exists()).toBe(false);

    await rows[0]!.trigger("click");
    await rows[1]!.trigger("click");
    await nextTick();

    expect(wrapper.findAll(".adk-trace-tool__detail")).toHaveLength(2);
    expect(wrapper.find(".vis-stub").exists()).toBe(false);
    expect(wrapper.find(".adk-json-label--error").exists()).toBe(true);
    expect(wrapper.text()).toContain("insufficient buying power");
  });

  it("omits the group duration suffix when tool timing is unknown", async () => {
    const wrapper = mountTrace(
      [
        buildEntry({
          id: "tool-group-timeless",
          kind: "tool_group",
          toolCalls: [
            buildToolCall({
              id: "call-timeless",
              toolName: "market.candles",
              createdAt: undefined,
              startedAt: undefined,
              updatedAt: undefined,
              completedAt: undefined,
              durationMs: undefined,
            }),
          ],
        }),
      ],
      { attachRun: buildRun() },
    );

    await wrapper.get(".adk-turn-trace__header").trigger("click");
    await nextTick();

    const summary = wrapper.get(".adk-trace-group__summary");
    expect(summary.text()).toContain("已查询了 1 项");
    expect(summary.text()).not.toContain("·");
  });

  it("shows a bare worked header when no timing information exists", () => {
    const wrapper = mountTrace([
      buildEntry({
        id: "tool-group-undated",
        kind: "tool_group",
        createdAt: undefined,
        updatedAt: undefined,
        toolCalls: [
          buildToolCall({
            id: "call-undated",
            toolName: "market.candles",
            createdAt: undefined,
            startedAt: undefined,
            updatedAt: undefined,
            completedAt: undefined,
            durationMs: undefined,
          }),
        ],
      }),
    ]);

    expect(wrapper.get(".adk-turn-trace__label").text()).toBe("已工作");
  });
});
