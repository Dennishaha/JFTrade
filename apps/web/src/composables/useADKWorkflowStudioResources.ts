import { ref } from "vue";

import type {
  ADKWorkflowDefinition,
  ADKWorkflowTrigger,
  ADKWorkflowTriggerLog,
} from "@/contracts";
import { triggerNodeId } from "@/features/adkWorkflowCanvasGraph";
import {
  fallbackPage,
  fetchADKWorkflowTriggerLogs,
  fetchADKWorkflowTriggers,
  fetchADKWorkflows,
  type PageEnvelope,
} from "./adkWorkflowsApi";

export interface ADKWorkflowStudioResourcesApi {
  fetchWorkflows: typeof fetchADKWorkflows;
  fetchTriggers: typeof fetchADKWorkflowTriggers;
  fetchLogs: typeof fetchADKWorkflowTriggerLogs;
}

export function useADKWorkflowStudioResources(options: {
  api?: ADKWorkflowStudioResourcesApi;
  onError?: (message: string) => void;
  onEmptyWorkflows?: () => void;
  onSelectedTrigger?: (trigger: ADKWorkflowTrigger) => void;
  getSelectedNodeId?: () => string;
  onRefreshNodeData?: () => void;
} = {}) {
  const api = options.api ?? {
    fetchWorkflows: fetchADKWorkflows,
    fetchTriggers: fetchADKWorkflowTriggers,
    fetchLogs: fetchADKWorkflowTriggerLogs,
  };

  const loading = ref(false);
  const triggerLoading = ref(false);
  const logLoading = ref(false);
  const workflows = ref<ADKWorkflowDefinition[]>([]);
  const triggers = ref<ADKWorkflowTrigger[]>([]);
  const logs = ref<ADKWorkflowTriggerLog[]>([]);
  const selectedWorkflowId = ref("");
  const selectedLogId = ref("");
  const workflowStatusFilter = ref("");
  const logStatusFilter = ref("");
  const logTriggerFilter = ref("");
  const workflowPage = ref<PageEnvelope>(createWorkflowStudioPage());
  const logPage = ref<PageEnvelope>(createWorkflowStudioPage());

  async function refreshWorkflows(): Promise<void> {
    loading.value = true;
    options.onError?.("");
    try {
      const response = await api.fetchWorkflows(workflowPage.value, workflowStatusFilter.value);
      workflows.value = response.workflows ?? [];
      workflowPage.value =
        response.page ??
        fallbackPage(workflowPage.value.limit, workflowPage.value.offset, workflows.value.length);
      if (
        selectedWorkflowId.value === "" ||
        !workflows.value.some((workflow) => workflow.id === selectedWorkflowId.value)
      ) {
        selectedWorkflowId.value = workflows.value[0]?.id ?? "";
        if (selectedWorkflowId.value === "") {
          options.onEmptyWorkflows?.();
        }
      }
    } catch (error) {
      options.onError?.(error instanceof Error ? error.message : "加载工作流失败");
    } finally {
      loading.value = false;
    }
  }

  async function refreshTriggers(workflowId = selectedWorkflowId.value): Promise<void> {
    if (workflowId === "") {
      triggers.value = [];
      return;
    }
    triggerLoading.value = true;
    try {
      const next = await api.fetchTriggers(workflowId);
      const others = triggers.value.filter((trigger) => trigger.workflowId !== workflowId);
      triggers.value = [...others, ...next];
      const selectedNodeId = options.getSelectedNodeId?.() ?? "";
      if (selectedNodeId.startsWith("trigger:")) {
        const current = next.find((trigger) => triggerNodeId(trigger.id) === selectedNodeId);
        if (current) {
          options.onSelectedTrigger?.(current);
        }
      }
      options.onRefreshNodeData?.();
    } catch (error) {
      options.onError?.(error instanceof Error ? error.message : "加载触发器失败");
    } finally {
      triggerLoading.value = false;
    }
  }

  async function refreshLogs(): Promise<void> {
    logLoading.value = true;
    try {
      const filters: { workflowId?: string; triggerId?: string; status?: string } = {};
      if (selectedWorkflowId.value) filters.workflowId = selectedWorkflowId.value;
      if (logTriggerFilter.value) filters.triggerId = logTriggerFilter.value;
      if (logStatusFilter.value) filters.status = logStatusFilter.value;
      const response = await api.fetchLogs(logPage.value, filters);
      logs.value = response.logs ?? [];
      if (logs.value.length > 0 && !logs.value.some((log) => log.id === selectedLogId.value)) {
        selectedLogId.value = logs.value[0]!.id;
      }
      logPage.value =
        response.page ?? fallbackPage(logPage.value.limit, logPage.value.offset, logs.value.length);
      options.onRefreshNodeData?.();
    } catch (error) {
      options.onError?.(error instanceof Error ? error.message : "加载触发日志失败");
    } finally {
      logLoading.value = false;
    }
  }

  function upsertWorkflow(workflow: ADKWorkflowDefinition): void {
    const index = workflows.value.findIndex((item) => item.id === workflow.id);
    if (index >= 0) {
      workflows.value.splice(index, 1, workflow);
    } else {
      workflows.value.unshift(workflow);
    }
  }

  function removeWorkflow(workflowId: string): void {
    workflows.value = workflows.value.filter((workflow) => workflow.id !== workflowId);
    selectedWorkflowId.value = workflows.value[0]?.id ?? "";
    if (selectedWorkflowId.value === "") options.onEmptyWorkflows?.();
  }

  function removeTrigger(triggerId: string): void {
    triggers.value = triggers.value.filter((trigger) => trigger.id !== triggerId);
  }

  function previousWorkflowPage(): void {
    workflowPage.value.offset = Math.max(0, workflowPage.value.offset - workflowPage.value.limit);
    void refreshWorkflows();
  }

  function nextWorkflowPage(): void {
    if (!workflowPage.value.hasMore) return;
    workflowPage.value.offset += workflowPage.value.limit;
    void refreshWorkflows();
  }

  function previousLogPage(): void {
    logPage.value.offset = Math.max(0, logPage.value.offset - logPage.value.limit);
    void refreshLogs();
  }

  function nextLogPage(): void {
    if (!logPage.value.hasMore) return;
    logPage.value.offset += logPage.value.limit;
    void refreshLogs();
  }

  return {
    loading,
    triggerLoading,
    logLoading,
    workflows,
    triggers,
    logs,
    selectedWorkflowId,
    selectedLogId,
    workflowStatusFilter,
    logStatusFilter,
    logTriggerFilter,
    workflowPage,
    logPage,
    refreshWorkflows,
    refreshTriggers,
    refreshLogs,
    upsertWorkflow,
    removeWorkflow,
    removeTrigger,
    previousWorkflowPage,
    nextWorkflowPage,
    previousLogPage,
    nextLogPage,
  };
}

export function createWorkflowStudioPage(): PageEnvelope {
  return {
    limit: 20,
    offset: 0,
    total: 0,
    returned: 0,
    hasMore: false,
  };
}
