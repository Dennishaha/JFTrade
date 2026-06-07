// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import { nextTick } from "vue";
import { createMemoryHistory, createRouter } from "vue-router";

import type {
  ADKApproval,
  ADKChatResponse,
  ADKMessage,
  ADKRun,
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

  it("renders final run tool calls and updates the pending message after approval", async () => {
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
    const finalMessage: ADKMessage = {
      id: "msg-final",
      sessionId: "session-1",
      role: "assistant",
      content: "策略草稿已保存，并已基于工具结果完成分析。",
      createdAt: "2026-06-06T00:00:01Z",
    };

    streamADKChatMock.mockImplementation(async (_payload, onEvent) => {
      const response: ADKChatResponse = {
        reply: "等待审批",
        session: buildSession(),
        run: pendingRun,
        pendingApprovals: [pendingApproval],
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
        message: finalMessage,
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
    expect(document.body.textContent).toContain("已完成");
  });

  it("restores persisted pre-tool content from saved runs", async () => {
    const savedRun = buildRun({
      id: "run-restored",
      finalMessageId: "msg-restored",
      status: "COMPLETED",
      preToolContent: "先查组合，再整理系统状态。",
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
    const savedMessage: ADKMessage = {
      id: "msg-restored",
      sessionId: "session-1",
      role: "assistant",
      content: "先查组合，再整理系统状态。最终结论已经整理完成。",
      createdAt: "2026-06-06T00:00:02Z",
    };

    mountADKPage({
      sessionDetail: {
        session: buildSession(),
        messages: [{ id: "msg-user", sessionId: "session-1", role: "user", content: "查看系统状态", createdAt: "2026-06-06T00:00:01Z" }, savedMessage],
      },
      runs: [savedRun],
    });
    await flushRequests();

    document.querySelector<HTMLElement>(".adk-session-item")?.click();
    await flushRequests();

    expect(document.body.textContent).toContain("先查组合，再整理系统状态。");
    expect(document.body.textContent).toContain("最终结论已经整理完成。");
    expect(document.body.textContent).toContain("portfolio.summary");
  });
});

function mountADKPage(options: {
  providerHasKey?: boolean;
  approvals?: ADKApproval[];
  approvalResolution?: unknown;
  sessionDetail?: { session: ReturnType<typeof buildSession>; messages: ADKMessage[] };
  runs?: ADKRun[];
} = {}) {
  document.body.innerHTML = "<div id='root'></div>";
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
        return createResponse(options.sessionDetail ?? { session: buildSession(), messages: [] });
      }
      return createResponse({ sessions: [buildSession()] });
    }
    if (url.includes("/api/v1/adk/runs?sessionId=")) {
      const offsetMatch = url.match(/[?&]offset=(\d+)/);
      const offset = offsetMatch ? Number(offsetMatch[1]) : 0;
      const runs = offset === 0 ? (options.runs ?? []) : [];
      return createResponse({
        runs,
        page: {
          limit: 100,
          offset,
          total: options.runs?.length ?? 0,
          returned: runs.length,
          hasMore: false,
        },
      });
    }
    if (url.includes("/api/v1/adk/approvals/approval-1/approve")) {
      return createResponse(options.approvalResolution);
    }
    if (url.includes("/api/v1/adk/approvals")) {
      return createResponse({ approvals: options.approvals ?? [] });
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

function vuetifyStubs() {
  return {
    "v-alert": { template: "<div><slot /></div>" },
    "v-btn": {
      props: ["disabled"],
      emits: ["click"],
      template:
        "<button type='button' :disabled='disabled' :class='$attrs.class' @click=\"$emit('click')\"><slot /></button>",
    },
    "v-chip": { template: "<span><slot /></span>" },
    "v-expansion-panel": { template: "<div><slot /></div>" },
    "v-expansion-panel-text": { template: "<div><slot /></div>" },
    "v-expansion-panel-title": { template: "<div><slot /></div>" },
    "v-expansion-panels": { template: "<div><slot /></div>" },
    "v-icon": { template: "<span><slot /></span>" },
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
