<script setup lang="ts">
import { computed } from "vue";

import type { ADKToolDescriptor } from "@/contracts";
import { useOptionalTheme } from "@/composables/useTheme";

defineProps<{
  tools: ADKToolDescriptor[];
  filteredTools: ADKToolDescriptor[];
  selectedTool: ADKToolDescriptor | null;
  toolCategoryFilter: string;
  toolCategoryOptions: Array<string | undefined>;
  toolRiskFilter: string;
  toolRiskOptions: Array<string | undefined>;
  toolSearchQuery: string;
  toolDetailDialogOpen: boolean;
  preview: (value: unknown) => string;
  formatPermissionMode: (mode: string) => string;
  riskColor: (risk?: string) => string;
  riskLabel: (risk?: string) => string;
  openToolDetail: (toolName: string) => void;
  closeToolDetail: () => void;
}>();

defineEmits<{
  "update:toolCategoryFilter": [value: string];
  "update:toolRiskFilter": [value: string];
  "update:toolSearchQuery": [value: string];
  "update:toolDetailDialogOpen": [value: boolean];
}>();

const appTheme = useOptionalTheme();
const appThemeMode = computed(() => appTheme?.theme.value ?? "dark");

function outputSummaryText(tool: ADKToolDescriptor | null): string {
  const text = tool?.outputSummary?.trim() ?? "";
  return text === "" ? "未提供" : text;
}

function requiresApprovalText(tool: ADKToolDescriptor | null): string[] {
  if (!tool || (tool.requiresApprovalIn?.length ?? 0) === 0) {
    return ["无额外审批模式限制"];
  }
  return tool.requiresApprovalIn;
}

const toolPermissionLabels: Record<string, string> = {
  read: "读取",
  read_internal: "内部读取",
  read_external: "外部读取",
  write_external: "外部写入",
  write_strategy: "写入策略",
  optimize_strategy: "优化策略",
  create_strategy_instance: "创建策略实例",
  install_skill: "安装技能",
  execute_workflow: "执行工作流",
  workflow_internal: "工作流内部操作",
  write_workflow: "写入工作流",
  write_memory: "写入记忆",
  write_task: "写入任务",
  live_trading: "实盘交易",
};

function toolPermissionLabel(permission?: string): string {
  return toolPermissionLabels[permission?.trim().toLowerCase() ?? ""] ?? "未知权限";
}

function toolPermissionColor(permission?: string): string {
  const value = permission?.trim().toLowerCase() ?? "";
  if (value === "live_trading") return "error";
  if (value.startsWith("write_") || value === "install_skill") return "warning";
  if (value === "optimize_strategy" || value === "create_strategy_instance") return "warning";
  if (value.startsWith("read")) return "info";
  return "default";
}

</script>

<template>
  <v-theme-provider :theme="appThemeMode">
    <section class="grid gap-4">
      <v-card>
        <v-card-title class="flex flex-wrap items-center justify-between gap-3">
          <v-chip size="small" variant="tonal">
            {{ filteredTools.length }}/{{ tools.length }} 个工具
          </v-chip>
        </v-card-title>
        <v-card-subtitle class="text-caption text-medium-emphasis">
          浏览项目内置 ADK 工具名称、用途与调用定义。
        </v-card-subtitle>
        <v-card-text>
          <v-form class="grid gap-3 md:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)_minmax(0,1fr)]" aria-label="工具目录筛选"
            @submit.prevent>
            <v-text-field :model-value="toolSearchQuery" label="搜索工具名称或描述" density="comfortable"
              prepend-inner-icon="mdi-magnify" clearable hide-details
              @update:model-value="$emit('update:toolSearchQuery', $event ?? '')" />
            <v-select :model-value="toolCategoryFilter" label="按工具类别过滤" density="comfortable" clearable hide-details
              :items="toolCategoryOptions" @update:model-value="$emit('update:toolCategoryFilter', $event ?? '')" />
            <v-select :model-value="toolRiskFilter" label="按风险等级过滤" density="comfortable" clearable hide-details
              :items="toolRiskOptions" @update:model-value="$emit('update:toolRiskFilter', $event ?? '')" />
          </v-form>
        </v-card-text>
      </v-card>

      <v-card>
        <v-table class="adk-tools-table" density="compact" hover>
          <colgroup>
            <col class="adk-tools-table__tool-column" />
            <col class="adk-tools-table__category-column" />
            <col class="adk-tools-table__permission-column" />
            <col class="adk-tools-table__risk-column" />
            <col class="adk-tools-table__requirement-column" />
            <col class="adk-tools-table__action-column" />
          </colgroup>
          <thead>
            <tr>
              <th scope="col">工具</th>
              <th scope="col" class="text-center">分类</th>
              <th scope="col" class="text-center">权限</th>
              <th scope="col" class="text-center">风险</th>
              <th scope="col" class="text-center">调用要求</th>
              <th scope="col" class="text-center">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="tool in filteredTools" :key="tool.name" class="adk-tool-row" @click="openToolDetail(tool.name)">
              <td>
                <div class="flex min-w-0 items-center gap-2">
                  <span class="min-w-0 truncate font-weight-medium">{{ tool.name }}</span>
                  <span class="min-w-0 truncate text-caption text-medium-emphasis">
                    {{ tool.displayName || "未命名" }}
                  </span>
                </div>
                <div class="mt-0.5 max-w-xl truncate text-caption text-medium-emphasis">
                  {{ tool.description }}
                </div>
              </td>
              <td>
                <div class="flex justify-center">
                  <v-chip size="x-small" variant="tonal">
                    {{ tool.category || "未分类" }}
                  </v-chip>
                </div>
              </td>
              <td>
                <div class="flex justify-center">
                  <v-chip size="x-small" variant="tonal" :color="toolPermissionColor(tool.permission)">
                    {{ toolPermissionLabel(tool.permission) }}
                  </v-chip>
                </div>
              </td>
              <td>
                <div class="flex justify-center">
                  <v-chip size="x-small" variant="tonal" :color="riskColor(tool.riskLevel)">
                    {{ riskLabel(tool.riskLevel) }}
                  </v-chip>
                </div>
              </td>
              <td>
                <div class="flex justify-center">
                  <v-chip v-if="tool.requiredSkill" size="x-small" variant="tonal" color="info">
                    需加载 Skill
                  </v-chip>
                  <span v-else class="text-body-2 text-medium-emphasis">无</span>
                </div>
              </td>
              <td class="text-right">
                <div class="flex justify-center">
                  <v-btn size="small" variant="text" color="primary" :aria-label="`查看工具 ${tool.name} 的定义`"
                    @click.stop="openToolDetail(tool.name)">
                    查看定义
                  </v-btn>
                </div>
              </td>
            </tr>
            <tr v-if="filteredTools.length === 0">
              <td colspan="6" class="py-8 text-center text-body-2 text-medium-emphasis">
                当前筛选条件下没有匹配的工具。
              </td>
            </tr>
          </tbody>
        </v-table>
      </v-card>

      <v-dialog :model-value="toolDetailDialogOpen" max-width="820" scrollable
        @update:model-value="$emit('update:toolDetailDialogOpen', $event)">
        <v-card>
          <v-card-title class="flex items-center justify-between gap-2 px-4 py-2 text-subtitle-1">
            <span>{{ selectedTool?.name ?? "工具定义" }}</span>
            <v-btn icon="mdi-close" variant="text" size="small" @click="closeToolDetail" />
          </v-card-title>
          <v-card-text class="grid gap-2 px-4 pb-2 pt-1">
            <div class="grid gap-2 md:grid-cols-2">
              <div class="rounded border bg-surface p-2 text-high-emphasis">
                <div class="text-caption text-medium-emphasis">名称</div>
                <div class="mt-1 text-body-2">{{ selectedTool?.name ?? "-" }}</div>
              </div>
              <div class="rounded border bg-surface p-2 text-high-emphasis">
                <div class="text-caption text-medium-emphasis">显示名称</div>
                <div class="mt-1 text-body-2">{{ selectedTool?.displayName ?? "-" }}</div>
              </div>
              <div class="rounded border bg-surface p-2 text-high-emphasis">
                <div class="text-caption text-medium-emphasis">分类</div>
                <div class="mt-1 text-body-2">{{ selectedTool?.category ?? "-" }}</div>
              </div>
              <div class="rounded border bg-surface p-2 text-high-emphasis">
                <div class="text-caption text-medium-emphasis">权限</div>
                <div class="mt-1">
                  <v-chip size="x-small" variant="tonal" :color="toolPermissionColor(selectedTool?.permission)">
                    {{ toolPermissionLabel(selectedTool?.permission) }}
                  </v-chip>
                </div>
              </div>
            </div>

            <div class="grid gap-2 md:grid-cols-2">
              <div class="rounded border bg-surface p-2 text-high-emphasis">
                <div class="text-caption text-medium-emphasis">描述</div>
                <div class="mt-1 text-body-2">{{ selectedTool?.description ?? "-" }}</div>
              </div>
              <div class="rounded border bg-surface p-2 text-high-emphasis">
                <div class="text-caption text-medium-emphasis">输出摘要</div>
                <div class="mt-1 text-body-2">{{ outputSummaryText(selectedTool) }}</div>
              </div>
            </div>

            <div v-if="selectedTool?.requiredSkill" class="rounded border bg-surface p-2 text-high-emphasis">
              <div class="text-caption text-medium-emphasis">调用前置 Skill</div>
              <div class="mt-1 text-body-2">
                当前 invocation 必须先加载
                {{ selectedTool.requiredSkill }}；下一条用户消息需要重新加载。
              </div>
            </div>

            <div class="grid gap-2 md:grid-cols-3">
              <div class="rounded border bg-surface p-2 text-high-emphasis">
                <div class="text-caption text-medium-emphasis">风险等级</div>
                <div class="mt-1">
                  <v-chip size="x-small" variant="tonal" :color="riskColor(selectedTool?.riskLevel)">
                    {{ riskLabel(selectedTool?.riskLevel) }}
                  </v-chip>
                </div>
              </div>
              <div class="rounded border bg-surface p-2 text-high-emphasis">
                <div class="text-caption text-medium-emphasis">允许模式</div>
                <div class="mt-1 flex flex-wrap gap-1">
                  <v-chip v-for="mode in selectedTool?.allowedModes ?? []" :key="mode" size="x-small" variant="tonal">
                    {{ formatPermissionMode(mode) }}
                  </v-chip>
                </div>
              </div>
              <div class="rounded border bg-surface p-2 text-high-emphasis">
                <div class="text-caption text-medium-emphasis">审批限制</div>
                <div class="mt-1 flex flex-wrap gap-1">
                  <v-chip v-for="mode in requiresApprovalText(selectedTool)" :key="mode" size="x-small" variant="tonal">
                    {{ mode === "无额外审批模式限制" ? mode : formatPermissionMode(mode) }}
                  </v-chip>
                </div>
              </div>
            </div>

            <div class="rounded border bg-surface p-2 text-high-emphasis">
              <div class="text-caption text-medium-emphasis">输入定义</div>
              <pre
                class="mt-1 max-h-64 overflow-auto rounded border bg-surface-variant p-2 text-caption text-high-emphasis">{{ preview(selectedTool?.inputSchema ?? {}) }}</pre>
            </div>
          </v-card-text>
          <v-card-actions class="justify-end px-4 py-1">
            <v-btn size="small" variant="text" @click="closeToolDetail">关闭</v-btn>
          </v-card-actions>
        </v-card>
      </v-dialog>
    </section>
  </v-theme-provider>
</template>

<style scoped>
.adk-tools-table :deep(table) {
  width: 100%;
  table-layout: fixed;
}

.adk-tools-table__tool-column {
  width: 38%;
}

.adk-tools-table__category-column,
.adk-tools-table__permission-column,
.adk-tools-table__requirement-column {
  width: 13%;
}

.adk-tools-table__risk-column {
  width: 10%;
}

.adk-tools-table__action-column {
  width: 13%;
}
</style>
