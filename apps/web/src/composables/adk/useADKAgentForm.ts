import { ref } from "vue";

import type {
  ADKAgent,
  ADKPermissionMode,
  ADKProvider,
  ADKReasoningEffort,
  ADKSkill,
  ADKToolDescriptor,
  ADKWorkMode,
} from "@/types";

import { deleteADKAgent, saveADKAgent } from "@/composables/adk/adkSettingsApi";
import { useActionConfirmation } from "@/composables/shared/useActionConfirmation";

function createAgentForm(_providers: ADKProvider[], _tools: ADKToolDescriptor[], skills: ADKSkill[]) {
  return {
    id: "",
    name: "新 Agent",
    instruction: "",
    providerId: "",
    model: "",
    reasoningEffort: "" as ADKReasoningEffort | "",
    tools: [],
    skills: skills.map((skill) => skill.id),
    permissionMode: "approval" as ADKPermissionMode,
    memoryEnabled: true,
    recentUserWindow: 6,
    workMode: "chat" as ADKWorkMode,
    loopMaxIterations: 5,
    status: "ENABLED",
  };
}

function selectableAgentWorkMode(mode?: string): ADKWorkMode {
  if (mode === "loop") return mode;
  return "chat";
}

export function useADKAgentForm(
  providers: { value: ADKProvider[] },
  tools: { value: ADKToolDescriptor[] },
  skills: { value: ADKSkill[] },
  refreshAll: () => Promise<void>,
  successMessage: { value: string },
  errorMessage: { value: string },
) {
  const agentForm = ref({
    id: "",
    name: "新 Agent",
    instruction: "",
    providerId: "",
    model: "",
    reasoningEffort: "" as ADKReasoningEffort | "",
    tools: [] as string[],
    skills: [] as string[],
    permissionMode: "approval" as ADKPermissionMode,
    memoryEnabled: true,
    recentUserWindow: 6,
    workMode: "chat" as ADKWorkMode,
    loopMaxIterations: 5,
    status: "ENABLED",
  });

  const actionConfirmation = useActionConfirmation();

  async function persistAgent(): Promise<void> {
    try {
      await saveADKAgent(agentForm.value);
      successMessage.value = "Agent 已保存";
      await refreshAll();
    } catch (error) {
      errorMessage.value = error instanceof Error ? error.message : "保存失败";
    }
  }

  function saveAgent(): Promise<void> {
    if (agentForm.value.id !== "" && agentForm.value.status === "DISABLED") {
      // 确认由宿主组件渲染 ActionConfirmDialog 完成；保存编辑弹窗先行关闭，
      // 与原生 window.confirm 取消后同样退出编辑态的终态一致。
      void actionConfirmation
        .requestConfirmation({
          title: "禁用 Agent",
          message: "确认禁用这个 Agent？禁用后将不能用于新对话。",
          confirmLabel: "禁用",
        })
        .then((confirmed) => {
          if (confirmed !== null) void persistAgent();
        });
      return Promise.resolve();
    }
    return persistAgent();
  }

  function editAgent(agent: ADKAgent): void {
    agentForm.value = {
      id: agent.id,
      name: agent.name,
      instruction: agent.instruction,
      providerId: agent.providerId,
      model: agent.model,
      reasoningEffort: agent.reasoningEffort ?? "",
      tools: [...agent.tools],
      skills: [...agent.skills],
      permissionMode: agent.permissionMode,
      memoryEnabled: agent.memoryEnabled,
      recentUserWindow: agent.recentUserWindow ?? 6,
      workMode: selectableAgentWorkMode(agent.workMode),
      loopMaxIterations: agent.loopMaxIterations ?? 5,
      status: agent.status,
    };
  }

  function newAgentForm(): void {
    agentForm.value = createAgentForm(providers.value, tools.value, skills.value);
  }

  function duplicateAgent(agent: ADKAgent): void {
    agentForm.value = {
      id: "",
      name: `${agent.name} Copy`,
      instruction: agent.instruction,
      providerId: agent.providerId,
      model: agent.model,
      reasoningEffort: agent.reasoningEffort ?? "",
      tools: [...agent.tools],
      skills: [...agent.skills],
      permissionMode: agent.permissionMode,
      memoryEnabled: agent.memoryEnabled,
      recentUserWindow: agent.recentUserWindow ?? 6,
      workMode: selectableAgentWorkMode(agent.workMode),
      loopMaxIterations: agent.loopMaxIterations ?? 5,
      status: "ENABLED",
    };
  }

  async function deleteAgent(agentId: string): Promise<void> {
    try {
      await deleteADKAgent(agentId);
      await refreshAll();
    } catch (error) {
      errorMessage.value = error instanceof Error ? error.message : "删除失败";
    }
  }

  return {
    actionConfirmation,
    agentForm,
    saveAgent,
    editAgent,
    newAgentForm,
    duplicateAgent,
    deleteAgent,
  };
}
