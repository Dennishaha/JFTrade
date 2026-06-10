import { ref } from "vue";

import type {
  ADKAgent,
  ADKPermissionMode,
  ADKProvider,
  ADKSkill,
  ADKToolDescriptor,
} from "@/contracts";

import { deleteADKAgent, saveADKAgent } from "./adkSettingsApi";

function createAgentForm(providers: ADKProvider[], tools: ADKToolDescriptor[], skills: ADKSkill[]) {
  return {
    id: "",
    name: "新 Agent",
    instruction: "",
    providerId: providers[0]?.id ?? "",
    model: "",
    tools: tools.slice(0, 8).map((tool) => tool.name),
    skills: skills.map((skill) => skill.id),
    permissionMode: "approval" as ADKPermissionMode,
    memoryEnabled: true,
    recentUserWindow: 6,
    status: "ENABLED",
  };
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
    name: "投资分析助手",
    instruction:
      "你是 JFTrade 投资分析 agent。优先使用内部行情、账户、策略和回测工具；涉及安装 skill、保存策略、运行优化或改变自动化状态时遵守当前权限模式。",
    providerId: "",
    model: "",
    tools: [] as string[],
    skills: [] as string[],
    permissionMode: "approval" as ADKPermissionMode,
    memoryEnabled: true,
    recentUserWindow: 6,
    status: "ENABLED",
  });

  async function saveAgent(): Promise<void> {
    if (agentForm.value.id !== "" && agentForm.value.status === "DISABLED") {
      const confirmed = window.confirm("确认禁用这个 Agent？禁用后将不能用于新对话。");
      if (!confirmed) return;
    }
    try {
      await saveADKAgent(agentForm.value);
      successMessage.value = "Agent 已保存";
      await refreshAll();
    } catch (error) {
      errorMessage.value = error instanceof Error ? error.message : "保存失败";
    }
  }

  function editAgent(agent: ADKAgent): void {
    agentForm.value = {
      id: agent.id,
      name: agent.name,
      instruction: agent.instruction,
      providerId: agent.providerId,
      model: agent.model,
      tools: [...agent.tools],
      skills: [...agent.skills],
      permissionMode: agent.permissionMode,
      memoryEnabled: agent.memoryEnabled,
      recentUserWindow: agent.recentUserWindow ?? 6,
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
      tools: [...agent.tools],
      skills: [...agent.skills],
      permissionMode: agent.permissionMode,
      memoryEnabled: agent.memoryEnabled,
      recentUserWindow: agent.recentUserWindow ?? 6,
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
    agentForm,
    saveAgent,
    editAgent,
    newAgentForm,
    duplicateAgent,
    deleteAgent,
  };
}
