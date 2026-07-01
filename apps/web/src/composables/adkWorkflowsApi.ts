import type {
  ADKWorkflowDefinition,
  ADKWorkflowDefinitionWriteRequest,
  ADKWorkflowInvocationResult,
  ADKWorkflowTrigger,
  ADKWorkflowTriggerLog,
  ADKWorkflowTriggerSaveResult,
  ADKWorkflowTriggerWriteRequest,
} from "@/contracts";

import { fetchEnvelope, fetchEnvelopeWithInit } from "./apiClient";

export interface PageEnvelope {
  limit: number;
  offset: number;
  total: number;
  returned: number;
  hasMore: boolean;
}

interface WorkflowsResponse {
  workflows: ADKWorkflowDefinition[];
  page?: PageEnvelope;
}

interface WorkflowTriggersResponse {
  triggers: ADKWorkflowTrigger[];
}

interface WorkflowTriggerLogsResponse {
  logs: ADKWorkflowTriggerLog[];
  page?: PageEnvelope;
}

export async function fetchADKWorkflows(
  page: PageEnvelope,
  statusFilter = "",
): Promise<WorkflowsResponse> {
  const params = new URLSearchParams({
    limit: String(page.limit),
    offset: String(page.offset),
  });
  if (statusFilter.trim() !== "") {
    params.set("status", statusFilter.trim());
  }
  return fetchEnvelope<WorkflowsResponse>(
    `/api/v1/adk/workflows?${params.toString()}`,
  );
}

export async function saveADKWorkflow(
  workflow: ADKWorkflowDefinitionWriteRequest,
): Promise<ADKWorkflowDefinition> {
  const id = workflow.id?.trim() ?? "";
  const path =
    id === ""
      ? "/api/v1/adk/workflows"
      : `/api/v1/adk/workflows/${encodeURIComponent(id)}`;
  return fetchEnvelopeWithInit<ADKWorkflowDefinition>(path, {
    method: id === "" ? "POST" : "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(workflow),
  });
}

export async function deleteADKWorkflow(
  workflowId: string,
): Promise<ADKWorkflowDefinition> {
  const response = await fetchEnvelopeWithInit<{
    deleted: boolean;
    workflow: ADKWorkflowDefinition;
  }>(`/api/v1/adk/workflows/${encodeURIComponent(workflowId)}`, {
    method: "DELETE",
  });
  return response.workflow;
}

export async function runADKWorkflow(
  workflowId: string,
  inputs: Record<string, unknown> = {},
): Promise<ADKWorkflowInvocationResult> {
  return fetchEnvelopeWithInit<ADKWorkflowInvocationResult>(
    `/api/v1/adk/workflows/${encodeURIComponent(workflowId)}/run`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ inputs }),
    },
  );
}

export async function fetchADKWorkflowTriggers(
  workflowId: string,
): Promise<ADKWorkflowTrigger[]> {
  const response = await fetchEnvelope<WorkflowTriggersResponse>(
    `/api/v1/adk/workflows/${encodeURIComponent(workflowId)}/triggers`,
  );
  return response.triggers ?? [];
}

export async function saveADKWorkflowTrigger(
  workflowId: string,
  trigger: ADKWorkflowTriggerWriteRequest,
): Promise<ADKWorkflowTriggerSaveResult> {
  const id = trigger.id?.trim() ?? "";
  const path =
    id === ""
      ? `/api/v1/adk/workflows/${encodeURIComponent(workflowId)}/triggers`
      : `/api/v1/adk/workflows/${encodeURIComponent(workflowId)}/triggers/${encodeURIComponent(id)}`;
  return fetchEnvelopeWithInit<ADKWorkflowTriggerSaveResult>(path, {
    method: id === "" ? "POST" : "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(trigger),
  });
}

export async function deleteADKWorkflowTrigger(
  workflowId: string,
  triggerId: string,
): Promise<ADKWorkflowTrigger> {
  const response = await fetchEnvelopeWithInit<{
    deleted: boolean;
    trigger: ADKWorkflowTrigger;
  }>(
    `/api/v1/adk/workflows/${encodeURIComponent(workflowId)}/triggers/${encodeURIComponent(triggerId)}`,
    { method: "DELETE" },
  );
  return response.trigger;
}

export async function runADKWorkflowTrigger(
  triggerId: string,
  inputs: Record<string, unknown> = {},
): Promise<ADKWorkflowInvocationResult> {
  return fetchEnvelopeWithInit<ADKWorkflowInvocationResult>(
    `/api/v1/adk/workflow-triggers/${encodeURIComponent(triggerId)}/run`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ inputs }),
    },
  );
}

export async function fetchADKWorkflowTriggerLogs(
  page: PageEnvelope,
  filters: { workflowId?: string; triggerId?: string; status?: string } = {},
): Promise<WorkflowTriggerLogsResponse> {
  const params = new URLSearchParams({
    limit: String(page.limit),
    offset: String(page.offset),
  });
  if (filters.workflowId) params.set("workflowId", filters.workflowId);
  if (filters.triggerId) params.set("triggerId", filters.triggerId);
  if (filters.status) params.set("status", filters.status);
  return fetchEnvelope<WorkflowTriggerLogsResponse>(
    `/api/v1/adk/workflow-trigger-logs?${params.toString()}`,
  );
}

export function fallbackPage(
  limit: number,
  offset: number,
  total: number,
): PageEnvelope {
  return { limit, offset, total, returned: total, hasMore: false };
}

export function pageSummary(page: PageEnvelope): string {
  if (page.total === 0) return "0 / 0";
  return `${page.offset + 1}-${page.offset + page.returned} / ${page.total}`;
}
