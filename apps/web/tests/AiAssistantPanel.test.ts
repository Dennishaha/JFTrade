// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import { nextTick } from "vue";

import type {
  ADKApproval,
  ADKApprovalResolution,
  ADKChatResponse,
  ADKRun,
  ADKTimelineEntry,
} from "@/contracts";

import AiAssistantPanel from "../src/layout/AiAssistantPanel.vue";
import { createResponse, flushRequests } from "./helpers";

const { monitorADKRunContinuationMock, streamADKChatMock } = vi.hoisted(() => ({
  monitorADKRunContinuationMock: vi.fn(),
  streamADKChatMock: vi.fn(),
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

describe("AiAssistantPanel", () => {
  it("reuses shared dock thread and composer components", async () => {
    mountPanel();
    await flushRequests();

    expect(document.querySelector(".adk-thread--dock")).not.toBeNull();
    expect(document.querySelector(".adk-chat-thread--dock")).not.toBeNull();
    expect(document.querySelector(".adk-empty--dock")).not.toBeNull();
    expect(document.querySelector(".adk-composer--dock")).not.toBeNull();
    expect(document.querySelector(".adk-agent-select")).toBeNull();
    expect(document.querySelector(".adk-provider-select")).toBeNull();
    expect(document.body.textContent).toContain("开始与侧栏助手对话");
  });

  it("refreshes dock approval state to RUNNING and keeps the composer editable", async () => {
    const approval = buildApproval("approval-1", "run-dock");
    const pendingRun = buildRun({
      id: "run-dock",
      status: "PENDING_APPROVAL",
      toolCalls: [
        buildToolCall("tool-dock", "run-dock", "strategy.save_draft", "PENDING_APPROVAL"),
      ],
      pendingApprovals: [approval],
    });
    const runningRun = buildRun({
      id: "run-dock",
      status: "RUNNING",
      toolCalls: [
        buildToolCall("tool-dock", "run-dock", "strategy.save_draft", "RUNNING"),
      ],
      pendingApprovals: [],
    });
    const completedRun = buildRun({
      id: "run-dock",
      status: "COMPLETED",
      toolCalls: [
        buildToolCall("tool-dock", "run-dock", "strategy.save_draft", "SUCCEEDED"),
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
        run: pendingRun,
        session: buildSession(),
        pendingApprovals: [approval],
        timeline: pendingApprovalTimeline(pendingRun, [approval], "dock approval"),
      };
      await onEvent({ type: "session", session: response.session });
      await onEvent({ type: "final", response });
      return response;
    });

    mountPanel({
      approvals: [approval],
      approvalResolutionById: {
        "approval-1": {
          approval: { ...approval, status: "APPROVED" },
          run: runningRun,
        },
      },
      sessionDetailSequence: [
        {
          session: buildSession(),
          timeline: [
            buildTimelineEntry("tool_group", {
              id: "dock-running-tools",
              runId: runningRun.id,
              toolCalls: runningRun.toolCalls,
              createdAt: "2026-06-09T00:00:02Z",
            }),
          ],
        },
        {
          session: buildSession(),
          timeline: [
            buildTimelineEntry("tool_group", {
              id: "dock-running-tools-2",
              runId: runningRun.id,
              toolCalls: runningRun.toolCalls,
              createdAt: "2026-06-09T00:00:02Z",
            }),
          ],
        },
        {
          session: buildSession(),
          timeline: [
            buildTimelineEntry("tool_group", {
              id: "dock-completed-tools",
              runId: completedRun.id,
              toolCalls: completedRun.toolCalls,
              createdAt: "2026-06-09T00:00:02Z",
            }),
            buildTimelineEntry("assistant_message", {
              id: "dock-answer",
              runId: completedRun.id,
              text: "dock approval completed",
              createdAt: "2026-06-09T00:00:03Z",
            }),
          ],
        },
      ],
    });
    await flushRequests();

    await sendDockMessage("dock approval");

    expect(document.body.textContent).toContain("PENDING_APPROVAL");
    expect(document.querySelector("input")?.hasAttribute("disabled")).toBe(false);

    clickButtonByText("批准");
    await flushRequests();

    expect(document.querySelector(".adk-run-spinner")).not.toBeNull();
    expect(document.querySelector(".adk-approvals-approve-all")).toBeNull();
    expect(document.querySelector("input")?.hasAttribute("disabled")).toBe(false);
    expect(document.body.textContent).not.toContain("dock approval completed");

    finishContinuation();
    await flushRequests();

    expect(document.body.textContent).toContain("dock approval completed");
  });

  it("queues dock messages and auto-dispatches them after the blocking run completes", async () => {
    const approval = buildApproval("approval-queue", "run-queue");
    const pendingRun = buildRun({
      id: "run-queue",
      status: "PENDING_APPROVAL",
      toolCalls: [
        buildToolCall("tool-queue", "run-queue", "strategy.save_draft", "PENDING_APPROVAL"),
      ],
      pendingApprovals: [approval],
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
      id: "run-queue-next",
      status: "COMPLETED",
      userMessage: "dock follow-up",
    });

    streamADKChatMock
      .mockImplementationOnce(async (_payload, onEvent) => {
        const response: ADKChatResponse = {
          reply: "waiting",
          run: pendingRun,
          session: buildSession(),
          pendingApprovals: [approval],
          timeline: pendingApprovalTimeline(pendingRun, [approval], "dock first"),
        };
        await onEvent({ type: "session", session: response.session });
        await onEvent({ type: "final", response });
        return response;
      })
      .mockImplementationOnce(async (payload, onEvent) => {
        const response: ADKChatResponse = {
          reply: "queued sent",
          run: queuedRun,
          session: buildSession(),
          pendingApprovals: [],
          timeline: [
            buildTimelineEntry("user_message", {
              id: "dock-queued-user",
              text: String(payload.message),
              createdAt: "2026-06-09T00:00:04Z",
            }),
            buildTimelineEntry("assistant_message", {
              id: "dock-queued-answer",
              runId: queuedRun.id,
              text: "dock follow-up completed",
              createdAt: "2026-06-09T00:00:05Z",
            }),
          ],
        };
        await onEvent({ type: "session", session: response.session });
        await onEvent({ type: "final", response });
        return response;
      });

    mountPanel({
      approvals: [approval],
      approvalResolutionById: {
        "approval-queue": {
          approval: { ...approval, status: "APPROVED" },
          run: completedRun,
        },
      },
      sessionDetail: {
        session: buildSession(),
        timeline: [
          buildTimelineEntry("assistant_message", {
            id: "dock-complete",
            runId: completedRun.id,
            text: "dock first completed",
            createdAt: "2026-06-09T00:00:03Z",
          }),
        ],
      },
    });
    await flushRequests();

    await sendDockMessage("dock first");
    await sendDockMessage("dock follow-up");

    expect(streamADKChatMock).toHaveBeenCalledTimes(1);
    expect(document.body.textContent).toContain("dock follow-up");

    clickButtonByText("批准");
    await flushRequests();

    expect(streamADKChatMock).toHaveBeenCalledTimes(2);
    expect(streamADKChatMock.mock.calls[1]?.[0]).toMatchObject({
      message: "dock follow-up",
    });
    expect(document.body.textContent).toContain("dock follow-up completed");
  });

  it("cancels the current dock run and sends the interrupt message before the rest of the queue", async () => {
    const approval = buildApproval("approval-interrupt", "run-interrupt");
    const pendingRun = buildRun({
      id: "run-interrupt",
      status: "PENDING_APPROVAL",
      toolCalls: [
        buildToolCall("tool-interrupt", "run-interrupt", "strategy.save_draft", "PENDING_APPROVAL"),
      ],
      pendingApprovals: [approval],
    });
    const cancelledRun = buildRun({
      id: "run-interrupt",
      status: "CANCELLED",
      pendingApprovals: [],
    });
    const urgentRun = buildRun({
      id: "run-urgent",
      status: "COMPLETED",
      userMessage: "dock urgent",
    });
    const normalRun = buildRun({
      id: "run-normal",
      status: "COMPLETED",
      userMessage: "dock normal",
    });

    streamADKChatMock
      .mockImplementationOnce(async (_payload, onEvent) => {
        const response: ADKChatResponse = {
          reply: "waiting",
          run: pendingRun,
          session: buildSession(),
          pendingApprovals: [approval],
          timeline: pendingApprovalTimeline(pendingRun, [approval], "dock first"),
        };
        await onEvent({ type: "session", session: response.session });
        await onEvent({ type: "final", response });
        return response;
      })
      .mockImplementationOnce(async (payload, onEvent) => {
        const response: ADKChatResponse = {
          reply: "urgent done",
          run: urgentRun,
          session: buildSession(),
          pendingApprovals: [],
          timeline: [
            buildTimelineEntry("user_message", {
              id: "dock-urgent-user",
              text: String(payload.message),
              createdAt: "2026-06-09T00:00:04Z",
            }),
            buildTimelineEntry("assistant_message", {
              id: "dock-urgent-answer",
              runId: urgentRun.id,
              text: "dock urgent completed",
              createdAt: "2026-06-09T00:00:05Z",
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
          run: normalRun,
          session: buildSession(),
          pendingApprovals: [],
          timeline: [
            buildTimelineEntry("user_message", {
              id: "dock-normal-user",
              text: String(payload.message),
              createdAt: "2026-06-09T00:00:06Z",
            }),
            buildTimelineEntry("assistant_message", {
              id: "dock-normal-answer",
              runId: normalRun.id,
              text: "dock normal completed",
              createdAt: "2026-06-09T00:00:07Z",
            }),
          ],
        };
        await onEvent({ type: "session", session: response.session });
        await onEvent({ type: "final", response });
        return response;
      });

    const fetchMock = mountPanel({
      approvals: [approval],
      cancelRunById: {
        "run-interrupt": cancelledRun,
      },
      sessionDetail: {
        session: buildSession(),
        timeline: [
          buildTimelineEntry("assistant_message", {
            id: "dock-cancelled",
            runId: cancelledRun.id,
            text: "dock first cancelled",
            createdAt: "2026-06-09T00:00:03Z",
          }),
        ],
      },
    });
    await flushRequests();

    await sendDockMessage("dock first");
    await sendDockMessage("dock normal");

    const input = document.querySelector("input")!;
    input.value = "dock urgent";
    input.dispatchEvent(new Event("input"));
    await nextTick();
    clickButtonByText("打断后发送");
    await flushRequests();

    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining("/api/v1/adk/runs/run-interrupt/cancel"),
      expect.objectContaining({ method: "POST" }),
    );
    expect(streamADKChatMock).toHaveBeenCalledTimes(3);
    expect(streamADKChatMock.mock.calls[1]?.[0]).toMatchObject({
      message: "dock urgent",
    });
    expect(streamADKChatMock.mock.calls[2]?.[0]).toMatchObject({
      message: "dock normal",
    });
    expect(document.body.textContent).toContain("dock normal completed");
  });

  it("keeps rendering when the final response contains null ADK arrays", async () => {
    streamADKChatMock.mockImplementationOnce(async (_payload, onEvent) => {
      const response: ADKChatResponse = {
        reply: "你好！我是JFTrade投资分析助手。",
        session: buildSession(),
        run: {
          ...buildRun({ id: "run-null-arrays", status: "COMPLETED" }),
          toolCalls: null as unknown as ADKRun["toolCalls"],
          pendingApprovals: null as unknown as ADKRun["pendingApprovals"],
        },
        pendingApprovals: null as unknown as ADKApproval[],
        timeline: [
          buildTimelineEntry("assistant_reasoning", {
            id: "dock-reasoning",
            text: "深度思考内容",
            createdAt: "2026-06-09T00:00:01Z",
          }),
          buildTimelineEntry("tool_group", {
            id: "dock-null-tools",
            runId: "run-null-arrays",
            toolCalls: null as unknown as ADKTimelineEntry["toolCalls"],
            createdAt: "2026-06-09T00:00:02Z",
          }),
          buildTimelineEntry("assistant_message", {
            id: "dock-null-answer",
            runId: "run-null-arrays",
            text: "你好！我是JFTrade投资分析助手。",
            createdAt: "2026-06-09T00:00:03Z",
          }),
        ],
      };
      await onEvent({ type: "session", session: response.session });
      await onEvent({ type: "final", response });
      return response;
    });

    mountPanel();
    await flushRequests();

    await sendDockMessage("你好");

    expect(document.body.textContent).toContain("查看深度思考");
    expect(document.body.textContent).toContain("你好！我是JFTrade投资分析助手。");
    expect(document.body.textContent).not.toContain(
      "Cannot read properties of null",
    );
  });
});

function mountPanel(options: {
  approvals?: ADKApproval[];
  approvalResolutionById?: Record<string, ADKApprovalResolution>;
  cancelRunById?: Record<string, ADKRun>;
  sessionDetail?: { session: ReturnType<typeof buildSession>; timeline: ADKTimelineEntry[] };
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

  const fetchMock = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    const approvalActionMatch = url.match(/\/api\/v1\/adk\/approvals\/([^/]+)\/(approve|deny)$/);
    if (approvalActionMatch) {
      const approvalId = approvalActionMatch[1]!;
      state.approvals = state.approvals.filter((approval) => approval.id !== approvalId);
      return createResponse(
        options.approvalResolutionById?.[approvalId] ?? {
          approval: {
            ...buildApproval(approvalId, "run-1"),
            status: init?.method === "POST" ? "APPROVED" : "DENIED",
          },
          run: buildRun({ status: "COMPLETED", pendingApprovals: [] }),
        },
      );
    }
    if (url.includes("/api/v1/adk/approvals")) {
      return createResponse({ approvals: state.approvals });
    }
    const cancelRunMatch = url.match(/\/api\/v1\/adk\/runs\/([^/]+)\/cancel$/);
    if (cancelRunMatch) {
      const runId = decodeURIComponent(cancelRunMatch[1]!);
      return createResponse(
        options.cancelRunById?.[runId] ??
          buildRun({ id: runId, status: "CANCELLED", pendingApprovals: [] }),
      );
    }
    if (/\/api\/v1\/adk\/sessions\/[^/]+$/.test(url)) {
      const detail =
        state.sessionDetailSequence.length > 1
          ? state.sessionDetailSequence.shift()!
          : state.sessionDetailSequence[0]!;
      return createResponse(detail);
    }
    return createResponse({});
  });
  vi.stubGlobal("fetch", fetchMock);

  mount(AiAssistantPanel, {
    attachTo: "#root",
    global: {
      stubs: vuetifyStubs(),
    },
  });

  return fetchMock;
}

function buildApproval(id: string, runId: string): ADKApproval {
  return {
    id,
    runId,
    agentId: "agent-1",
    toolName: "strategy.save_draft",
    input: { prompt: "save" },
    status: "PENDING",
    reason: "Needs review",
    createdAt: "2026-06-09T00:00:00Z",
    updatedAt: "2026-06-09T00:00:00Z",
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
    createdAt: "2026-06-09T00:00:00Z",
    updatedAt: "2026-06-09T00:00:00Z",
  };
}

function buildRun(overrides: Partial<ADKRun>): ADKRun {
  return {
    id: "run-1",
    sessionId: "session-1",
    agentId: "agent-1",
    status: "COMPLETED",
    message: "completed",
    userMessage: "dock message",
    toolSummaries: [],
    toolCalls: [],
    pendingApprovals: [],
    createdAt: "2026-06-09T00:00:00Z",
    updatedAt: "2026-06-09T00:00:00Z",
    ...overrides,
  };
}

function buildSession() {
  return {
    id: "session-1",
    agentId: "agent-1",
    title: "Dock Session",
    createdAt: "2026-06-09T00:00:00Z",
    updatedAt: "2026-06-09T00:00:00Z",
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
    createdAt: overrides.createdAt ?? "2026-06-09T00:00:00Z",
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
      createdAt: "2026-06-09T00:00:00Z",
    }),
    buildTimelineEntry("tool_group", {
      id: `tools-${run.id}`,
      runId: run.id,
      toolCalls: run.toolCalls,
      createdAt: "2026-06-09T00:00:01Z",
    }),
    buildTimelineEntry("approval_group", {
      id: `approvals-${run.id}`,
      runId: run.id,
      approvals,
      createdAt: "2026-06-09T00:00:02Z",
    }),
  ];
}

async function sendDockMessage(text: string): Promise<void> {
  const input = document.querySelector("input")!;
  input.value = text;
  input.dispatchEvent(new Event("input"));
  await nextTick();
  document.querySelector<HTMLButtonElement>(".adk-composer-send--dock")?.click();
  await flushRequests();
}

function clickButtonByText(text: string): void {
  Array.from(document.querySelectorAll<HTMLButtonElement>("button"))
    .find((button) => button.textContent?.includes(text))
    ?.click();
}

function vuetifyStubs() {
  return {
    "v-alert": {
      emits: ["click:close"],
      template: "<div :class='$attrs.class'><slot /></div>",
    },
    "v-btn": {
      props: ["disabled", "loading"],
      emits: ["click"],
      template:
        "<button type='button' :disabled='disabled' :class='$attrs.class' @click=\"$emit('click')\"><slot /></button>",
    },
    "v-chip": {
      emits: ["click"],
      template: "<button type='button' :class='$attrs.class' @click=\"$emit('click')\"><slot /></button>",
    },
    "v-card": { template: "<div><slot /></div>" },
    "v-card-text": { template: "<div><slot /></div>" },
    "v-card-title": { template: "<div><slot /></div>" },
    "v-icon": { template: "<span><slot /></span>" },
    "v-menu": {
      template: "<div><slot name='activator' :props='{}' /><slot /></div>",
    },
    "v-progress-circular": { template: "<span />" },
    "v-select": {
      props: ["modelValue", "items"],
      emits: ["update:modelValue"],
      template:
        "<select :value='modelValue' :class='$attrs.class' @change=\"$emit('update:modelValue', $event.target.value)\"><option v-for='item in items' :key='item.value' :value='item.value'>{{ item.title }}</option></select>",
    },
    "v-textarea": {
      props: ["modelValue", "disabled"],
      emits: ["update:modelValue"],
      template:
        "<textarea :value='modelValue' :disabled='disabled' :class='$attrs.class' @input=\"$emit('update:modelValue', $event.target.value)\" />",
    },
    "v-text-field": {
      props: ["modelValue", "disabled"],
      emits: ["update:modelValue"],
      template:
        "<input :value='modelValue' :disabled='disabled' :class='$attrs.class' @input=\"$emit('update:modelValue', $event.target.value)\" />",
    },
  };
}
