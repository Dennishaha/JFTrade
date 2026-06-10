<script setup lang="ts">
import { computed, ref, watch } from "vue";

import type { ADKSessionContextSnapshot } from "@/contracts";

import type { QueuedChatMessage } from "../../composables/adkChatRuntime";

interface SlashCommandItem {
  id: "context" | "compact" | "compact-aggressive";
  command: string;
  title: string;
  description: string;
  disabled?: boolean;
}

const props = withDefaults(
  defineProps<{
    variant?: "page" | "dock";
    activeRunId?: string;
    activeRunStatus?: string;
    agentOptions?: { title: string; value: string }[];
    canInterruptChat?: boolean;
    canSendChat: boolean;
    chatDraft: string;
    composerBlockMessage?: string;
    contextBusy?: boolean;
    contextDetailsOpen?: boolean;
    contextSnapshot?: ADKSessionContextSnapshot | null;
    hasBlockingRun?: boolean;
    interruptingRunId?: string;
    loading?: boolean;
    placeholder?: string;
    providerOptions?: { title: string; value: string }[];
    queuedMessages?: QueuedChatMessage[];
    queueDispatchingId?: string;
    revokeQueuedMessage?: (messageId: string) => void | Promise<void>;
    savingProviderSelection?: boolean;
    selectedAgentId?: string;
    selectedProviderId?: string;
    sendingChat: boolean;
    slashCommands?: SlashCommandItem[];
    suggestions?: string[];
    cancelActiveRun?: () => void | Promise<void>;
    handleAgentChange?: () => void;
    handleComposerKeydown?: (event: KeyboardEvent) => void;
    handleProviderChange?: (providerId: string) => void | Promise<void>;
    interruptAndQueueChat?: () => void | Promise<void>;
    openContextDetails?: () => void;
    openProviderSettings?: () => void;
    runSlashCommand?: (command: SlashCommandItem["id"]) => void | Promise<void>;
    sendChat: () => void | Promise<void>;
    applySuggestion?: (value: string) => void | Promise<void>;
  }>(),
  {
    variant: "page",
    activeRunId: "",
    activeRunStatus: "",
    agentOptions: () => [],
    canInterruptChat: false,
    composerBlockMessage: "",
    contextBusy: false,
    contextDetailsOpen: false,
    contextSnapshot: null,
    hasBlockingRun: false,
    interruptingRunId: "",
    loading: false,
    placeholder: "输入问题或任务...",
    providerOptions: () => [],
    queuedMessages: () => [],
    queueDispatchingId: "",
    savingProviderSelection: false,
    selectedAgentId: "",
    selectedProviderId: "",
    slashCommands: () => [],
    suggestions: () => [],
  },
);

const emit = defineEmits<{
  "update:chatDraft": [value: string];
  "update:contextDetailsOpen": [value: boolean];
  "update:selectedAgentId": [value: string];
  "update:selectedProviderId": [value: string];
}>();

const selectedSlashIndex = ref(0);
const dismissedSlashDraft = ref("");

const contextMenuOpen = computed({
  get: () => props.contextDetailsOpen,
  set: (value: boolean) => emit("update:contextDetailsOpen", value),
});

const slashDraft = computed(() => props.chatDraft.trimStart());
const filteredSlashCommands = computed(() => {
  const query = slashDraft.value.startsWith("/")
    ? slashDraft.value.slice(1).toLowerCase()
    : "";
  if (!slashDraft.value.startsWith("/")) return [];
  return props.slashCommands.filter((item) => {
    if (query === "") return true;
    const haystack =
      `${item.command} ${item.title} ${item.description}`.toLowerCase();
    return haystack.includes(query);
  });
});
const showSlashMenu = computed(
  () =>
    slashDraft.value.startsWith("/") &&
    filteredSlashCommands.value.length > 0 &&
    dismissedSlashDraft.value !== slashDraft.value,
);
const selectedSlashCommand = computed(() => {
  if (!showSlashMenu.value) return null;
  const index = Math.min(
    selectedSlashIndex.value,
    Math.max(filteredSlashCommands.value.length - 1, 0),
  );
  return filteredSlashCommands.value[index] ?? null;
});
const exactSlashCommand = computed(
  () =>
    props.slashCommands.find(
      (item) => item.command.toLowerCase() === slashDraft.value.toLowerCase(),
    ) ?? null,
);
const queueItems = computed(() => props.queuedMessages ?? []);
const sendButtonLoading = computed(
  () => props.sendingChat && !props.hasBlockingRun && props.chatDraft.trim() === "",
);
const showInterruptButton = computed(
  () => props.canInterruptChat && props.chatDraft.trim() !== "",
);
const showStopButton = computed(
  () => props.activeRunId !== "" && (props.hasBlockingRun || props.sendingChat),
);

const contextTone = computed(() => {
  switch (props.contextSnapshot?.status) {
    case "critical":
      return "error";
    case "near_limit":
    case "warning":
      return "warning";
    case "healthy":
      return "success";
    default:
      return "default";
  }
});
const hasContextUsage = computed(() => {
  if (!props.contextSnapshot) return false;
  return (
    (props.contextSnapshot.currentInputTokens ?? 0) > 0 ||
    (props.contextSnapshot.projectedNextTurnTokens ?? 0) > 0 ||
    (props.contextSnapshot.activeHandoffCount ?? 0) > 0 ||
    (props.contextSnapshot.retainedRecentUserCount ?? 0) > 0
  );
});
const hasKnownContextWindow = computed(
  () =>
    !!props.contextSnapshot &&
    props.contextSnapshot.contextWindowTokens > 0 &&
    props.contextSnapshot.status !== "unknown",
);
const contextStatusLabel = computed(() => {
  switch (props.contextSnapshot?.status) {
    case "critical":
      return "危险";
    case "near_limit":
      return "接近上限";
    case "warning":
      return "注意";
    case "healthy":
      return "正常";
    default:
      return "未知";
  }
});
const contextPercent = computed(() => {
  const ratio = props.contextSnapshot?.usageRatio ?? 0;
  if (!hasKnownContextWindow.value) {
    return "未知";
  }
  return `${Math.max(0, Math.round(ratio * 100))}%`;
});
const contextPillLabel = computed(() => {
  if (!hasContextUsage.value) {
    return "";
  }
  if (!hasKnownContextWindow.value) {
    return `${formatTokenCount(props.contextSnapshot?.currentInputTokens ?? 0)} Tokens`;
  }
  return `${contextPercent.value} ${contextStatusLabel.value}`;
});
const contextSummaryPreview = computed(() => {
  const preview =
    props.contextSnapshot?.latestHandoffPreview?.trim() ??
    props.contextSnapshot?.summaryPreview?.trim() ??
    "";
  if (preview === "") return "暂无 handoff 摘要";
  return preview;
});
const breakdownRows = computed(() => {
  const breakdown = props.contextSnapshot?.breakdown;
  if (!breakdown) return [];
  return [
    { label: "系统指令", value: breakdown.instructionTokens },
    { label: "handoff 摘要", value: breakdown.handoffTokens },
    { label: "近期用户原文", value: breakdown.recentUserTokens },
    { label: "受保护尾部", value: breakdown.protectedTailTokens },
    { label: "工具声明", value: breakdown.toolDeclarationTokens },
    { label: "其他可见内容", value: breakdown.otherVisibleTokens },
    { label: "待发送输入", value: breakdown.pendingUserTokens },
  ];
});

watch(filteredSlashCommands, (items) => {
  if (items.length === 0) {
    selectedSlashIndex.value = 0;
    return;
  }
  selectedSlashIndex.value = Math.min(
    selectedSlashIndex.value,
    items.length - 1,
  );
});

watch(
  () => props.chatDraft,
  () => {
    if (dismissedSlashDraft.value !== slashDraft.value) {
      dismissedSlashDraft.value = "";
    }
  },
);

function openContextPopover(): void {
  contextMenuOpen.value = true;
  props.openContextDetails?.();
}

async function handlePrimaryAction(): Promise<void> {
  if (
    showSlashMenu.value &&
    selectedSlashCommand.value &&
    !selectedSlashCommand.value.disabled
  ) {
    await executeSlashCommand(selectedSlashCommand.value);
    return;
  }
  if (exactSlashCommand.value && !exactSlashCommand.value.disabled) {
    await executeSlashCommand(exactSlashCommand.value);
    return;
  }
  await props.sendChat();
}

async function executeSlashCommand(command: SlashCommandItem): Promise<void> {
  if (command.disabled) return;
  dismissedSlashDraft.value = "";
  emit("update:chatDraft", "");
  await props.runSlashCommand?.(command.id);
}

function dismissSlashMenu(): void {
  if (!showSlashMenu.value) return;
  dismissedSlashDraft.value = slashDraft.value;
}

function handleKeydown(event: KeyboardEvent): void {
  if (showSlashMenu.value) {
    if (event.key === "ArrowDown") {
      event.preventDefault();
      selectedSlashIndex.value =
        (selectedSlashIndex.value + 1) % filteredSlashCommands.value.length;
      return;
    }
    if (event.key === "ArrowUp") {
      event.preventDefault();
      selectedSlashIndex.value =
        (selectedSlashIndex.value + filteredSlashCommands.value.length - 1) %
        filteredSlashCommands.value.length;
      return;
    }
    if (event.key === "Escape") {
      event.preventDefault();
      dismissSlashMenu();
      return;
    }
    if (
      event.key === "Enter" &&
      !event.shiftKey &&
      !event.isComposing &&
      selectedSlashCommand.value
    ) {
      event.preventDefault();
      void executeSlashCommand(selectedSlashCommand.value);
      return;
    }
  }
  props.handleComposerKeydown?.(event);
}

function formatTokenCount(value: number): string {
  return Math.max(0, value).toLocaleString("zh-CN");
}

function contextWindowLabel(
  snapshot: ADKSessionContextSnapshot | null | undefined,
): string {
  if (!snapshot || snapshot.contextWindowTokens <= 0) {
    return "未配置";
  }
  return formatTokenCount(snapshot.contextWindowTokens);
}

function compactionModeLabel(mode?: string): string {
  switch (mode) {
    case "manual":
      return "手动";
    case "auto":
      return "自动";
    case "aggressive":
      return "激进";
    default:
      return "未执行";
  }
}

function queueItemBadge(item: QueuedChatMessage, index: number): string {
  if (props.queueDispatchingId === item.id) {
    return "sending next";
  }
  if (index === 0 && item.mode === "interrupt" && props.hasBlockingRun) {
    return "interrupting";
  }
  if (item.mode === "interrupt") {
    return "interrupt";
  }
  return "queued";
}

function queueItemStateClass(item: QueuedChatMessage, index: number): string {
  return `is-${queueItemBadge(item, index).replace(/\s+/g, "-")}`;
}

function canRevokeQueueItem(item: QueuedChatMessage): boolean {
  return props.queueDispatchingId !== item.id;
}
</script>

<template>
  <div
    class="adk-composer"
    :class="{ 'adk-composer--dock': variant === 'dock' }"
  >
    <div class="adk-composer-card">
      <div v-if="composerBlockMessage" class="adk-composer-block">
        <v-icon size="13">fa-solid fa-circle-exclamation</v-icon>
        <span>{{ composerBlockMessage }}</span>
      </div>

      <div v-if="variant === 'dock' && hasContextUsage" class="adk-context-row">
        <v-menu
          v-model="contextMenuOpen"
          location="top start"
          :close-on-content-click="false"
          open-on-hover
        >
          <template #activator="{ props: menuProps }">
            <v-btn
              v-bind="menuProps"
              size="small"
              variant="tonal"
              class="adk-context-pill"
              :color="contextTone"
              :loading="contextBusy"
              @click="openContextPopover"
            >
              {{ contextPillLabel }}
            </v-btn>
          </template>

          <v-card min-width="360" class="adk-context-card">
            <v-card-title class="text-subtitle-2">上下文使用情况</v-card-title>
            <v-card-text class="adk-context-card__body">
              <div class="adk-context-stat">
                <span>当前输入 Token</span>
                <strong>{{
                  formatTokenCount(contextSnapshot?.currentInputTokens ?? 0)
                }}</strong>
              </div>
              <div class="adk-context-stat">
                <span>下一轮预计 Token</span>
                <strong>{{
                  formatTokenCount(
                    contextSnapshot?.projectedNextTurnTokens ?? 0,
                  )
                }}</strong>
              </div>
              <div class="adk-context-stat">
                <span>模型窗口</span>
                <strong>{{ contextWindowLabel(contextSnapshot) }}</strong>
              </div>
              <div class="adk-context-stat">
                <span>状态</span>
                <strong>{{ contextStatusLabel }}</strong>
              </div>
              <div class="adk-context-stat">
                <span>保留最近用户消息条数</span>
                <strong>{{ contextSnapshot?.recentUserWindow ?? 0 }}</strong>
              </div>
              <div class="adk-context-stat">
                <span>当前保留用户消息</span>
                <strong>{{
                  contextSnapshot?.retainedRecentUserCount ?? 0
                }}</strong>
              </div>
              <div class="adk-context-stat">
                <span>生效 handoff 段数</span>
                <strong>{{ contextSnapshot?.activeHandoffCount ?? 0 }}</strong>
              </div>
              <div class="adk-context-stat">
                <span>最近压缩方式</span>
                <strong>{{
                  compactionModeLabel(contextSnapshot?.lastCompactionMode)
                }}</strong>
              </div>
              <div class="adk-context-stat">
                <span>自动压缩</span>
                <strong>{{ contextSnapshot?.autoCompacted ? "是" : "否" }}</strong>
              </div>
              <div class="adk-context-stat">
                <span>降级摘要</span>
                <strong>{{ contextSnapshot?.degradedSummary ? "是" : "否" }}</strong>
              </div>
              <div class="adk-context-breakdown">
                <div class="adk-context-summary__title">Token 构成</div>
                <div
                  v-for="item in breakdownRows"
                  :key="item.label"
                  class="adk-context-stat"
                >
                  <span>{{ item.label }}</span>
                  <strong>{{ formatTokenCount(item.value) }}</strong>
                </div>
              </div>
              <div class="adk-context-summary">
                <div class="adk-context-summary__title">最新 handoff 摘要</div>
                <div class="adk-context-summary__content">
                  {{ contextSummaryPreview }}
                </div>
              </div>
              <div
                v-if="contextSnapshot?.lastCompactionReason"
                class="adk-context-summary"
              >
                <div class="adk-context-summary__title">最近压缩原因</div>
                <div class="adk-context-summary__content">
                  {{ contextSnapshot.lastCompactionReason }}
                </div>
              </div>
              <div
                v-if="contextSnapshot?.lastCompactedAt"
                class="adk-context-summary"
              >
                <div class="adk-context-summary__title">最近压缩时间</div>
                <div class="adk-context-summary__content">
                  {{ contextSnapshot.lastCompactedAt }}
                </div>
              </div>
            </v-card-text>
          </v-card>
        </v-menu>
      </div>

      <div v-if="queueItems.length > 0" class="adk-queue-strip">
        <div class="adk-queue-strip__header">
          <span class="adk-queue-strip__title">待发送队列</span>
          <span v-if="hasBlockingRun" class="adk-queue-strip__hint">
            当前运行结束后自动发送
          </span>
        </div>
        <div class="adk-queue-list">
          <div
            v-for="(item, index) in queueItems"
            :key="item.id"
            class="adk-queue-item"
          >
            <span class="adk-queue-item__index">#{{ index + 1 }}</span>
            <span
              class="adk-queue-item__badge"
              :class="queueItemStateClass(item, index)"
            >
              {{ queueItemBadge(item, index) }}
            </span>
            <span class="adk-queue-item__text" :title="item.text">{{ item.text }}</span>
            <button
              type="button"
              class="adk-queue-item__remove"
              :disabled="!canRevokeQueueItem(item)"
              @click="void revokeQueuedMessage?.(item.id)"
            >
              撤回
            </button>
          </div>
        </div>
      </div>

      <template v-if="variant === 'page'">
        <div class="adk-composer-input-wrap">
          <v-textarea
            :model-value="chatDraft"
            :placeholder="placeholder"
            variant="plain"
            density="compact"
            :rows="1"
            auto-grow
            :max-rows="6"
            hide-details
            class="adk-composer-input"
            @update:model-value="$emit('update:chatDraft', $event ?? '')"
            @keydown="handleKeydown"
          />
          <div v-if="showSlashMenu" class="adk-slash-menu">
            <button
              v-for="(item, index) in filteredSlashCommands"
              :key="item.command"
              type="button"
              class="adk-slash-menu__item"
              :class="{
                'adk-slash-menu__item--active': index === selectedSlashIndex,
                'adk-slash-menu__item--disabled': item.disabled,
              }"
              :disabled="item.disabled"
              @mousedown.prevent
              @click="void executeSlashCommand(item)"
            >
              <div class="adk-slash-menu__command">{{ item.command }}</div>
              <div class="adk-slash-menu__meta">
                <span class="adk-slash-menu__title">{{ item.title }}</span>
                <span class="adk-slash-menu__desc">{{ item.description }}</span>
              </div>
            </button>
          </div>
        </div>

        <div class="adk-composer-controls">
          <div class="adk-composer-left">
            <v-btn
              icon="fa-solid fa-plus"
              variant="text"
              size="small"
              title="添加模型服务"
              @click="openProviderSettings?.()"
            />
            <v-select
              :model-value="selectedAgentId"
              :items="agentOptions"
              density="compact"
              variant="plain"
              hide-details
              placeholder="选择 Agent"
              class="adk-agent-select"
              @update:model-value="
                $emit('update:selectedAgentId', $event ?? '');
                handleAgentChange?.();
              "
            />
          </div>

          <div class="adk-composer-right">
            <v-progress-circular
              v-if="loading"
              indeterminate
              size="18"
              width="2"
            />

            <v-menu
              v-if="hasContextUsage"
              v-model="contextMenuOpen"
              location="top start"
              :close-on-content-click="false"
              open-on-hover
            >
              <template #activator="{ props: menuProps }">
                <v-btn
                  v-bind="menuProps"
                  size="small"
                  variant="tonal"
                  class="adk-context-pill rounded-xl"
                  :color="contextTone"
                  :loading="contextBusy"
                  @click="openContextPopover"
                >
                  {{ contextPillLabel }}
                </v-btn>
              </template>

              <v-card min-width="360" class="adk-context-card">
                <v-card-title class="text-subtitle-2">
                  上下文使用情况
                </v-card-title>
                <v-card-text class="adk-context-card__body">
                  <div class="adk-context-stat">
                    <span>当前输入 Token</span>
                    <strong>{{
                      formatTokenCount(contextSnapshot?.currentInputTokens ?? 0)
                    }}</strong>
                  </div>
                  <div class="adk-context-stat">
                    <span>下一轮预计 Token</span>
                    <strong>{{
                      formatTokenCount(
                        contextSnapshot?.projectedNextTurnTokens ?? 0,
                      )
                    }}</strong>
                  </div>
                  <div class="adk-context-stat">
                    <span>模型窗口</span>
                    <strong>{{ contextWindowLabel(contextSnapshot) }}</strong>
                  </div>
                  <div class="adk-context-stat">
                    <span>状态</span>
                    <strong>{{ contextStatusLabel }}</strong>
                  </div>
                  <div class="adk-context-stat">
                    <span>保留最近用户消息条数</span>
                    <strong>{{ contextSnapshot?.recentUserWindow ?? 0 }}</strong>
                  </div>
                  <div class="adk-context-stat">
                    <span>当前保留用户消息</span>
                    <strong>{{
                      contextSnapshot?.retainedRecentUserCount ?? 0
                    }}</strong>
                  </div>
                  <div class="adk-context-stat">
                    <span>生效 handoff 段数</span>
                    <strong>{{ contextSnapshot?.activeHandoffCount ?? 0 }}</strong>
                  </div>
                  <div class="adk-context-stat">
                    <span>最近压缩方式</span>
                    <strong>{{
                      compactionModeLabel(contextSnapshot?.lastCompactionMode)
                    }}</strong>
                  </div>
                  <div class="adk-context-stat">
                    <span>自动压缩</span>
                    <strong>{{
                      contextSnapshot?.autoCompacted ? "是" : "否"
                    }}</strong>
                  </div>
                  <div class="adk-context-stat">
                    <span>降级摘要</span>
                    <strong>{{
                      contextSnapshot?.degradedSummary ? "是" : "否"
                    }}</strong>
                  </div>
                  <div class="adk-context-breakdown">
                    <div class="adk-context-summary__title">Token 构成</div>
                    <div
                      v-for="item in breakdownRows"
                      :key="item.label"
                      class="adk-context-stat"
                    >
                      <span>{{ item.label }}</span>
                      <strong>{{ formatTokenCount(item.value) }}</strong>
                    </div>
                  </div>
                  <div class="adk-context-summary">
                    <div class="adk-context-summary__title">
                      最新 handoff 摘要
                    </div>
                    <div class="adk-context-summary__content">
                      {{ contextSummaryPreview }}
                    </div>
                  </div>
                  <div
                    v-if="contextSnapshot?.lastCompactionReason"
                    class="adk-context-summary"
                  >
                    <div class="adk-context-summary__title">最近压缩原因</div>
                    <div class="adk-context-summary__content">
                      {{ contextSnapshot.lastCompactionReason }}
                    </div>
                  </div>
                  <div
                    v-if="contextSnapshot?.lastCompactedAt"
                    class="adk-context-summary"
                  >
                    <div class="adk-context-summary__title">最近压缩时间</div>
                    <div class="adk-context-summary__content">
                      {{ contextSnapshot.lastCompactedAt }}
                    </div>
                  </div>
                </v-card-text>
              </v-card>
            </v-menu>

            <v-select
              :model-value="selectedProviderId"
              :items="providerOptions"
              density="compact"
              variant="plain"
              hide-details
              placeholder="选择模型"
              class="adk-provider-select"
              :disabled="
                selectedAgentId === '' ||
                providerOptions.length === 0 ||
                savingProviderSelection
              "
              :loading="savingProviderSelection"
              @update:model-value="
                $emit('update:selectedProviderId', $event ?? '');
                handleProviderChange?.(($event ?? '') as string);
              "
            />

            <v-btn
              icon="fa-solid fa-gear"
              variant="text"
              size="small"
              title="Agent 设置"
              @click="openProviderSettings?.()"
            />
            <v-btn
              v-if="showStopButton"
              icon="fa-solid fa-stop"
              variant="text"
              color="error"
              size="small"
              title="停止运行"
              @click="cancelActiveRun?.()"
            />
            <v-btn
              v-if="showInterruptButton"
              class="adk-composer-interrupt"
              variant="outlined"
              color="warning"
              :disabled="!canInterruptChat"
              @click="void interruptAndQueueChat?.()"
            >
              打断后发送
            </v-btn>
            <v-btn
              color="primary"
              :loading="sendButtonLoading"
              :disabled="!canSendChat"
              icon="fa-solid fa-paper-plane"
              class="adk-composer-send"
              @click="void handlePrimaryAction()"
            />
          </div>
        </div>
      </template>

      <template v-else>
        <div v-if="suggestions.length > 0" class="adk-composer-suggestions">
          <v-btn
            v-for="suggestion in suggestions"
            :key="suggestion"
            class="adk-composer-suggestion"
            variant="text"
            size="x-small"
            @click="
              applySuggestion
                ? applySuggestion(suggestion)
                : $emit('update:chatDraft', suggestion)
            "
          >
            {{ suggestion }}
          </v-btn>
        </div>

        <div class="adk-composer-input-wrap">
          <form
            class="adk-composer-dock-form"
            @submit.prevent="void handlePrimaryAction()"
          >
            <v-text-field
              :model-value="chatDraft"
              :placeholder="placeholder"
              variant="plain"
              density="compact"
              hide-details
              class="adk-composer-input adk-composer-input--dock"
              @update:model-value="$emit('update:chatDraft', $event ?? '')"
              @keydown="handleKeydown"
            />
            <v-btn
              v-if="showInterruptButton"
              class="adk-composer-interrupt adk-composer-interrupt--dock"
              variant="outlined"
              color="warning"
              :disabled="!canInterruptChat"
              @click="void interruptAndQueueChat?.()"
            >
              打断后发送
            </v-btn>
            <v-btn
              v-if="showStopButton"
              variant="text"
              color="error"
              class="adk-composer-stop adk-composer-stop--dock"
              @click="cancelActiveRun?.()"
            >
              停止
            </v-btn>
            <v-btn
              color="primary"
              :loading="sendButtonLoading"
              :disabled="!canSendChat"
              class="adk-composer-send adk-composer-send--dock"
              @click="void handlePrimaryAction()"
            >
              发送
            </v-btn>
          </form>
          <div v-if="showSlashMenu" class="adk-slash-menu adk-slash-menu--dock">
            <button
              v-for="(item, index) in filteredSlashCommands"
              :key="item.command"
              type="button"
              class="adk-slash-menu__item"
              :class="{
                'adk-slash-menu__item--active': index === selectedSlashIndex,
                'adk-slash-menu__item--disabled': item.disabled,
              }"
              :disabled="item.disabled"
              @mousedown.prevent
              @click="void executeSlashCommand(item)"
            >
              <div class="adk-slash-menu__command">{{ item.command }}</div>
              <div class="adk-slash-menu__meta">
                <span class="adk-slash-menu__title">{{ item.title }}</span>
                <span class="adk-slash-menu__desc">{{ item.description }}</span>
              </div>
            </button>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped>
.adk-context-row {
  display: flex;
  justify-content: flex-start;
  margin-bottom: 8px;
}

.adk-context-pill {
  text-transform: none;
  letter-spacing: 0;
}

.adk-context-card__body {
  display: grid;
  gap: 8px;
}

.adk-context-stat {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  font-size: 13px;
}

.adk-context-breakdown {
  display: grid;
  gap: 6px;
  margin-top: 4px;
}

.adk-context-summary {
  display: grid;
  gap: 4px;
  margin-top: 4px;
}

.adk-context-summary__title {
  font-size: 12px;
  color: rgb(100 116 139);
}

.adk-context-summary__content {
  font-size: 12px;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-word;
}

.adk-queue-strip {
  display: grid;
  gap: 8px;
  margin-bottom: 10px;
  padding: 10px 12px;
  border: 1px solid rgb(226 232 240);
  border-radius: 14px;
  background: rgb(248 250 252);
}

.adk-queue-strip__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.adk-queue-strip__title {
  font-size: 12px;
  font-weight: 600;
  color: rgb(15 23 42);
}

.adk-queue-strip__hint {
  font-size: 12px;
  color: rgb(100 116 139);
}

.adk-queue-list {
  display: grid;
  gap: 8px;
}

.adk-queue-item {
  display: grid;
  grid-template-columns: auto auto 1fr auto;
  gap: 8px;
  align-items: center;
  min-width: 0;
}

.adk-queue-item__index {
  font-size: 12px;
  color: rgb(100 116 139);
}

.adk-queue-item__badge {
  padding: 2px 8px;
  border-radius: 999px;
  font-size: 11px;
  line-height: 1.4;
  text-transform: lowercase;
  color: rgb(15 23 42);
  background: rgb(226 232 240);
}

.adk-queue-item__badge.is-queued {
  background: rgb(226 232 240);
}

.adk-queue-item__badge.is-interrupt,
.adk-queue-item__badge.is-interrupting {
  color: rgb(146 64 14);
  background: rgb(254 215 170);
}

.adk-queue-item__badge.is-sending-next {
  color: rgb(29 78 216);
  background: rgb(191 219 254);
}

.adk-queue-item__text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13px;
  color: rgb(15 23 42);
}

.adk-queue-item__remove {
  border: 0;
  background: transparent;
  color: rgb(220 38 38);
  font-size: 12px;
  cursor: pointer;
}

.adk-queue-item__remove:disabled {
  opacity: 0.45;
  cursor: default;
}

.adk-composer-input-wrap {
  position: relative;
}

.adk-composer-dock-form {
  display: flex;
  align-items: center;
  gap: 8px;
}

.adk-composer-interrupt {
  text-transform: none;
}

.adk-composer-interrupt--dock,
.adk-composer-stop--dock {
  flex-shrink: 0;
}

.adk-slash-menu {
  position: absolute;
  left: 0;
  right: 0;
  bottom: calc(100% + 8px);
  z-index: 20;
  display: grid;
  gap: 6px;
  padding: 8px;
  border: 1px solid rgb(203 213 225);
  border-radius: 14px;
  background: rgba(255, 255, 255, 0.98);
  box-shadow: 0 18px 32px rgba(15, 23, 42, 0.14);
}

.adk-slash-menu--dock {
  left: 8px;
  right: 8px;
}

.adk-slash-menu__item {
  display: grid;
  grid-template-columns: 130px 1fr;
  gap: 10px;
  align-items: start;
  width: 100%;
  padding: 10px 12px;
  border: 0;
  border-radius: 10px;
  background: transparent;
  text-align: left;
  cursor: pointer;
}

.adk-slash-menu__item--active {
  background: rgb(241 245 249);
}

.adk-slash-menu__item--disabled {
  cursor: not-allowed;
  opacity: 0.55;
}

.adk-slash-menu__command {
  font-family:
    ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono",
    "Courier New", monospace;
  font-size: 12px;
  color: rgb(15 23 42);
}

.adk-slash-menu__meta {
  display: grid;
  gap: 2px;
}

.adk-slash-menu__title {
  font-size: 13px;
  font-weight: 600;
  color: rgb(15 23 42);
}

.adk-slash-menu__desc {
  font-size: 12px;
  color: rgb(100 116 139);
}
</style>
