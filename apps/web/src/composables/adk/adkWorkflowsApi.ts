import type {
  ADKWorkflowDefinition,
  ADKWorkflowDefinitionWriteRequest,
  ADKWorkflowInvocationResult,
  ADKWorkflowTrigger,
  ADKWorkflowTriggerLog,
  ADKWorkflowTriggerSaveResult,
  ADKWorkflowTriggerWriteRequest,
} from "@/types";

import {
  apiDeletePath,
  apiGetPath,
  apiPost,
  apiPostPath,
  apiPutPath,
} from "@/composables/shared/apiClient";
import {
  requireADKPage,
  requireADKWorkflowDefinition,
  requireADKWorkflowDefinitions,
  requireADKWorkflowInvocation,
  requireADKWorkflowTrigger,
  requireADKWorkflowTriggerLogs,
  requireADKWorkflowTriggers,
  requireADKWorkflowTriggerSave,
} from "@/composables/adk/adkApiMappers";

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
  const response = await apiGetPath(
    "/api/v1/adk/workflows",
    `/api/v1/adk/workflows?${params.toString()}`,
  );
  return {
    workflows: requireADKWorkflowDefinitions(response.workflows),
    ...(response.page === undefined
      ? {}
      : { page: requireADKPage(response.page) }),
  };
}

export async function saveADKWorkflow(
  workflow: ADKWorkflowDefinitionWriteRequest,
): Promise<ADKWorkflowDefinition> {
  const id = workflow.id?.trim() ?? "";
  if (id === "") {
    return requireADKWorkflowDefinition(
      await apiPost("/api/v1/adk/workflows", workflow),
    );
  }
  return requireADKWorkflowDefinition(
    await apiPutPath(
      "/api/v1/adk/workflows/{workflowId}",
      `/api/v1/adk/workflows/${encodeURIComponent(id)}`,
      workflow,
    ),
  );
}

export async function deleteADKWorkflow(
  workflowId: string,
): Promise<ADKWorkflowDefinition> {
  const response = await apiDeletePath(
    "/api/v1/adk/workflows/{workflowId}",
    `/api/v1/adk/workflows/${encodeURIComponent(workflowId)}`,
  );
  return requireADKWorkflowDefinition(response.workflow);
}

export async function runADKWorkflow(
  workflowId: string,
  inputs: Record<string, unknown> = {},
): Promise<ADKWorkflowInvocationResult> {
  return requireADKWorkflowInvocation(
    await apiPostPath(
      "/api/v1/adk/workflows/{workflowId}/run",
      `/api/v1/adk/workflows/${encodeURIComponent(workflowId)}/run`,
      { inputs },
    ),
  );
}

export async function fetchADKWorkflowTriggers(
  workflowId: string,
): Promise<ADKWorkflowTrigger[]> {
  const response = await apiGetPath(
    "/api/v1/adk/workflows/{workflowId}/triggers",
    `/api/v1/adk/workflows/${encodeURIComponent(workflowId)}/triggers`,
  );
  return response.triggers === undefined
    ? []
    : requireADKWorkflowTriggers(response.triggers);
}

export async function saveADKWorkflowTrigger(
  workflowId: string,
  trigger: ADKWorkflowTriggerWriteRequest,
): Promise<ADKWorkflowTriggerSaveResult> {
  const id = trigger.id?.trim() ?? "";
  if (id === "") {
    return requireADKWorkflowTriggerSave(
      await apiPostPath(
        "/api/v1/adk/workflows/{workflowId}/triggers",
        `/api/v1/adk/workflows/${encodeURIComponent(workflowId)}/triggers`,
        trigger,
      ),
    );
  }
  return requireADKWorkflowTriggerSave(
    await apiPutPath(
      "/api/v1/adk/workflows/{workflowId}/triggers/{triggerId}",
      `/api/v1/adk/workflows/${encodeURIComponent(workflowId)}/triggers/${encodeURIComponent(id)}`,
      trigger,
    ),
  );
}

export async function deleteADKWorkflowTrigger(
  workflowId: string,
  triggerId: string,
): Promise<ADKWorkflowTrigger> {
  const response = await apiDeletePath(
    "/api/v1/adk/workflows/{workflowId}/triggers/{triggerId}",
    `/api/v1/adk/workflows/${encodeURIComponent(workflowId)}/triggers/${encodeURIComponent(triggerId)}`,
  );
  return requireADKWorkflowTrigger(response.trigger);
}

export async function runADKWorkflowTrigger(
  triggerId: string,
  inputs: Record<string, unknown> = {},
): Promise<ADKWorkflowInvocationResult> {
  return requireADKWorkflowInvocation(
    await apiPostPath(
      "/api/v1/adk/workflow-triggers/{triggerId}/run",
      `/api/v1/adk/workflow-triggers/${encodeURIComponent(triggerId)}/run`,
      { inputs },
    ),
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
  const response = await apiGetPath(
    "/api/v1/adk/workflow-trigger-logs",
    `/api/v1/adk/workflow-trigger-logs?${params.toString()}`,
  );
  return {
    logs: requireADKWorkflowTriggerLogs(response.logs),
    ...(response.page === undefined
      ? {}
      : { page: requireADKPage(response.page) }),
  };
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
