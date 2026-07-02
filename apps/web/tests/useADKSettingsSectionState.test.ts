// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h, nextTick } from "vue";

import type {
  ADKAgent,
  ADKApproval,
  ADKAuditEvent,
  ADKMemoryEntry,
  ADKOptimizationTask,
  ADKProvider,
  ADKRun,
  ADKSkill,
  ADKTask,
  ADKToolDescriptor,
} from "@/contracts";

import type {
  ADKMetricsResponse,
  PageEnvelope,
} from "../src/composables/adkSettingsApi";
import { useADKSettingsSectionState } from "../src/composables/useADKSettingsSectionState";
import { flushRequests } from "./helpers";

const {
  cancelADKOptimizationTaskMock,
  cancelADKRunMock,
  deleteADKAgentMock,
  deleteADKProviderMock,
  fetchADKApprovalsPageMock,
  fetchADKAuditPageMock,
  fetchADKMemoryMock,
  fetchADKMetricsMock,
  fetchADKOptimizationTasksMock,
  fetchADKRunsPageMock,
  fetchADKSettingsSnapshotMock,
  fetchADKSkillsMock,
  fetchADKTasksMock,
  installADKSkillMock,
  resumeADKRunMock,
  saveADKAgentMock,
  saveADKProviderMock,
  saveADKRuntimeSettingsMock,
  setADKDefaultProviderMock,
  testADKProviderMock,
  uninstallADKSkillMock,
} = vi.hoisted(() => ({
  cancelADKOptimizationTaskMock: vi.fn(),
  cancelADKRunMock: vi.fn(),
  deleteADKAgentMock: vi.fn(),
  deleteADKProviderMock: vi.fn(),
  fetchADKApprovalsPageMock: vi.fn(),
  fetchADKAuditPageMock: vi.fn(),
  fetchADKMemoryMock: vi.fn(),
  fetchADKMetricsMock: vi.fn(),
  fetchADKOptimizationTasksMock: vi.fn(),
  fetchADKRunsPageMock: vi.fn(),
  fetchADKSettingsSnapshotMock: vi.fn(),
  fetchADKSkillsMock: vi.fn(),
  fetchADKTasksMock: vi.fn(),
  installADKSkillMock: vi.fn(),
  resumeADKRunMock: vi.fn(),
  saveADKAgentMock: vi.fn(),
  saveADKProviderMock: vi.fn(),
  saveADKRuntimeSettingsMock: vi.fn(),
  setADKDefaultProviderMock: vi.fn(),
  testADKProviderMock: vi.fn(),
  uninstallADKSkillMock: vi.fn(),
}));

vi.mock("../src/composables/adkSettingsApi", async () => {
  const actual = await vi.importActual<
    typeof import("../src/composables/adkSettingsApi")
  >("../src/composables/adkSettingsApi");
  return {
    ...actual,
    cancelADKOptimizationTask: cancelADKOptimizationTaskMock,
    cancelADKRun: cancelADKRunMock,
    deleteADKAgent: deleteADKAgentMock,
    deleteADKProvider: deleteADKProviderMock,
    fetchADKApprovalsPage: fetchADKApprovalsPageMock,
    fetchADKAuditPage: fetchADKAuditPageMock,
    fetchADKMemory: fetchADKMemoryMock,
    fetchADKMetrics: fetchADKMetricsMock,
    fetchADKOptimizationTasks: fetchADKOptimizationTasksMock,
    fetchADKRunsPage: fetchADKRunsPageMock,
    fetchADKSettingsSnapshot: fetchADKSettingsSnapshotMock,
    fetchADKSkills: fetchADKSkillsMock,
    fetchADKTasks: fetchADKTasksMock,
    installADKSkill: installADKSkillMock,
    resumeADKRun: resumeADKRunMock,
    saveADKAgent: saveADKAgentMock,
    saveADKProvider: saveADKProviderMock,
    saveADKRuntimeSettings: saveADKRuntimeSettingsMock,
    setADKDefaultProvider: setADKDefaultProviderMock,
    testADKProvider: testADKProviderMock,
    uninstallADKSkill: uninstallADKSkillMock,
  };
});

const wrappers: Array<ReturnType<typeof mount>> = [];

beforeEach(() => {
  vi.clearAllMocks();

  fetchADKSettingsSnapshotMock.mockResolvedValue(buildSnapshot());
  fetchADKRunsPageMock.mockImplementation(
    async (page: PageEnvelope, status: string) => ({
      runs:
        status === "RUNNING"
          ? [buildRun({ id: "run-running", status: "RUNNING" })]
          : [
              buildRun({ id: "run-failed", status: "FAILED" }),
              buildRun({ id: "run-running", status: "RUNNING" }),
            ],
      page: buildPage(page.limit, page.offset, page.offset === 0),
    }),
  );
  fetchADKApprovalsPageMock.mockImplementation(
    async (page: PageEnvelope, status: string) => ({
      approvals: [
        buildApproval({
          id: `approval-${status || "all"}-${page.offset}`,
          status: status || "PENDING",
        }),
      ],
      page: buildPage(page.limit, page.offset, page.offset === 0),
    }),
  );
  fetchADKAuditPageMock.mockImplementation(
    async (page: PageEnvelope, kind: string) => ({
      events: [
        buildAuditEvent({
          id: `audit-${kind || "all"}-${page.offset}`,
          kind: kind || "run.updated",
        }),
      ],
      page: buildPage(page.limit, page.offset, page.offset === 0),
    }),
  );
  fetchADKSkillsMock.mockResolvedValue([
    buildSkill({ id: "research", source: "https://skills.example/research" }),
  ]);
  fetchADKOptimizationTasksMock.mockResolvedValue([
    buildOptimizationTask({ id: "opt-refresh" }),
  ]);
  fetchADKTasksMock.mockResolvedValue([buildTask()]);
  fetchADKMemoryMock.mockResolvedValue([buildMemoryEntry()]);
  fetchADKMetricsMock.mockResolvedValue(buildMetrics());
  cancelADKRunMock.mockResolvedValue(undefined);
  resumeADKRunMock.mockResolvedValue(undefined);
  cancelADKOptimizationTaskMock.mockResolvedValue(undefined);
  installADKSkillMock.mockResolvedValue(undefined);
  uninstallADKSkillMock.mockResolvedValue(undefined);
  saveADKRuntimeSettingsMock.mockResolvedValue({
    runTimeoutMs: 60_000,
    streamIdleTimeoutMs: 900_000,
  });
});

afterEach(() => {
  for (const wrapper of wrappers.splice(0)) wrapper.unmount();
});

describe("useADKSettingsSectionState", () => {
  it("hydrates state, clamps runtime values, and derives tool metadata from live settings", async () => {
    fetchADKSettingsSnapshotMock.mockResolvedValueOnce(
      buildSnapshot({
        providers: [
          buildProvider({
            id: "provider-fallback",
            displayName: "Fallback",
            model: "gpt-4.1-mini",
            default: false,
            hasApiKey: false,
          }),
          buildProvider({
            id: "provider-default",
            displayName: "OpenAI",
            model: "gpt-4.1",
            default: true,
          }),
        ],
        tools: [
          buildTool({
            name: "system.status",
            displayName: "系统状态",
            category: "system",
            riskLevel: "low",
          }),
          buildTool({
            name: "trading.submit_order",
            displayName: "提交订单",
            description: "向交易系统提交订单。",
            category: "trading",
            permission: "trade_execute",
            riskLevel: "critical",
            outputSummary: "已发送到经纪商",
          }),
        ],
        skills: [
          buildSkill({ id: "risk" }),
          buildSkill({
            id: "research",
            source: "https://skills.example/research",
          }),
        ],
        runtimeSettings: {
          runTimeoutMs: 1_000,
          streamIdleTimeoutMs: 99_999_999,
        },
      }),
    );
    fetchADKRunsPageMock.mockResolvedValueOnce({
      runs: [
        buildRun({ id: "run-failed", status: "FAILED" }),
        buildRun({ id: "run-running", status: "RUNNING" }),
        buildRun({ id: "run-pending", status: "PENDING_APPROVAL" }),
      ],
    });
    fetchADKApprovalsPageMock.mockResolvedValueOnce({
      approvals: [buildApproval()],
    });
    fetchADKAuditPageMock.mockResolvedValueOnce({
      events: [buildAuditEvent()],
    });

    const state = await mountState();

    expect(state.loading.value).toBe(false);
    expect(state.runtimeSettingsForm.value).toEqual({
      runTimeoutSeconds: 60,
      streamIdleTimeoutSeconds: 900,
    });
    expect(state.agentForm.value.skills).toEqual(["risk", "research"]);
    expect(state.providerOptions.value).toEqual([
      { title: "默认模型 · OpenAI · gpt-4.1", value: "" },
      {
        title: "Fallback · gpt-4.1-mini · 未配置密钥",
        value: "provider-fallback",
      },
      {
        title: "OpenAI · gpt-4.1 · 默认",
        value: "provider-default",
      },
    ]);
    expect(state.runPage.value).toEqual({
      limit: 20,
      offset: 0,
      total: 3,
      returned: 3,
      hasMore: false,
    });
    expect(state.approvalPage.value).toEqual({
      limit: 10,
      offset: 0,
      total: 1,
      returned: 1,
      hasMore: false,
    });
    expect(state.auditPage.value).toEqual({
      limit: 12,
      offset: 0,
      total: 1,
      returned: 1,
      hasMore: false,
    });

    expect(state.filteredRuns.value.map((run) => run.id)).toEqual([
      "run-failed",
      "run-pending",
    ]);
    expect(state.pendingApprovals.value).toHaveLength(1);
    expect(state.skillOptions.value).toEqual([
      { title: "风险控制", value: "risk" },
      { title: "研究助手", value: "research" },
    ]);
    expect(state.toolCategoryOptions.value).toEqual(["system", "trading"]);
    expect(state.toolRiskOptions.value).toEqual(["critical", "low"]);

    state.toolSearchQuery.value = "提交订单";
    await nextTick();
    expect(state.filteredTools.value.map((tool) => tool.name)).toEqual([
      "trading.submit_order",
    ]);

    state.toolSearchQuery.value = "";
    state.toolCategoryFilter.value = "trading";
    state.toolRiskFilter.value = "critical";
    await nextTick();
    expect(state.filteredTools.value.map((tool) => tool.name)).toEqual([
      "trading.submit_order",
    ]);
    expect(state.toolOptions.value).toEqual([
      { title: "提交订单 (trading.submit_order)", value: "trading.submit_order" },
    ]);

    state.openToolDetail("trading.submit_order");
    expect(state.toolDetailDialogOpen.value).toBe(true);
    expect(state.selectedTool.value?.name).toBe("trading.submit_order");
    state.closeToolDetail();
    expect(state.toolDetailDialogOpen.value).toBe(false);

    state.applyAgentTemplate(
      buildTemplate({
        name: "策略模板",
        tools: ["trading.submit_order"],
        skills: ["research"],
        workMode: "loop",
        loopMaxIterations: 9,
      }),
    );
    expect(state.agentForm.value).toMatchObject({
      id: "",
      name: "策略模板",
      tools: ["trading.submit_order"],
      skills: ["research"],
      workMode: "loop",
      loopMaxIterations: 9,
    });
    expect(state.agentTemplateNotice.value).toContain("策略模板");
  });

  it("reacts to filters, tasks, memory queries, and page navigation using the current live state", async () => {
    const state = await mountState();

    state.runStatusFilter.value = "RUNNING";
    state.approvalStatusFilter.value = "APPROVED";
    state.auditKindFilter.value = "task.created";
    await flushRequests();

    expect(fetchADKRunsPageMock).toHaveBeenLastCalledWith(
      expect.objectContaining({ limit: 20, offset: 0 }),
      "RUNNING",
    );
    expect(fetchADKApprovalsPageMock).toHaveBeenLastCalledWith(
      expect.objectContaining({ limit: 10, offset: 0 }),
      "APPROVED",
    );
    expect(fetchADKAuditPageMock).toHaveBeenLastCalledWith(
      expect.objectContaining({ limit: 12, offset: 0 }),
      "task.created",
    );

    await state.nextRunsPage();
    expect(fetchADKRunsPageMock).toHaveBeenLastCalledWith(
      expect.objectContaining({ limit: 20, offset: 20 }),
      "RUNNING",
    );
    await state.previousRunsPage();
    expect(fetchADKRunsPageMock).toHaveBeenLastCalledWith(
      expect.objectContaining({ limit: 20, offset: 0 }),
      "RUNNING",
    );

    await state.nextApprovalsPage();
    expect(fetchADKApprovalsPageMock).toHaveBeenLastCalledWith(
      expect.objectContaining({ limit: 10, offset: 10 }),
      "APPROVED",
    );
    await state.previousApprovalsPage();
    expect(fetchADKApprovalsPageMock).toHaveBeenLastCalledWith(
      expect.objectContaining({ limit: 10, offset: 0 }),
      "APPROVED",
    );

    await state.nextAuditPage();
    expect(fetchADKAuditPageMock).toHaveBeenLastCalledWith(
      expect.objectContaining({ limit: 12, offset: 12 }),
      "task.created",
    );
    await state.previousAuditPage();
    expect(fetchADKAuditPageMock).toHaveBeenLastCalledWith(
      expect.objectContaining({ limit: 12, offset: 0 }),
      "task.created",
    );

    state.taskStatusFilter.value = "DONE";
    state.taskAgentFilter.value = "agent-special";
    await flushRequests();
    expect(fetchADKTasksMock).toHaveBeenLastCalledWith({
      status: "DONE",
      agentId: "agent-special",
    });

    state.memoryScopeFilter.value = "agent";
    state.memoryAgentFilter.value = "agent-special";
    state.memoryKeyFilter.value = "preferred_market";
    await flushRequests();
    expect(fetchADKMemoryMock).toHaveBeenLastCalledWith({
      scope: "agent",
      agentId: "agent-special",
      key: "preferred_market",
    });

    await state.refreshMetrics();
    expect(fetchADKMetricsMock).toHaveBeenCalled();
  });

  it("executes real state actions for runs, skills, and runtime settings", async () => {
    const state = await mountState();
    const run = buildRun({
      id: "run-timeout",
      status: "TIMED_OUT",
      workMode: "loop",
      workflowStatus: "RUNNING",
    });
    const task = buildOptimizationTask({ id: "opt-running" });

    await state.cancelRun(run);
    expect(cancelADKRunMock).toHaveBeenCalledWith("run-timeout");
    expect(fetchADKRunsPageMock.mock.calls.length).toBeGreaterThan(1);
    expect(fetchADKApprovalsPageMock.mock.calls.length).toBeGreaterThan(1);
    expect(fetchADKAuditPageMock.mock.calls.length).toBeGreaterThan(1);
    expect(fetchADKMetricsMock.mock.calls.length).toBeGreaterThan(0);

    await state.resumeRun(run);
    expect(resumeADKRunMock).toHaveBeenCalledWith("run-timeout");
    expect(state.successMessage.value).toBe("已继续运行目标");

    await state.cancelOptimizationTask(task);
    expect(cancelADKOptimizationTaskMock).toHaveBeenCalledWith("opt-running");
    expect(fetchADKOptimizationTasksMock).toHaveBeenCalled();

    await state.installSkill();
    expect(installADKSkillMock).not.toHaveBeenCalled();

    state.skillUrl.value = "  https://skills.example/alpha.git  ";
    await state.installSkill();
    expect(installADKSkillMock).toHaveBeenCalledWith(
      "https://skills.example/alpha.git",
    );
    expect(state.skillUrl.value).toBe("");
    expect(fetchADKSkillsMock).toHaveBeenCalled();

    await state.uninstallSkill(buildSkill({ id: "builtin", source: "builtin" }));
    expect(uninstallADKSkillMock).not.toHaveBeenCalledWith("builtin");
    expect(state.errorMessage.value).toBe("内部来源的技能不允许卸载");

    await state.uninstallSkill(
      buildSkill({
        id: "external",
        source: "https://skills.example/external",
      }),
    );
    expect(uninstallADKSkillMock).toHaveBeenCalledWith("external");

    state.runtimeSettingsForm.value = {
      runTimeoutSeconds: 99_999,
      streamIdleTimeoutSeconds: Number.NaN,
    };
    saveADKRuntimeSettingsMock.mockResolvedValueOnce({
      runTimeoutMs: 50_000,
      streamIdleTimeoutMs: 2_000_000,
    });
    fetchADKSettingsSnapshotMock.mockResolvedValueOnce(
      buildSnapshot({
        runtimeSettings: {
          runTimeoutMs: 50_000,
          streamIdleTimeoutMs: 2_000_000,
        },
      }),
    );
    await state.saveRuntimeSettings();

    expect(saveADKRuntimeSettingsMock).toHaveBeenLastCalledWith({
      runTimeoutMs: 43_200_000,
      streamIdleTimeoutMs: 300_000,
    });
    expect(state.runtimeSettingsForm.value).toEqual({
      runTimeoutSeconds: 60,
      streamIdleTimeoutSeconds: 900,
    });
    expect(state.successMessage.value).toBe("ADK 运行时设置已保存");
  });

  it("surfaces business failures with the correct fallback messages", async () => {
    const state = await mountState();
    const run = buildRun();
    const task = buildOptimizationTask();

    fetchADKSettingsSnapshotMock.mockRejectedValueOnce("offline");
    await state.refreshAll();
    expect(state.errorMessage.value).toBe("加载智能体配置失败");

    cancelADKRunMock.mockRejectedValueOnce(new Error("run locked"));
    await state.cancelRun(run);
    expect(state.errorMessage.value).toBe("run locked");

    resumeADKRunMock.mockRejectedValueOnce("resume failed");
    await state.resumeRun(run);
    expect(state.errorMessage.value).toBe("继续运行失败");

    cancelADKOptimizationTaskMock.mockRejectedValueOnce("busy");
    await state.cancelOptimizationTask(task);
    expect(state.errorMessage.value).toBe("取消优化任务失败");

    state.skillUrl.value = "https://skills.example/fail";
    installADKSkillMock.mockRejectedValueOnce(new Error("install failed"));
    await state.installSkill();
    expect(state.errorMessage.value).toBe("install failed");

    uninstallADKSkillMock.mockRejectedValueOnce("blocked");
    await state.uninstallSkill(
      buildSkill({
        id: "skill-fail",
        source: "https://skills.example/fail",
      }),
    );
    expect(state.errorMessage.value).toBe("卸载失败");

    saveADKRuntimeSettingsMock.mockRejectedValueOnce("bad settings");
    await state.saveRuntimeSettings();
    expect(state.errorMessage.value).toBe("保存运行时设置失败");
  });
});

async function mountState() {
  let state!: ReturnType<typeof useADKSettingsSectionState>;
  const wrapper = mount(
    defineComponent({
      setup() {
        state = useADKSettingsSectionState();
        return () => h("div");
      },
    }),
  );
  wrappers.push(wrapper);
  await flushRequests();
  return state;
}

function buildSnapshot(
  overrides: Partial<{
    providers: ADKProvider[];
    agents: ADKAgent[];
    tools: ADKToolDescriptor[];
    skills: ADKSkill[];
    runtimeSettings: { runTimeoutMs: number; streamIdleTimeoutMs: number };
    optimizationTasks: ADKOptimizationTask[];
    tasks: ADKTask[];
    memoryEntries: ADKMemoryEntry[];
    agentTemplates: Array<Omit<ADKAgent, "createdAt" | "updatedAt">>;
    metrics: ADKMetricsResponse;
  }> = {},
) {
  return {
    providers: overrides.providers ?? [buildProvider()],
    agents: overrides.agents ?? [buildAgent()],
    tools: overrides.tools ?? [buildTool()],
    skills: overrides.skills ?? [buildSkill()],
    runtimeSettings:
      overrides.runtimeSettings ?? {
        runTimeoutMs: 1_800_000,
        streamIdleTimeoutMs: 300_000,
      },
    optimizationTasks:
      overrides.optimizationTasks ?? [buildOptimizationTask()],
    tasks: overrides.tasks ?? [buildTask()],
    memoryEntries: overrides.memoryEntries ?? [buildMemoryEntry()],
    agentTemplates: overrides.agentTemplates ?? [buildTemplate()],
    metrics: overrides.metrics ?? buildMetrics(),
  };
}

function buildPage(limit: number, offset: number, hasMore: boolean): PageEnvelope {
  return {
    limit,
    offset,
    total: hasMore ? offset + limit + 1 : offset + 1,
    returned: 1,
    hasMore,
  };
}

function buildProvider(overrides: Partial<ADKProvider> = {}): ADKProvider {
  return {
    id: "provider-1",
    displayName: "OpenAI",
    baseUrl: "https://api.openai.com/v1",
    model: "gpt-4o-mini",
    requestTimeoutMs: 180_000,
    enabled: true,
    default: true,
    hasApiKey: true,
    createdAt: "2026-07-01T00:00:00Z",
    updatedAt: "2026-07-01T00:00:00Z",
    ...overrides,
  };
}

function buildAgent(overrides: Partial<ADKAgent> = {}): ADKAgent {
  return {
    id: "agent-1",
    name: "交易助手",
    instruction: "关注交易风险。",
    providerId: "provider-1",
    model: "gpt-4o-mini",
    tools: [],
    skills: [],
    permissionMode: "approval",
    memoryEnabled: true,
    recentUserWindow: 6,
    workMode: "chat",
    loopMaxIterations: 5,
    status: "ENABLED",
    createdAt: "2026-07-01T00:00:00Z",
    updatedAt: "2026-07-01T00:00:00Z",
    ...overrides,
  };
}

function buildTemplate(
  overrides: Partial<Omit<ADKAgent, "createdAt" | "updatedAt">> = {},
): Omit<ADKAgent, "createdAt" | "updatedAt"> {
  const agent = buildAgent(overrides);
  const { createdAt: _createdAt, updatedAt: _updatedAt, ...template } = agent;
  return template;
}

function buildTool(overrides: Partial<ADKToolDescriptor> = {}): ADKToolDescriptor {
  return {
    name: "system.status",
    displayName: "系统状态",
    description: "读取系统状态摘要。",
    category: "system",
    permission: "system_read",
    riskLevel: "low",
    allowedModes: ["approval", "less_approval", "all"],
    requiresApprovalIn: [],
    ...overrides,
  };
}

function buildSkill(overrides: Partial<ADKSkill> = {}): ADKSkill {
  return {
    id: "risk",
    displayName: overrides.id === "research" ? "研究助手" : "风险控制",
    description: "帮助完成投资分析。",
    source: "builtin",
    installPath: "",
    createdAt: "2026-07-01T00:00:00Z",
    updatedAt: "2026-07-01T00:00:00Z",
    ...overrides,
  };
}

function buildRun(overrides: Partial<ADKRun> = {}): ADKRun {
  return {
    id: "run-1",
    sessionId: "session-1",
    agentId: "agent-1",
    providerId: "provider-1",
    providerName: "OpenAI",
    model: "gpt-4o-mini",
    status: "FAILED",
    workMode: "chat",
    workflowStatus: "",
    userMessage: "检查持仓",
    message: "",
    failureReason: "tool failed",
    toolCalls: [],
    degraded: false,
    createdAt: "2026-07-01T00:00:00Z",
    updatedAt: "2026-07-01T00:01:00Z",
    ...overrides,
  } as ADKRun;
}

function buildApproval(overrides: Partial<ADKApproval> = {}): ADKApproval {
  return {
    id: "approval-1",
    runId: "run-1",
    agentId: "agent-1",
    toolName: "trading.submit_order",
    reason: "需要确认",
    status: "PENDING",
    updatedAt: "2026-07-01T00:00:00Z",
    functionCallId: "fc-1",
    confirmationCallId: "cc-1",
    ...overrides,
  } as ADKApproval;
}

function buildOptimizationTask(
  overrides: Partial<ADKOptimizationTask> = {},
): ADKOptimizationTask {
  return {
    id: "opt-1",
    status: "running",
    progress: { completed: 2, total: 5 },
    createdAt: "2026-07-01T00:00:00Z",
    updatedAt: "2026-07-01T00:05:00Z",
    ...overrides,
  } as ADKOptimizationTask;
}

function buildTask(overrides: Partial<ADKTask> = {}): ADKTask {
  return {
    id: "task-1",
    title: "Follow-up review",
    description: "复核执行结果",
    status: "TODO",
    agentId: "agent-1",
    createdAt: "2026-07-01T00:00:00Z",
    updatedAt: "2026-07-01T00:00:00Z",
    ...overrides,
  } as ADKTask;
}

function buildMemoryEntry(
  overrides: Partial<ADKMemoryEntry> = {},
): ADKMemoryEntry {
  return {
    id: "memory-1",
    scope: "agent",
    key: "preferred_market",
    value: "HK",
    agentId: "agent-1",
    createdAt: "2026-07-01T00:00:00Z",
    updatedAt: "2026-07-01T00:00:00Z",
    ...overrides,
  } as ADKMemoryEntry;
}

function buildAuditEvent(
  overrides: Partial<ADKAuditEvent> = {},
): ADKAuditEvent {
  return {
    id: "audit-1",
    kind: "run.updated",
    detail: "运行状态已更新",
    createdAt: "2026-07-01T00:00:00Z",
    subjectId: "run-1",
    metadata: { status: "FAILED" },
    ...overrides,
  } as ADKAuditEvent;
}

function buildMetrics(overrides: Partial<ADKMetricsResponse> = {}): ADKMetricsResponse {
  return {
    runs: {
      total: 4,
      byStatus: {},
      byAgent: {},
      byProvider: {},
      lifecycle: {
        failed: 1,
        timedOut: 1,
        cancelled: 0,
        resumed: 1,
        orphaned: 0,
      },
    },
    tools: {
      total: 10,
      successful: 8,
      averageDurationMs: 800,
      byName: {},
      byStatus: {},
    },
    approvals: {
      pending: 1,
      total: 2,
      approved: 1,
      denied: 0,
      recoverablePending: 1,
      pendingWaitMs: { average: 500, max: 1_200 },
      resolutionWaitMs: { average: 900, max: 1_500, count: 2 },
    },
    usage: {
      samples: 2,
      tokensInTotal: 100,
      tokensOutTotal: 200,
      tokensInAverage: 50,
      tokensOutAverage: 100,
    },
    ...overrides,
  };
}
