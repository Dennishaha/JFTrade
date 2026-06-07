<script setup lang="ts">
defineProps<{
  activeRunId: string;
  agentOptions: { title: string; value: string }[];
  canSendChat: boolean;
  chatDraft: string;
  composerBlockMessage: string;
  loading: boolean;
  providerOptions: { title: string; value: string }[];
  savingProviderSelection: boolean;
  selectedAgentId: string;
  selectedProviderId: string;
  sendingChat: boolean;
  cancelActiveRun: () => void | Promise<void>;
  handleAgentChange: () => void;
  handleComposerKeydown: (event: KeyboardEvent) => void;
  handleProviderChange: (providerId: string) => void | Promise<void>;
  openProviderSettings: () => void;
  sendChat: () => void | Promise<void>;
}>();

defineEmits<{
  "update:chatDraft": [value: string];
  "update:selectedAgentId": [value: string];
  "update:selectedProviderId": [value: string];
}>();
</script>

<template>
  <div class="adk-composer">
    <div class="adk-composer-card">
      <div v-if="composerBlockMessage" class="adk-composer-block">
        <v-icon size="13">fa-solid fa-circle-exclamation</v-icon>
        <span>{{ composerBlockMessage }}</span>
      </div>
      <v-textarea
        :model-value="chatDraft"
        placeholder="输入问题或任务…"
        variant="plain"
        density="compact"
        :rows="1"
        auto-grow
        :max-rows="6"
        hide-details
        :disabled="sendingChat"
        class="adk-composer-input"
        @update:model-value="$emit('update:chatDraft', $event ?? '')"
        @keydown="handleComposerKeydown"
      />
      <div class="adk-composer-controls">
        <div class="adk-composer-left">
          <v-btn
            icon="fa-solid fa-plus"
            variant="text"
            size="small"
            title="添加模型提供商"
            @click="openProviderSettings"
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
              handleAgentChange();
            "
          />
        </div>
        <div class="adk-composer-right">
          <v-progress-circular v-if="loading" indeterminate size="18" width="2" />
          <v-select
            :model-value="selectedProviderId"
            :items="providerOptions"
            density="compact"
            variant="plain"
            hide-details
            placeholder="选择模型"
            class="adk-provider-select"
            :disabled="selectedAgentId === '' || providerOptions.length === 0 || savingProviderSelection"
            :loading="savingProviderSelection"
            @update:model-value="
              $emit('update:selectedProviderId', $event ?? '');
              handleProviderChange(($event ?? '') as string);
            "
          />
          <v-btn
            icon="fa-solid fa-gear"
            variant="text"
            size="small"
            title="Agents 配置"
            @click="openProviderSettings"
          />
          <v-btn
            v-if="sendingChat && activeRunId"
            icon="fa-solid fa-stop"
            variant="text"
            color="error"
            size="small"
            title="停止运行"
            @click="cancelActiveRun"
          />
          <v-btn
            color="primary"
            :loading="sendingChat"
            :disabled="!canSendChat"
            icon="fa-solid fa-paper-plane"
            class="adk-composer-send"
            @click="sendChat"
          />
        </div>
      </div>
    </div>
  </div>
</template>
