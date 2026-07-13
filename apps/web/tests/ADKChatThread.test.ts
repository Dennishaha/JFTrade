// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineComponent, nextTick } from "vue";

import type { ADKTimelineEntry } from "@/contracts";

import ADKChatThread from "../src/components/adk-page/ADKChatThread.vue";
import { createTimelineEntryState } from "../src/composables/adkTimeline";

function mountThread(
  entries: ADKTimelineEntry[],
  options: {
    renderMarkdown?: (content: string) => string;
    activityIndicator?: "idle" | "typing" | "child_finished";
    props?: Partial<InstanceType<typeof ADKChatThread>["$props"]>;
    stubs?: Record<string, unknown>;
  } = {},
) {
  return mount(ADKChatThread, {
    props: {
      timelineEntries: entries.map((entry) => createTimelineEntryState(entry)),
      sendingChat: false,
      activityIndicator: options.activityIndicator ?? "idle",
      errorMessage: "",
      approvalsBusy: false,
      suggestions: [],
      emptyStateTitle: "空",
      emptyStateHint: "空",
      approvalTool: () => undefined,
      clearErrorMessage: () => {},
      preview: (value: unknown) => String(value ?? ""),
      renderMarkdown: options.renderMarkdown ?? ((content: string) => content),
      resolveApprovalGroup: () => {},
      resolveApproval: () => {},
      ...options.props,
    },
    global: {
      stubs: {
        ADKRunTrace: { template: "<div />" },
        "v-alert": { template: "<div :class='$attrs.class'><slot /></div>" },
        "v-chip": { template: "<button><slot /></button>" },
        "v-icon": { template: "<span><slot /></span>" },
        "v-progress-circular": { template: "<span />" },
        "v-progress-linear": { template: "<div class='v-progress-linear-stub' />" },
        ...options.stubs,
      },
    },
  });
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("ADKChatThread", () => {
  it("replaces typing dots with a child-finished status", () => {
    const wrapper = mountThread([], { activityIndicator: "child_finished" });

    expect(wrapper.find(".adk-typing").exists()).toBe(false);
    expect(wrapper.find(".adk-child-finished-status").text()).toContain(
      "子智能体已结束，主智能体继续处理中",
    );
  });

  it("renders structured run errors as one-line alerts with expandable details", async () => {
    const wrapper = mountThread([], {
      props: {
        errorMessage:
          "模型调用失败：服务商余额不足\nprovider returned 402\n错误码：MODEL_CALL_FAILED · Run：run-1",
      },
    });

    const alert = wrapper.get(".adk-inline-alert");
    expect(alert.find(".adk-inline-alert__title").text()).toBe(
      "模型调用失败：服务商余额不足",
    );
    expect(alert.text()).not.toContain("provider returned 402");
    expect(alert.text()).not.toContain("MODEL_CALL_FAILED");

    await alert.get(".adk-inline-alert__toggle").trigger("click");

    expect(alert.text()).toContain("provider returned 402");
    expect(alert.text()).toContain("MODEL_CALL_FAILED");
    expect(alert.text()).toContain("run-1");
  });

  it("renders child run groups with the child trace component", () => {
    const childTraceStub = defineComponent({
      props: ["item", "variant"],
      template: "<div class='child-trace-stub'>{{ item.stepTitle }} / {{ item.status }} / {{ variant }}</div>",
    });
    const wrapper = mountThread(
      [
        {
          id: "child-entry",
          sessionId: "session-1",
          runId: "parent-run",
          kind: "child_run_group",
          createdAt: "2026-06-17T00:00:00Z",
          sequence: 1,
          status: "final",
          text: "启动子智能体 #1：不应显示",
          childRunItem: {
            id: "child-run",
            index: 1,
            stepTitle: "检查子智能体",
            status: "FAILED",
            pendingApprovalCount: 0,
          },
        } as never,
      ],
      {
        stubs: {
          ADKChildRunTrace: childTraceStub,
        },
      },
    );

    expect(wrapper.find(".child-trace-stub").text()).toContain("检查子智能体");
    expect(wrapper.find(".child-trace-stub").text()).toContain("timeline");
    expect(wrapper.text()).not.toContain("启动子智能体");
  });

  it("keeps a pending input request as a compact timeline status", () => {
    const wrapper = mountThread([
      {
        id: "input-request-entry",
        sessionId: "session-1",
        runId: "run-1",
        kind: "input_request",
        createdAt: "2026-07-12T00:00:00Z",
        status: "final",
        inputRequest: {
          id: "input-1",
          runId: "run-1",
          agentId: "agent-1",
          functionCallId: "call-1",
          title: "选择执行方式",
          status: "PENDING",
          questions: [
            {
              id: "q1",
              question: "如何继续？",
              allowOther: false,
              options: [
                { id: "q1-o1", label: "稳妥" },
                { id: "q1-o2", label: "快速" },
              ],
            },
          ],
          answers: [],
          createdAt: "2026-07-12T00:00:00Z",
          updatedAt: "2026-07-12T00:00:00Z",
        },
      },
    ]);

    const notice = wrapper.get(".adk-input-request-notice");
    expect(notice.classes()).toContain("is-pending");
    expect(notice.text()).toContain("选择执行方式");
    expect(notice.text()).toContain("正在等待你的回答");
    expect(wrapper.find(".adk-input-card").exists()).toBe(false);
  });

  it("shows original user prompt by default and toggles processed prompt", async () => {
    const wrapper = mountThread([
      {
        id: "entry-user-prompt",
        sessionId: "session-1",
        runId: "run-1",
        kind: "user_message",
        createdAt: "2026-06-17T00:00:00Z",
        sequence: 1,
        text: "设计个适合 tme 的策略",
        originalText: "设计个适合 tme 的策略",
        processedText: "请推进这个目标。\n\n用户原始目标：设计个适合 tme 的策略",
      },
    ]);

    expect(wrapper.text()).toContain("设计个适合 tme 的策略");
    expect(wrapper.text()).not.toContain("请推进这个目标");
    expect(wrapper.find(".adk-bubble--user-processed").exists()).toBe(false);

    const promptRow = wrapper.find(".adk-user-prompt-row");
    expect(promptRow.exists()).toBe(true);
    expect(promptRow.find(".adk-user-prompt-toggle").exists()).toBe(true);

    await wrapper.findAll("button").find((button) => button.text() === "可观测")?.trigger("click");
    await nextTick();

    expect(wrapper.text()).toContain("请推进这个目标");
    expect(wrapper.find(".adk-bubble--user-processed").exists()).toBe(true);

    await wrapper.findAll("button").find((button) => button.text() === "原文")?.trigger("click");
    await nextTick();

    expect(wrapper.text()).toContain("设计个适合 tme 的策略");
    expect(wrapper.text()).not.toContain("请推进这个目标");
    expect(wrapper.find(".adk-bubble--user-processed").exists()).toBe(false);
  });

  it("does not show a prompt toggle for unchanged user prompts", () => {
    const wrapper = mountThread([
      {
        id: "entry-user-plain",
        sessionId: "session-1",
        kind: "user_message",
        createdAt: "2026-06-17T00:00:00Z",
        sequence: 1,
        text: "普通问题",
      },
    ]);

    expect(wrapper.text()).toContain("普通问题");
    expect(wrapper.text()).not.toContain("可观测");
  });

  it("caches rendered markdown across unrelated thread updates", async () => {
    const renderMarkdown = vi.fn((content: string) => `<p>${content}</p>`);
    const wrapper = mountThread(
      [
        {
          id: "entry-assistant",
          sessionId: "session-1",
          kind: "assistant_message",
          createdAt: "2026-06-17T00:00:00Z",
          sequence: 1,
          text: "**hello**",
        },
      ],
      { renderMarkdown },
    );

    expect(renderMarkdown).toHaveBeenCalledTimes(1);

    await wrapper.setProps({ errorMessage: "warning" });
    await nextTick();

    expect(renderMarkdown).toHaveBeenCalledTimes(1);
  });

  it("opens rendered markdown links through the shared external link opener", async () => {
    const open = vi.spyOn(window, "open").mockImplementation(() => null);
    const wrapper = mountThread(
      [
        {
          id: "entry-assistant-link",
          sessionId: "session-1",
          kind: "assistant_message",
          createdAt: "2026-06-17T00:00:00Z",
          sequence: 1,
          text: "docs",
        },
      ],
      {
        renderMarkdown: () =>
          '<p><a href="https://docs.example.com/reference">文档参考</a></p>',
      },
    );

    await wrapper.get(".adk-markdown a").trigger("click");

    expect(open).toHaveBeenCalledWith(
      "https://docs.example.com/reference",
      "_blank",
      "noopener,noreferrer",
    );
  });

  it("shows provider hints, suggestion chips, and timeline window actions", async () => {
    const wrapper = mountThread(
      [
        {
          id: "entry-assistant-window",
          sessionId: "session-1",
          kind: "assistant_message",
          createdAt: "2026-06-17T00:00:00Z",
          sequence: 4,
          text: "最新回复",
        },
      ],
      {
        props: {
          suggestions: ["复盘组合", "检查风险限额"],
          emptyStateProviderHint: "先配置模型服务再开始对话",
          timelineTotal: 10,
          timelineWindowStart: 2,
          timelineWindowEnd: 4,
          timelineAtLatest: false,
        },
      },
    );

    expect(wrapper.text()).toContain("3-4 / 10");

    const windowButtons = wrapper.findAll(".adk-timeline-window button");
    await windowButtons[0]!.trigger("click");
    await windowButtons[1]!.trigger("click");
    await windowButtons[2]!.trigger("click");
    expect(wrapper.emitted("showOlderTimeline")).toHaveLength(1);
    expect(wrapper.emitted("showNewerTimeline")).toHaveLength(1);
    expect(wrapper.emitted("showLatestTimeline")).toHaveLength(1);

    const emptyWrapper = mountThread([], {
      props: {
        suggestions: ["先看持仓"],
        emptyStateProviderHint: "先配置模型服务再开始对话",
      },
    });
    expect(emptyWrapper.text()).toContain("先配置模型服务再开始对话");
    expect(emptyWrapper.text()).toContain("先看持仓");
    await emptyWrapper.get("button").trigger("click");
    expect(emptyWrapper.emitted("update:chatDraft")?.at(-1)).toEqual([
      "先看持仓",
    ]);
  });

  it("renders context notices and toggles assistant reasoning details", async () => {
    const wrapper = mountThread([
      {
        id: "entry-notice-stream",
        sessionId: "session-1",
        kind: "context_notice",
        createdAt: "2026-06-17T00:00:00Z",
        sequence: 1,
        status: "streaming",
        text: "上下文刷新中",
      },
      {
        id: "entry-notice-error",
        sessionId: "session-1",
        kind: "context_notice",
        createdAt: "2026-06-17T00:00:01Z",
        sequence: 2,
        status: "error",
        text: "上下文刷新失败",
      },
      {
        id: "entry-notice-ok",
        sessionId: "session-1",
        kind: "context_notice",
        createdAt: "2026-06-17T00:00:02Z",
        sequence: 3,
        status: "completed",
        text: "上下文已同步",
      },
      {
        id: "entry-reasoning",
        sessionId: "session-1",
        kind: "assistant_reasoning",
        createdAt: "2026-06-17T00:00:03Z",
        sequence: 4,
        text: "这里是深度思考内容",
      },
    ]);

    expect(wrapper.find(".v-progress-linear-stub").exists()).toBe(true);
    expect(wrapper.find(".adk-context-notice.is-streaming").exists()).toBe(true);
    expect(wrapper.find(".adk-context-notice.is-error").exists()).toBe(true);
    expect(wrapper.text()).toContain("fa-solid fa-circle-exclamation");
    expect(wrapper.text()).toContain("fa-solid fa-check");
    expect(wrapper.text()).toContain("查看深度思考");
    expect(wrapper.text()).not.toContain("这里是深度思考内容");

    await wrapper.get(".adk-reasoning-toggle").trigger("click");
    await nextTick();

    expect(wrapper.text()).toContain("隐藏深度思考");
    expect(wrapper.text()).toContain("这里是深度思考内容");
  });

  it("passes active tool-group state through to the run trace and reacts to trace events", async () => {
    const runTraceStub = defineComponent({
      props: [
        "run",
        "toolProgress",
        "busy",
        "variant",
        "summaryExpanded",
        "expandedToolCallIds",
      ],
      emits: ["update:summaryExpanded", "update:expandedToolCallIds"],
      template: `
        <div class="trace-stub">
          <span class="trace-run-status">{{ run.status }}</span>
          <span class="trace-tool-status">{{ run.toolCalls?.[0]?.status }}</span>
          <span class="trace-progress">{{ toolProgress }}</span>
          <span class="trace-busy">{{ busy ? 'busy' : 'idle' }}</span>
          <span class="trace-variant">{{ variant }}</span>
          <button type="button" class="emit-summary" @click="$emit('update:summaryExpanded', true)">summary</button>
          <button type="button" class="emit-tools" @click="$emit('update:expandedToolCallIds', ['tool-1'])">tools</button>
        </div>
      `,
    });
    const wrapper = mountThread(
      [
        {
          id: "entry-tool-group",
          sessionId: "session-1",
          runId: "run-1",
          kind: "tool_group",
          createdAt: "2026-06-17T00:00:00Z",
          sequence: 1,
          status: "streaming",
          toolCalls: [
            {
              id: "tool-1",
              runId: "run-1",
              toolName: "portfolio.summary",
              permission: "read",
              status: "PENDING",
              input: { query: "portfolio.summary" },
              requiresUser: false,
              createdAt: "2026-06-17T00:00:00Z",
              updatedAt: "2026-06-17T00:00:00Z",
            },
          ],
        },
      ],
      {
        props: {
          activeRunId: "run-1",
          activeRunStatus: "RUNNING",
          hasBlockingRun: true,
          sendingChat: true,
          timelineAtLatest: true,
        },
        stubs: {
          ADKRunTrace: runTraceStub,
        },
      },
    );

    const trace = wrapper.findComponent(runTraceStub);
    expect(trace.text()).toContain("RUNNING");
    expect(trace.text()).toContain("工具执行中...");
    expect(trace.text()).toContain("busy");
    expect(trace.text()).toContain("RUNNING");
    expect(trace.text()).toContain("timeline");

    await trace.get(".emit-summary").trigger("click");
    await nextTick();
    expect(trace.props("summaryExpanded")).toBe(true);

    await trace.get(".emit-tools").trigger("click");
    await nextTick();
    expect(trace.props("expandedToolCallIds")).toEqual(["tool-1"]);
  });

  it("derives waiting progress labels for pending tool groups", () => {
    const runTraceStub = defineComponent({
      props: ["toolProgress", "run"],
      template:
        "<div class='trace-stub'>{{ toolProgress }} / {{ run.toolCalls?.[0]?.status }}</div>",
    });

    const pendingApproval = mountThread(
      [
        {
          id: "entry-tool-approval",
          sessionId: "session-1",
          runId: "run-approval",
          kind: "tool_group",
          createdAt: "2026-06-17T00:00:00Z",
          sequence: 1,
          status: "idle",
          toolCalls: [
            {
              id: "tool-approval",
              runId: "run-approval",
              toolName: "trade.submit",
              permission: "write",
              status: "PENDING_APPROVAL",
              input: {},
              requiresUser: true,
              createdAt: "2026-06-17T00:00:00Z",
              updatedAt: "2026-06-17T00:00:00Z",
            },
          ],
        },
      ],
      {
        props: {
          activeRunId: "run-approval",
          activeRunStatus: "PENDING_APPROVAL",
          hasBlockingRun: true,
        },
        stubs: {
          ADKRunTrace: runTraceStub,
        },
      },
    );
    expect(pendingApproval.text()).toContain("等待审批...");
    expect(pendingApproval.text()).toContain("PENDING_APPROVAL");

    const pendingExecution = mountThread(
      [
        {
          id: "entry-tool-pending",
          sessionId: "session-1",
          runId: "run-pending",
          kind: "tool_group",
          createdAt: "2026-06-17T00:00:00Z",
          sequence: 1,
          status: "streaming",
          toolCalls: [
            {
              id: "tool-pending",
              runId: "run-pending",
              toolName: "trade.plan",
              permission: "read",
              status: "PENDING",
              input: {},
              requiresUser: false,
              createdAt: "2026-06-17T00:00:00Z",
              updatedAt: "2026-06-17T00:00:00Z",
            },
          ],
        },
        {
          id: "entry-tool-streaming",
          sessionId: "session-1",
          runId: "run-streaming",
          kind: "tool_group",
          createdAt: "2026-06-17T00:00:01Z",
          sequence: 2,
          status: "streaming",
          toolCalls: [
            {
              id: "tool-streaming",
              runId: "run-streaming",
              toolName: "market.scan",
              permission: "read",
              status: "RUNNING",
              input: {},
              requiresUser: false,
              createdAt: "2026-06-17T00:00:01Z",
              updatedAt: "2026-06-17T00:00:01Z",
            },
          ],
        },
      ],
      {
        props: {
          activeRunId: "run-pending",
          activeRunStatus: "PENDING",
          hasBlockingRun: true,
        },
        stubs: {
          ADKRunTrace: runTraceStub,
        },
      },
    );
    expect(pendingExecution.text()).toContain("等待执行...");
    expect(pendingExecution.text()).toContain("工具执行中...");
  });
});
