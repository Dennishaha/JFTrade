<script setup lang="ts">
import type { ADKAgent, ADKPermissionMode, ADKProvider, ADKSkill, ADKToolDescriptor } from "@jftrade/ui-contracts";

defineProps<{
  agentForm: {
    id: string;
    name: string;
    instruction: string;
    providerId: string;
    model: string;
    tools: string[];
    skills: string[];
    permissionMode: ADKPermissionMode;
    memoryEnabled: boolean;
    status: string;
  };
  agents: ADKAgent[];
  providerOptions: Array<{ title: string; value: string }>;
  toolOptions: Array<{ title: string; value: string }>;
  skillOptions: Array<{ title: string; value: string }>;
  permissionModes: Array<{ title: string; value: ADKPermissionMode }>;
  tools: ADKToolDescriptor[];
  formatPermission: (mode: string) => string;
  riskColor: (risk?: string) => string;
  riskLabel: (risk?: string) => string;
  saveAgent: () => void | Promise<void>;
  newAgentForm: () => void;
  editAgent: (agent: ADKAgent) => void;
  duplicateAgent: (agent: ADKAgent) => void;
  deleteAgent: (agentId: string) => void | Promise<void>;
}>();
</script>

<template>
  <section class="grid gap-5 lg:grid-cols-[1fr_1.4fr]">
    <v-card flat class="card-shell border-0">
      <v-card-title class="flex items-center justify-between gap-2">
        <span>{{ agentForm.id ? "编辑 Agent" : "新建 Agent" }}</span>
        <v-btn v-if="agentForm.id" size="x-small" variant="text" @click="newAgentForm">
          新建
        </v-btn>
      </v-card-title>
      <v-card-text class="grid gap-3">
        <v-text-field v-model="agentForm.name" label="名称" density="comfortable" />
        <v-select
          v-model="agentForm.providerId"
          :items="providerOptions"
          label="Provider"
          density="comfortable"
          clearable
        />
        <v-text-field v-model="agentForm.model" label="覆盖模型（可选）" density="comfortable" />
        <v-select
          v-model="agentForm.permissionMode"
          :items="permissionModes"
          label="权限模式"
          density="comfortable"
        />
        <v-select
          v-model="agentForm.tools"
          :items="toolOptions"
          label="启用 Tools"
          density="comfortable"
          multiple
          chips
          closable-chips
        />
        <div class="grid gap-2 rounded border border-slate-200 p-3">
          <div class="text-xs font-medium text-slate-600">已接入运行时工具</div>
          <div class="flex flex-wrap gap-1">
            <v-chip
              v-for="tool in tools"
              :key="tool.name"
              size="x-small"
              variant="tonal"
              :color="riskColor(tool.riskLevel)"
              :title="`${tool.description} ${tool.outputSummary ?? ''}`"
            >
              {{ tool.name }} · {{ riskLabel(tool.riskLevel) }}
            </v-chip>
          </div>
        </div>
        <v-select
          v-model="agentForm.skills"
          :items="skillOptions"
          label="启用 Skills"
          density="comfortable"
          multiple
          chips
          closable-chips
        />
        <v-textarea
          v-model="agentForm.instruction"
          label="Instruction"
          :rows="4"
          density="comfortable"
        />
        <div class="flex gap-6">
          <v-switch v-model="agentForm.memoryEnabled" label="记忆" color="primary" hide-details />
          <v-switch
            v-model="agentForm.status"
            true-value="ENABLED"
            false-value="DISABLED"
            label="启用"
            color="primary"
            hide-details
          />
        </div>
        <v-btn color="primary" block @click="saveAgent">保存 Agent</v-btn>
      </v-card-text>
    </v-card>

    <div class="grid auto-rows-max gap-3">
      <v-card
        v-for="agent in agents"
        :key="agent.id"
        flat
        class="card-shell border-0"
      >
        <v-card-text>
          <div class="flex items-start justify-between gap-2">
            <div class="min-w-0 flex-1">
              <div class="flex flex-wrap items-center gap-2">
                <span class="font-semibold text-slate-900">{{ agent.name }}</span>
                <v-chip
                  size="x-small"
                  variant="tonal"
                  :color="agent.status === 'ENABLED' ? 'success' : 'default'"
                >
                  {{ agent.status }}
                </v-chip>
              </div>
              <div class="text-xs text-slate-500">{{ formatPermission(agent.permissionMode) }}</div>
              <div class="mt-1 flex flex-wrap gap-1">
                <v-chip v-for="t in agent.tools.slice(0, 5)" :key="t" size="x-small" variant="outlined">
                  {{ t }}
                </v-chip>
                <span v-if="agent.tools.length > 5" class="text-xs text-slate-500">
                  +{{ agent.tools.length - 5 }}
                </span>
              </div>
            </div>
            <div class="flex shrink-0 flex-col gap-1">
              <v-btn size="x-small" variant="outlined" @click="editAgent(agent)">编辑</v-btn>
              <v-btn size="x-small" variant="outlined" @click="duplicateAgent(agent)">复制</v-btn>
              <v-btn size="x-small" variant="outlined" color="error" @click="deleteAgent(agent.id)">删除</v-btn>
            </div>
          </div>
        </v-card-text>
      </v-card>
      <div v-if="agents.length === 0" class="text-sm text-slate-500">
        尚未创建任何 Agent。
      </div>
    </div>
  </section>
</template>
