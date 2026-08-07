import { describe, expect, it } from "vitest";

import {
  requireADKAgentTemplates,
  requireADKAgent,
  requireADKAgents,
  requireADKApproval,
  requireADKApprovalResolution,
  requireADKApprovals,
  requireADKAuditEvents,
  requireADKComposerState,
  requireADKContextSnapshot,
  requireADKInputResolution,
  requireADKMemoryEntries,
  requireADKMemoryEntry,
  requireADKMetrics,
  requireADKOptimizationTask,
  requireADKOptimizationTasks,
  requireADKPage,
  requireADKProvider,
  requireADKProviders,
  requireADKRun,
  requireADKRuns,
  requireADKRuntimeSettings,
  requireADKSession,
  requireADKSessions,
  requireADKSkill,
  requireADKSkills,
  requireADKTask,
  requireADKTasks,
  requireADKTimeline,
  requireADKToolDescriptors,
  requireADKWorkflowDefinition,
  requireADKWorkflowDefinitions,
  requireADKWorkflowInvocation,
  requireADKWorkflowTrigger,
  requireADKWorkflowTriggerLogs,
  requireADKWorkflowTriggerSave,
  requireADKWorkflowTriggers,
  requireMCPSettingsSnapshot,
  requireMCPTokenResetResult,
} from "@/composables/adk/adkApiMappers";

describe("ADK API wire mappers", () => {
  it("accepts a complete agent and preserves supported enum values", () => {
    const agent = buildAgent({ permissionMode: "less_approval", workMode: "loop" });

    expect(requireADKAgent(agent)).toBe(agent);
    expect(requireADKAgent(agent)).toMatchObject({
      permissionMode: "less_approval",
      workMode: "loop",
    });
  });

  it("rejects missing or null required fields at the API boundary", () => {
    expect(() => requireADKAgent({ ...buildAgent(), id: undefined })).toThrow(
      "ADK API response is invalid: agent",
    );
    expect(() => requireADKRun(null)).toThrow(
      "ADK API response is invalid: run",
    );
  });

  it("rejects unknown closed permission and work-mode enum values", () => {
    expect(() =>
      requireADKAgent(buildAgent({ permissionMode: "unrestricted" })),
    ).toThrow("ADK API response is invalid: agent");
    expect(() => requireADKAgent(buildAgent({ workMode: "autopilot" }))).toThrow(
      "ADK API response is invalid: agent",
    );
  });

  it("normalizes nullable collection fields emitted by legacy sessions", () => {
    const [entry] = requireADKTimeline([
      {
        id: "timeline-1",
        sessionId: "session-1",
        kind: "assistant_message",
        createdAt: "2026-07-26T00:00:00Z",
        sequence: 1,
        text: "restored",
        toolCalls: null,
        approvals: null,
      },
    ]);

    expect(entry?.toolCalls).toEqual([]);
    expect(entry?.approvals).toEqual([]);
  });

  it("normalizes null tool approval modes emitted by Go empty slices", () => {
    const [tool] = requireADKToolDescriptors([
      {
        name: "market.quote",
        displayName: "Market quote",
        description: "Reads the latest quote.",
        category: "market",
        permission: "read",
        allowedModes: ["approval", "less_approval", "all"],
        requiresApprovalIn: null,
      },
    ]);

    expect(tool?.requiresApprovalIn).toEqual([]);
  });

  it("normalizes optional agent template fields omitted by Go", () => {
    const template = {
      id: "jftrade-default",
      name: "默认助手",
      instruction: "Help the user.",
      providerId: "",
      permissionMode: "approval",
      memoryEnabled: true,
      status: "ENABLED",
    };

    expect(requireADKAgentTemplates([template])).toEqual([
      {
        ...template,
        model: "",
        tools: [],
        skills: [],
        recentUserWindow: 6,
        workMode: "chat",
        loopMaxIterations: 5,
      },
    ]);
  });

  it("accepts both Provider API protocols while retaining a strict provider contract", () => {
    const provider = {
      id: "provider-1",
      displayName: "Local provider",
      baseUrl: "http://127.0.0.1:11434",
      model: "model-a",
      requestTimeoutMs: 30_000,
      enabled: true,
      default: true,
      hasApiKey: false,
      createdAt: NOW,
      updatedAt: NOW,
    };

    expect(requireADKProvider({ ...provider, apiProtocol: "chat_completions" })).toMatchObject({
      apiProtocol: "chat_completions",
    });
    expect(requireADKProvider({ ...provider, apiProtocol: "responses" })).toMatchObject({
      apiProtocol: "responses",
    });
    expect(() =>
      requireADKProvider({ ...provider, apiProtocol: "unsupported" }),
    ).toThrow("ADK API response is invalid: provider");
  });

  it("rejects malformed required and optional agent template fields", () => {
    const template = {
      id: "jftrade-default",
      name: "默认助手",
      instruction: "Help the user.",
      providerId: "",
      permissionMode: "approval",
      memoryEnabled: true,
      status: "ENABLED",
    };

    for (const malformed of [
      { ...template, name: 42 },
      { ...template, permissionMode: "unrestricted" },
      { ...template, tools: null },
      null,
    ]) {
      expect(() => requireADKAgentTemplates([malformed])).toThrow(
        "ADK API response is invalid: agent templates",
      );
    }

    expect(() => requireADKAgentTemplates(template)).toThrow(
      "ADK API response is invalid: agent templates",
    );
    expect(() => requireADKToolDescriptors({})).toThrow(
      "ADK API response is invalid: tools",
    );
  });

  it("validates persisted provider, agent, tool, skill, task, and memory records", () => {
    const provider = {
      id: "provider-1",
      displayName: "Local provider",
      baseUrl: "http://127.0.0.1:11434",
      model: "model-a",
      requestTimeoutMs: 30_000,
      enabled: true,
      default: true,
      hasApiKey: false,
      defaultHeaders: { "X-Provider": "local" },
      createdAt: NOW,
      updatedAt: NOW,
    };
    const template = {
      ...buildAgent(),
      createdAt: undefined,
      updatedAt: undefined,
    };
    delete template.createdAt;
    delete template.updatedAt;
    const tool = {
      name: "market.quote",
      displayName: "Market quote",
      description: "Reads the latest quote.",
      category: "market",
      permission: "read",
      allowedModes: ["approval", "less_approval", "all"],
      requiresApprovalIn: ["approval"],
    };
    const skill = {
      id: "research",
      displayName: "Research",
      description: "Researches a symbol.",
      source: "builtin",
      installPath: "",
      createdAt: NOW,
      updatedAt: NOW,
    };
    const task = {
      id: "task-1",
      title: "Research AAPL",
      status: "completed",
      createdAt: NOW,
      updatedAt: NOW,
    };
    const memory = {
      id: "memory-1",
      key: "risk-profile",
      value: "conservative",
      scope: "session",
      createdAt: NOW,
      updatedAt: NOW,
    };

    expect(requireADKProvider(provider)).toBe(provider);
    expect(requireADKProviders([provider])).toEqual([provider]);
    expect(requireADKAgents([buildAgent()])).toHaveLength(1);
    expect(requireADKAgentTemplates([template])).toEqual([template]);
    expect(requireADKToolDescriptors([tool])).toEqual([tool]);
    expect(requireADKSkill(skill)).toBe(skill);
    expect(requireADKSkills([skill])).toEqual([skill]);
    expect(requireADKTask(task)).toBe(task);
    expect(requireADKTasks([task])).toEqual([task]);
    expect(requireADKMemoryEntry(memory)).toBe(memory);
    expect(requireADKMemoryEntries([memory])).toEqual([memory]);
    expect(requireADKAuditEvents([{
      id: "audit-1",
      kind: "tool_call",
      detail: "market.quote succeeded",
      createdAt: NOW,
    }])).toHaveLength(1);
    expect(requireADKPage({
      limit: 20,
      offset: 0,
      total: 21,
      returned: 20,
      hasMore: true,
    })).toMatchObject({ returned: 20, hasMore: true });
    expect(requireADKRuntimeSettings({
      runTimeoutMs: 120_000,
      streamIdleTimeoutMs: 30_000,
    })).toMatchObject({ runTimeoutMs: 120_000 });
  });

  it("validates nested run approvals, user input, workflow plans, and resolutions", () => {
    const approval = buildApproval();
    const inputRequest = buildInputRequest();
    const run = buildRun({
      toolCalls: [buildToolCall()],
      pendingApprovals: [approval],
      inputRequest,
      inputRequests: [inputRequest],
      workflowPlan: [{
        title: "Research",
        status: "completed",
        dependsOn: ["setup"],
        routes: ["trade"],
      }],
    });
    const message = {
      id: "message-1",
      sessionId: "session-1",
      role: "assistant",
      kind: "text",
      content: "Ready to continue.",
      createdAt: NOW,
    };

    expect(requireADKRun(run)).toMatchObject({
      toolCalls: [{ toolName: "market.quote" }],
      inputRequest: { functionCallId: "call-ask-user" },
      workflowPlan: [{ dependsOn: ["setup"], routes: ["trade"] }],
    });
    expect(requireADKRuns([
      { ...run, toolCalls: null, pendingApprovals: null },
    ])).toMatchObject([{ toolCalls: [], pendingApprovals: [] }]);
    expect(requireADKApproval(approval)).toBe(approval);
    expect(requireADKApprovals([approval])).toEqual([approval]);
    expect(requireADKApprovalResolution({
      approval,
      run,
      parentRun: run,
      message,
    })).toMatchObject({
      approval: { id: "approval-1" },
      message: { id: "message-1" },
    });
    expect(requireADKInputResolution({
      request: inputRequest,
      run,
      parentRun: run,
      message,
    })).toMatchObject({
      request: { id: "input-1" },
      run: { id: "run-1" },
    });
    expect(requireADKTimeline([{
      id: "timeline-full",
      sessionId: "session-1",
      kind: "tool",
      createdAt: NOW,
      sequence: 2,
      toolCalls: [buildToolCall()],
      approvals: [approval],
      inputRequest,
    }])).toMatchObject([{
      toolCalls: [{ id: "tool-call-1" }],
      approvals: [{ id: "approval-1" }],
      inputRequest: { id: "input-1" },
    }]);
  });

  it("validates session composer state, context accounting, and optimization progress", () => {
    const session = buildSession();
    const context = buildContextSnapshot();
    const optimizationTask = {
      id: "optimization-1",
      status: "running",
      objective: "maximize Sharpe ratio",
      runs: [{
        definitionId: "definition-1",
        runId: "backtest-1",
        status: "completed",
      }],
      progress: {
        total: 5,
        running: 1,
        completed: 3,
        failed: 1,
        cancelled: 0,
      },
      createdAt: NOW,
      updatedAt: NOW,
    };

    expect(requireADKSession(session)).toBe(session);
    expect(requireADKSessions([session])).toEqual([session]);
    expect(requireADKComposerState({
      sessionId: "session-1",
      chatDraft: "Compare the two strategies",
      providerIdOverride: "provider-1",
      modelOverride: "model-a",
      workModeOverride: "loop",
      permissionModeOverride: "approval",
      goalObjectiveDraft: "Choose the more robust strategy",
      goalObjectiveTouched: true,
    })).toMatchObject({
      sessionId: "session-1",
      goalObjectiveTouched: true,
    });
    expect(requireADKContextSnapshot(context)).toMatchObject({
      currentInputTokens: 1_000,
      rawBreakdown: { instructionTokens: 100 },
      autoCompacted: false,
    });
    expect(requireADKOptimizationTask(optimizationTask)).toBe(optimizationTask);
    expect(requireADKOptimizationTasks([optimizationTask])).toEqual([
      optimizationTask,
    ]);
  });

  it("validates workflow canvas, trigger execution, and invocation responses", () => {
    const workflow = buildWorkflow();
    const trigger = buildTrigger();
    const inputRequest = buildInputRequest();
    const run = buildRun({ inputRequest, inputRequests: [inputRequest] });
    const response = {
      reply: "Workflow completed.",
      session: buildSession(),
      run,
      pendingApprovals: [buildApproval()],
      timeline: [{
        id: "timeline-workflow",
        sessionId: "session-1",
        kind: "assistant_message",
        createdAt: NOW,
        sequence: 3,
        toolCalls: [buildToolCall()],
        approvals: [buildApproval()],
        inputRequest,
      }],
      inputRequest,
      context: buildContextSnapshot(),
    };
    const log = {
      id: "trigger-log-1",
      workflowId: "workflow-1",
      triggerType: "manual",
      status: "completed",
      result: {
        format: "markdown",
        markdown: "## Result",
        rawResponse: response,
      },
      nodeRuns: [{
        nodeId: "research",
        nodeType: "agent",
        status: "completed",
      }],
      createdAt: NOW,
      updatedAt: NOW,
    };

    const mappedWorkflow = requireADKWorkflowDefinition(workflow);
    expect(mappedWorkflow.canvasGraph?.nodes?.[0]).toMatchObject({
      id: "research",
    });
    expect(mappedWorkflow.canvasGraph?.edges?.[0]).toMatchObject({
      source: "research",
      target: "report",
    });
    expect(requireADKWorkflowDefinitions([workflow])).toEqual([workflow]);
    expect(requireADKWorkflowTrigger(trigger)).toBe(trigger);
    expect(requireADKWorkflowTriggers([trigger])).toEqual([trigger]);
    expect(requireADKWorkflowTriggerLogs([log])).toMatchObject([{
      result: { format: "markdown", rawResponse: { reply: "Workflow completed." } },
      nodeRuns: [{ nodeId: "research" }],
    }]);
    expect(requireADKWorkflowInvocation({
      workflow,
      log,
      trigger,
      response,
    })).toMatchObject({
      workflow: { id: "workflow-1" },
      trigger: { id: "trigger-1" },
      response: { reply: "Workflow completed." },
    });
    expect(requireADKWorkflowTriggerSave({
      trigger,
      secret: "webhook-secret",
    })).toMatchObject({
      trigger: { id: "trigger-1" },
      secret: "webhook-secret",
    });
  });

  it("validates MCP settings and aggregate ADK metrics including nullable usage", () => {
    const snapshot = {
      settings: {
        enabled: true,
        port: 3010,
        authMode: "token",
        tokenConfigured: true,
      },
      status: {
        running: false,
        endpoint: "http://127.0.0.1:3010/mcp",
        lastError: "port unavailable",
      },
    };

    expect(requireMCPSettingsSnapshot(snapshot)).toBe(snapshot);
    expect(requireMCPTokenResetResult({
      ...snapshot,
      token: "new-token",
    })).toMatchObject({ token: "new-token" });
    expect(requireADKMetrics({
      runs: {
        total: 10,
        last7Days: 4,
        byStatus: { completed: 8, failed: 2 },
        byAgent: { "agent-1": 10 },
        byProvider: { "provider-1": 10 },
        lifecycle: {
          failed: 2,
          timedOut: 1,
          cancelled: 0,
          resumed: 1,
          orphaned: 0,
        },
      },
      tools: {
        total: 20,
        successful: 18,
        averageDurationMs: 125,
        byName: { "market.quote": 20 },
        byStatus: { completed: 18, failed: 2 },
      },
      approvals: {
        pending: 1,
        total: 5,
        last7Days: 2,
        approved: 3,
        denied: 1,
        recoverablePending: 1,
        pendingWaitMs: { average: 100, max: 200 },
        resolutionWaitMs: { average: 300, max: 500, count: 4 },
      },
      usage: {
        samples: 10,
        tokensInTotal: 5_000,
        tokensOutTotal: null,
        tokensInAverage: 500,
        tokensOutAverage: null,
      },
      sessions: { total: 6, last7Days: 3 },
      workflows: {
        definitions: 2,
        enabledDefinitions: 1,
        triggers: 3,
        enabledTriggers: 2,
        invocations: 8,
        invocationsLast7Days: 5,
        byStatus: { SUCCEEDED: 7, FAILED: 1 },
        byTriggerType: { manual: 8 },
      },
      measurementWindow: { days: 7, since: "2026-07-21T00:00:00Z" },
    })).toMatchObject({
      runs: { total: 10 },
      approvals: { recoverablePending: 1 },
      usage: { tokensOutTotal: null },
    });
  });

  it("rejects malformed nested records instead of accepting partial wire shapes", () => {
    expect(() => requireADKRuns({})).toThrow(
      "ADK API response is invalid: runs",
    );
    expect(() => requireADKTimeline({})).toThrow(
      "ADK API response is invalid: timeline",
    );
    expect(() => requireADKProviders([null])).toThrow(
      "ADK API response is invalid: providers",
    );
    expect(() => requireADKToolDescriptors([{
      name: "market.quote",
      displayName: "Market quote",
      description: "Reads quotes",
      category: "market",
      permission: "read",
      allowedModes: ["autopilot"],
      requiresApprovalIn: [],
    }])).toThrow("ADK API response is invalid: tools");
    expect(() => requireADKRun(buildRun({
      inputRequest: buildInputRequest({
        questions: [{
          id: "question-1",
          question: "Choose a market",
          options: [{ id: "hk", label: "Hong Kong" }, 42],
          allowOther: false,
        }],
      }),
    }))).toThrow("ADK API response is invalid: run");
    expect(() => requireADKWorkflowDefinition({
      ...buildWorkflow(),
      canvasGraph: {
        nodes: [{ id: "bad", type: "agent", position: { x: "left", y: 0 } }],
      },
    })).toThrow("ADK API response is invalid: workflow");
    expect(() => requireMCPSettingsSnapshot({
      settings: {
        enabled: true,
        port: 3010,
        authMode: "password",
        tokenConfigured: false,
      },
      status: { running: false, endpoint: "" },
    })).toThrow("ADK API response is invalid: MCP settings");
    expect(() => requireADKMetrics({
      runs: {
        total: 1,
        byStatus: { completed: "one" },
        byAgent: {},
        byProvider: {},
        lifecycle: {},
      },
      tools: {},
      approvals: {},
      usage: {},
      sessions: {},
      workflows: {},
      measurementWindow: {},
    })).toThrow("ADK API response is invalid: metrics");
  });
});

const NOW = "2026-07-26T00:00:00Z";

function buildAgent(overrides: Record<string, unknown> = {}) {
  return {
    id: "agent-1",
    name: "Research agent",
    instruction: "Research the selected market.",
    providerId: "provider-1",
    model: "model-a",
    tools: ["market.quote"],
    skills: ["research"],
    permissionMode: "approval",
    memoryEnabled: true,
    recentUserWindow: 8,
    workMode: "chat",
    loopMaxIterations: 4,
    status: "ENABLED",
    createdAt: "2026-07-26T00:00:00Z",
    updatedAt: "2026-07-26T00:00:00Z",
    ...overrides,
  };
}

function buildToolCall(overrides: Record<string, unknown> = {}) {
  return {
    id: "tool-call-1",
    runId: "run-1",
    toolName: "market.quote",
    permission: "read",
    status: "completed",
    requiresUser: false,
    createdAt: NOW,
    updatedAt: NOW,
    ...overrides,
  };
}

function buildApproval(overrides: Record<string, unknown> = {}) {
  return {
    id: "approval-1",
    runId: "run-1",
    agentId: "agent-1",
    toolName: "trading.place_order",
    status: "pending",
    reason: "Real trading requires approval.",
    createdAt: NOW,
    updatedAt: NOW,
    ...overrides,
  };
}

function buildInputRequest(overrides: Record<string, unknown> = {}) {
  return {
    id: "input-1",
    runId: "run-1",
    agentId: "agent-1",
    functionCallId: "call-ask-user",
    status: "pending",
    questions: [{
      id: "question-1",
      question: "Which market should be researched?",
      options: [{
        id: "hk",
        label: "Hong Kong",
        description: "Use the HK market.",
        recommended: true,
      }],
      allowOther: true,
    }],
    answers: [{
      questionId: "question-1",
      optionId: "hk",
      otherText: "",
    }],
    createdAt: NOW,
    updatedAt: NOW,
    ...overrides,
  };
}

function buildRun(overrides: Record<string, unknown> = {}) {
  return {
    id: "run-1",
    sessionId: "session-1",
    agentId: "agent-1",
    status: "completed",
    message: "Research completed.",
    toolCalls: [],
    pendingApprovals: [],
    createdAt: NOW,
    updatedAt: NOW,
    ...overrides,
  };
}

function buildSession(overrides: Record<string, unknown> = {}) {
  return {
    id: "session-1",
    agentId: "agent-1",
    title: "Market research",
    createdAt: NOW,
    updatedAt: NOW,
    ...overrides,
  };
}

function buildContextSnapshot(overrides: Record<string, unknown> = {}) {
  const breakdown = {
    instructionTokens: 100,
    handoffTokens: 50,
    recentUserTokens: 200,
    protectedTailTokens: 100,
    otherVisibleTokens: 250,
    pendingUserTokens: 50,
    toolDeclarationTokens: 250,
  };
  return {
    sessionId: "session-1",
    currentInputTokens: 1_000,
    projectedNextTurnTokens: 1_200,
    contextWindowTokens: 8_192,
    usageRatio: 0.12,
    status: "healthy",
    recentUserWindow: 8,
    retainedRecentUserCount: 3,
    activeHandoffCount: 1,
    breakdown,
    rawBreakdown: { ...breakdown },
    autoCompacted: false,
    degradedSummary: false,
    ...overrides,
  };
}

function buildWorkflow(overrides: Record<string, unknown> = {}) {
  return {
    id: "workflow-1",
    name: "Daily research",
    status: "enabled",
    agentId: "agent-1",
    workMode: "loop",
    promptTemplate: "Research {{symbol}}",
    canvasGraph: {
      nodes: [
        { id: "research", type: "agent", position: { x: 10, y: 20 } },
        { id: "report", type: "output", position: { x: 200, y: 20 } },
      ],
      edges: [{ id: "edge-1", source: "research", target: "report" }],
    },
    createdAt: NOW,
    updatedAt: NOW,
    ...overrides,
  };
}

function buildTrigger(overrides: Record<string, unknown> = {}) {
  return {
    id: "trigger-1",
    workflowId: "workflow-1",
    type: "manual",
    title: "Run now",
    status: "enabled",
    createdAt: NOW,
    updatedAt: NOW,
    ...overrides,
  };
}
