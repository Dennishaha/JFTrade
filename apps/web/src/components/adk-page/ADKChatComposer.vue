<script setup lang="ts">
import { computed, ref, watch } from "vue";

import type {
  ADKAgent,
  ADKPermissionMode,
  ADKProvider,
  ADKSessionContextSnapshot,
  ADKWorkMode,
} from "@/contracts";

import type { QueuedChatMessage } from "../../composables/adkChatRuntime";

interface SlashCommandItem {
  id: "context" | "compact" | "compact-aggressive";
  command: string;
  title: string;
  description: string;
  disabled?: boolean;
}

interface ProviderOption {
  title: string;
  value: string;
  providerId?: string;
  displayName?: string;
  model?: string;
  isDefault?: boolean;
}

const props = withDefaults(
  defineProps<{
    layout?: "desktop" | "mobile";
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
    goalObjectiveDraft?: string;
    goalObjectiveError?: string;
    goalObjectiveSaving?: boolean;
    goalLifecycleBusy?: boolean;
    goalPaused?: boolean;
    goalTimedOut?: boolean;
    goalPauseRequested?: boolean;
    showGoalObjectiveEditor?: boolean;
    canSaveGoalObjective?: boolean;
    canPauseGoal?: boolean;
    canResumeGoal?: boolean;
    hasBlockingRun?: boolean;
    interruptingRunId?: string;
    loading?: boolean;
    placeholder?: string;
    providerOptions?: ProviderOption[];
    queuedMessages?: QueuedChatMessage[];
    queueDispatchingId?: string;
    revokeQueuedMessage?: (messageId: string) => void | Promise<void>;
    savingProviderSelection?: boolean;
    selectedAgent?: ADKAgent | null;
    selectedAgentId?: string;
    selectedSessionId?: string;
    selectedProvider?: ADKProvider | null;
    selectedProviderId?: string;
    sendingChat: boolean;
    slashCommands?: SlashCommandItem[];
    suggestions?: string[];
    defaultWorkMode?: ADKWorkMode | string;
    defaultPermissionMode?: ADKPermissionMode | string;
    permissionModeOverride?: string;
    workModeOverride?: string;
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
    cancelGoalObjective?: () => void | Promise<void>;
    pauseGoalRun?: () => void | Promise<void>;
    resumeGoalRun?: () => void | Promise<void>;
    updateGoalObjective?: () => void | Promise<void>;
    updateGoalObjectiveDraft?: (value: string) => void;
  }>(),
  {
    layout: "desktop",
    activeRunId: "",
    activeRunStatus: "",
    agentOptions: () => [],
    canInterruptChat: false,
    composerBlockMessage: "",
    contextBusy: false,
    contextDetailsOpen: false,
    contextSnapshot: null,
    goalObjectiveDraft: "",
    goalObjectiveError: "",
    goalObjectiveSaving: false,
    goalLifecycleBusy: false,
    goalPaused: false,
    goalTimedOut: false,
    goalPauseRequested: false,
    showGoalObjectiveEditor: false,
    canSaveGoalObjective: false,
    canPauseGoal: false,
    canResumeGoal: false,
    hasBlockingRun: false,
    interruptingRunId: "",
    loading: false,
    placeholder: "输入问题或任务...",
    providerOptions: () => [],
    queuedMessages: () => [],
    queueDispatchingId: "",
    savingProviderSelection: false,
    selectedAgent: null,
    selectedAgentId: "",
    selectedSessionId: "",
    selectedProvider: null,
    selectedProviderId: "",
    slashCommands: () => [],
    suggestions: () => [],
    defaultWorkMode: "chat",
    defaultPermissionMode: "approval",
    permissionModeOverride: "",
    workModeOverride: "",
  },
);

const emit = defineEmits<{
  "update:chatDraft": [value: string];
  "update:contextDetailsOpen": [value: boolean];
  "update:selectedAgentId": [value: string];
  "update:selectedProviderId": [value: string];
  "update:permissionModeOverride": [value: string];
  "update:workModeOverride": [value: string];
}>();

const mobileControlsExpanded = ref(false);
const selectedSlashIndex = ref(0);
const dismissedSlashDraft = ref("");
const goalEditorExpanded = ref(false);
const isMobileLayout = computed(() => props.layout === "mobile");
const supportedWorkModes: Array<{ title: string; value: ADKWorkMode }> = [
  { title: "对话", value: "chat" },
  { title: "目标", value: "loop" },
];
const normalizedDefaultWorkMode = computed<ADKWorkMode>(() => {
  if (props.defaultWorkMode === "loop") {
    return props.defaultWorkMode;
  }
  return "chat";
});
const workModeOptions = computed(() =>
  supportedWorkModes.map((mode) => ({
    ...mode,
    isDefault: mode.value === normalizedDefaultWorkMode.value,
  })),
);
const effectiveWorkModeSelection = computed(
  () => props.workModeOverride || normalizedDefaultWorkMode.value,
);

interface PermissionModeOption {
  title: string;
  value: ADKPermissionMode;
  icon: string;
  tone: "approval" | "less" | "all";
  description: string;
}

const permissionModeOptions: PermissionModeOption[] = [
  {
    title: "请求批准",
    value: "approval",
    icon: "fa-solid fa-shield-halved",
    tone: "approval",
    description: "低风险操作自动执行，敏感操作请求确认",
  },
  {
    title: "减少审批",
    value: "less_approval",
    icon: "fa-solid fa-shield",
    tone: "less",
    description: "减少中等风险操作的确认次数",
  },
  {
    title: "完全访问",
    value: "all",
    icon: "fa-solid fa-triangle-exclamation",
    tone: "all",
    description: "不受限制地访问互联网和本机文件",
  },
];

const normalizedDefaultPermissionMode = computed<ADKPermissionMode>(() => {
  if (
    props.defaultPermissionMode === "less_approval" ||
    props.defaultPermissionMode === "all"
  ) {
    return props.defaultPermissionMode;
  }
  return "approval";
});
const effectivePermissionMode = computed<ADKPermissionMode>(() => {
  if (
    props.permissionModeOverride === "less_approval" ||
    props.permissionModeOverride === "all"
  ) {
    return props.permissionModeOverride;
  }
  if (props.permissionModeOverride === "approval") return "approval";
  return normalizedDefaultPermissionMode.value;
});
const effectivePermissionOption = computed(
  () =>
    permissionModeOptions.find(
      (option) => option.value === effectivePermissionMode.value,
    ) ?? permissionModeOptions[0]!,
);

function updatePermissionModeSelection(mode: ADKPermissionMode): void {
  emit(
    "update:permissionModeOverride",
    mode === normalizedDefaultPermissionMode.value ? "" : mode,
  );
}

function updateWorkModeSelection(mode?: string | null): void {
  emit(
    "update:workModeOverride",
    mode === normalizedDefaultWorkMode.value ? "" : (mode ?? ""),
  );
}

function updateAgentSelection(agentId: string): void {
  emit("update:selectedAgentId", agentId);
  props.handleAgentChange?.();
}

function updateProviderSelection(providerId: string): void {
  emit("update:selectedProviderId", providerId);
  void props.handleProviderChange?.(providerId);
}

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
const goalObjectiveSummary = computed(() => {
  const summary = props.goalObjectiveDraft.trim();
  return summary || "尚未设置目标";
});
const goalObjectiveStatus = computed(() => {
  if (props.goalObjectiveSaving) return "保存中";
  if (props.goalObjectiveError) return "保存失败";
  if (props.goalTimedOut) return "已超时";
  if (props.goalPaused) return "已暂停";
  if (props.goalPauseRequested) return "暂停中";
  if (props.activeRunId)
    return props.canSaveGoalObjective ? "已修改" : "运行中";
  return "待发送";
});
const goalObjectiveTone = computed(() => {
  if (props.goalObjectiveError) return "is-error";
  if (props.goalPaused || props.goalTimedOut || props.goalPauseRequested) return "is-warning";
  if (props.goalObjectiveSaving || props.canSaveGoalObjective)
    return "is-warning";
  if (props.activeRunId) return "is-info";
  return "is-muted";
});
const showGoalLifecycleButton = computed(
  () =>
    props.canPauseGoal ||
    props.canResumeGoal ||
    props.goalPauseRequested ||
    props.goalPaused ||
    props.goalTimedOut,
);
const goalLifecycleButtonLabel = computed(() => {
  if (props.goalTimedOut) return "继续目标";
  if (props.goalPaused) return "运行目标";
  if (props.goalPauseRequested) return "暂停中";
  return "暂停目标";
});
const goalLifecycleButtonIcon = computed(() =>
  props.goalPaused || props.goalTimedOut ? "fa-solid fa-play" : "fa-solid fa-pause",
);
const goalLifecycleButtonDisabled = computed(
  () =>
    props.goalLifecycleBusy ||
    props.goalPauseRequested ||
    (!props.canPauseGoal && !props.canResumeGoal),
);
const sendButtonLoading = computed(
  () =>
    (props.sendingChat || props.hasBlockingRun) &&
    props.chatDraft.trim() === "",
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
const showContextControl = computed(
  () =>
    props.selectedSessionId.trim() !== "" ||
    hasContextUsage.value ||
    props.contextBusy ||
    props.contextDetailsOpen ||
    props.contextSnapshot != null,
);
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
const contextProgressValue = computed(() => {
  if (!hasKnownContextWindow.value) return 0;
  const ratio = props.contextSnapshot?.usageRatio ?? 0;
  return Math.min(100, Math.max(0, Math.round(ratio * 100)));
});
const contextProgressColor = computed(() => {
  switch (contextTone.value) {
    case "error":
      return "error";
    case "warning":
      return "warning";
    case "success":
      return "success";
    default:
      return "secondary";
  }
});
const contextPillLabel = computed(() => {
  if (props.contextBusy && !props.contextSnapshot) return "上下文...";
  if (hasKnownContextWindow.value) {
    return `${contextPercent.value} ${contextStatusLabel.value}`;
  }
  if (hasContextUsage.value) {
    return `${formatTokenCount(props.contextSnapshot?.currentInputTokens ?? 0)} Tokens`;
  }
  return "上下文";
});
const selectedAgentLabel = computed(
  () =>
    props.selectedAgent?.name?.trim() ||
    props.agentOptions
      .find((item) => item.value === props.selectedAgentId)
      ?.title.split(" · ")[0]
      ?.trim() ||
    "Agent",
);
const selectedProviderLabel = computed(
  () =>
    props.selectedProvider?.model?.trim() ||
    props.providerOptions
      .find((item) => item.value === props.selectedProviderId)
      ?.model
      ?.trim() ||
    props.providerOptions
      .find((item) => item.value === props.selectedProviderId)
      ?.title.split(" · ")[1]
      ?.trim() ||
    props.providerOptions
      .find((item) => item.value === props.selectedProviderId)
      ?.title.split(" · ")[0]
      ?.trim() ||
    "模型",
);
const selectedProviderTitle = computed(() => {
  const provider = props.selectedProvider;
  if (!provider) return selectedProviderLabel.value;
  return `${provider.displayName} · ${provider.model}`;
});
const selectedWorkModeLabel = computed(
  () =>
    workModeOptions.value.find(
      (item) => item.value === effectiveWorkModeSelection.value,
    )?.title ?? "对话",
);
const mobileContextSummaryLabel = computed(() => {
  if (!showContextControl.value) {
    return props.contextBusy ? "上下文..." : "上下文";
  }
  return contextPillLabel.value;
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
const rawBreakdownRows = computed(() => {
  const breakdown = props.contextSnapshot?.rawBreakdown;
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
const rawContextDiagnosticsVisible = computed(() => {
  const snapshot = props.contextSnapshot;
  if (!snapshot) return false;
  return (
    (snapshot.trimmedToolResponseCount ?? 0) > 0 ||
    (snapshot.rawCurrentInputTokens ?? snapshot.currentInputTokens) >
      snapshot.currentInputTokens ||
    (snapshot.rawProjectedNextTurnTokens ?? snapshot.projectedNextTurnTokens) >
      snapshot.projectedNextTurnTokens
  );
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
watch(
  () => props.showGoalObjectiveEditor,
  (show) => {
    if (!show) {
      goalEditorExpanded.value = false;
    }
  },
);
watch(
  () => props.goalObjectiveError,
  (error) => {
    if (error) {
      goalEditorExpanded.value = true;
    }
  },
);
watch(
  () => props.layout,
  (layout) => {
    if (layout !== "mobile") {
      mobileControlsExpanded.value = false;
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

async function handleCancelGoalObjective(): Promise<void> {
  if (props.cancelGoalObjective) {
    await props.cancelGoalObjective();
    return;
  }
  if (props.activeRunId) {
    await props.cancelActiveRun?.();
    return;
  }
  props.updateGoalObjectiveDraft?.("");
  emit("update:workModeOverride", "");
}

async function handleGoalLifecycleAction(): Promise<void> {
  if (props.goalLifecycleBusy || props.goalPauseRequested) return;
  if (props.goalPaused) {
    await props.resumeGoalRun?.();
    return;
  }
  await props.pauseGoalRun?.();
}

function handleGoalObjectiveInput(value: string | null): void {
  props.updateGoalObjectiveDraft?.(value ?? "");
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

function contextRevisionLabel(
  snapshot: ADKSessionContextSnapshot | null | undefined,
): string {
  const revision = snapshot?.contextRevisionId?.trim() ?? "";
  if (revision === "") return "未生成";
  return revision.length > 18 ? `${revision.slice(0, 18)}...` : revision;
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
    :class="{ 'adk-composer--mobile': layout === 'mobile' }"
  >
    <div class="adk-composer-card">
      <div v-if="composerBlockMessage" class="adk-composer-block">
        <v-icon size="13">fa-solid fa-circle-exclamation</v-icon>
        <span>{{ composerBlockMessage }}</span>
      </div>

      <div
        v-if="showGoalObjectiveEditor"
        class="adk-goal-editor"
        :class="{ 'is-expanded': goalEditorExpanded }"
      >
        <div class="adk-goal-editor__header">
          <button
            type="button"
            class="adk-goal-editor__summary-button"
            :aria-expanded="goalEditorExpanded ? 'true' : 'false'"
            @click="goalEditorExpanded = !goalEditorExpanded"
          >
            <span class="adk-goal-editor__title">
              <span>目标</span>
              <span class="adk-goal-editor__count">1</span>
            </span>
            <span class="adk-goal-editor__badge" :class="goalObjectiveTone">
              {{ goalObjectiveStatus }}
            </span>
            <span
              class="adk-goal-editor__summary"
              :title="goalObjectiveSummary"
            >
              {{ goalObjectiveSummary }}
            </span>
            <span class="adk-goal-editor__toggle">
              {{ goalEditorExpanded ? "收起" : "展开" }}
            </span>
          </button>
          <span class="adk-goal-editor__icon-group">
            <button
              v-if="showGoalLifecycleButton"
              type="button"
              class="adk-goal-editor__action"
              :class="{ 'is-busy': goalLifecycleBusy || goalPauseRequested }"
              :title="goalLifecycleButtonLabel"
              :aria-label="goalLifecycleButtonLabel"
              :disabled="goalLifecycleButtonDisabled"
              @click="void handleGoalLifecycleAction()"
            >
              <v-icon size="12">{{ goalLifecycleButtonIcon }}</v-icon>
              <span>{{ goalLifecycleButtonLabel }}</span>
            </button>
            <button
              type="button"
              class="adk-goal-editor__icon"
              title="编辑目标"
              aria-label="编辑目标"
              @click="goalEditorExpanded = true"
            >
              <v-icon size="12">fa-solid fa-pen</v-icon>
            </button>
            <button
              type="button"
              class="adk-goal-editor__icon"
              title="取消目标"
              aria-label="取消目标"
              @click="void handleCancelGoalObjective()"
            >
              <v-icon size="12">fa-solid fa-arrow-rotate-left</v-icon>
            </button>
          </span>
        </div>
        <div v-if="goalEditorExpanded" class="adk-goal-editor__body">
          <span v-if="goalObjectiveError" class="adk-goal-editor__error">
            {{ goalObjectiveError }}
          </span>
          <v-textarea
            :model-value="goalObjectiveDraft"
            variant="plain"
            density="compact"
            :rows="2"
            auto-grow
            :max-rows="4"
            hide-details
            class="adk-goal-editor__input"
            @update:model-value="handleGoalObjectiveInput"
          />
          <v-btn
            v-if="activeRunId"
            size="small"
            variant="tonal"
            color="primary"
            :loading="goalObjectiveSaving"
            :disabled="!canSaveGoalObjective"
            @click="void updateGoalObjective?.()"
          >
            保存
          </v-btn>
        </div>
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
            <span class="adk-queue-item__text" :title="item.text">{{
              item.text
            }}</span>
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

      <div class="adk-composer-input-wrap">
        <v-textarea
          :model-value="chatDraft"
          :placeholder="placeholder"
          variant="plain"
          density="compact"
          :rows="1"
          auto-grow
          :max-rows="layout === 'mobile' ? 5 : 6"
          hide-details
          class="adk-composer-input"
          @update:model-value="$emit('update:chatDraft', $event ?? '')"
          @keydown="handleKeydown"
        />
        <div
          v-if="showSlashMenu"
          class="adk-slash-menu"
          :class="{ 'adk-slash-menu--mobile': layout === 'mobile' }"
        >
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

      <div
        v-if="isMobileLayout"
        class="adk-mobile-composer-summary"
        data-testid="adk-mobile-composer-summary"
      >
        <div class="adk-mobile-composer-summary__chips">
          <span
            class="adk-mobile-composer-summary__chip"
            :title="`Agent · ${selectedAgentLabel}`"
          >
            <span class="adk-mobile-composer-summary__label">Agent</span>
            <span class="adk-mobile-composer-summary__value">{{
              selectedAgentLabel
            }}</span>
          </span>
          <span
            class="adk-mobile-composer-summary__chip"
            :title="`模型 · ${selectedProviderLabel}`"
          >
            <span class="adk-mobile-composer-summary__label">模型</span>
            <span class="adk-mobile-composer-summary__value">{{
              selectedProviderLabel
            }}</span>
          </span>
          <span
            class="adk-mobile-composer-summary__chip"
            :title="`审批 · ${effectivePermissionOption.title}`"
          >
            <span class="adk-mobile-composer-summary__label">审批</span>
            <span class="adk-mobile-composer-summary__value">{{
              effectivePermissionOption.title
            }}</span>
          </span>
          <span
            class="adk-mobile-composer-summary__chip"
            :title="`模式 · ${selectedWorkModeLabel}`"
          >
            <span class="adk-mobile-composer-summary__label">模式</span>
            <span class="adk-mobile-composer-summary__value">{{
              selectedWorkModeLabel
            }}</span>
          </span>
          <span
            class="adk-mobile-composer-summary__chip adk-mobile-composer-summary__chip--context"
            :title="`上下文 · ${mobileContextSummaryLabel}`"
          >
            <span class="adk-mobile-composer-summary__label">上下文</span>
            <span class="adk-mobile-composer-summary__value">{{
              mobileContextSummaryLabel
            }}</span>
          </span>
        </div>
        <button
          type="button"
          class="adk-mobile-composer-summary__toggle"
          data-testid="adk-mobile-composer-toggle"
          :aria-expanded="mobileControlsExpanded ? 'true' : 'false'"
          @click="mobileControlsExpanded = !mobileControlsExpanded"
        >
          {{ mobileControlsExpanded ? "收起" : "设置" }}
        </button>
      </div>

      <div
        class="adk-composer-controls"
        :class="{
          'adk-composer-controls--mobile': isMobileLayout,
          'is-expanded': isMobileLayout && mobileControlsExpanded,
        }"
      >
        <div
          v-if="!isMobileLayout || mobileControlsExpanded"
          class="adk-composer-left"
          :data-testid="isMobileLayout ? 'adk-mobile-controls-panel' : undefined"
        >
          <v-btn
            icon="fa-solid fa-plus"
            variant="text"
            size="small"
            title="添加模型服务"
            @click="openProviderSettings?.()"
          />
          <v-menu location="top start">
            <template #activator="{ props: menuProps }">
              <button
                v-bind="menuProps"
                type="button"
                class="adk-permission-trigger"
                :class="`is-${effectivePermissionOption.tone}`"
                :title="`审批等级：${effectivePermissionOption.title}`"
              >
                <v-icon size="15">{{ effectivePermissionOption.icon }}</v-icon>
                <span>{{ effectivePermissionOption.title }}</span>
                <v-icon size="12">fa-solid fa-chevron-down</v-icon>
              </button>
            </template>
            <v-list class="adk-permission-menu" density="compact">
              <v-list-item
                v-for="option in permissionModeOptions"
                :key="option.value"
                class="adk-permission-option"
                :class="[
                  `is-${option.tone}`,
                  { 'is-selected': option.value === effectivePermissionMode },
                ]"
                @click="updatePermissionModeSelection(option.value)"
              >
                <template #prepend>
                  <v-icon size="16">{{ option.icon }}</v-icon>
                </template>
                <v-list-item-title>
                  {{ option.title }}
                  <v-chip
                    v-if="option.value === normalizedDefaultPermissionMode"
                    size="x-small"
                    variant="tonal"
                    class="ml-1"
                  >
                    默认
                  </v-chip>
                </v-list-item-title>
                <v-list-item-subtitle>{{ option.description }}</v-list-item-subtitle>
              </v-list-item>
            </v-list>
          </v-menu>
          <select
            class="adk-compat-select adk-agent-select"
            :value="selectedAgentId"
            tabindex="-1"
            aria-hidden="true"
            @change="
              updateAgentSelection(
                (($event.target as HTMLSelectElement | null)?.value ?? ''),
              )
            "
          >
            <option
              v-for="agent in agentOptions"
              :key="`compat-agent-${agent.value}`"
              :value="agent.value"
            >
              {{ agent.title }}
            </option>
          </select>
          <v-menu location="top start">
            <template #activator="{ props: menuProps }">
              <button
                v-bind="menuProps"
                type="button"
                class="adk-inline-trigger adk-agent-trigger"
                :title="`Agent：${selectedAgentLabel}`"
              >
                <span>{{ selectedAgentLabel }}</span>
                <v-icon size="12">fa-solid fa-chevron-down</v-icon>
              </button>
            </template>
            <v-list class="adk-compact-menu" density="compact">
              <v-list-item
                v-for="agent in agentOptions"
                :key="agent.value"
                :active="agent.value === selectedAgentId"
                @click="updateAgentSelection(agent.value)"
              >
                <v-list-item-title>{{ agent.title.split(" · ")[0] }}</v-list-item-title>
                <v-list-item-subtitle>{{ agent.title }}</v-list-item-subtitle>
              </v-list-item>
            </v-list>
          </v-menu>
          <v-menu location="top start">
            <template #activator="{ props: menuProps }">
              <button
                v-bind="menuProps"
                type="button"
                class="adk-inline-trigger adk-work-mode-trigger"
                :title="`模式：${selectedWorkModeLabel}`"
              >
                <span>{{ selectedWorkModeLabel }}</span>
                <v-icon size="12">fa-solid fa-chevron-down</v-icon>
              </button>
            </template>
            <v-list class="adk-compact-menu" density="compact">
              <v-list-item
                v-for="mode in workModeOptions"
                :key="mode.value"
                :active="mode.value === effectiveWorkModeSelection"
                @click="updateWorkModeSelection(mode.value)"
              >
                <v-list-item-title>
                  {{ mode.title }}
                  <span v-if="mode.isDefault" class="adk-sr-only">
                    {{ mode.title }}默认
                  </span>
                </v-list-item-title>
                <template #append>
                  <v-chip v-if="mode.isDefault" size="x-small" variant="tonal">
                    默认
                  </v-chip>
                </template>
              </v-list-item>
            </v-list>
          </v-menu>
          <select
            class="adk-compat-select adk-work-mode-select"
            :value="effectiveWorkModeSelection"
            tabindex="-1"
            aria-hidden="true"
            @change="
              updateWorkModeSelection(
                (($event.target as HTMLSelectElement | null)?.value ?? ''),
              )
            "
          >
            <option
              v-for="mode in workModeOptions"
              :key="`compat-mode-${mode.value}`"
              :value="mode.value"
            >
              {{ mode.title }}
            </option>
          </select>
        </div>

        <div class="adk-composer-right">
          <div
            v-if="!isMobileLayout || mobileControlsExpanded"
            class="adk-composer-utility"
          >
            <v-menu
              v-if="showContextControl"
              v-model="contextMenuOpen"
              location="top start"
              :close-on-content-click="false"
              open-on-hover
            >
              <template #activator="{ props: menuProps }">
                <button
                  v-bind="menuProps"
                  type="button"
                  class="adk-context-ring adk-context-pill"
                  :class="`is-${contextTone}`"
                  :title="`上下文：${contextPillLabel}`"
                  @click="openContextPopover"
                >
                  <v-progress-circular
                    :model-value="contextProgressValue"
                    :indeterminate="contextBusy && !hasKnownContextWindow"
                    :color="contextProgressColor"
                    size="24"
                    width="4"
                  />
                  <span class="adk-sr-only">{{ contextPillLabel }}</span>
                </button>
              </template>

              <v-card min-width="360" class="adk-context-card">
                <v-card-title class="text-subtitle-2">
                  上下文使用情况
                </v-card-title>
                <v-card-text class="adk-context-card__body">
                  <div class="adk-context-stat">
                    <span>当前上下文 Token</span>
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
                    <strong>{{
                      contextSnapshot?.activeHandoffCount ?? 0
                    }}</strong>
                  </div>
                  <div class="adk-context-stat">
                    <span>当前上下文版本</span>
                    <strong>{{ contextRevisionLabel(contextSnapshot) }}</strong>
                  </div>
                  <div class="adk-context-stat">
                    <span>已压缩事件数</span>
                    <strong>{{
                      contextSnapshot?.compactedEventCount ?? 0
                    }}</strong>
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
                  <template v-if="rawContextDiagnosticsVisible">
                    <div class="adk-context-stat">
                      <span>原始会话当前估算</span>
                      <strong>{{
                        formatTokenCount(
                          contextSnapshot?.rawCurrentInputTokens ??
                            contextSnapshot?.currentInputTokens ??
                            0,
                        )
                      }}</strong>
                    </div>
                    <div class="adk-context-stat">
                      <span>原始会话下一轮估算</span>
                      <strong>{{
                        formatTokenCount(
                          contextSnapshot?.rawProjectedNextTurnTokens ??
                            contextSnapshot?.projectedNextTurnTokens ??
                            0,
                        )
                      }}</strong>
                    </div>
                    <div class="adk-context-stat">
                      <span>已裁剪工具响应</span>
                      <strong>{{
                        contextSnapshot?.trimmedToolResponseCount ?? 0
                      }}</strong>
                    </div>
                  </template>
                  <div class="adk-context-breakdown">
                    <div class="adk-context-summary__title">
                      当前上下文 Token 构成
                    </div>
                    <div
                      v-for="item in breakdownRows"
                      :key="item.label"
                      class="adk-context-stat"
                    >
                      <span>{{ item.label }}</span>
                      <strong>{{ formatTokenCount(item.value) }}</strong>
                    </div>
                  </div>
                  <div
                    v-if="
                      rawContextDiagnosticsVisible &&
                      rawBreakdownRows.length > 0
                    "
                    class="adk-context-breakdown"
                  >
                    <div class="adk-context-summary__title">
                      原始会话诊断 Token 构成
                    </div>
                    <div
                      v-for="item in rawBreakdownRows"
                      :key="`page-raw-${item.label}`"
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
                    v-if="contextSnapshot?.contextRevisionCreatedAt"
                    class="adk-context-summary"
                  >
                    <div class="adk-context-summary__title">版本创建时间</div>
                    <div class="adk-context-summary__content">
                      {{ contextSnapshot.contextRevisionCreatedAt }}
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

            <v-menu location="top end">
              <template #activator="{ props: menuProps }">
                <button
                  v-bind="menuProps"
                  type="button"
                  class="adk-inline-trigger adk-provider-trigger"
                  :title="`模型：${selectedProviderTitle}`"
                  :disabled="
                    selectedAgentId === '' ||
                    providerOptions.length === 0 ||
                    savingProviderSelection
                  "
                >
                  <span>{{ selectedProviderLabel }}</span>
                  <v-progress-circular
                    v-if="savingProviderSelection"
                    indeterminate
                    size="14"
                    width="2"
                  />
                  <v-icon v-else size="12">fa-solid fa-chevron-down</v-icon>
                </button>
              </template>
              <v-list class="adk-compact-menu adk-provider-menu" density="compact">
                <v-list-item
                  v-for="provider in providerOptions"
                  :key="provider.value"
                  :active="provider.value === selectedProviderId"
                  @click="updateProviderSelection(provider.value)"
                >
                  <v-list-item-title class="adk-provider-menu__title">
                    <span>{{ provider.model || provider.title.split(" · ")[1] || provider.title.split(" · ")[0] }}</span>
                    <v-chip
                      v-if="provider.isDefault"
                      size="x-small"
                      color="primary"
                      variant="tonal"
                    >
                      默认
                    </v-chip>
                  </v-list-item-title>
                  <v-list-item-subtitle>{{ provider.title }}</v-list-item-subtitle>
                </v-list-item>
              </v-list>
            </v-menu>
            <select
              class="adk-compat-select adk-provider-select"
              :value="selectedProviderId"
              :disabled="
                selectedAgentId === '' ||
                providerOptions.length === 0 ||
                savingProviderSelection
              "
              tabindex="-1"
              aria-hidden="true"
              @change="
                updateProviderSelection(
                  (($event.target as HTMLSelectElement | null)?.value ?? ''),
                )
              "
            >
              <option
                v-for="provider in providerOptions"
                :key="`compat-provider-${provider.value}`"
                :value="provider.value"
              >
                {{ provider.title }}
              </option>
            </select>

            <v-btn
              icon="fa-solid fa-gear"
              variant="text"
              size="small"
              title="Agent 设置"
              @click="openProviderSettings?.()"
            />
          </div>

          <div class="adk-composer-actions">
            <v-progress-linear
              v-if="loading"
              indeterminate
              rounded
              color="primary"
              class="adk-inline-progress"
            />
            <v-btn
              v-if="showStopButton"
              icon="fa-solid fa-stop"
              variant="tonal"
              color="error"
              size="small"
              title="停止运行"
              aria-label="停止运行"
              class="adk-composer-stop"
              @click="cancelActiveRun?.()"
            />
            <v-btn
              v-if="showInterruptButton"
              icon
              class="adk-composer-interrupt"
              variant="tonal"
              color="warning"
              size="small"
              title="打断后发送"
              aria-label="打断后发送"
              :disabled="!canInterruptChat"
              @click="void interruptAndQueueChat?.()"
            >
              <v-icon size="15">fa-solid fa-level-down-alt</v-icon>
              <span class="adk-sr-only">打断后发送</span>
            </v-btn>
            <v-btn
              icon
              color="primary"
              size="small"
              :loading="sendButtonLoading"
              :disabled="!canSendChat"
              title="发送"
              aria-label="发送"
              class="adk-composer-send"
              @click="void handlePrimaryAction()"
            >
              <v-icon size="14">fa-solid fa-paper-plane</v-icon>
              <span class="adk-sr-only">发送</span>
            </v-btn>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.adk-mobile-composer-summary {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 8px 8px;
}

.adk-mobile-composer-summary__chips {
  min-width: 0;
  flex: 1 1 auto;
  display: flex;
  gap: 6px;
  overflow-x: auto;
  scrollbar-width: none;
}

.adk-mobile-composer-summary__chips::-webkit-scrollbar {
  display: none;
}

.adk-mobile-composer-summary__chip {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  min-width: 0;
  max-width: 168px;
  padding: 4px 8px;
  border: 1px solid color-mix(in srgb, var(--tv-border) 86%, var(--tv-accent) 14%);
  border-radius: 999px;
  background: color-mix(in srgb, var(--tv-bg-surface) 90%, transparent);
  color: var(--tv-text-muted);
  flex: 0 0 auto;
  font-size: 11px;
  line-height: 1.2;
}

.adk-mobile-composer-summary__chip--context {
  max-width: 176px;
}

.adk-mobile-composer-summary__label {
  flex: 0 0 auto;
  color: var(--tv-text-dim);
}

.adk-mobile-composer-summary__value {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--tv-text);
}

.adk-mobile-composer-summary__toggle {
  flex: 0 0 auto;
  min-height: 28px;
  padding: 0 10px;
  border: 1px solid var(--adk-accent-border);
  border-radius: 999px;
  background: var(--adk-accent-bg-soft);
  color: var(--adk-accent-fg);
  font-size: 11px;
  font-weight: 600;
  cursor: pointer;
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

.adk-goal-editor {
  display: grid;
  gap: 8px;
  margin-bottom: 10px;
  padding: 10px 12px;
  border: 1px solid var(--tv-border);
  border-radius: 8px;
  background: color-mix(in srgb, var(--tv-bg-surface) 88%, transparent);
  color: var(--tv-text);
}

.adk-goal-editor.is-expanded {
  border-color: color-mix(in srgb, var(--tv-accent) 34%, var(--tv-border));
  background: color-mix(in srgb, var(--tv-accent) 7%, var(--tv-bg-surface));
}

.adk-goal-editor__header {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 10px;
  align-items: center;
}

.adk-goal-editor__summary-button {
  display: grid;
  grid-template-columns: auto auto minmax(0, 1fr) auto;
  gap: 8px;
  align-items: center;
  min-width: 0;
  border: 0;
  padding: 0;
  background: transparent;
  color: inherit;
  cursor: pointer;
  text-align: left;
}

.adk-goal-editor__title {
  display: inline-flex;
  gap: 6px;
  align-items: center;
  min-width: 0;
  font-size: 12px;
  font-weight: 700;
  color: var(--tv-text);
}

.adk-goal-editor__count {
  min-width: 20px;
  padding: 1px 6px;
  border-radius: 999px;
  background: color-mix(in srgb, var(--tv-text-dim) 18%, transparent);
  color: var(--tv-text-muted);
  font-size: 11px;
  line-height: 1.5;
  text-align: center;
}

.adk-goal-editor__badge {
  justify-self: start;
  padding: 2px 8px;
  border-radius: 999px;
  font-size: 11px;
  line-height: 1.4;
}

.adk-goal-editor__badge.is-muted {
  color: var(--adk-muted-fg);
  background: var(--adk-muted-bg);
}

.adk-goal-editor__badge.is-info {
  color: var(--adk-info-fg);
  background: var(--adk-info-bg);
}

.adk-goal-editor__badge.is-warning {
  color: var(--adk-warning-fg);
  background: var(--adk-warning-bg);
}

.adk-goal-editor__badge.is-error {
  color: var(--adk-error-fg);
  background: var(--adk-error-bg);
}

.adk-goal-editor__summary {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--tv-text-muted);
  font-size: 12px;
}

.adk-goal-editor__toggle {
  color: var(--adk-accent-fg);
  font-size: 12px;
  white-space: nowrap;
}

.adk-goal-editor__icon-group {
  display: inline-flex;
  gap: 6px;
  align-items: center;
}

.adk-goal-editor__icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  border: 1px solid var(--adk-accent-border);
  border-radius: 999px;
  background: var(--adk-accent-bg-soft);
  color: var(--adk-accent-fg);
  cursor: pointer;
}

.adk-goal-editor__action {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  min-width: 76px;
  height: 26px;
  padding: 0 10px;
  border: 1px solid var(--adk-accent-border);
  border-radius: 999px;
  background: var(--adk-accent-bg);
  color: var(--adk-accent-fg);
  font-size: 12px;
  font-weight: 600;
  line-height: 1;
  white-space: nowrap;
  cursor: pointer;
}

.adk-goal-editor__icon:disabled,
.adk-goal-editor__action:disabled {
  cursor: default;
  opacity: 0.55;
}

.adk-goal-editor__icon.is-busy,
.adk-goal-editor__action.is-busy {
  color: var(--adk-warning-fg);
  background: var(--adk-warning-bg);
}

.adk-goal-editor__body {
  display: flex;
  gap: 10px;
  align-items: center;
}

.adk-goal-editor__error {
  flex: 0 0 auto;
  max-width: 180px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12px;
  color: var(--adk-error-fg);
}

.adk-goal-editor__input {
  min-width: 0;
  flex: 1 1 auto;
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
  color: var(--adk-queue-queued-fg);
  background: var(--adk-queue-queued-bg);
}

.adk-queue-item__badge.is-queued {
  color: var(--adk-queue-queued-fg);
  background: var(--adk-queue-queued-bg);
}

.adk-queue-item__badge.is-interrupt,
.adk-queue-item__badge.is-interrupting {
  color: var(--adk-queue-interrupt-fg);
  background: var(--adk-queue-interrupt-bg);
}

.adk-queue-item__badge.is-sending-next {
  color: var(--adk-queue-sending-fg);
  background: var(--adk-queue-sending-bg);
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
  color: var(--adk-danger-fg);
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

.adk-permission-trigger {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 7px;
  min-height: 32px;
  padding: 0 7px;
  border: 0;
  border-radius: 9px;
  background: transparent;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
}

.adk-permission-trigger:hover {
  background: rgba(148, 163, 184, 0.12);
}

.adk-inline-trigger {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  min-width: 0;
  max-width: 210px;
  min-height: 32px;
  padding: 0 7px;
  border: 0;
  border-radius: 9px;
  background: transparent;
  color: var(--tv-text-muted);
  font-size: 13px;
  line-height: 1;
  cursor: pointer;
}

.adk-inline-trigger:hover,
.adk-inline-trigger[aria-expanded="true"] {
  background: rgba(148, 163, 184, 0.12);
  color: var(--tv-text);
}

.adk-inline-trigger:disabled {
  cursor: default;
  opacity: 0.5;
}

.adk-inline-trigger > span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.adk-compat-select,
.adk-sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}

.adk-agent-trigger {
  max-width: clamp(96px, 18vw, 180px);
}

.adk-provider-trigger {
  max-width: clamp(92px, 19vw, 220px);
}

.adk-work-mode-trigger {
  max-width: 92px;
}

.adk-context-ring {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 34px;
  height: 34px;
  border: 0;
  border-radius: 999px;
  background: transparent;
  color: var(--tv-text-muted);
  cursor: pointer;
}

.adk-context-ring:hover,
.adk-context-ring[aria-expanded="true"] {
  background: rgba(148, 163, 184, 0.12);
}

.adk-context-ring.is-warning {
  color: var(--adk-warning-fg);
}

.adk-context-ring.is-error {
  color: var(--adk-error-fg);
}

.adk-compact-menu {
  min-width: 220px;
  max-width: min(360px, 92vw);
  padding: 6px;
  border-radius: 12px;
}

.adk-provider-menu {
  min-width: 280px;
}

.adk-provider-menu__title {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.adk-provider-menu__title > span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.adk-permission-trigger.is-approval,
.adk-permission-option.is-approval :deep(.v-icon) {
  color: rgb(22 163 74);
}

.adk-permission-trigger.is-less,
.adk-permission-option.is-less :deep(.v-icon) {
  color: rgb(217 119 6);
}

.adk-permission-trigger.is-all,
.adk-permission-option.is-all :deep(.v-icon) {
  color: rgb(220 38 38);
}

.adk-permission-menu {
  min-width: 310px;
  padding: 6px;
  border-radius: 12px;
}

.adk-permission-option {
  margin: 2px 0;
  border-radius: 9px;
}

.adk-permission-option.is-selected {
  background: rgba(148, 163, 184, 0.12);
}

.adk-composer-stop,
.adk-composer-interrupt,
.adk-composer-send {
  flex: 0 0 38px;
  width: 38px;
  height: 38px;
  min-width: 38px;
  border-radius: 999px;
  padding: 0;
  text-transform: none;
}

.adk-composer-stop :deep(.v-btn__content),
.adk-composer-interrupt :deep(.v-btn__content),
.adk-composer-send :deep(.v-btn__content) {
  gap: 0;
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

.adk-slash-menu--mobile {
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

.adk-composer-controls--mobile {
  align-items: stretch;
  flex-direction: column;
}

.adk-composer-utility,
.adk-composer-actions {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

.adk-composer--mobile .adk-composer-left,
.adk-composer--mobile .adk-composer-right {
  width: 100%;
  gap: 8px;
  flex-wrap: wrap;
}

.adk-composer--mobile .adk-composer-controls {
  gap: 8px;
  padding: 0 8px 8px;
}

.adk-composer--mobile .adk-permission-trigger {
  max-width: 148px;
}

.adk-composer--mobile .adk-agent-trigger,
.adk-composer--mobile .adk-provider-trigger {
  flex: 1 1 130px;
  max-width: none;
}

.adk-composer--mobile .adk-work-mode-trigger {
  flex: 0 0 auto;
}

.adk-composer--mobile .adk-context-ring {
  flex: 0 0 34px;
}

.adk-composer--mobile .adk-inline-progress {
  flex: 1 1 100%;
  width: 100%;
  max-width: none;
}

.adk-composer--mobile .adk-composer-utility,
.adk-composer--mobile .adk-composer-actions {
  width: 100%;
}

.adk-composer--mobile .adk-composer-actions {
  justify-content: flex-end;
}

.adk-composer--mobile .adk-goal-editor {
  margin: 0 8px 8px;
  padding: 8px 10px;
  border-radius: 14px;
  gap: 6px;
}

.adk-composer--mobile .adk-goal-editor__header,
.adk-composer--mobile .adk-goal-editor__summary-button,
.adk-composer--mobile .adk-goal-editor__icon-group,
.adk-composer--mobile .adk-goal-editor__body {
  gap: 6px;
}

.adk-composer--mobile .adk-goal-editor__body {
  flex-wrap: wrap;
}

.adk-composer--mobile .adk-queue-strip {
  margin: 0 8px 8px;
  padding: 8px 10px;
  border-radius: 12px;
}

.adk-composer--mobile .adk-queue-item {
  gap: 6px;
}

.adk-composer--mobile .adk-composer-stop,
.adk-composer--mobile .adk-composer-interrupt,
.adk-composer--mobile .adk-composer-send {
  flex: 0 0 auto;
}

.adk-composer--mobile .adk-slash-menu__item {
  grid-template-columns: 1fr;
}
</style>
