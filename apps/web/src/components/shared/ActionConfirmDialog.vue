<script setup lang="ts">
withDefaults(defineProps<{
  busy?: boolean;
  confirmLabel?: string;
  message: string;
  open: boolean;
  title: string;
}>(), {
  busy: false,
  confirmLabel: "确认",
});

const emit = defineEmits<{
  close: [];
  confirm: [];
}>();
</script>

<template>
  <div
    v-if="open"
    class="action-confirm"
    role="dialog"
    aria-modal="true"
    :aria-label="title"
    @click.self="busy || emit('close')"
  >
    <section class="action-confirm__panel">
      <header class="action-confirm__header">
        <h2>{{ title }}</h2>
        <button
          type="button"
          aria-label="关闭确认弹窗"
          :disabled="busy"
          @click="emit('close')"
        >
          ×
        </button>
      </header>
      <p>{{ message }}</p>
      <footer class="action-confirm__actions">
        <button type="button" :disabled="busy" @click="emit('close')">
          取消
        </button>
        <button
          type="button"
          class="action-confirm__submit"
          data-testid="action-confirm-submit"
          :disabled="busy"
          @click="emit('confirm')"
        >
          {{ busy ? "正在处理…" : confirmLabel }}
        </button>
      </footer>
    </section>
  </div>
</template>

<style scoped>
.action-confirm {
  position: fixed;
  z-index: var(--tv-z-dialog);
  display: grid;
  inset: 0;
  place-items: center;
  padding: 20px;
  background: rgba(2, 6, 23, 0.66);
}

.action-confirm__panel {
  width: min(480px, 100%);
  overflow: hidden;
  border: 1px solid var(--tv-status-error-border);
  border-radius: 10px;
  background: var(--tv-bg-elevated);
  box-shadow: 0 22px 70px rgba(0, 0, 0, 0.38);
  color: var(--tv-text);
}

.action-confirm__header,
.action-confirm__actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 14px 16px;
}

.action-confirm__header {
  border-bottom: 1px solid var(--tv-border);
}

.action-confirm__header h2 {
  margin: 0;
  font-size: 16px;
}

.action-confirm__header button {
  min-width: 28px;
  border: 0;
  background: transparent;
  color: var(--tv-text-muted);
  cursor: pointer;
  font-size: 20px;
}

.action-confirm__panel p {
  margin: 0;
  padding: 18px 16px;
  color: var(--tv-text-muted);
  font-size: 13px;
  line-height: 1.65;
  overflow-wrap: anywhere;
}

.action-confirm__actions {
  justify-content: flex-end;
  border-top: 1px solid var(--tv-border);
}

.action-confirm__actions button {
  min-height: 34px;
  padding: 0 14px;
  border: 1px solid var(--tv-border);
  border-radius: 5px;
  background: var(--tv-bg-surface-2);
  color: var(--tv-text);
  cursor: pointer;
}

.action-confirm__actions .action-confirm__submit {
  border-color: var(--tv-status-error-border);
  background: var(--tv-status-error-bg);
  color: var(--tv-status-error-fg);
  font-weight: 700;
}

.action-confirm button:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}
</style>
