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
} from "@jftrade/ui-contracts";

import ADKPage from "../src/pages/ADKPage.vue";
import { createResponse, flushRequests } from "./helpers";

const { streamADKChatMock } = vi.hoisted(() => ({
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

afterEach(() => {
  vi.unstubAllGlobals();
  streamADKChatMock.mockReset();
});

describe("ADKPage", () => {
  it("shows a composer block when the selected provider has no API key", async () => {
    mountADKPage({
      providerHasKey: false,
    });
    await flushRequests();

    expect(document.body.textContent).toContain("当前 Provider 未配置 API Key");
  });

  it("renders final run tool calls and refreshes the timeline after approval", async () => {
    const pendingApproval: ADKApproval = {
      id: "approval-1",
      runId: "run-1",
      agentId: "agent-1",
      toolName: "strategy.save_draft",
      input: { query: "@strategy.save_draft" },
      status: "PENDING",
      reason: "需要审批",
      createdAt: "2026-06-06T00:00:00Z",
      updatedAt: "2026-06-06T00:00:00Z",
    };
    const pendingRun = buildRun({
      status: "PENDING_APPROVAL",
      toolCalls: [{
        id: "tool-1",
        runId: "run-1",
        toolName: "strategy.save_draft",
        permission: "write_strategy",
        status: "PENDING_APPROVAL",
        input: { query: "@strategy.save_draft" },
        requiresUser: true,
        createdAt: "2026-06-06T00:00:00Z",
        updatedAt: "2026-06-06T00:00:00Z",
      }],
      pendingApprovals: [pendingApproval],
    });
    const completedRun = buildRun({
      status: "COMPLETED",
      toolCalls: [{
        ...pendingRun.toolCalls[0]!,
        status: "SUCCEEDED",
        output: { saved: true },
        requiresUser: false,
      }],
      pendingApprovals: [{ ...pendingApproval, status: "APPROVED" }],
    });
    const pendingTimeline = [
      buildTimelineEntry("user_message", {
        id: "entry-user",
        text: "@strategy.save_draft 保存策略",
        createdAt: "2026-06-06T00:00:00Z",
      }),
      buildTimelineEntry("tool_group", {
        id: "entry-tools",
        runId: pendingRun.id,
        toolCalls: pendingRun.toolCalls,
        createdAt: "2026-06-06T00:00:01Z",
      }),
      buildTimelineEntry("approval_group", {
        id: "entry-approvals",
        runId: pendingRun.id,
        approvals: [pendingApproval],
        createdAt: "2026-06-06T00:00:02Z",
      }),
    ];
    const completedTimeline = [
      buildTimelineEntry("user_message", {
        id: "entry-user",
        text: "@strategy.save_draft 保存策略",
        createdAt: "2026-06-06T00:00:00Z",
      }),
      buildTimelineEntry("tool_group", {
        id: "entry-tools",
        runId: completedRun.id,
        toolCalls: completedRun.toolCalls,
        createdAt: "2026-06-06T00:00:01Z",
      }),
      buildTimelineEntry("assistant_message", {
        id: "entry-final",
        runId: completedRun.id,
        text: "策略草稿已保存，并已基于工具结果完成分析。",
        createdAt: "2026-06-06T00:00:03Z",
      }),
    ];

    streamADKChatMock.mockImplementation(async (_payload, onEvent) => {
      const response: ADKChatResponse = {
        reply: "等待审批",
        session: buildSession(),
        run: pendingRun,
        pendingApprovals: [pendingApproval],
        timeline: pendingTimeline,
      };
      await onEvent({ type: "session", session: response.session });
      await onEvent({ type: "final", response });
      return response;
    });

    const fetchMock = mountADKPage({
      approvals: [pendingApproval],
      approvalResolution: {
        approval: { ...pendingApproval, status: "APPROVED" },
        run: completedRun,
      },
      sessionDetail: {
        session: buildSession(),
        timeline: completedTimeline,
      },
    });
    await flushRequests();

    const textarea = document.querySelector("textarea")!;
    textarea.value = "@strategy.save_draft 保存策略";
    textarea.dispatchEvent(new Event("input"));
    await nextTick();
    document.querySelector<HTMLButtonElement>(".adk-composer-send")?.click();
    await flushRequests();

    expect(document.body.textContent).toContain("PENDING_APPROVAL");
    expect(document.body.textContent).toContain("strategy.save_draft");

    Array.from(document.querySelectorAll<HTMLButtonElement>("button"))
      .find((button) => button.textContent?.includes("批准"))
      ?.click();
    await flushRequests();

    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining("/api/v1/adk/approvals/approval-1/approve"),
      expect.objectContaining({ method: "POST" }),
    );
    expect(document.body.textContent).toContain("策略草稿已保存");
  });

  it("approves all pending approvals from the inline approval group", async () => {
    const approvalA: ADKApproval = {
      id: "approval-1",
      runId: "run-1",
      agentId: "agent-1",
      toolName: "strategy.save_draft",
      input: { query: "@strategy.save_draft" },
      status: "PENDING",
      reason: "needs approval",
      createdAt: "2026-06-06T00:00:00Z",
      updatedAt: "2026-06-06T00:00:00Z",
    };
    const approvalB: ADKApproval = {
      ...approvalA,
      id: "approval-2",
      toolName: "strategy.publish",
      input: { query: "@strategy.publish" },
    };

    streamADKChatMock.mockImplementation(async (_payload, onEvent) => {
      const response: ADKChatResponse = {
        reply: "waiting for approvals",
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
          run: buildRun({ status: "COMPLETED" }),
        },
        "approval-2": {
          approval: { ...approvalB, status: "APPROVED" },
          run: buildRun({ status: "COMPLETED" }),
        },
      },
    });
    await flushRequests();

    const textarea = document.querySelector("textarea")!;
    textarea.value = "batch approve";
    textarea.dispatchEvent(new Event("input"));
    await nextTick();
    document.querySelector<HTMLButtonElement>(".adk-composer-send")?.click();
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

  it("restores persisted timeline entries for saved sessions", async () => {
    const savedRun = buildRun({
      id: "run-restored",
      status: "COMPLETED",
      toolCalls: [{
        id: "tool-restored",
        runId: "run-restored",
        toolName: "portfolio.summary",
        permission: "read",
        status: "SUCCEEDED",
        input: { scope: "all" },
        output: { ok: true },
        requiresUser: false,
        createdAt: "2026-06-06T00:00:00Z",
        updatedAt: "2026-06-06T00:00:00Z",
      }],
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
    streamADKChatMock.mockImplementation(async (_payload, onEvent) => {
      await onEvent({ type: "session", session: buildSession() });
      await onEvent({ type: "error", message: "stream exploded" });
      throw new Error("stream exploded");
    });

    mountADKPage();
    await flushRequests();

    const textarea = document.querySelector("textarea")!;
    textarea.value = "check failed run";
    textarea.dispatchEvent(new Event("input"));
    await nextTick();
    document.querySelector<HTMLButtonElement>(".adk-composer-send")?.click();
    await flushRequests();

    expect(document.querySelector(".adk-thread")?.textContent).toContain("stream exploded");
    expect(document.querySelector(".adk-inline-alert")?.textContent).toContain("stream exploded");
    expect(document.querySelector(".adk-composer")?.textContent).not.toContain("stream exploded");
  });

  it("keeps deep reasoning collapsed until the user expands it", async () => {
    streamADKChatMock.mockImplementation(async (_payload, onEvent) => {
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

    const textarea = document.querySelector("textarea")!;
    textarea.value = "show reasoning";
    textarea.dispatchEvent(new Event("input"));
    await nextTick();
    document.querySelector<HTMLButtonElement>(".adk-composer-send")?.click();
    await flushRequests();

    expect(document.body.textContent).toContain("查看深度思考");
    expect(document.body.textContent).not.toContain("Detailed chain of thought preview.");

    Array.from(document.querySelectorAll<HTMLButtonElement>("button"))
      .find((button) => button.textContent?.includes("查看深度思考"))
      ?.click();
    await nextTick();

    expect(document.body.textContent).toContain("隐藏深度思考");
    expect(document.body.textContent).toContain("Detailed chain of thought preview.");
  });
});

function mountADKPage(options: {
  providerHasKey?: boolean;
  approvals?: ADKApproval[];
  approvalResolution?: unknown;
  approvalResolutionById?: Record<string, unknown>;
  sessionDetail?: { session: ReturnType<typeof buildSession>; timeline: ADKTimelineEntry[] };
} = {}) {
  document.body.innerHTML = "<div id='root'></div>";
  const state = {
    approvals: [...(options.approvals ?? [])],
    sessionDetail: options.sessionDetail ?? { session: buildSession(), timeline: [] },
  };
  const fetchMock = vi.fn(async (input: string | URL | Request) => {
    const url = String(input);
    if (url.includes("/api/v1/adk/agents")) {
      return createResponse({ agents: [buildAgent()] });
    }
    if (url.includes("/api/v1/adk/providers")) {
      return createResponse({ providers: [buildProvider(options.providerHasKey ?? true)] });
    }
    if (url.includes("/api/v1/adk/sessions")) {
      if (/\/api\/v1\/adk\/sessions\/[^/]+$/.test(url)) {
        return createResponse(state.sessionDetail);
      }
      return createResponse({ sessions: [buildSession()] });
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

function buildAgent() {
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

function vuetifyStubs() {
  return {
    "v-alert": { template: "<div><slot /></div>" },
    "v-btn": {
      props: ["disabled"],
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
      props: ["modelValue"],
      emits: ["update:modelValue"],
      template: "<textarea :value='modelValue' @input=\"$emit('update:modelValue', $event.target.value)\" />",
    },
    "v-text-field": {
      props: ["modelValue"],
      emits: ["update:modelValue"],
      template: "<input :value='modelValue' @input=\"$emit('update:modelValue', $event.target.value)\" />",
    },
  };
}
