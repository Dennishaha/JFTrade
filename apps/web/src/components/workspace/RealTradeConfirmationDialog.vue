<script setup lang="ts">
const props = defineProps<{
  accountId: string;
  confirmationMatches: boolean;
  confirmationText: string;
  maxOrderNotional: number | null;
  maxOrderQuantity: number | null;
  modelValue: boolean;
  orderSummary: string | null | undefined;
  realTradingEnabled: boolean;
  requiredConfirmationText: string;
  submitting: boolean;
}>();

const emit = defineEmits<{
  cancel: [];
  confirm: [];
  "update:confirmationText": [value: string];
  "update:modelValue": [value: boolean];
}>();
</script>

<template>
  <v-dialog
    :model-value="modelValue"
    max-width="520"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <v-card class="tv-real-confirmation">
      <v-card-title class="tv-real-confirmation__title">
        确认实盘下单
      </v-card-title>
      <v-card-text class="tv-real-confirmation__body">
        <div class="tv-real-confirmation__summary">
          {{ props.orderSummary }}
        </div>
        <div class="tv-real-confirmation__grid">
          <div>
            <span>账户</span>
            <strong>{{ accountId || "未指定" }}</strong>
          </div>
          <div>
            <span>实盘总闸</span>
            <strong>{{ realTradingEnabled ? "已开放" : "未开放" }}</strong>
          </div>
          <div>
            <span>数量限额</span>
            <strong>{{ maxOrderQuantity ?? "暂无" }}</strong>
          </div>
          <div>
            <span>金额限额</span>
            <strong>{{ maxOrderNotional ?? "暂无" }}</strong>
          </div>
        </div>
        <div class="tv-real-confirmation__notice">
          后端仍会按运行时风控最终判定。请输入
          <code>{{ requiredConfirmationText }}</code>
          继续。
        </div>
        <input
          :value="confirmationText"
          class="tv-input tv-real-confirmation__input"
          autocomplete="off"
          spellcheck="false"
          :placeholder="requiredConfirmationText"
          @input="emit('update:confirmationText', ($event.target as HTMLInputElement).value)"
          @keydown.enter.prevent="emit('confirm')"
        />
      </v-card-text>
      <v-card-actions class="tv-real-confirmation__actions">
        <v-btn variant="text" size="small" @click="emit('cancel')">
          取消
        </v-btn>
        <v-btn
          color="error"
          variant="flat"
          size="small"
          data-testid="real-trade-confirm-submit"
          :disabled="!confirmationMatches || submitting"
          :loading="submitting"
          @click="emit('confirm')"
        >
          确认提交实盘单
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<style scoped>
.tv-real-confirmation {
  border: 1px solid var(--tv-border);
  background: var(--tv-bg-surface);
  color: var(--tv-text);
}

.tv-real-confirmation__title {
  color: var(--tv-text);
  font-size: 16px;
  font-weight: 700;
}

.tv-real-confirmation__body {
  display: grid;
  gap: 10px;
}

.tv-real-confirmation__summary {
  border: 1px solid color-mix(in srgb, var(--tv-accent-strong) 42%, var(--tv-border));
  border-radius: 6px;
  padding: 9px 10px;
  background: color-mix(in srgb, var(--tv-accent-strong) 10%, transparent);
  color: var(--tv-text);
  font-size: 13px;
  font-weight: 700;
}

.tv-real-confirmation__grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
}

.tv-real-confirmation__grid div {
  min-width: 0;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  padding: 8px;
  background: rgba(255, 255, 255, 0.03);
}

.tv-real-confirmation__grid span {
  display: block;
  color: var(--tv-text-muted);
  font-size: 10px;
}

.tv-real-confirmation__grid strong {
  display: block;
  margin-top: 3px;
  color: var(--tv-text);
  font-size: 12px;
  overflow-wrap: anywhere;
}

.tv-real-confirmation__notice {
  color: var(--tv-text-muted);
  font-size: 12px;
  line-height: 1.5;
}

.tv-real-confirmation__notice code {
  border-radius: 4px;
  padding: 1px 4px;
  background: rgba(255, 255, 255, 0.08);
  color: var(--tv-text);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", monospace;
  font-size: 11px;
}

.tv-real-confirmation__input {
  width: 100%;
}

.tv-real-confirmation__actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
