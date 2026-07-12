<script setup lang="ts">
import type { ADKToolDescriptor } from "@/contracts";

const props = defineProps<{
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

function handleToolCardKeydown(event: KeyboardEvent, toolName: string): void {
  if (event.key !== "Enter" && event.key !== " ") {
    return;
  }
  event.preventDefault();
  props.openToolDetail(toolName);
}
</script>

<template>
  <section class="grid gap-4">
    <v-card flat class="card-shell border-0">
      <v-card-title class="flex flex-wrap items-center justify-between gap-3">
        <div>
          <div class="text-base font-semibold text-slate-900">工具目录</div>
          <div class="mt-1 text-xs text-slate-500">
            浏览项目内置 ADK 工具名称、用途与调用定义。
          </div>
        </div>
        <v-chip size="small" variant="tonal">
          {{ filteredTools.length }}/{{ tools.length }} 个工具
        </v-chip>
      </v-card-title>
      <v-card-text class="grid gap-3">
        <div class="grid gap-3 md:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)_minmax(0,1fr)]">
          <v-text-field
            :model-value="toolSearchQuery"
            label="搜索工具名称或描述"
            density="comfortable"
            clearable
            @update:model-value="$emit('update:toolSearchQuery', $event ?? '')"
          />
          <v-select
            :model-value="toolCategoryFilter"
            label="按工具类别过滤"
            density="comfortable"
            clearable
            :items="toolCategoryOptions"
            @update:model-value="$emit('update:toolCategoryFilter', $event ?? '')"
          />
          <v-select
            :model-value="toolRiskFilter"
            label="按风险等级过滤"
            density="comfortable"
            clearable
            :items="toolRiskOptions"
            @update:model-value="$emit('update:toolRiskFilter', $event ?? '')"
          />
        </div>
      </v-card-text>
    </v-card>

    <div class="adk-tools-grid">
      <v-card
        v-for="tool in filteredTools"
        :key="tool.name"
        flat
        class="adk-tool-card card-shell border-0"
        role="button"
        tabindex="0"
        @click="openToolDetail(tool.name)"
        @keydown="handleToolCardKeydown($event, tool.name)"
      >
        <v-card-text>
          <div class="min-w-0">
            <div class="flex flex-wrap items-center gap-2">
              <span class="font-semibold text-slate-900">{{ tool.name }}</span>
              <v-chip size="x-small" variant="tonal">
                {{ tool.displayName || "未命名" }}
              </v-chip>
              <v-chip size="x-small" variant="tonal">
                {{ tool.category || "未分类" }}
              </v-chip>
              <v-chip
                size="x-small"
                variant="tonal"
                :color="riskColor(tool.riskLevel)"
              >
                {{ riskLabel(tool.riskLevel) }}
              </v-chip>
              <v-chip v-if="tool.requiredSkill" size="x-small" variant="tonal" color="info">
                需加载 Skill
              </v-chip>
            </div>
            <div class="mt-1 text-xs text-slate-500">
              权限：{{ tool.permission }}
            </div>
            <div class="mt-2 text-sm text-slate-600">
              {{ tool.description }}
            </div>
          </div>
        </v-card-text>
      </v-card>

      <v-card v-if="filteredTools.length === 0" flat class="card-shell border-0">
        <v-card-text class="text-sm text-slate-500">
          当前筛选条件下没有匹配的工具。
        </v-card-text>
      </v-card>
    </div>

    <v-dialog
      :model-value="toolDetailDialogOpen"
      max-width="900"
      content-class="adk-tool-dialog-overlay"
      scrollable
      @update:model-value="$emit('update:toolDetailDialogOpen', $event)"
    >
      <v-card class="adk-tool-dialog">
        <v-card-title class="flex items-center justify-between gap-3">
          <span>{{ selectedTool?.name ?? "工具定义" }}</span>
          <v-btn icon="mdi-close" variant="text" size="small" @click="closeToolDetail" />
        </v-card-title>
        <v-card-text class="adk-tool-dialog__body grid gap-4">
          <div class="grid gap-3 md:grid-cols-2">
            <div class="adk-tool-detail-card rounded border p-3">
              <div class="adk-tool-detail-label text-xs font-medium">名称</div>
              <div class="adk-tool-detail-value mt-1 text-sm">{{ selectedTool?.name ?? "-" }}</div>
            </div>
            <div class="adk-tool-detail-card rounded border p-3">
              <div class="adk-tool-detail-label text-xs font-medium">显示名称</div>
              <div class="adk-tool-detail-value mt-1 text-sm">{{ selectedTool?.displayName ?? "-" }}</div>
            </div>
            <div class="adk-tool-detail-card rounded border p-3">
              <div class="adk-tool-detail-label text-xs font-medium">分类</div>
              <div class="adk-tool-detail-value mt-1 text-sm">{{ selectedTool?.category ?? "-" }}</div>
            </div>
            <div class="adk-tool-detail-card rounded border p-3">
              <div class="adk-tool-detail-label text-xs font-medium">权限</div>
              <div class="adk-tool-detail-value mt-1 text-sm">{{ selectedTool?.permission ?? "-" }}</div>
            </div>
          </div>

          <div class="adk-tool-detail-card rounded border p-3">
            <div class="adk-tool-detail-label text-xs font-medium">描述</div>
            <div class="adk-tool-detail-muted mt-1 text-sm">{{ selectedTool?.description ?? "-" }}</div>
          </div>

          <div
            v-if="selectedTool?.requiredSkill"
            class="adk-tool-detail-card rounded border p-3"
          >
            <div class="adk-tool-detail-label text-xs font-medium">调用前置 Skill</div>
            <div class="adk-tool-detail-muted mt-1 text-sm">
              当前 invocation 必须先加载
              {{ selectedTool.requiredSkill }}；下一条用户消息需要重新加载。
            </div>
          </div>

          <div class="grid gap-3 md:grid-cols-3">
            <div class="adk-tool-detail-card rounded border p-3">
              <div class="adk-tool-detail-label text-xs font-medium">风险等级</div>
              <div class="mt-2">
                <v-chip size="small" variant="tonal" :color="riskColor(selectedTool?.riskLevel)">
                  {{ riskLabel(selectedTool?.riskLevel) }}
                </v-chip>
              </div>
            </div>
            <div class="adk-tool-detail-card rounded border p-3">
              <div class="adk-tool-detail-label text-xs font-medium">允许模式</div>
              <div class="mt-2 flex flex-wrap gap-1">
                <v-chip
                  v-for="mode in selectedTool?.allowedModes ?? []"
                  :key="mode"
                  size="x-small"
                  variant="tonal"
                >
                  {{ formatPermissionMode(mode) }}
                </v-chip>
              </div>
            </div>
            <div class="adk-tool-detail-card rounded border p-3">
              <div class="adk-tool-detail-label text-xs font-medium">审批限制</div>
              <div class="mt-2 flex flex-wrap gap-1">
                <v-chip
                  v-for="mode in requiresApprovalText(selectedTool)"
                  :key="mode"
                  size="x-small"
                  variant="tonal"
                >
                  {{ mode === "无额外审批模式限制" ? mode : formatPermissionMode(mode) }}
                </v-chip>
              </div>
            </div>
          </div>

          <div class="adk-tool-detail-card rounded border p-3">
            <div class="adk-tool-detail-label text-xs font-medium">输出摘要</div>
            <div class="adk-tool-detail-muted mt-1 text-sm">{{ outputSummaryText(selectedTool) }}</div>
          </div>

          <div class="adk-tool-detail-card rounded border p-3">
            <div class="adk-tool-detail-label text-xs font-medium">输入定义</div>
            <pre class="adk-tool-schema mt-2 overflow-x-auto rounded p-3 text-xs">{{ preview(selectedTool?.inputSchema ?? {}) }}</pre>
          </div>
        </v-card-text>
        <v-card-actions class="justify-end">
          <v-btn variant="text" @click="closeToolDetail">关闭</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </section>
</template>

<style scoped>
.adk-tools-grid {
  display: grid;
  gap: 0.75rem;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 20rem), 1fr));
}

.adk-tool-card {
  cursor: pointer;
  transition: transform 0.16s ease, box-shadow 0.16s ease;
}

.adk-tool-card:hover,
.adk-tool-card:focus-visible {
  transform: translateY(-1px);
  box-shadow: 0 12px 28px rgba(15, 23, 42, 0.08);
}

.adk-tool-card:focus-visible {
  outline: 2px solid rgba(59, 130, 246, 0.35);
  outline-offset: 2px;
}

.adk-tool-dialog {
  display: flex;
  flex-direction: column;
  max-height: 100%;
  overflow: hidden;
  background: rgb(var(--v-theme-surface));
  color: var(--card-text-1);
}

.adk-tool-dialog__body {
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
}

:global(.adk-tool-dialog-overlay) {
  max-height: 80dvh;
}

:global(.adk-tool-dialog-overlay .v-card) {
  max-height: 100%;
}

.adk-tool-detail-card {
  border-color: var(--card-border);
  background: var(--card-surface-raised);
}

.adk-tool-detail-label {
  color: var(--card-text-3);
}

.adk-tool-detail-value {
  color: var(--card-text-1);
}

.adk-tool-detail-muted {
  color: var(--card-text-2);
}

.adk-tool-schema {
  background: color-mix(in srgb, var(--card-surface) 92%, #020617);
  color: var(--card-text-1);
}
</style>
