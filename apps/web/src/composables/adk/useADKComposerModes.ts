import { computed, ref, watch } from "vue";

import type {
  ADKProvider,
  ADKPermissionMode,
  ADKReasoningEffort,
  ADKWorkMode,
} from "@/types";
import {
  ADK_REASONING_EFFORT_LABELS,
  ADK_REASONING_EFFORTS,
  isADKReasoningEffortSupported,
  supportedADKReasoningEfforts,
} from "@/composables/adk/adkReasoning";

interface ComposerModeProps {
  defaultWorkMode: ADKWorkMode | string;
  defaultPermissionMode: ADKPermissionMode | string;
  defaultReasoningEffort: ADKReasoningEffort | string;
  selectedProvider?: ADKProvider | null;
  permissionModeOverride: string;
  reasoningEffortOverride: ADKReasoningEffort | "" | string;
  workModeOverride: string;
}

interface PermissionModeOption {
  title: string;
  value: ADKPermissionMode;
  icon: string;
  tone: "approval" | "less" | "all";
  description: string;
}

type ModeEmit = {
  (event: "update:permissionModeOverride", value: string): void;
  (event: "update:reasoningEffortOverride", value: ADKReasoningEffort | ""): void;
  (event: "update:workModeOverride", value: string): void;
};

const supportedWorkModes: Array<{ title: string; value: ADKWorkMode }> = [
  { title: "对话", value: "chat" },
  { title: "目标", value: "loop" },
];

const permissionModeOptions: PermissionModeOption[] = [
  {
    title: "请求批准",
    value: "approval",
    icon: "fa-solid fa-shield-halved",
    tone: "approval",
    description: "低风险读取自动执行，中风险及以上操作请求确认",
  },
  {
    title: "减少审批",
    value: "less_approval",
    icon: "fa-solid fa-shield",
    tone: "less",
    description: "减少普通写入和优化操作的确认次数，交易仍需确认",
  },
  {
    title: "完全访问",
    value: "all",
    icon: "fa-solid fa-triangle-exclamation",
    tone: "all",
    description: "读取和普通写入自动执行，实盘下单与撤单仍逐次确认",
  },
];

const reasoningEffortDescriptions: Record<ADKReasoningEffort, string> = {
  low: "优先降低延迟和推理消耗",
  medium: "平衡质量、延迟和消耗",
  high: "复杂问题优先提高推理深度",
  xhigh: "困难问题投入更多推理",
  max: "最困难问题优先质量，延迟和消耗最高",
};

const allReasoningEffortOptions = ADK_REASONING_EFFORTS.map((value) => ({
  title: ADK_REASONING_EFFORT_LABELS[value],
  value,
  description: reasoningEffortDescriptions[value],
}));

type ReasoningEffortOption = {
  title: string;
  value: ADKReasoningEffort | "";
  description: string;
};

function normalizeReasoningEffort(value: string): ADKReasoningEffort | "" {
  if (ADK_REASONING_EFFORTS.includes(value as ADKReasoningEffort)) {
    return value as ADKReasoningEffort;
  }
  return "";
}

export function useADKComposerModes(props: ComposerModeProps, emit: ModeEmit) {
  const reasoningOverrideNotice = ref("");
  const normalizedDefaultWorkMode = computed<ADKWorkMode>(() =>
    props.defaultWorkMode === "loop" ? "loop" : "chat",
  );
  const workModeOptions = computed(() =>
    supportedWorkModes.map((mode) => ({
      ...mode,
      isDefault: mode.value === normalizedDefaultWorkMode.value,
    })),
  );
  const effectiveWorkModeSelection = computed(
    () => props.workModeOverride || normalizedDefaultWorkMode.value,
  );
  const normalizedDefaultPermissionMode = computed<ADKPermissionMode>(() => {
    if (props.defaultPermissionMode === "less_approval" || props.defaultPermissionMode === "all") {
      return props.defaultPermissionMode;
    }
    return "approval";
  });
  const effectivePermissionMode = computed<ADKPermissionMode>(() => {
    if (props.permissionModeOverride === "less_approval" || props.permissionModeOverride === "all") {
      return props.permissionModeOverride;
    }
    return props.permissionModeOverride === "approval"
      ? "approval"
      : normalizedDefaultPermissionMode.value;
  });
  const effectivePermissionOption = computed(
    () => permissionModeOptions.find((option) => option.value === effectivePermissionMode.value) ?? permissionModeOptions[0]!,
  );
  const normalizedDefaultReasoningEffort = computed<ADKReasoningEffort | "">(() =>
    normalizeReasoningEffort(props.defaultReasoningEffort),
  );
  const supportedReasoningEfforts = computed(() =>
    props.selectedProvider
      ? supportedADKReasoningEfforts(props.selectedProvider)
      : [],
  );
  const reasoningEffortOptions = computed<ReasoningEffortOption[]>(() => [
    {
      title: "跟随 Agent",
      value: "",
      description: "使用 Agent 默认等级，空值表示不发送推理参数",
    },
    ...allReasoningEffortOptions.filter((option) =>
      supportedReasoningEfforts.value.includes(option.value),
    ),
  ]);
  const effectiveReasoningEffort = computed<ADKReasoningEffort | "">(() => {
    const override = normalizeReasoningEffort(props.reasoningEffortOverride);
    if (override !== "" && isADKReasoningEffortSupported(props.selectedProvider ?? undefined, override)) {
      return override;
    }
    return normalizedDefaultReasoningEffort.value;
  });
  const effectiveReasoningOption = computed(
    () => reasoningEffortOptions.value.find((option) => option.value === effectiveReasoningEffort.value) ?? reasoningEffortOptions.value[0]!,
  );

  watch(
    () => [props.selectedProvider?.id ?? "", normalizeReasoningEffort(props.reasoningEffortOverride)] as const,
    ([, override]) => {
      if (
        override !== "" &&
        !isADKReasoningEffortSupported(props.selectedProvider ?? undefined, override)
      ) {
        reasoningOverrideNotice.value = "当前 Provider 不支持已选择的推理等级，已恢复为跟随 Agent。";
        emit("update:reasoningEffortOverride", "");
      } else if (override !== "") {
        reasoningOverrideNotice.value = "";
      }
    },
  );

  const updatePermissionModeSelection = (mode: ADKPermissionMode) => emit(
    "update:permissionModeOverride",
    mode === normalizedDefaultPermissionMode.value ? "" : mode,
  );
  const updateReasoningEffortSelection = (effort: ADKReasoningEffort | "") => {
    reasoningOverrideNotice.value = "";
    emit(
      "update:reasoningEffortOverride",
      effort === "" || effort === normalizedDefaultReasoningEffort.value ? "" : effort,
    );
  };
  const updateWorkModeSelection = (mode?: string | null) => emit(
    "update:workModeOverride",
    mode === normalizedDefaultWorkMode.value ? "" : (mode ?? ""),
  );

  return {
    supportedWorkModes,
    normalizedDefaultWorkMode,
    workModeOptions,
    effectiveWorkModeSelection,
    permissionModeOptions,
    normalizedDefaultPermissionMode,
    effectivePermissionMode,
    effectivePermissionOption,
    reasoningEffortOptions,
    supportedReasoningEfforts,
    normalizedDefaultReasoningEffort,
    effectiveReasoningEffort,
    effectiveReasoningOption,
    reasoningOverrideNotice,
    updatePermissionModeSelection,
    updateReasoningEffortSelection,
    updateWorkModeSelection,
  };
}
