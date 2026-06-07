import type {
  ADKAgent,
  ADKApproval,
  ADKAuditEvent,
  ADKOptimizationTask,
  ADKProvider,
  ADKRun,
  ADKSkill,
  ADKToolDescriptor,
} from "@jftrade/ui-contracts";

import { fetchEnvelope, fetchEnvelopeWithInit } from "./apiClient";

export interface PageEnvelope {
  limit: number;
  offset: number;
  total: number;
  returned: number;
  hasMore: boolean;
}

export interface ADKMetricsResponse {
  runs: {
    total: number;
    byStatus: Record<string, number>;
    byAgent: Record<string, number>;
    byProvider: Record<string, number>;
    lifecycle: {
      failed: number;
      timedOut: number;
      cancelled: number;
      resumed: number;
      orphaned: number;
    };
  };
  tools: {
    total: number;
    successful: number;
    averageDurationMs: number;
    byName: Record<string, number>;
    byStatus: Record<string, number>;
  };
  approvals: {
    pending: number;
    total: number;
    approved: number;
    denied: number;
    recoverablePending: number;
    pendingWaitMs: { average: number; max: number };
    resolutionWaitMs: { average: number; max: number; count: number };
  };
  usage: {
    samples: number;
    tokensInTotal: number | null;
    tokensOutTotal: number | null;
    tokensInAverage: number | null;
    tokensOutAverage: number | null;
  };
}

interface ProvidersResponse { providers: ADKProvider[] }
interface AgentsResponse { agents: ADKAgent[] }
interface ToolsResponse { tools: ADKToolDescriptor[] }
interface SkillsResponse { skills: ADKSkill[] }
interface RunsResponse { runs: ADKRun[]; page?: PageEnvelope }
interface ApprovalsResponse { approvals: ADKApproval[]; page?: PageEnvelope }
interface OptimizationTasksResponse { tasks: ADKOptimizationTask[] }
interface AuditResponse { events: ADKAuditEvent[]; page?: PageEnvelope }

export async function fetchADKSettingsSnapshot(): Promise<{
  providers: ADKProvider[];
  agents: ADKAgent[];
  tools: ADKToolDescriptor[];
  skills: ADKSkill[];
  optimizationTasks: ADKOptimizationTask[];
  metrics: ADKMetricsResponse;
}> {
  const [providers, agents, tools, skills, optimizationTasks, metrics] = await Promise.all([
    fetchEnvelope<ProvidersResponse>("/api/v1/adk/providers"),
    fetchEnvelope<AgentsResponse>("/api/v1/adk/agents"),
    fetchEnvelope<ToolsResponse>("/api/v1/adk/tools"),
    fetchEnvelope<SkillsResponse>("/api/v1/adk/skills"),
    fetchEnvelope<OptimizationTasksResponse>("/api/v1/adk/optimization-tasks?limit=20"),
    fetchEnvelope<ADKMetricsResponse>("/api/v1/adk/metrics"),
  ]);

  return {
    providers: providers.providers,
    agents: agents.agents,
    tools: tools.tools,
    skills: skills.skills,
    optimizationTasks: optimizationTasks.tasks,
    metrics,
  };
}

export async function fetchADKRunsPage(page: PageEnvelope, runStatusFilter: string): Promise<RunsResponse> {
  return fetchEnvelope<RunsResponse>(buildRunsURL(page, runStatusFilter));
}

export async function fetchADKApprovalsPage(page: PageEnvelope, approvalStatusFilter: string): Promise<ApprovalsResponse> {
  return fetchEnvelope<ApprovalsResponse>(buildApprovalsURL(page, approvalStatusFilter));
}

export async function fetchADKAuditPage(page: PageEnvelope, auditKindFilter: string): Promise<AuditResponse> {
  return fetchEnvelope<AuditResponse>(buildAuditURL(page, auditKindFilter));
}

export async function fetchADKSkills(): Promise<ADKSkill[]> {
  const response = await fetchEnvelope<SkillsResponse>("/api/v1/adk/skills");
  return response.skills;
}

export async function fetchADKOptimizationTasks(): Promise<ADKOptimizationTask[]> {
  const response = await fetchEnvelope<OptimizationTasksResponse>("/api/v1/adk/optimization-tasks?limit=20");
  return response.tasks;
}

export async function fetchADKMetrics(): Promise<ADKMetricsResponse> {
  return fetchEnvelope<ADKMetricsResponse>("/api/v1/adk/metrics");
}

export async function saveADKProvider(provider: {
  id: string;
  displayName: string;
  baseUrl: string;
  model: string;
  apiKey: string;
  enabled: boolean;
}): Promise<ADKProvider> {
  return fetchEnvelopeWithInit<ADKProvider>("/api/v1/adk/providers", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(provider),
  });
}

export async function testADKProvider(providerId: string): Promise<Record<string, unknown>> {
  return fetchEnvelopeWithInit<Record<string, unknown>>(
    `/api/v1/adk/providers/${encodeURIComponent(providerId)}/test`,
    { method: "POST" },
  );
}

export async function deleteADKProvider(providerId: string): Promise<void> {
  await fetchEnvelopeWithInit(`/api/v1/adk/providers/${encodeURIComponent(providerId)}`, {
    method: "DELETE",
  });
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
  status: string;
}): Promise<ADKAgent> {
  return fetchEnvelopeWithInit<ADKAgent>("/api/v1/adk/agents", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(agent),
  });
}

export async function deleteADKAgent(agentId: string): Promise<void> {
  await fetchEnvelopeWithInit(`/api/v1/adk/agents/${encodeURIComponent(agentId)}`, {
    method: "DELETE",
  });
}

export async function cancelADKRun(runId: string): Promise<ADKRun> {
  return fetchEnvelopeWithInit<ADKRun>(`/api/v1/adk/runs/${encodeURIComponent(runId)}/cancel`, {
    method: "POST",
  });
}

export async function cancelADKOptimizationTask(taskId: string): Promise<ADKOptimizationTask> {
  return fetchEnvelopeWithInit<ADKOptimizationTask>(
    `/api/v1/adk/optimization-tasks/${encodeURIComponent(taskId)}/cancel`,
    { method: "POST" },
  );
}

export async function installADKSkill(url: string): Promise<ADKSkill> {
  return fetchEnvelopeWithInit<ADKSkill>("/api/v1/adk/skills", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ url }),
  });
}

export async function uninstallADKSkill(skillId: string): Promise<void> {
  await fetchEnvelopeWithInit(`/api/v1/adk/skills/${encodeURIComponent(skillId)}`, {
    method: "DELETE",
  });
}

export function fallbackPage(limit: number, offset: number, total: number): PageEnvelope {
  return { limit, offset, total, returned: total, hasMore: false };
}

export function previousPage(page: { offset: number; limit: number }): void {
  page.offset = Math.max(0, page.offset - page.limit);
}

export function nextPage(page: { offset: number; limit: number }, current: PageEnvelope): void {
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

function buildApprovalsURL(page: PageEnvelope, approvalStatusFilter: string): string {
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
