import type {
  ADKAgent,
  ADKApproval,
  ADKAuditEvent,
  ADKMemoryFilters,
  ADKMemoryEntry,
  ADKOptimizationTask,
  ADKProvider,
  ADKRun,
  ADKSkill,
  ADKTask,
  ADKTaskFilters,
  ADKTaskPatch,
  ADKToolDescriptor,
  MCPServerSettings,
  MCPServerSettingsSnapshot,
  MCPServerTokenResetResult,
} from "@/types";
import type { ADKRuntimeSettings } from "@/contracts";

import {
  apiDeletePath,
  apiGet,
  apiGetPath,
  apiPost,
  apiPostAction,
  apiPostPathAction,
  apiPut,
  apiPutPath,
} from "./apiClient";
import {
  requireADKAgent,
  requireADKAgents,
  requireADKAgentTemplates,
  requireADKApprovals,
  requireADKAuditEvents,
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
  requireADKSkill,
  requireADKSkills,
  requireADKTask,
  requireADKTasks,
  requireADKToolDescriptors,
  requireMCPSettingsSnapshot,
  requireMCPTokenResetResult,
  type ADKMetricsView,
} from "./adkApiMappers";
import { normalizeADKRun, normalizeADKRunList } from "./adkNormalization";

export {
  deleteADKWorkflow,
  deleteADKWorkflowTrigger,
  fetchADKWorkflowTriggerLogs,
  fetchADKWorkflowTriggers,
  fetchADKWorkflows,
  runADKWorkflow,
  runADKWorkflowTrigger,
  saveADKWorkflow,
  saveADKWorkflowTrigger,
} from "./adkWorkflowsApi";

export interface PageEnvelope {
  limit: number;
  offset: number;
  total: number;
  returned: number;
  hasMore: boolean;
}

export type ADKMetricsResponse = ADKMetricsView;

interface RunsResponse {
  runs: ADKRun[];
  page?: PageEnvelope;
}
interface ApprovalsResponse {
  approvals: ADKApproval[];
  page?: PageEnvelope;
}
interface AuditResponse {
  events: ADKAuditEvent[];
  page?: PageEnvelope;
}
export async function fetchADKSettingsSnapshot(): Promise<{
  providers: ADKProvider[];
  agents: ADKAgent[];
  tools: ADKToolDescriptor[];
  skills: ADKSkill[];
  runtimeSettings: ADKRuntimeSettings;
  optimizationTasks: ADKOptimizationTask[];
  tasks: ADKTask[];
  memoryEntries: ADKMemoryEntry[];
  agentTemplates: Array<Omit<ADKAgent, "createdAt" | "updatedAt">>;
  metrics: ADKMetricsResponse;
}> {
  const [snapshot, optimizationTasks, tasks, memory, templates, metrics] =
    await Promise.all([
      apiGet("/api/v1/adk"),
      apiGetPath(
        "/api/v1/adk/optimization-tasks",
        "/api/v1/adk/optimization-tasks?limit=20",
      ),
      apiGetPath("/api/v1/adk/tasks", "/api/v1/adk/tasks?limit=20"),
      apiGet("/api/v1/adk/memory"),
      apiGet("/api/v1/adk/agent-templates"),
      apiGet("/api/v1/adk/metrics"),
    ]);

  return {
    providers: requireADKProviders(snapshot.providers),
    agents: requireADKAgents(snapshot.agents),
    tools: requireADKToolDescriptors(snapshot.tools),
    skills: requireADKSkills(snapshot.skills),
    runtimeSettings:
      snapshot.runtimeSettings === undefined
        ? { runTimeoutMs: 1_800_000, streamIdleTimeoutMs: 300_000 }
        : requireADKRuntimeSettings(snapshot.runtimeSettings),
    optimizationTasks: requireADKOptimizationTasks(optimizationTasks.tasks),
    tasks: requireADKTasks(tasks.tasks),
    memoryEntries: requireADKMemoryEntries(memory.entries),
    agentTemplates: requireADKAgentTemplates(templates.templates),
    metrics: requireADKMetrics(metrics),
  };
}

export async function fetchADKRunsPage(
  page: PageEnvelope,
  runStatusFilter: string,
): Promise<RunsResponse> {
  const response = await apiGetPath(
    "/api/v1/adk/runs",
    buildRunsURL(page, runStatusFilter),
  );
  return {
    runs: normalizeADKRunList(requireADKRuns(response.runs)),
    ...(response.page === undefined
      ? {}
      : { page: requireADKPage(response.page) }),
  };
}

export async function fetchADKApprovalsPage(
  page: PageEnvelope,
  approvalStatusFilter: string,
): Promise<ApprovalsResponse> {
  const response = await apiGetPath(
    "/api/v1/adk/approvals",
    buildApprovalsURL(page, approvalStatusFilter),
  );
  return {
    approvals: requireADKApprovals(response.approvals),
    ...(response.page === undefined
      ? {}
      : { page: requireADKPage(response.page) }),
  };
}

export async function fetchADKAuditPage(
  page: PageEnvelope,
  auditKindFilter: string,
): Promise<AuditResponse> {
  const response = await apiGetPath(
    "/api/v1/adk/audit",
    buildAuditURL(page, auditKindFilter),
  );
  return {
    events: requireADKAuditEvents(response.events),
    ...(response.page === undefined
      ? {}
      : { page: requireADKPage(response.page) }),
  };
}

export async function fetchADKSkills(): Promise<ADKSkill[]> {
  const response = await apiGet("/api/v1/adk/skills");
  return requireADKSkills(response.skills);
}

export async function fetchADKOptimizationTasks(): Promise<
  ADKOptimizationTask[]
> {
  const response = await apiGetPath(
    "/api/v1/adk/optimization-tasks",
    "/api/v1/adk/optimization-tasks?limit=20",
  );
  return requireADKOptimizationTasks(response.tasks);
}

export async function fetchADKTasks(
  filters: ADKTaskFilters = {},
): Promise<ADKTask[]> {
  const params = new URLSearchParams();
  params.set("limit", String(filters.limit ?? 20));
  if (filters.offset !== undefined)
    params.set("offset", String(filters.offset));
  if (filters.status) params.set("status", filters.status);
  if (filters.agentId) params.set("agentId", filters.agentId);
  if (filters.runId) params.set("runId", filters.runId);
  const response = await apiGetPath(
    "/api/v1/adk/tasks",
    `/api/v1/adk/tasks?${params.toString()}`,
  );
  return requireADKTasks(response.tasks);
}

export async function saveADKTask(task: {
  id?: string;
  title: string;
  description?: string;
  status?: string;
  agentId?: string;
  runId?: string;
  dependsOn?: string[];
  order?: number;
  modeHint?: string;
  agentRole?: string;
  plannerStepId?: string;
  planSource?: string;
  workflowMode?: string;
  objective?: string;
  message?: string;
  executor?: string;
  childProviderId?: string;
  childModel?: string;
  resultSummary?: string;
  plannerWarnings?: string[];
}): Promise<ADKTask> {
  return requireADKTask(await apiPost("/api/v1/adk/tasks", task));
}

export async function updateADKTask(
  taskId: string,
  patch: ADKTaskPatch,
): Promise<ADKTask> {
  return requireADKTask(
    await apiPutPath(
      "/api/v1/adk/tasks/{taskId}",
      `/api/v1/adk/tasks/${encodeURIComponent(taskId)}`,
      patch,
    ),
  );
}

export async function deleteADKTask(taskId: string): Promise<void> {
  await apiDeletePath(
    "/api/v1/adk/tasks/{taskId}",
    `/api/v1/adk/tasks/${encodeURIComponent(taskId)}`,
  );
}

export async function fetchADKMemory(
  filters: ADKMemoryFilters = {},
): Promise<ADKMemoryEntry[]> {
  const params = new URLSearchParams();
  if (filters.scope) params.set("scope", filters.scope);
  if (filters.agentId) params.set("agentId", filters.agentId);
  if (filters.key) params.set("key", filters.key);
  const suffix = params.toString();
  const response = await apiGetPath(
    "/api/v1/adk/memory",
    `/api/v1/adk/memory${suffix ? `?${suffix}` : ""}`,
  );
  return requireADKMemoryEntries(response.entries);
}

export async function saveADKMemory(entry: {
  agentId?: string;
  key: string;
  value: string;
  scope?: string;
}): Promise<ADKMemoryEntry> {
  return requireADKMemoryEntry(await apiPost("/api/v1/adk/memory", entry));
}

export async function deleteADKMemory(memoryId: string): Promise<void> {
  await apiDeletePath(
    "/api/v1/adk/memory/{memoryId}",
    `/api/v1/adk/memory/${encodeURIComponent(memoryId)}`,
  );
}

export async function fetchADKMetrics(): Promise<ADKMetricsResponse> {
  return requireADKMetrics(await apiGet("/api/v1/adk/metrics"));
}

export async function saveADKProvider(provider: {
  id: string;
  displayName: string;
  baseUrl: string;
  model: string;
  contextWindowTokens: number;
  requestTimeoutMs: number;
  apiKey: string;
  enabled: boolean;
}): Promise<ADKProvider> {
  return requireADKProvider(await apiPost("/api/v1/adk/providers", provider));
}

export async function fetchADKRuntimeSettings(): Promise<ADKRuntimeSettings> {
  return requireADKRuntimeSettings(await apiGet("/api/v1/settings/adk"));
}

export async function saveADKRuntimeSettings(
  settings: ADKRuntimeSettings,
): Promise<ADKRuntimeSettings> {
  return requireADKRuntimeSettings(
    await apiPut("/api/v1/settings/adk", settings),
  );
}

export async function fetchMCPServerSettings(): Promise<MCPServerSettingsSnapshot> {
  return requireMCPSettingsSnapshot(await apiGet("/api/v1/settings/adk/mcp"));
}

export async function saveMCPServerSettings(settings: Pick<MCPServerSettings, "enabled" | "port" | "authMode">): Promise<MCPServerSettingsSnapshot> {
  return requireMCPSettingsSnapshot(
    await apiPut("/api/v1/settings/adk/mcp", settings),
  );
}

export async function resetMCPServerToken(): Promise<MCPServerTokenResetResult> {
  return requireMCPTokenResetResult(
    await apiPostAction("/api/v1/settings/adk/mcp/token/reset"),
  );
}

export async function testADKProvider(
  providerId: string,
): Promise<Record<string, unknown>> {
  return {
    ...(await apiPostPathAction(
    "/api/v1/adk/providers/{providerId}/test",
    `/api/v1/adk/providers/${encodeURIComponent(providerId)}/test`,
    )),
  };
}

export async function setADKDefaultProvider(
  providerId: string,
): Promise<ADKProvider> {
  return requireADKProvider(
    await apiPostPathAction(
      "/api/v1/adk/providers/{providerId}/default",
      `/api/v1/adk/providers/${encodeURIComponent(providerId)}/default`,
    ),
  );
}

export async function deleteADKProvider(providerId: string): Promise<void> {
  await apiDeletePath(
    "/api/v1/adk/providers/{providerId}",
    `/api/v1/adk/providers/${encodeURIComponent(providerId)}`,
  );
}

export async function saveADKAgent(agent: {
  id: string;
  name: string;
  instruction: string;
  providerId: string;
  model: string;
  tools: string[];
  skills: string[];
  permissionMode: string;
  memoryEnabled: boolean;
  recentUserWindow: number;
  workMode: ADKAgent["workMode"];
  loopMaxIterations: number;
  status: string;
}): Promise<ADKAgent> {
  return requireADKAgent(await apiPost("/api/v1/adk/agents", agent));
}

export async function deleteADKAgent(agentId: string): Promise<void> {
  await apiDeletePath(
    "/api/v1/adk/agents/{agentId}",
    `/api/v1/adk/agents/${encodeURIComponent(agentId)}`,
  );
}

export async function cancelADKRun(runId: string): Promise<ADKRun> {
  return normalizeADKRun(
    requireADKRun(
      await apiPostPathAction(
      "/api/v1/adk/runs/{runId}/cancel",
      `/api/v1/adk/runs/${encodeURIComponent(runId)}/cancel`,
      ),
    ),
  );
}

export async function resumeADKRun(runId: string): Promise<ADKRun> {
  return normalizeADKRun(
    requireADKRun(
      await apiPostPathAction(
      "/api/v1/adk/runs/{runId}/resume",
      `/api/v1/adk/runs/${encodeURIComponent(runId)}/resume`,
      ),
    ),
  );
}

export async function cancelADKOptimizationTask(
  taskId: string,
): Promise<ADKOptimizationTask> {
  return requireADKOptimizationTask(
    await apiPostPathAction(
      "/api/v1/adk/optimization-tasks/{taskId}/cancel",
      `/api/v1/adk/optimization-tasks/${encodeURIComponent(taskId)}/cancel`,
    ),
  );
}

export async function installADKSkill(url: string): Promise<ADKSkill> {
  return requireADKSkill(await apiPost("/api/v1/adk/skills", { url }));
}

export async function uninstallADKSkill(skillId: string): Promise<void> {
  await apiDeletePath(
    "/api/v1/adk/skills/{skillId}",
    `/api/v1/adk/skills/${encodeURIComponent(skillId)}`,
  );
}

export function fallbackPage(
  limit: number,
  offset: number,
  total: number,
): PageEnvelope {
  return { limit, offset, total, returned: total, hasMore: false };
}

export function previousPage(page: { offset: number; limit: number }): void {
  page.offset = Math.max(0, page.offset - page.limit);
}

export function nextPage(
  page: { offset: number; limit: number },
  current: PageEnvelope,
): void {
  if (!current.hasMore) return;
  page.offset += page.limit;
}

export function pageSummary(page: PageEnvelope): string {
  if (page.total === 0) return "0 / 0";
  return `${page.offset + 1}-${page.offset + page.returned} / ${page.total}`;
}

function buildRunsURL(page: PageEnvelope, runStatusFilter: string): string {
  const params = new URLSearchParams({
    limit: String(page.limit),
    offset: String(page.offset),
  });
  if (runStatusFilter !== "" && runStatusFilter !== "attention") {
    params.set("status", runStatusFilter);
  }
  return `/api/v1/adk/runs?${params.toString()}`;
}

function buildApprovalsURL(
  page: PageEnvelope,
  approvalStatusFilter: string,
): string {
  const params = new URLSearchParams({
    limit: String(page.limit),
    offset: String(page.offset),
  });
  if (approvalStatusFilter !== "") {
    params.set("status", approvalStatusFilter);
  }
  return `/api/v1/adk/approvals?${params.toString()}`;
}

function buildAuditURL(page: PageEnvelope, auditKindFilter: string): string {
  const params = new URLSearchParams({
    limit: String(page.limit),
    offset: String(page.offset),
  });
  if (auditKindFilter.trim() !== "") {
    params.set("kind", auditKindFilter.trim());
  }
  return `/api/v1/adk/audit?${params.toString()}`;
}
