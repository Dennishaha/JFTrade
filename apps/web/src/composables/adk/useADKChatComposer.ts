import {
  computed,
  inject,
  ref,
  toRefs,
  watch,
  type InjectionKey,
} from "vue";

import type {
  ADKAgent,
  ADKPermissionMode,
  ADKProvider,
  ADKSessionContextSnapshot,
  ADKWorkMode,
} from "@/types";

import type { QueuedChatMessage } from "@/composables/adk/adkChatRuntime";
import {
  compactionModeLabel,
  contextRevisionLabel,
  contextWindowLabel,
  formatTokenCount,
} from "@/composables/adk/adkChatComposerPresentation";

export interface SlashCommandItem {
  id: "context" | "compact" | "compact-aggressive";
  command: string;
  title: string;
  description: string;
  disabled?: boolean;
}
export interface ProviderOption {
  title: string;
  value: string;
  providerId?: string;
  displayName?: string;
  model?: string;
  isDefault?: boolean;
}

export interface ADKChatComposerProps {
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

}

export interface ADKChatComposerEmit {
  (event: "update:chatDraft", value: string): void;
  (event: "update:contextDetailsOpen", value: boolean): void;
  (event: "update:selectedAgentId", value: string): void;
  (event: "update:selectedProviderId", value: string): void;
  (event: "update:permissionModeOverride", value: string): void;
  (event: "update:workModeOverride", value: string): void;
}

type ADKChatComposerResolvedKey =
  | "layout"
  | "activeRunId"
  | "activeRunStatus"
  | "agentOptions"
  | "canInterruptChat"
  | "canSendChat"
  | "chatDraft"
  | "composerBlockMessage"
  | "contextBusy"
  | "contextDetailsOpen"
  | "contextSnapshot"
  | "goalObjectiveDraft"
  | "goalObjectiveError"
  | "goalObjectiveSaving"
  | "goalLifecycleBusy"
  | "goalPaused"
  | "goalTimedOut"
  | "goalPauseRequested"
  | "showGoalObjectiveEditor"
  | "canSaveGoalObjective"
  | "canPauseGoal"
  | "canResumeGoal"
  | "hasBlockingRun"
  | "interruptingRunId"
  | "loading"
  | "placeholder"
  | "providerOptions"
  | "queuedMessages"
  | "queueDispatchingId"
  | "savingProviderSelection"
  | "selectedAgent"
  | "selectedAgentId"
  | "selectedSessionId"
  | "selectedProvider"
  | "selectedProviderId"
  | "sendingChat"
  | "slashCommands"
  | "suggestions"
  | "defaultWorkMode"
  | "defaultPermissionMode"
  | "permissionModeOverride"
  | "workModeOverride"
  | "sendChat";

type ADKChatComposerRuntimeProps = Readonly<
  { [Key in keyof ADKChatComposerProps]: ADKChatComposerProps[Key] | undefined } &
  {
    [Key in ADKChatComposerResolvedKey]-?: Exclude<
      ADKChatComposerProps[Key],
      undefined
    >;
  }
>;

export function useADKChatComposer(
  props: ADKChatComposerRuntimeProps,
  emit: ADKChatComposerEmit,
) {
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
  if (props.goalPaused || props.goalTimedOut) {
    await props.resumeGoalRun?.();
    return;
  }
  await props.pauseGoalRun?.();
}

function updateChatDraft(value: string | null): void {
  emit("update:chatDraft", value ?? "");
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

  return {
    ...toRefs(props),
    mobileControlsExpanded,
    selectedSlashIndex,
    dismissedSlashDraft,
    goalEditorExpanded,
    isMobileLayout,
    supportedWorkModes,
    normalizedDefaultWorkMode,
    workModeOptions,
    effectiveWorkModeSelection,
    permissionModeOptions,
    normalizedDefaultPermissionMode,
    effectivePermissionMode,
    effectivePermissionOption,
    contextMenuOpen,
    slashDraft,
    filteredSlashCommands,
    showSlashMenu,
    selectedSlashCommand,
    exactSlashCommand,
    queueItems,
    goalObjectiveSummary,
    goalObjectiveStatus,
    goalObjectiveTone,
    showGoalLifecycleButton,
    goalLifecycleButtonLabel,
    goalLifecycleButtonIcon,
    goalLifecycleButtonDisabled,
    sendButtonLoading,
    showInterruptButton,
    showStopButton,
    contextTone,
    hasContextUsage,
    showContextControl,
    hasKnownContextWindow,
    contextStatusLabel,
    contextPercent,
    contextProgressValue,
    contextProgressColor,
    contextPillLabel,
    selectedAgentLabel,
    selectedProviderLabel,
    selectedProviderTitle,
    selectedWorkModeLabel,
    mobileContextSummaryLabel,
    contextSummaryPreview,
    breakdownRows,
    rawBreakdownRows,
    rawContextDiagnosticsVisible,
    updatePermissionModeSelection,
    updateWorkModeSelection,
    updateAgentSelection,
    updateProviderSelection,
    openContextPopover,
    handlePrimaryAction,
    handleCancelGoalObjective,
    handleGoalLifecycleAction,
    updateChatDraft,
    handleGoalObjectiveInput,
    executeSlashCommand,
    dismissSlashMenu,
    handleKeydown,
    formatTokenCount,
    contextWindowLabel,
    contextRevisionLabel,
    compactionModeLabel,
    queueItemBadge,
    queueItemStateClass,
    canRevokeQueueItem,
  };
}

export type ADKChatComposerContext = ReturnType<typeof useADKChatComposer>;

export const adkChatComposerContextKey: InjectionKey<ADKChatComposerContext> =
  Symbol("adk-chat-composer-context");

export function useADKChatComposerContext(): ADKChatComposerContext {
  const context = inject(adkChatComposerContextKey);
  if (context == null) {
    throw new Error("ADK chat composer context is unavailable");
  }
  return context;
}
