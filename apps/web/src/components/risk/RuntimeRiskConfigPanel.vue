<script setup lang="ts">
import { computed, reactive, watch } from "vue";

import type { RealTradeRiskStateResponse } from "@/contracts";

const props = defineProps<{
  loading?: boolean;
  riskState: RealTradeRiskStateResponse;
}>();

const emit = defineEmits<{
  disable: [payload: { operatorId: string; reason: string }];
  save: [
    payload: {
      realTradingEnabled: boolean;
      maxOrderQuantity: number | null;
      maxOrderNotional: number | null;
      operatorId: string;
      reason: string;
    },
  ];
}>();

const form = reactive({
  realTradingEnabled: false,
  maxOrderQuantity: "",
  maxOrderNotional: "",
  operatorId: "local",
  reason: "",
});

watch(
  () => props.riskState,
  (state) => {
    form.realTradingEnabled = state.realTradingEnabled;
    form.maxOrderQuantity =
      state.runtimeConfiguredMaxOrderQuantity == null
        ? ""
        : String(state.runtimeConfiguredMaxOrderQuantity);
    form.maxOrderNotional =
      state.runtimeConfiguredMaxOrderNotional == null
        ? ""
        : String(state.runtimeConfiguredMaxOrderNotional);
  },
  { immediate: true },
);

const parsedQuantity = computed(() => parseNullableNumber(form.maxOrderQuantity));
const parsedNotional = computed(() => parseNullableNumber(form.maxOrderNotional));
const realTradingEnabledChanged = computed(
  () => form.realTradingEnabled !== props.riskState.realTradingEnabled,
);
const maxOrderQuantityChanged = computed(
  () =>
    !Object.is(
      parsedQuantity.value,
      props.riskState.runtimeConfiguredMaxOrderQuantity,
    ),
);
const maxOrderNotionalChanged = computed(
  () =>
    !Object.is(
      parsedNotional.value,
      props.riskState.runtimeConfiguredMaxOrderNotional,
    ),
);
const hasValidLimit = computed(
  () =>
    (parsedQuantity.value != null && parsedQuantity.value > 0) ||
    (parsedNotional.value != null && parsedNotional.value > 0),
);
const formError = computed(() => {
  if (parsedQuantity.value === Number.NEGATIVE_INFINITY) {
    return "单笔最大数量需要是正数。";
  }
  if (parsedNotional.value === Number.NEGATIVE_INFINITY) {
    return "单笔最大金额需要是正数。";
  }
  if (form.realTradingEnabled && !hasValidLimit.value) {
    return "启用实盘前至少填写一个正数限额。";
  }
  return "";
});
const canSave = computed(() => formError.value === "");

function parseNullableNumber(value: string): number | null {
  const trimmed = value.trim();
  if (trimmed === "") return null;
  const parsed = Number(trimmed);
  if (!Number.isFinite(parsed) || parsed <= 0) return Number.NEGATIVE_INFINITY;
  return parsed;
}

function formatCurrentLimit(value: number | null): string {
  return value == null ? "未设置" : String(value);
}

function submit() {
  if (!canSave.value) return;
  emit("save", {
    realTradingEnabled: form.realTradingEnabled,
    maxOrderQuantity: parsedQuantity.value,
    maxOrderNotional: parsedNotional.value,
    operatorId: form.operatorId.trim() || "local",
    reason: form.reason.trim(),
  });
}

function disable() {
  emit("disable", {
    operatorId: form.operatorId.trim() || "local",
    reason: form.reason.trim(),
  });
}
</script>

<template>
  <v-card flat class="card-shell border-0">
    <div class="px-4 pt-4">
      <div>
        <div class="text-xl font-semibold text-slate-900">实盘总闸与单笔限额</div>
        <div class="mt-1 text-sm text-slate-500">
          这些值立即写入运行时配置，保存后不需要重启。
        </div>
      </div>
    </div>

    <v-card-text>
      <div class="grid gap-4">
        <div class="grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center">
          <v-switch
            v-model="form.realTradingEnabled"
            color="teal"
            hide-details
            label="允许实盘下单"
          />
          <div
            class="flex items-center gap-2 text-xs text-slate-500"
            data-status-for="real-trading-enabled"
          >
            <span>当前：{{ riskState.realTradingEnabled ? "开启" : "关闭" }}</span>
            <v-chip
              v-if="realTradingEnabledChanged"
              color="warning"
              size="x-small"
              variant="tonal"
            >
              已修改
            </v-chip>
          </div>
        </div>

        <div class="grid gap-3 sm:grid-cols-2">
          <div class="grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center">
            <v-text-field
              v-model="form.maxOrderQuantity"
              density="compact"
              hide-details
              inputmode="decimal"
              label="单笔最大数量"
              variant="outlined"
            />
            <div
              class="flex items-center gap-2 text-xs text-slate-500"
              data-status-for="max-order-quantity"
            >
              <span>
                当前：{{ formatCurrentLimit(riskState.runtimeConfiguredMaxOrderQuantity) }}
              </span>
              <v-chip
                v-if="maxOrderQuantityChanged"
                color="warning"
                size="x-small"
                variant="tonal"
              >
                已修改
              </v-chip>
            </div>
          </div>
          <div class="grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center">
            <v-text-field
              v-model="form.maxOrderNotional"
              density="compact"
              hide-details
              inputmode="decimal"
              label="单笔最大金额"
              variant="outlined"
            />
            <div
              class="flex items-center gap-2 text-xs text-slate-500"
              data-status-for="max-order-notional"
            >
              <span>
                当前：{{ formatCurrentLimit(riskState.runtimeConfiguredMaxOrderNotional) }}
              </span>
              <v-chip
                v-if="maxOrderNotionalChanged"
                color="warning"
                size="x-small"
                variant="tonal"
              >
                已修改
              </v-chip>
            </div>
          </div>
        </div>

        <div class="grid gap-3 sm:grid-cols-2">
          <v-text-field
            v-model="form.operatorId"
            density="compact"
            hide-details
            label="操作员"
            variant="outlined"
          />
          <v-text-field
            v-model="form.reason"
            density="compact"
            hide-details
            label="原因"
            variant="outlined"
          />
        </div>

        <v-alert
          v-if="formError"
          type="warning"
          variant="tonal"
          density="compact"
        >
          {{ formError }}
        </v-alert>

        <div class="flex flex-wrap gap-2">
          <v-btn
            color="teal"
            :disabled="!canSave"
            :loading="loading"
            variant="flat"
            @click="submit"
          >
            保存运行时配置
          </v-btn>
          <v-btn
            :loading="loading"
            variant="outlined"
            @click="disable"
          >
            关闭实盘配置
          </v-btn>
        </div>
      </div>
    </v-card-text>
  </v-card>
</template>
