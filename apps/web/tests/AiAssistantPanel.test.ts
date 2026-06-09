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
} from "@jftrade/ui-contracts";

import AiAssistantPanel from "../src/layout/AiAssistantPanel.vue";
import { createResponse, flushRequests } from "./helpers";

const { streamADKChatMock } = vi.hoisted(() => ({
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

afterEach(() => {
  vi.unstubAllGlobals();
  streamADKChatMock.mockReset();
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
    expect(document.body.textContent).toContain("查看系统状态");
  });

  it("renders dock chat messages, reasoning, tool summary, and single approval actions", async () => {
    const approval = buildApproval("approval-1", "strategy.save_draft");
    const run = buildRun({
      status: "PENDING_APPROVAL",
      toolCalls: [
        buildToolCall("tool-1", "strategy.load_context", "SUCCEEDED"),
        buildToolCall("tool-2", "strategy.save_draft", "PENDING_APPROVAL"),
      ],
      pendingApprovals: [approval],
    });

    streamADKChatMock.mockImplementation(async (_payload, onEvent) => {
      const response: ADKChatResponse = {
        reply: "Here is the next step.",
        reasoningContent: "Detailed reasoning for the sidebar assistant.",
        run,
        session: {
          id: "session-1",
          agentId: "agent-1",
          title: "Dock Session",
          createdAt: "2026-06-09T00:00:00Z",
          updatedAt: "2026-06-09T00:00:00Z",
        },
        pendingApprovals: [approval],
        timeline: [
          buildTimelineEntry("user_message", {
            id: "entry-user",
            text: "check dock flow",
            createdAt: "2026-06-09T00:00:00Z",
          }),
          buildTimelineEntry("assistant_reasoning", {
            id: "entry-reasoning",
            runId: run.id,
            text: "Detailed reasoning for the sidebar assistant.",
            createdAt: "2026-06-09T00:00:01Z",
          }),
          buildTimelineEntry("tool_group", {
            id: "entry-tools",
            runId: run.id,
            toolCalls: run.toolCalls,
            createdAt: "2026-06-09T00:00:02Z",
          }),
          buildTimelineEntry("approval_group", {
            id: "entry-approvals",
            runId: run.id,
            approvals: [approval],
            createdAt: "2026-06-09T00:00:03Z",
          }),
          buildTimelineEntry("assistant_message", {
            id: "entry-answer",
            runId: run.id,
            text: "Here is the next step.",
            createdAt: "2026-06-09T00:00:04Z",
          }),
        ],
      };
      await onEvent({ type: "final", response });
      return response;
    });

    const fetchMock = mountPanel({
      approvals: [approval],
      approvalResolutionById: {
        "approval-1": {
          approval: { ...approval, status: "APPROVED" },
          run: buildRun({
            status: "COMPLETED",
            toolCalls: run.toolCalls,
            pendingApprovals: [],
          }),
        },
      },
    });
    await flushRequests();

    const input = document.querySelector("input");
    expect(input).not.toBeNull();
    input!.value = "check dock flow";
    input!.dispatchEvent(new Event("input"));
    await nextTick();

    Array.from(document.querySelectorAll<HTMLButtonElement>("button"))
      .find((button) => button.textContent?.includes("发送"))
      ?.click();
    await flushRequests();

    expect(document.body.textContent).toContain("check dock flow");
    expect(document.body.textContent).toContain("Here is the next step.");
    expect(document.body.textContent).toContain("调用了 2 个工具");
    expect(document.querySelector(".adk-approvals-approve-all")).not.toBeNull();
    expect(document.querySelector(".adk-approvals-deny-all")).not.toBeNull();

    Array.from(document.querySelectorAll<HTMLButtonElement>("button"))
      .find((button) => button.textContent?.includes("查看深度思考"))
      ?.click();
    await nextTick();

    expect(document.body.textContent).toContain("Detailed reasoning for the sidebar assistant.");

    Array.from(document.querySelectorAll<HTMLButtonElement>("button"))
      .find((button) => button.textContent?.includes("调用了 2 个工具"))
      ?.click();
    await nextTick();

    expect(document.body.textContent).toContain("strategy.load_context");
    expect(document.body.textContent).toContain("strategy.save_draft");

    Array.from(document.querySelectorAll<HTMLButtonElement>("button"))
      .find((button) => button.textContent?.includes("批准"))
      ?.click();
    await flushRequests();

    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining("/api/v1/adk/approvals/approval-1/approve"),
      expect.objectContaining({ method: "POST" }),
    );
  });

  it("approves all pending dock approvals through the shared approval bar", async () => {
    const approvalA = buildApproval("approval-1", "strategy.save_draft");
    const approvalB = buildApproval("approval-2", "strategy.publish");

    streamADKChatMock.mockImplementation(async (_payload, onEvent) => {
      const response: ADKChatResponse = {
        reply: "waiting for approvals",
        run: buildRun({
          status: "PENDING_APPROVAL",
          toolCalls: [
            buildToolCall("tool-1", "strategy.save_draft", "PENDING_APPROVAL"),
            buildToolCall("tool-2", "strategy.publish", "PENDING_APPROVAL"),
          ],
          pendingApprovals: [approvalA, approvalB],
        }),
        session: {
          id: "session-1",
          agentId: "agent-1",
          title: "Dock Session",
          createdAt: "2026-06-09T00:00:00Z",
          updatedAt: "2026-06-09T00:00:00Z",
        },
        pendingApprovals: [approvalA, approvalB],
        timeline: [
          buildTimelineEntry("user_message", {
            id: "entry-user",
            text: "batch dock approvals",
            createdAt: "2026-06-09T00:00:00Z",
          }),
          buildTimelineEntry("approval_group", {
            id: "entry-approvals",
            approvals: [approvalA, approvalB],
            createdAt: "2026-06-09T00:00:01Z",
          }),
        ],
      };
      await onEvent({ type: "final", response });
      return response;
    });

    const fetchMock = mountPanel({
      approvals: [approvalA, approvalB],
      approvalResolutionById: {
        "approval-1": {
          approval: { ...approvalA, status: "APPROVED" },
          run: buildRun({ id: "run-1", status: "COMPLETED", pendingApprovals: [] }),
        },
        "approval-2": {
          approval: { ...approvalB, status: "APPROVED" },
          run: buildRun({ id: "run-2", status: "COMPLETED", pendingApprovals: [] }),
        },
      },
    });
    await flushRequests();

    const input = document.querySelector("input");
    expect(input).not.toBeNull();
    input!.value = "batch dock approvals";
    input!.dispatchEvent(new Event("input"));
    await nextTick();

    Array.from(document.querySelectorAll<HTMLButtonElement>("button"))
      .find((button) => button.textContent?.includes("发送"))
      ?.click();
    await flushRequests();

    document.querySelector<HTMLButtonElement>(".adk-approvals-approve-all")?.click();
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
});

function mountPanel(options: {
  approvals?: ADKApproval[];
  approvalResolutionById?: Record<string, ADKApprovalResolution>;
  sessionDetail?: { session: ReturnType<typeof buildSession>; timeline: ADKTimelineEntry[] };
} = {}) {
  document.body.innerHTML = "<div id='root'></div>";
  const state = {
    approvals: [...(options.approvals ?? [])],
    sessionDetail: options.sessionDetail ?? { session: buildSession(), timeline: [] },
  };

  const fetchMock = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    const approvalActionMatch = url.match(/\/api\/v1\/adk\/approvals\/([^/]+)\/(approve|deny)$/);
    if (approvalActionMatch) {
      const approvalId = approvalActionMatch[1]!;
      state.approvals = state.approvals.filter((approval) => approval.id !== approvalId);
      return createResponse(options.approvalResolutionById?.[approvalId] ?? {
        approval: { ...buildApproval(approvalId, "unknown"), status: init?.method === "POST" ? "APPROVED" : "DENIED" },
        run: buildRun({ status: "COMPLETED", pendingApprovals: [] }),
      });
    }
    if (url.includes("/api/v1/adk/approvals")) {
      return createResponse({ approvals: state.approvals });
    }
    if (/\/api\/v1\/adk\/runs\/[^/]+$/.test(url)) {
      return createResponse(buildRun({ status: "COMPLETED", pendingApprovals: [] }));
    }
    if (/\/api\/v1\/adk\/sessions\/[^/]+$/.test(url)) {
      return createResponse(state.sessionDetail);
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

function buildApproval(id: string, toolName: string): ADKApproval {
  return {
    id,
    runId: "run-1",
    agentId: "agent-1",
    toolName,
    input: { prompt: toolName },
    status: "PENDING",
    reason: "Needs review",
    createdAt: "2026-06-09T00:00:00Z",
    updatedAt: "2026-06-09T00:00:00Z",
  };
}

function buildToolCall(id: string, toolName: string, status: string) {
  return {
    id,
    runId: "run-1",
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
    "v-icon": { template: "<span><slot /></span>" },
    "v-progress-circular": { template: "<span />" },
    "v-select": {
      props: ["modelValue", "items"],
      emits: ["update:modelValue"],
      template:
        "<select :value='modelValue' :class='$attrs.class' @change=\"$emit('update:modelValue', $event.target.value)\"><option v-for='item in items' :key='item.value' :value='item.value'>{{ item.title }}</option></select>",
    },
    "v-textarea": {
      props: ["modelValue"],
      emits: ["update:modelValue"],
      template:
        "<textarea :value='modelValue' :class='$attrs.class' @input=\"$emit('update:modelValue', $event.target.value)\" />",
    },
    "v-text-field": {
      props: ["modelValue"],
      emits: ["update:modelValue"],
      template:
        "<input :value='modelValue' :class='$attrs.class' @input=\"$emit('update:modelValue', $event.target.value)\" />",
    },
  };
}
