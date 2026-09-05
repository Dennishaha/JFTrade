<script setup lang="ts">
import { ref, watch } from "vue";

const props = defineProps<{
  brokerId: string;
  error?: string | null;
  modelValue: boolean;
  unlocking?: boolean;
}>();

const emit = defineEmits<{
  cancel: [];
  submit: [password: string];
  "update:modelValue": [value: boolean];
}>();

const password = ref("");

watch(
  () => props.modelValue,
  (open) => {
    if (open) {
      password.value = "";
    }
  },
);

function onCancel() {
  password.value = "";
  emit("cancel");
  emit("update:modelValue", false);
}

function onSubmit() {
  if (!password.value || props.unlocking) {
    return;
  }
  const pwd = password.value;
  password.value = "";
  emit("submit", pwd);
}
</script>

<template>
  <v-dialog
    :model-value="modelValue"
    max-width="440"
    persistent
    @update:model-value="emit('update:modelValue', $event)"
  >
    <v-card class="tv-broker-unlock">
      <v-card-title class="tv-broker-unlock__title">
        <span class="fa-solid fa-lock" aria-hidden="true"></span>
        解锁券商交易权限
      </v-card-title>
      <v-card-text class="tv-broker-unlock__body">
        <p class="tv-broker-unlock__desc">
          当前券商 <strong>{{ brokerId.toUpperCase() }}</strong> 实盘交易通道处于锁定状态。请输入交易密码以完成解锁并提交订单。
        </p>

        <div v-if="error" class="tv-broker-unlock__error" role="alert">
          <span class="fa-solid fa-triangle-exclamation" aria-hidden="true"></span>
          <span>{{ error }}</span>
        </div>

        <div class="tv-broker-unlock__field">
          <label for="broker-unlock-password">交易密码</label>
          <input
            id="broker-unlock-password"
            v-model="password"
            type="password"
            class="tv-broker-unlock__input"
            placeholder="请输入交易密码"
            autocomplete="new-password"
            :disabled="unlocking"
            @keydown.enter.prevent="onSubmit"
          />
        </div>
        <p class="tv-broker-unlock__hint">
          密码仅在本地计算哈希后发送至券商网关，严禁也不保留任何明文凭据。
        </p>
      </v-card-text>
      <v-card-actions class="tv-broker-unlock__actions">
        <v-spacer />
        <button
          type="button"
          class="tv-broker-unlock__btn tv-broker-unlock__btn--cancel"
          :disabled="unlocking"
          @click="onCancel"
        >
          取消
        </button>
        <button
          type="button"
          class="tv-broker-unlock__btn tv-broker-unlock__btn--confirm"
          :disabled="!password || unlocking"
          @click="onSubmit"
        >
          {{ unlocking ? "解锁中..." : "解锁并继续" }}
        </button>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<style scoped>
.tv-broker-unlock {
  background: var(--tv-bg-surface);
  color: var(--tv-text);
  border: 1px solid var(--tv-border);
  border-radius: 8px;
}

.tv-broker-unlock__title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 16px;
  font-weight: 600;
  padding: 16px 20px 8px;
  color: var(--tv-text);
}

.tv-broker-unlock__body {
  padding: 8px 20px 16px;
}

.tv-broker-unlock__desc {
  font-size: 13px;
  line-height: 1.5;
  color: var(--tv-text-muted);
  margin-bottom: 12px;
}

.tv-broker-unlock__desc strong {
  color: var(--tv-text);
}

.tv-broker-unlock__error {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  margin-bottom: 12px;
  border-radius: 4px;
  background: rgba(242, 54, 69, 0.15);
  border: 1px solid rgba(242, 54, 69, 0.4);
  color: #f23645;
  font-size: 13px;
}

.tv-broker-unlock__field {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-bottom: 8px;
}

.tv-broker-unlock__field label {
  font-size: 12px;
  font-weight: 500;
  color: var(--tv-text-muted);
}

.tv-broker-unlock__input {
  width: 100%;
  height: 38px;
  padding: 0 12px;
  background: rgba(255, 255, 255, 0.05);
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  color: var(--tv-text);
  font-size: 14px;
  outline: none;
  transition: border-color 0.2s;
}

.tv-broker-unlock__input:focus {
  border-color: var(--tv-accent);
}

.tv-broker-unlock__hint {
  font-size: 11px;
  color: var(--tv-text-muted);
  margin-top: 4px;
  margin-bottom: 0;
}

.tv-broker-unlock__actions {
  padding: 8px 20px 16px;
  display: flex;
  gap: 12px;
}

.tv-broker-unlock__btn {
  height: 34px;
  padding: 0 16px;
  border-radius: 4px;
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  border: none;
  transition: background-color 0.2s, opacity 0.2s;
}

.tv-broker-unlock__btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.tv-broker-unlock__btn--cancel {
  background: rgba(255, 255, 255, 0.08);
  color: var(--tv-text);
}

.tv-broker-unlock__btn--cancel:hover:not(:disabled) {
  background: rgba(255, 255, 255, 0.12);
}

.tv-broker-unlock__btn--confirm {
  background: var(--tv-accent);
  color: #ffffff;
}

.tv-broker-unlock__btn--confirm:hover:not(:disabled) {
  background: var(--tv-accent-hover, var(--tv-accent));
}
</style>
