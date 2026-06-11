// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import { nextTick } from "vue";
import { createMemoryHistory, createRouter } from "vue-router";

import type {
  ADKApproval,
  ADKChatResponse,
  ADKRun,
  ADKTimelineEntry,
} from "@/contracts";

import ADKPage from "../src/pages/ADKPage.vue";
import { createResponse, flushRequests } from "./helpers";

const { monitorADKRunContinuationMock, streamADKChatMock } = vi.hoisted(() => ({
  monitorADKRunContinuationMock: vi.fn(),
  streamADKChatMock: vi.fn(),
}));

vi.mock("mermaid", () => ({
  default: {
    initialize: vi.fn(),
    run: vi.fn(),
  },
}));

vi.mock("../src/composables/adkChatStream", async () => {
  const actual = await vi.importActual<typeof import("../src/composables/adkChatStream")>(
    "../src/composables/adkChatStream",
  );
  return {
    ...actual,
    streamADKChat: streamADKChatMock,
  };
});

vi.mock("../src/composables/adkRunContinuation", async () => {
  const actual =
    await vi.importActual<typeof import("../src/composables/adkRunContinuation")>(
      "../src/composables/adkRunContinuation",
    );
  return {
    ...actual,
    monitorADKRunContinuation: monitorADKRunContinuationMock,
  };
});

afterEach(() => {
  vi.unstubAllGlobals();
  streamADKChatMock.mockReset();
  monitorADKRunContinuationMock.mockReset();
  monitorADKRunContinuationMock.mockImplementation(async (run) => run);
});

describe("ADKPage", () => {
  it("shows a composer block when the selected provider has no API key", async () => {
    mountADKPage({ providerHasKey: false });
    await flushRequests();

    expect(document.body.textContent).toContain("API Key");
  });

  it("keeps generic hints even when the selected agent exposes strategy Pine tools", async () => {
    mountADKPage({
      agent: {
        tools: [
          "strategy.pine_spec",
          "strategy.validate_pine",
          "strategy.save_definition",
          "strategy.update_instance_mode",
        ],
        skills: ["jftrade-strategy"],
      },
    });
    await flushRequests();

    expect(document.body.textContent).toContain("查看系统状态");
    expect(document.body.textContent).toContain("当前行情订阅");
    expect(document.body.textContent).not.toContain("解释当前 JFTrade Pine Script v6 定义");
    expect(document.querySelector("textarea")?.getAttribute("placeholder")).toBe("输入问题或任务...");
  });

  it("refreshes approval state to RUNNING, hides the approval bar, and keeps input editable", async () => {
    const pendingApproval = buildApproval("approval-1", "run-approval");
    const pendingRun = buildRun({
      id: "run-approval",
      status: "PENDING_APPROVAL",
      toolCalls: [
        buildToolCall("tool-1", "run-approval", "strategy.save_draft", "PENDING_APPROVAL"),
      ],
      pendingApprovals: [pendingApproval],
    });
    const runningRun = buildRun({
      id: "run-approval",
      status: "RUNNING",
      toolCalls: [
        buildToolCall("tool-1", "run-approval", "strategy.save_draft", "RUNNING"),
      ],
      pendingApprovals: [],
    });
    const completedRun = buildRun({
      id: "run-approval",
      status: "COMPLETED",
      toolCalls: [
        buildToolCall("tool-1", "run-approval", "strategy.save_draft", "SUCCEEDED"),
      ],
      pendingApprovals: [],
    });

    let finishContinuation!: () => void;
    monitorADKRunContinuationMock.mockImplementationOnce(async (run, options) => {
      await options?.onProgress?.(runningRun, run!);
      await new Promise<void>((resolve) => {
        finishContinuation = resolve;
      });
      await options?.onTerminal?.(completedRun);
      return completedRun;
    });

    streamADKChatMock.mockImplementationOnce(async (_payload, onEvent) => {
      const response: ADKChatResponse = {
        reply: "waiting",
        session: buildSession(),
        run: pendingRun,
        pendingApprovals: [pendingApproval],
        timeline: pendingApprovalTimeline(pendingRun, [pendingApproval], "approve this"),
      };
      await onEvent({ type: "session", session: response.session });
      await onEvent({ type: "final", response });
      return response;
    });

    mountADKPage({
      approvals: [pendingApproval],
      approvalResolution: {
        approval: { ...pendingApproval, status: "APPROVED" },
        run: runningRun,
      },
      sessionDetailSequence: [
        {
          session: buildSession(),
          timeline: [
            buildTimelineEntry("tool_group", {
              id: "running-tools",
              runId: runningRun.id,
              toolCalls: runningRun.toolCalls,
              createdAt: "2026-06-06T00:00:02Z",
            }),
          ],
        },
        {
          session: buildSession(),
          timeline: [
            buildTimelineEntry("tool_group", {
              id: "running-tools-2",
              runId: runningRun.id,
              toolCalls: runningRun.toolCalls,
              createdAt: "2026-06-06T00:00:02Z",
            }),
          ],
        },
        {
          session: buildSession(),
          timeline: [
            buildTimelineEntry("tool_group", {
              id: "completed-tools",
              runId: completedRun.id,
              toolCalls: completedRun.toolCalls,
              createdAt: "2026-06-06T00:00:02Z",
            }),
            buildTimelineEntry("assistant_message", {
              id: "completed-answer",
              runId: completedRun.id,
              text: "approved and finished",
              createdAt: "2026-06-06T00:00:03Z",
            }),
          ],
        },
      ],
    });
    await flushRequests();

    await sendPageMessage("approve this");

    expect(document.body.textContent).toContain("PENDING_APPROVAL");
    expect(document.querySelector("textarea")?.hasAttribute("disabled")).toBe(false);

    clickButtonByText("批准");
    await flushRequests();

    expect(document.querySelector(".adk-run-spinner")).not.toBeNull();
    expect(document.querySelector(".adk-approvals-approve-all")).toBeNull();
    expect(document.querySelector("textarea")?.hasAttribute("disabled")).toBe(false);
    expect(document.body.textContent).not.toContain("approved and finished");

    finishContinuation();
    await flushRequests();

    expect(document.body.textContent).toContain("approved and finished");
  });

  it("approves all pending approvals from the inline approval group", async () => {
    const approvalA = buildApproval("approval-1");
    const approvalB = {
      ...approvalA,
      id: "approval-2",
      toolName: "strategy.publish",
      input: { query: "@strategy.publish" },
    };

    streamADKChatMock.mockImplementationOnce(async (_payload, onEvent) => {
      const response: ADKChatResponse = {
        reply: "waiting",
        session: buildSession(),
        run: buildRun({
          status: "PENDING_APPROVAL",
          toolCalls: [],
          pendingApprovals: [approvalA, approvalB],
        }),
        pendingApprovals: [approvalA, approvalB],
        timeline: [
          buildTimelineEntry("user_message", {
            id: "entry-user",
            text: "batch approve",
            createdAt: "2026-06-06T00:00:00Z",
          }),
          buildTimelineEntry("approval_group", {
            id: "entry-approvals",
            approvals: [approvalA, approvalB],
            createdAt: "2026-06-06T00:00:01Z",
          }),
        ],
      };
      await onEvent({ type: "session", session: response.session });
      await onEvent({ type: "final", response });
      return response;
    });

    const fetchMock = mountADKPage({
      approvals: [approvalA, approvalB],
      approvalResolutionById: {
        "approval-1": {
          approval: { ...approvalA, status: "APPROVED" },
          run: buildRun({ id: "run-a", status: "COMPLETED" }),
        },
        "approval-2": {
          approval: { ...approvalB, status: "APPROVED" },
          run: buildRun({ id: "run-b", status: "COMPLETED" }),
        },
      },
    });
    await flushRequests();

    await sendPageMessage("batch approve");
    clickButtonByText("全部批准");
    await flushRequests();

    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining("/api/v1/adk/approvals/approval-1/approve"),
      expect.objectContaining({ method: "POST" }),
    );
    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining("/api/v1/adk/approvals/approval-2/approve"),
      expect.objectContaining({ method: "POST" }),
    );
  });

  it("queues, revokes, and auto-dispatches messages while a blocking run is active", async () => {
    const pendingApproval = buildApproval("approval-queue", "run-queue");
    const pendingRun = buildRun({
      id: "run-queue",
      status: "PENDING_APPROVAL",
      toolCalls: [
        buildToolCall("tool-queue", "run-queue", "strategy.save_draft", "PENDING_APPROVAL"),
      ],
      pendingApprovals: [pendingApproval],
    });
    const completedRun = buildRun({
      id: "run-queue",
      status: "COMPLETED",
      toolCalls: [
        buildToolCall("tool-queue", "run-queue", "strategy.save_draft", "SUCCEEDED"),
      ],
      pendingApprovals: [],
    });
    const queuedRun = buildRun({
      id: "run-queue-2",
      status: "COMPLETED",
      userMessage: "queued follow-up",
    });

    streamADKChatMock
      .mockImplementationOnce(async (_payload, onEvent) => {
        const response: ADKChatResponse = {
          reply: "waiting",
          session: buildSession(),
          run: pendingRun,
          pendingApprovals: [pendingApproval],
          timeline: pendingApprovalTimeline(pendingRun, [pendingApproval], "first request"),
        };
        await onEvent({ type: "session", session: response.session });
        await onEvent({ type: "final", response });
        return response;
      })
      .mockImplementationOnce(async (payload, onEvent) => {
        const response: ADKChatResponse = {
          reply: "queued done",
          session: buildSession(),
          run: queuedRun,
          pendingApprovals: [],
          timeline: [
            buildTimelineEntry("user_message", {
              id: "queued-user",
              text: String(payload.message),
              createdAt: "2026-06-06T00:00:04Z",
            }),
            buildTimelineEntry("assistant_message", {
              id: "queued-answer",
              runId: queuedRun.id,
              text: "queued follow-up completed",
              createdAt: "2026-06-06T00:00:05Z",
            }),
          ],
        };
        await onEvent({ type: "session", session: response.session });
        await onEvent({ type: "final", response });
        return response;
      });

    mountADKPage({
      approvals: [pendingApproval],
      approvalResolution: {
        approval: { ...pendingApproval, status: "APPROVED" },
        run: completedRun,
      },
      sessionDetail: {
        session: buildSession(),
        timeline: [
          buildTimelineEntry("tool_group", {
            id: "entry-tools-done",
            runId: completedRun.id,
            toolCalls: completedRun.toolCalls,
            createdAt: "2026-06-06T00:00:02Z",
          }),
          buildTimelineEntry("assistant_message", {
            id: "entry-answer",
            runId: completedRun.id,
            text: "first request done",
            createdAt: "2026-06-06T00:00:03Z",
          }),
        ],
      },
    });
    await flushRequests();

    await sendPageMessage("first request");
    expect(document.querySelector("textarea")?.hasAttribute("disabled")).toBe(false);

    await sendPageMessage("revoke me");
    expect(document.body.textContent).toContain("revoke me");
    clickButtonByText("撤回");
    await flushRequests();
    expect(document.body.textContent).not.toContain("revoke me");
    expect(streamADKChatMock).toHaveBeenCalledTimes(1);

    await sendPageMessage("queued follow-up");
    expect(document.body.textContent).toContain("queued follow-up");
    expect(streamADKChatMock).toHaveBeenCalledTimes(1);

    clickButtonByText("批准");
    await flushRequests();

    expect(streamADKChatMock).toHaveBeenCalledTimes(2);
    expect(streamADKChatMock.mock.calls[1]?.[0]).toMatchObject({
      message: "queued follow-up",
    });
    expect(document.body.textContent).toContain("queued follow-up completed");
  });

  it("interrupts the active run and sends the interrupt message before the rest of the queue", async () => {
    const pendingApproval = buildApproval("approval-interrupt", "run-interrupt");
    const pendingRun = buildRun({
      id: "run-interrupt",
      status: "PENDING_APPROVAL",
      toolCalls: [
        buildToolCall("tool-interrupt", "run-interrupt", "strategy.save_draft", "PENDING_APPROVAL"),
      ],
      pendingApprovals: [pendingApproval],
    });
    const cancelledRun = buildRun({
      id: "run-interrupt",
      status: "CANCELLED",
      pendingApprovals: [],
    });
    const urgentRun = buildRun({
      id: "run-urgent",
      status: "COMPLETED",
      userMessage: "urgent request",
    });
    const normalRun = buildRun({
      id: "run-normal",
      status: "COMPLETED",
      userMessage: "normal queued request",
    });

    streamADKChatMock
      .mockImplementationOnce(async (_payload, onEvent) => {
        const response: ADKChatResponse = {
          reply: "waiting",
          session: buildSession(),
          run: pendingRun,
          pendingApprovals: [pendingApproval],
          timeline: pendingApprovalTimeline(pendingRun, [pendingApproval], "first request"),
        };
        await onEvent({ type: "session", session: response.session });
        await onEvent({ type: "final", response });
        return response;
      })
      .mockImplementationOnce(async (payload, onEvent) => {
        const response: ADKChatResponse = {
          reply: "urgent done",
          session: buildSession(),
          run: urgentRun,
          pendingApprovals: [],
          timeline: [
            buildTimelineEntry("user_message", {
              id: "urgent-user",
              text: String(payload.message),
              createdAt: "2026-06-06T00:00:04Z",
            }),
            buildTimelineEntry("assistant_message", {
              id: "urgent-answer",
              runId: urgentRun.id,
              text: "urgent completed",
              createdAt: "2026-06-06T00:00:05Z",
            }),
          ],
        };
        await onEvent({ type: "session", session: response.session });
        await onEvent({ type: "final", response });
        return response;
      })
      .mockImplementationOnce(async (payload, onEvent) => {
        const response: ADKChatResponse = {
          reply: "normal done",
          session: buildSession(),
          run: normalRun,
          pendingApprovals: [],
          timeline: [
            buildTimelineEntry("user_message", {
              id: "normal-user",
              text: String(payload.message),
              createdAt: "2026-06-06T00:00:06Z",
            }),
            buildTimelineEntry("assistant_message", {
              id: "normal-answer",
              runId: normalRun.id,
              text: "normal completed",
              createdAt: "2026-06-06T00:00:07Z",
            }),
          ],
        };
        await onEvent({ type: "session", session: response.session });
        await onEvent({ type: "final", response });
        return response;
      });

    const fetchMock = mountADKPage({
      approvals: [pendingApproval],
      cancelRunById: {
        "run-interrupt": cancelledRun,
      },
      sessionDetail: {
        session: buildSession(),
        timeline: [
          buildTimelineEntry("assistant_message", {
            id: "cancelled-answer",
            runId: cancelledRun.id,
            text: "first request cancelled",
            createdAt: "2026-06-06T00:00:03Z",
          }),
        ],
      },
    });
    await flushRequests();

    await sendPageMessage("first request");
    await sendPageMessage("normal queued request");
    expect(document.body.textContent).toContain("normal queued request");

    const textarea = document.querySelector("textarea")!;
    textarea.value = "urgent request";
    textarea.dispatchEvent(new Event("input"));
    await nextTick();
    clickButtonByText("打断后发送");
    await flushRequests();

    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining("/api/v1/adk/runs/run-interrupt/cancel"),
      expect.objectContaining({ method: "POST" }),
    );
    expect(streamADKChatMock).toHaveBeenCalledTimes(3);
    expect(streamADKChatMock.mock.calls[1]?.[0]).toMatchObject({
      message: "urgent request",
    });
    expect(streamADKChatMock.mock.calls[2]?.[0]).toMatchObject({
      message: "normal queued request",
    });
    expect(document.body.textContent).toContain("normal completed");
  });

  it("restores persisted timeline entries for saved sessions", async () => {
    const savedRun = buildRun({
      id: "run-restored",
      status: "COMPLETED",
      toolCalls: [
        buildToolCall("tool-restored", "run-restored", "portfolio.summary", "SUCCEEDED"),
      ],
    });
    mountADKPage({
      sessionDetail: {
        session: buildSession(),
        timeline: [
          buildTimelineEntry("user_message", {
            id: "msg-user",
            text: "查看系统状态",
            createdAt: "2026-06-06T00:00:01Z",
          }),
          buildTimelineEntry("assistant_message", {
            id: "entry-pre",
            runId: savedRun.id,
            text: "先查组合，再整理系统状态。",
            createdAt: "2026-06-06T00:00:02Z",
          }),
          buildTimelineEntry("tool_group", {
            id: "entry-tools",
            runId: savedRun.id,
            toolCalls: savedRun.toolCalls,
            createdAt: "2026-06-06T00:00:03Z",
          }),
          buildTimelineEntry("assistant_message", {
            id: "entry-final",
            runId: savedRun.id,
            text: "最终结论已经整理完成。",
            createdAt: "2026-06-06T00:00:04Z",
          }),
        ],
      },
    });
    await flushRequests();

    document.querySelector<HTMLElement>(".adk-session-item")?.click();
    await flushRequests();

    expect(document.body.textContent).toContain("先查组合，再整理系统状态。");
    expect(document.body.textContent).toContain("最终结论已经整理完成。");
    expect(document.body.textContent).toContain("portfolio.summary");
  });

  it("renders chat alerts inside the chat thread", async () => {
    streamADKChatMock.mockImplementationOnce(async (_payload, onEvent) => {
      await onEvent({ type: "session", session: buildSession() });
      await onEvent({ type: "error", message: "stream exploded" });
      throw new Error("stream exploded");
    });

    mountADKPage();
    await flushRequests();

    await sendPageMessage("check failed run");

    expect(document.querySelector(".adk-thread")?.textContent).toContain("stream exploded");
    expect(document.querySelector(".adk-inline-alert")?.textContent).toContain("stream exploded");
    expect(document.querySelector(".adk-composer")?.textContent).not.toContain("stream exploded");
  });

  it("keeps deep reasoning collapsed until the user expands it", async () => {
    streamADKChatMock.mockImplementationOnce(async (_payload, onEvent) => {
      const response: ADKChatResponse = {
        reply: "Final answer.",
        reasoningContent: "Detailed chain of thought preview.",
        session: buildSession(),
        run: buildRun({ status: "COMPLETED" }),
        pendingApprovals: [],
        timeline: [
          buildTimelineEntry("user_message", {
            id: "entry-user",
            text: "show reasoning",
            createdAt: "2026-06-06T00:00:00Z",
          }),
          buildTimelineEntry("assistant_reasoning", {
            id: "entry-reasoning",
            text: "Detailed chain of thought preview.",
            createdAt: "2026-06-06T00:00:01Z",
          }),
          buildTimelineEntry("assistant_message", {
            id: "entry-answer",
            text: "Final answer.",
            createdAt: "2026-06-06T00:00:02Z",
          }),
        ],
      };
      await onEvent({ type: "session", session: response.session });
      await onEvent({ type: "final", response });
      return response;
    });

    mountADKPage();
    await flushRequests();

    await sendPageMessage("show reasoning");

    expect(document.body.textContent).toContain("查看深度思考");
    expect(document.body.textContent).not.toContain("Detailed chain of thought preview.");

    clickButtonByText("查看深度思考");
    await nextTick();

    expect(document.body.textContent).toContain("隐藏深度思考");
    expect(document.body.textContent).toContain("Detailed chain of thought preview.");
  });

  it("restores persisted timeline entries even when tool and approval arrays are null", async () => {
    mountADKPage({
      sessionDetail: {
        session: buildSession(),
        timeline: [
          buildTimelineEntry("user_message", {
            id: "msg-user-null",
            text: "你好",
            createdAt: "2026-06-06T00:00:01Z",
          }),
          buildTimelineEntry("tool_group", {
            id: "entry-tools-null",
            runId: "run-null-history",
            toolCalls: null as unknown as ADKTimelineEntry["toolCalls"],
            createdAt: "2026-06-06T00:00:02Z",
          }),
          buildTimelineEntry("approval_group", {
            id: "entry-approvals-null",
            runId: "run-null-history",
            approvals: null as unknown as ADKTimelineEntry["approvals"],
            createdAt: "2026-06-06T00:00:03Z",
          }),
          buildTimelineEntry("assistant_message", {
            id: "entry-answer-null",
            runId: "run-null-history",
            text: "历史记录已恢复。",
            createdAt: "2026-06-06T00:00:04Z",
          }),
        ],
      },
    });
    await flushRequests();

    document.querySelector<HTMLElement>(".adk-session-item")?.click();
    await flushRequests();

    expect(document.body.textContent).toContain("历史记录已恢复。");
    expect(document.body.textContent).not.toContain(
      "Cannot read properties of null",
    );
  });
});

function mountADKPage(options: {
  providerHasKey?: boolean;
  agent?: Partial<ReturnType<typeof buildAgentBase>>;
  approvals?: ADKApproval[];
  approvalResolution?: unknown;
  approvalResolutionById?: Record<string, unknown>;
  cancelRunById?: Record<string, ADKRun>;
  sessionDetail?: {
    session: ReturnType<typeof buildSession>;
    timeline: ADKTimelineEntry[];
  };
  sessionDetailSequence?: Array<{
    session: ReturnType<typeof buildSession>;
    timeline: ADKTimelineEntry[];
  }>;
} = {}) {
  document.body.innerHTML = "<div id='root'></div>";
  const state = {
    approvals: [...(options.approvals ?? [])],
    sessionDetailSequence: [
      ...(options.sessionDetailSequence ?? [
        options.sessionDetail ?? { session: buildSession(), timeline: [] },
      ]),
    ],
  };

  const fetchMock = vi.fn(async (input: string | URL | Request) => {
    const url = String(input);
    if (url.includes("/api/v1/adk/agents")) {
      return createResponse({ agents: [buildAgent(options.agent)] });
    }
    if (url.includes("/api/v1/adk/providers")) {
      return createResponse({ providers: [buildProvider(options.providerHasKey ?? true)] });
    }
    if (url.includes("/api/v1/adk/sessions")) {
      if (/\/api\/v1\/adk\/sessions\/[^/]+$/.test(url)) {
        const detail =
          state.sessionDetailSequence.length > 1
            ? state.sessionDetailSequence.shift()!
            : state.sessionDetailSequence[0]!;
        return createResponse(detail);
      }
      return createResponse({ sessions: [buildSession()] });
    }
    const cancelRunMatch = url.match(/\/api\/v1\/adk\/runs\/([^/]+)\/cancel$/);
    if (cancelRunMatch) {
      const runId = decodeURIComponent(cancelRunMatch[1]!);
      return createResponse(
        options.cancelRunById?.[runId] ??
          buildRun({ id: runId, status: "CANCELLED", pendingApprovals: [] }),
      );
    }
    const approvalActionMatch = url.match(/\/api\/v1\/adk\/approvals\/([^/]+)\/(approve|deny)$/);
    if (approvalActionMatch) {
      const approvalId = approvalActionMatch[1]!;
      state.approvals = state.approvals.filter((approval) => approval.id !== approvalId);
      if (options.approvalResolutionById?.[approvalId] !== undefined) {
        return createResponse(options.approvalResolutionById[approvalId]);
      }
      return createResponse(options.approvalResolution);
    }
    if (url.includes("/api/v1/adk/approvals")) {
      return createResponse({ approvals: state.approvals });
    }
    return createResponse({});
  });
  vi.stubGlobal("fetch", fetchMock);

  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: "/", component: { template: "<div />" } }],
  });
  mount(ADKPage, {
    attachTo: "#root",
    global: {
      plugins: [router],
      stubs: vuetifyStubs(),
    },
  });
  return fetchMock;
}

function buildProvider(hasApiKey: boolean) {
  return {
    id: "provider-1",
    displayName: "OpenAI",
    baseUrl: "https://api.openai.com/v1",
    model: "gpt-4o-mini",
    enabled: true,
    hasApiKey,
    createdAt: "2026-06-06T00:00:00Z",
    updatedAt: "2026-06-06T00:00:00Z",
  };
}

function buildApproval(id: string, runId = "run-1"): ADKApproval {
  return {
    id,
    runId,
    agentId: "agent-1",
    toolName: "strategy.save_draft",
    input: { query: "@strategy.save_draft" },
    status: "PENDING",
    reason: "needs approval",
    createdAt: "2026-06-06T00:00:00Z",
    updatedAt: "2026-06-06T00:00:00Z",
  };
}

function buildAgent(overrides: Partial<ReturnType<typeof buildAgentBase>> = {}) {
  return {
    ...buildAgentBase(),
    ...overrides,
  };
}

function buildAgentBase() {
  return {
    id: "agent-1",
    name: "投资分析助手",
    instruction: "test",
    providerId: "provider-1",
    model: "gpt-4o-mini",
    tools: ["strategy.save_draft"],
    skills: [],
    permissionMode: "approval",
    memoryEnabled: true,
    recentUserWindow: 6,
    status: "ENABLED",
    createdAt: "2026-06-06T00:00:00Z",
    updatedAt: "2026-06-06T00:00:00Z",
  };
}

function buildSession() {
  return {
    id: "session-1",
    agentId: "agent-1",
    title: "测试会话",
    createdAt: "2026-06-06T00:00:00Z",
    updatedAt: "2026-06-06T00:00:00Z",
  };
}

function buildToolCall(
  id: string,
  runId: string,
  toolName: string,
  status: string,
) {
  return {
    id,
    runId,
    toolName,
    permission: "write_strategy",
    status,
    input: { toolName },
    requiresUser: status === "PENDING_APPROVAL",
    createdAt: "2026-06-06T00:00:00Z",
    updatedAt: "2026-06-06T00:00:00Z",
  };
}

function buildRun(overrides: Partial<ADKRun>): ADKRun {
  return {
    id: "run-1",
    sessionId: "session-1",
    agentId: "agent-1",
    status: "COMPLETED",
    message: "completed",
    userMessage: "@strategy.save_draft 保存策略",
    toolSummaries: [],
    toolCalls: [],
    pendingApprovals: [],
    createdAt: "2026-06-06T00:00:00Z",
    updatedAt: "2026-06-06T00:00:00Z",
    ...overrides,
  };
}

function buildTimelineEntry(
  kind: ADKTimelineEntry["kind"],
  overrides: Partial<ADKTimelineEntry> = {},
): ADKTimelineEntry {
  return {
    id: overrides.id ?? `entry-${kind}`,
    sessionId: overrides.sessionId ?? "session-1",
    kind,
    createdAt: overrides.createdAt ?? "2026-06-06T00:00:00Z",
    sequence: overrides.sequence ?? 1,
    status: overrides.status ?? "final",
    runId: overrides.runId,
    text: overrides.text,
    toolCalls: overrides.toolCalls,
    approvals: overrides.approvals,
  };
}

function pendingApprovalTimeline(
  run: ADKRun,
  approvals: ADKApproval[],
  userText: string,
): ADKTimelineEntry[] {
  return [
    buildTimelineEntry("user_message", {
      id: `user-${run.id}`,
      text: userText,
      createdAt: "2026-06-06T00:00:00Z",
    }),
    buildTimelineEntry("tool_group", {
      id: `tools-${run.id}`,
      runId: run.id,
      toolCalls: run.toolCalls,
      createdAt: "2026-06-06T00:00:01Z",
    }),
    buildTimelineEntry("approval_group", {
      id: `approvals-${run.id}`,
      runId: run.id,
      approvals,
      createdAt: "2026-06-06T00:00:02Z",
    }),
  ];
}

async function sendPageMessage(text: string): Promise<void> {
  const textarea = document.querySelector("textarea")!;
  textarea.value = text;
  textarea.dispatchEvent(new Event("input"));
  await nextTick();
  document.querySelector<HTMLButtonElement>(".adk-composer-send")?.click();
  await flushRequests();
}

function clickButtonByText(text: string): void {
  Array.from(document.querySelectorAll<HTMLButtonElement>("button"))
    .find((button) => button.textContent?.includes(text))
    ?.click();
}

function vuetifyStubs() {
  return {
    "v-alert": { template: "<div><slot /></div>" },
    "v-btn": {
      props: ["disabled", "loading"],
      emits: ["click"],
      template:
        "<button type='button' :disabled='disabled' :class='$attrs.class' @click=\"$emit('click')\"><slot /></button>",
    },
    "v-card": { template: "<div><slot /></div>" },
    "v-card-text": { template: "<div><slot /></div>" },
    "v-card-title": { template: "<div><slot /></div>" },
    "v-chip": { template: "<span><slot /></span>" },
    "v-expansion-panel": { template: "<div><slot /></div>" },
    "v-expansion-panel-text": { template: "<div><slot /></div>" },
    "v-expansion-panel-title": { template: "<div><slot /></div>" },
    "v-expansion-panels": { template: "<div><slot /></div>" },
    "v-icon": { template: "<span><slot /></span>" },
    "v-menu": {
      template: "<div><slot name='activator' :props='{}' /><slot /></div>",
    },
    "v-progress-circular": { template: "<span />" },
    "v-select": {
      props: ["modelValue", "items"],
      emits: ["update:modelValue"],
      template: "<select :value='modelValue' @change=\"$emit('update:modelValue', $event.target.value)\"><option v-for='item in items' :key='item.value' :value='item.value'>{{ item.title }}</option></select>",
    },
    "v-textarea": {
      props: ["modelValue", "disabled"],
      emits: ["update:modelValue"],
      template: "<textarea :value='modelValue' :disabled='disabled' @input=\"$emit('update:modelValue', $event.target.value)\" />",
    },
    "v-text-field": {
      props: ["modelValue", "disabled"],
      emits: ["update:modelValue"],
      template: "<input :value='modelValue' :disabled='disabled' @input=\"$emit('update:modelValue', $event.target.value)\" />",
    },
  };
}
