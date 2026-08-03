// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

import {
  cancelADKOptimizationTask,
  cancelADKRun,
  deleteADKAgent,
  deleteADKMemory,
  deleteADKProvider,
  deleteADKTask,
  fallbackPage,
  fetchADKApprovalsPage,
  fetchADKAuditPage,
  fetchADKMemory,
  fetchADKMetrics,
  fetchADKOptimizationTasks,
  fetchADKRunsPage,
  fetchADKRuntimeSettings,
  fetchADKSettingsSnapshot,
  fetchADKSkills,
  fetchADKTasks,
  installADKSkill,
  nextPage,
  pageSummary,
  previousPage,
  resumeADKRun,
  saveADKAgent,
  saveADKMemory,
  saveADKProvider,
  saveADKRuntimeSettings,
  saveADKTask,
  setADKDefaultProvider,
  testADKProvider,
  uninstallADKSkill,
  updateADKTask,
} from "@/composables/adk/adkSettingsApi";
import { createResponse } from "../helpers";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("ADK settings API business contracts", () => {
  it("loads the settings dashboard in parallel and applies safe timeout defaults", async () => {
    const fetchMock = installRouteFetch({
      "/api/v1/adk": {
        providers: [buildProvider({ id: "provider-1" })],
        agents: [buildAgent({ id: "agent-1" })],
        tools: [buildTool({ name: "market.quote" })],
        skills: [buildSkill({ id: "skill-1" })],
        runtimeSettings: undefined,
      },
      "/api/v1/adk/optimization-tasks?limit=20": {
        tasks: [buildOptimizationTask({ id: "opt-1" })],
      },
      "/api/v1/adk/tasks?limit=20": {
        tasks: [buildTask({ id: "task-1" })],
      },
      "/api/v1/adk/memory": {
        entries: [buildMemory({ id: "memory-1" })],
      },
      "/api/v1/adk/agent-templates": {
        templates: [buildAgentTemplate({ id: "template-1" })],
      },
      "/api/v1/adk/metrics": buildMetrics(3),
    });

    const result = await fetchADKSettingsSnapshot();

    expect(fetchMock).toHaveBeenCalledTimes(6);
    expect(result).toMatchObject({
      providers: [{ id: "provider-1" }],
      agents: [{ id: "agent-1" }],
      optimizationTasks: [{ id: "opt-1" }],
      tasks: [{ id: "task-1" }],
      memoryEntries: [{ id: "memory-1" }],
      agentTemplates: [{ id: "template-1" }],
      runtimeSettings: {
        runTimeoutMs: 1_800_000,
        streamIdleTimeoutMs: 300_000,
      },
    });
  });

  it("builds paged run, approval, and audit queries from user filters", async () => {
    const fetchMock = installRouteFetch({
      "/api/v1/adk/runs?limit=25&offset=50&status=FAILED": {
        runs: [],
        page: fallbackPage(25, 50, 0),
      },
      "/api/v1/adk/runs?limit=25&offset=50": { runs: [] },
      "/api/v1/adk/approvals?limit=25&offset=50&status=PENDING": {
        approvals: [],
      },
      "/api/v1/adk/audit?limit=25&offset=50&kind=tool.call": { events: [] },
    });
    const page = fallbackPage(25, 50, 0);

    await fetchADKRunsPage(page, "FAILED");
    await fetchADKRunsPage(page, "attention");
    await fetchADKApprovalsPage(page, "PENDING");
    await fetchADKAuditPage(page, "  tool.call  ");

    expect(requestPaths(fetchMock)).toEqual([
      "/api/v1/adk/runs?limit=25&offset=50&status=FAILED",
      "/api/v1/adk/runs?limit=25&offset=50",
      "/api/v1/adk/approvals?limit=25&offset=50&status=PENDING",
      "/api/v1/adk/audit?limit=25&offset=50&kind=tool.call",
    ]);
  });

  it("queries tasks and memory with only meaningful filters", async () => {
    const fetchMock = installRouteFetch({
      "/api/v1/adk/tasks?limit=50&offset=20&status=RUNNING&agentId=agent%2Fone&runId=run%3F1": {
        tasks: [buildTask({ id: "task-filtered" })],
      },
      "/api/v1/adk/tasks?limit=20": { tasks: [] },
      "/api/v1/adk/memory?scope=project&agentId=agent%2Fone&key=risk%3Alimit": {
        entries: [buildMemory({ id: "memory-filtered" })],
      },
      "/api/v1/adk/memory": { entries: [] },
    });

    expect(
      await fetchADKTasks({
        limit: 50,
        offset: 20,
        status: "RUNNING",
        agentId: "agent/one",
        runId: "run?1",
      }),
    ).toEqual([buildTask({ id: "task-filtered" })]);
    expect(await fetchADKTasks()).toEqual([]);
    expect(
      await fetchADKMemory({
        scope: "project",
        agentId: "agent/one",
        key: "risk:limit",
      }),
    ).toEqual([buildMemory({ id: "memory-filtered" })]);
    expect(await fetchADKMemory()).toEqual([]);
    expect(fetchMock).toHaveBeenCalledTimes(4);
  });

  it("preserves task dependency and planning metadata in create and update payloads", async () => {
    const fetchMock = installRouteFetch({
      "/api/v1/adk/tasks": buildTask({ id: "task-1" }),
      "/api/v1/adk/tasks/task%2F1": buildTask({
        id: "task/1",
        status: "DONE",
      }),
    });
    const task = {
      title: "Validate strategy",
      description: "Run risk checks",
      dependsOn: ["research"],
      order: 2,
      modeHint: "loop",
      plannerStepId: "step-2",
      plannerWarnings: ["missing benchmark"],
    };

    await saveADKTask(task);
    await updateADKTask("task/1", { status: "DONE", resultSummary: "passed" });
    await deleteADKTask("task/1");

    expect(requestInit(fetchMock, 0)).toMatchObject({
      method: "POST",
      headers: { "Content-Type": "application/json" },
    });
    expect(JSON.parse(String(requestInit(fetchMock, 0).body))).toEqual(task);
    expect(requestInit(fetchMock, 1).method).toBe("PUT");
    expect(JSON.parse(String(requestInit(fetchMock, 1).body))).toEqual({
      status: "DONE",
      resultSummary: "passed",
    });
    expect(requestInit(fetchMock, 2).method).toBe("DELETE");
  });

  it("saves and removes scoped memory using encoded resource identifiers", async () => {
    const fetchMock = installRouteFetch({
      "/api/v1/adk/memory": buildMemory({ id: "memory-1" }),
      "/api/v1/adk/memory/project%2Frisk": undefined,
    });
    const entry = {
      agentId: "agent-1",
      key: "risk-limit",
      value: "2%",
      scope: "project",
    };

    await saveADKMemory(entry);
    await deleteADKMemory("project/risk");

    expect(JSON.parse(String(requestInit(fetchMock, 0).body))).toEqual(entry);
    expect(requestPaths(fetchMock)[1]).toBe(
      "/api/v1/adk/memory/project%2Frisk",
    );
    expect(requestInit(fetchMock, 1).method).toBe("DELETE");
  });

  it("sends provider, runtime, and agent settings without dropping operational limits", async () => {
    const provider = {
      id: "provider-loop",
      displayName: "Private gateway",
      baseUrl: "https://llm.example/v1",
      model: "reasoning-large",
      contextWindowTokens: 128_000,
      requestTimeoutMs: 240_000,
      apiKey: "secret",
      enabled: true,
    };
    const runtime = { runTimeoutMs: 600_000, streamIdleTimeoutMs: 45_000 };
    const agent = {
      id: "agent-loop",
      name: "Goal Agent",
      instruction: "Run toward the goal.",
      providerId: "provider-loop",
      model: "",
      tools: ["strategy.backtest"],
      skills: ["risk-review"],
      permissionMode: "approval",
      memoryEnabled: true,
      recentUserWindow: 6,
      workMode: "loop" as const,
      loopMaxIterations: 9,
      status: "ENABLED",
    };
    const fetchMock = installRouteFetch({
      "/api/v1/adk/providers": buildProvider(provider),
      "/api/v1/settings/adk": runtime,
      "/api/v1/adk/agents": buildAgent(agent),
    });

    await saveADKProvider(provider);
    expect(await fetchADKRuntimeSettings()).toEqual(runtime);
    await saveADKRuntimeSettings(runtime);
    await saveADKAgent(agent);

    expect(JSON.parse(String(requestInit(fetchMock, 0).body))).toEqual(provider);
    expect(requestInit(fetchMock, 1).method).toBe("GET");
    expect(requestInit(fetchMock, 2).method).toBe("PUT");
    expect(JSON.parse(String(requestInit(fetchMock, 3).body))).toMatchObject({
      workMode: "loop",
      loopMaxIterations: 9,
      tools: ["strategy.backtest"],
    });
  });

  it("encodes provider and agent lifecycle actions", async () => {
    const fetchMock = installRouteFetch({
      "/api/v1/adk/providers/provider%2Fone/test": { reply: "ready" },
      "/api/v1/adk/providers/provider%2Fone/default": buildProvider({
        id: "provider/one",
      }),
      "/api/v1/adk/providers/provider%2Fone": undefined,
      "/api/v1/adk/agents/agent%2Fone": undefined,
    });

    expect(await testADKProvider("provider/one")).toEqual({ reply: "ready" });
    await setADKDefaultProvider("provider/one");
    await deleteADKProvider("provider/one");
    await deleteADKAgent("agent/one");

    expect(requestPaths(fetchMock)).toEqual([
      "/api/v1/adk/providers/provider%2Fone/test",
      "/api/v1/adk/providers/provider%2Fone/default",
      "/api/v1/adk/providers/provider%2Fone",
      "/api/v1/adk/agents/agent%2Fone",
    ]);
    expect(fetchMock.mock.calls.map((call) => requestMethod(call[1]))).toEqual([
      "POST",
      "POST",
      "DELETE",
      "DELETE",
    ]);
  });

  it("normalizes run control responses and handles skill and optimization actions", async () => {
    const run = buildRun({
      id: "run/1",
      status: "RUNNING",
    });
    const fetchMock = installRouteFetch({
      "/api/v1/adk/runs/run%2F1/cancel": run,
      "/api/v1/adk/runs/run%2F1/resume": run,
      "/api/v1/adk/optimization-tasks/opt%2F1/cancel": buildOptimizationTask({
        id: "opt/1",
      }),
      "/api/v1/adk/skills": buildSkill({ id: "skill-1" }),
      "/api/v1/adk/skills/skill%2F1": undefined,
    });

    expect((await cancelADKRun("run/1")).id).toBe("run/1");
    expect((await resumeADKRun("run/1")).id).toBe("run/1");
    await cancelADKOptimizationTask("opt/1");
    await installADKSkill("https://skills.example/risk.git");
    await uninstallADKSkill("skill/1");

    expect(JSON.parse(String(requestInit(fetchMock, 3).body))).toEqual({
      url: "https://skills.example/risk.git",
    });
    expect(fetchMock.mock.calls.map((call) => requestMethod(call[1]))).toEqual([
      "POST",
      "POST",
      "POST",
      "POST",
      "DELETE",
    ]);
  });

  it("returns focused list endpoints without leaking response wrappers", async () => {
    installRouteFetch({
      "/api/v1/adk/skills": { skills: [buildSkill({ id: "skill-1" })] },
      "/api/v1/adk/optimization-tasks?limit=20": {
        tasks: [buildOptimizationTask({ id: "opt-1" })],
      },
      "/api/v1/adk/metrics": {
        ...buildMetrics(),
        approvals: { ...buildMetrics().approvals, pending: 2 },
      },
    });

    expect(await fetchADKSkills()).toEqual([buildSkill({ id: "skill-1" })]);
    expect(await fetchADKOptimizationTasks()).toEqual([
      buildOptimizationTask({ id: "opt-1" }),
    ]);
    expect(await fetchADKMetrics()).toMatchObject({ approvals: { pending: 2 } });
  });

  it("keeps pagination within server-reported boundaries", () => {
    const cursor = { offset: 25, limit: 25 };

    previousPage(cursor);
    expect(cursor.offset).toBe(0);
    previousPage(cursor);
    expect(cursor.offset).toBe(0);

    nextPage(cursor, { ...fallbackPage(25, 0, 100), hasMore: false });
    expect(cursor.offset).toBe(0);
    nextPage(cursor, { ...fallbackPage(25, 0, 100), hasMore: true });
    expect(cursor.offset).toBe(25);

    expect(pageSummary(fallbackPage(20, 0, 0))).toBe("0 / 0");
    expect(pageSummary({ limit: 20, offset: 20, total: 45, returned: 20, hasMore: true })).toBe(
      "21-40 / 45",
    );
  });
});

type FetchMock = ReturnType<typeof vi.fn>;

function installRouteFetch(routes: Record<string, unknown>): FetchMock {
  const fetchMock = vi.fn(async (input: string | URL | Request) => {
    const path = new URL(String(input), "http://localhost").pathname +
      new URL(String(input), "http://localhost").search;
    if (!Object.hasOwn(routes, path)) {
      throw new Error(`Unexpected request: ${path}`);
    }
    return createResponse(routes[path]);
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

function requestPaths(fetchMock: FetchMock): string[] {
  return fetchMock.mock.calls.map(([input]) => {
    const url = new URL(String(input), "http://localhost");
    return `${url.pathname}${url.search}`;
  });
}

function requestInit(fetchMock: FetchMock, index: number): RequestInit {
  return (fetchMock.mock.calls[index]?.[1] ?? {}) as RequestInit;
}

function requestMethod(init: unknown): string | undefined {
  return (init as RequestInit | undefined)?.method;
}

function buildProvider(overrides: Record<string, unknown> = {}) {
  return {
    id: "provider-1",
    displayName: "Provider",
    baseUrl: "https://llm.example/v1",
    model: "model-a",
    requestTimeoutMs: 120_000,
    enabled: true,
    default: false,
    hasApiKey: true,
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

function buildAgent(overrides: Record<string, unknown> = {}) {
  return {
    id: "agent-1",
    name: "Agent",
    instruction: "Help the user.",
    providerId: "provider-1",
    model: "model-a",
    tools: [],
    skills: [],
    permissionMode: "approval",
    memoryEnabled: true,
    recentUserWindow: 6,
    workMode: "chat",
    loopMaxIterations: 4,
    status: "ENABLED",
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

function buildAgentTemplate(overrides: Record<string, unknown> = {}) {
  const { createdAt: _createdAt, updatedAt: _updatedAt, ...template } =
    buildAgent(overrides);
  return template;
}

function buildTool(overrides: Record<string, unknown> = {}) {
  return {
    name: "market.quote",
    displayName: "Quote",
    description: "Read a quote",
    category: "market",
    permission: "read",
    allowedModes: ["approval", "less_approval", "all"],
    requiresApprovalIn: [],
    ...overrides,
  };
}

function buildSkill(overrides: Record<string, unknown> = {}) {
  return {
    id: "skill-1",
    displayName: "Risk review",
    description: "Review trading risk",
    source: "builtin",
    installPath: "/skills/risk-review",
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

function buildTask(overrides: Record<string, unknown> = {}) {
  return {
    id: "task-1",
    title: "Task",
    status: "PENDING",
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

function buildMemory(overrides: Record<string, unknown> = {}) {
  return {
    id: "memory-1",
    key: "risk-limit",
    value: "2%",
    scope: "project",
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

function buildRun(overrides: Record<string, unknown> = {}) {
  return {
    id: "run-1",
    sessionId: "session-1",
    agentId: "agent-1",
    status: "RUNNING",
    message: "working",
    toolCalls: [],
    pendingApprovals: [],
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:01Z",
    ...overrides,
  };
}

function buildOptimizationTask(overrides: Record<string, unknown> = {}) {
  return {
    id: "opt-1",
    status: "RUNNING",
    objective: "Improve the strategy",
    runs: [],
    progress: { total: 0, running: 0, completed: 0, failed: 0, cancelled: 0 },
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

function buildMetrics(total = 0) {
  return {
    runs: {
      total,
      last7Days: total,
      byStatus: {},
      byAgent: {},
      byProvider: {},
      lifecycle: { failed: 0, timedOut: 0, cancelled: 0, resumed: 0, orphaned: 0 },
    },
    tools: {
      total: 0,
      successful: 0,
      averageDurationMs: 0,
      byName: {},
      byStatus: {},
    },
    approvals: {
      pending: 0,
      total: 0,
      last7Days: 0,
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
    sessions: { total: 0, last7Days: 0 },
    workflows: {
      definitions: 0,
      enabledDefinitions: 0,
      triggers: 0,
      enabledTriggers: 0,
      invocations: 0,
      invocationsLast7Days: 0,
      byStatus: {},
      byTriggerType: {},
    },
    measurementWindow: { days: 7, since: "2026-07-19T00:00:00Z" },
  };
}
