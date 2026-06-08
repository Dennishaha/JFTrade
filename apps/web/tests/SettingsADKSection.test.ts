// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import { createMemoryHistory, createRouter } from "vue-router";

import SettingsADKSection from "../src/components/SettingsADKSection.vue";
import {
  buttonStub,
  iconStub,
  inputStub,
  selectStub,
  tabStub,
  tabsStub,
  windowItemStub,
  windowStub,
} from "./helpers";
import { createResponse, flushRequests } from "./helpers";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("SettingsADKSection", () => {
  it("requests next pages for runs, approvals, and audit feeds", async () => {
    document.body.innerHTML = "<div id='root'></div>";
    const fetchMock = vi.fn(async (input: string | URL | Request) => {
      const url = String(input);
      if (url.includes("/api/v1/adk/providers")) {
        return createResponse({ providers: [buildProvider()] });
      }
      if (url.includes("/api/v1/adk/agents")) {
        return createResponse({ agents: [buildAgent()] });
      }
      if (url.includes("/api/v1/adk/tools")) {
        return createResponse({ tools: [] });
      }
      if (url.includes("/api/v1/adk/skills")) {
        return createResponse({ skills: [] });
      }
      if (url.includes("/api/v1/adk/optimization-tasks")) {
        return createResponse({ tasks: [] });
      }
      if (url.includes("/api/v1/adk/tasks")) {
        return createResponse({
          tasks: [buildTask()],
          page: { limit: 20, offset: 0, total: 1, returned: 1, hasMore: false },
        });
      }
      if (url.includes("/api/v1/adk/memory")) {
        return createResponse({ entries: [buildMemoryEntry()] });
      }
      if (url.includes("/api/v1/adk/agent-templates")) {
        return createResponse({ templates: [] });
      }
      if (url.includes("/api/v1/adk/metrics")) {
        return createResponse(buildMetrics());
      }
      if (url.includes("/api/v1/adk/runs")) {
        return createResponse({
          runs: [{ ...buildRun(), id: `run-${url.includes("offset=20") ? "2" : "1"}` }],
          page: { limit: 20, offset: url.includes("offset=20") ? 20 : 0, total: 25, returned: 1, hasMore: !url.includes("offset=20") },
        });
      }
      if (url.includes("/api/v1/adk/approvals")) {
        return createResponse({
          approvals: [{ ...buildApproval(), id: `approval-${url.includes("offset=10") ? "2" : "1"}` }],
          page: { limit: 10, offset: url.includes("offset=10") ? 10 : 0, total: 15, returned: 1, hasMore: !url.includes("offset=10") },
        });
      }
      if (url.includes("/api/v1/adk/audit")) {
        return createResponse({
          events: [{ ...buildAuditEvent(), id: `audit-${url.includes("offset=12") ? "2" : "1"}` }],
          page: { limit: 12, offset: url.includes("offset=12") ? 12 : 0, total: 18, returned: 1, hasMore: !url.includes("offset=12") },
        });
      }
      return createResponse({});
    });
    vi.stubGlobal("fetch", fetchMock);

    const router = createADKSettingsRouter(
      "/settings/adk?tab=observation&view=workflow",
    );
    await router.isReady();

    const wrapper = mount(SettingsADKSection, {
      attachTo: "#root",
      global: {
        plugins: [router],
        stubs: {
          "v-alert": { template: "<div><slot /></div>" },
          "v-btn": buttonStub,
          "v-card": { template: "<section><slot /></section>" },
          "v-card-title": { template: "<div><slot /></div>" },
          "v-card-text": { template: "<div><slot /></div>" },
          "v-chip": { template: "<span><slot /></span>" },
          "v-icon": iconStub,
          "v-select": selectStub,
          "v-switch": { template: "<label><slot /></label>" },
          "v-tab": tabStub,
          "v-tabs": tabsStub,
          "v-text-field": inputStub,
          "v-textarea": inputStub,
          "v-window": windowStub,
          "v-window-item": windowItemStub,
        },
      },
    });

    await flushRequests();

    expect(router.currentRoute.value.query).toEqual({
      tab: "observation",
      view: "workflow",
    });

    const nextButtons = wrapper
      .findAll("button")
      .filter((button) => button.text().includes("下一页"));
    expect(nextButtons).toHaveLength(3);
    for (const button of nextButtons) {
      await button.trigger("click");
      await flushRequests();
    }

    const requestedURLs = fetchMock.mock.calls.map((call) => String(call[0]));
    expect(
      requestedURLs.some((url) =>
        url.endsWith("/api/v1/adk/runs?limit=20&offset=20"),
      ),
    ).toBe(true);
    expect(
      requestedURLs.some((url) =>
        url.endsWith(
          "/api/v1/adk/approvals?limit=10&offset=10&status=PENDING",
        ),
      ),
    ).toBe(true);
    expect(
      requestedURLs.some((url) =>
        url.endsWith("/api/v1/adk/audit?limit=12&offset=12"),
      ),
    ).toBe(true);

    expect(wrapper.text()).toContain("观察");
    expect(wrapper.text()).toContain("工作流");
    expect(wrapper.text()).toContain("运行与审计");
    expect(wrapper.text()).toContain("智能体任务");
    expect(wrapper.text()).toContain("智能体记忆");
    expect(wrapper.text()).toContain("Follow-up review");
    expect(wrapper.text()).toContain("preferred_market");
    expect(wrapper.text()).not.toContain("创建任务");
    expect(wrapper.text()).not.toContain("保存记忆");
    expect(wrapper.text()).not.toContain("Delete");
  });
});

function createADKSettingsRouter(path: string) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: "/settings/:section?", component: { template: "<div />" } }],
  });
  void router.push(path);
  return router;
}

function buildProvider() {
  return {
    id: "provider-1",
    displayName: "OpenAI",
    baseUrl: "https://api.openai.com/v1",
    model: "gpt-4o-mini",
    enabled: true,
    hasApiKey: true,
    createdAt: "2026-06-06T00:00:00Z",
    updatedAt: "2026-06-06T00:00:00Z",
  };
}

function buildAgent() {
  return {
    id: "agent-1",
    name: "Agent",
    instruction: "test",
    providerId: "provider-1",
    model: "gpt-4o-mini",
    tools: [],
    skills: [],
    permissionMode: "approval",
    memoryEnabled: true,
    status: "ENABLED",
    createdAt: "2026-06-06T00:00:00Z",
    updatedAt: "2026-06-06T00:00:00Z",
  };
}

function buildRun() {
  return {
    id: "run-1",
    sessionId: "session-1",
    agentId: "agent-1",
    status: "FAILED",
    message: "failed",
    toolCalls: [],
    pendingApprovals: [],
    createdAt: "2026-06-06T00:00:00Z",
    updatedAt: "2026-06-06T00:00:00Z",
  };
}

function buildApproval() {
  return {
    id: "approval-1",
    runId: "run-1",
    agentId: "agent-1",
    toolName: "strategy.save_draft",
    status: "PENDING",
    reason: "needs approval",
    createdAt: "2026-06-06T00:00:00Z",
    updatedAt: "2026-06-06T00:00:00Z",
  };
}

function buildTask() {
  return {
    id: "task-1",
    title: "Follow-up review",
    description: "Review risk state after the run.",
    status: "TODO",
    agentId: "agent-1",
    runId: "run-1",
    dependsOn: [],
    createdAt: "2026-06-06T00:00:00Z",
    updatedAt: "2026-06-06T00:00:00Z",
  };
}

function buildMemoryEntry() {
  return {
    id: "memory-1",
    key: "preferred_market",
    value: "HK",
    scope: "workspace",
    createdAt: "2026-06-06T00:00:00Z",
    updatedAt: "2026-06-06T00:00:00Z",
  };
}

function buildAuditEvent() {
  return {
    id: "audit-1",
    kind: "run.started",
    detail: "Run started",
    createdAt: "2026-06-06T00:00:00Z",
  };
}

function buildMetrics() {
  return {
    runs: {
      total: 1,
      byStatus: {},
      byAgent: {},
      byProvider: {},
      lifecycle: { failed: 1, timedOut: 0, cancelled: 0, resumed: 0, orphaned: 0 },
    },
    tools: { total: 0, successful: 0, averageDurationMs: 0, byName: {}, byStatus: {} },
    approvals: {
      pending: 1,
      total: 1,
      approved: 0,
      denied: 0,
      recoverablePending: 0,
      pendingWaitMs: { average: 0, max: 0 },
      resolutionWaitMs: { average: 0, max: 0, count: 0 },
    },
    usage: {
      samples: 0,
      tokensInTotal: null,
      tokensOutTotal: null,
      tokensInAverage: null,
      tokensOutAverage: null,
    },
  };
}
