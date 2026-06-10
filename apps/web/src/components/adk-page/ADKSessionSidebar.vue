<script setup lang="ts">
import type { ADKAgent, ADKSession } from "@/contracts";

defineProps<{
  selectedAgentId: string;
  selectedSessionId: string;
  selectedAgent: ADKAgent | null;
  sessionSearch: string;
  sessionAgentFilter: string;
  agentOptions: Array<{ title: string; value: string }>;
  visibleSessions: ADKSession[];
  sessions: ADKSession[];
  formatPermission: (mode: string) => string;
  sessionTitle: (session: ADKSession) => string;
  agentName: (agentId: string) => string;
  createNewSession: () => void | Promise<void>;
  selectSession: (sessionId: string) => void | Promise<void>;
  renameSession: (session: ADKSession) => void | Promise<void>;
  deleteSession: (sessionId: string) => void | Promise<void>;
}>();

defineEmits<{
  "update:sessionSearch": [value: string];
  "update:sessionAgentFilter": [value: string];
}>();
</script>

<template>
  <nav class="adk-sidebar">
    <div class="adk-sidebar-header">
      <span class="adk-brand">Agents</span>
      <v-btn
        icon="fa-solid fa-plus"
        size="small"
        variant="text"
        title="新建会话"
        :disabled="selectedAgentId === ''"
        @click="createNewSession"
      />
    </div>

    <div class="adk-session-list">
      <div class="adk-session-filters">
        <v-text-field
          :model-value="sessionSearch"
          placeholder="搜索会话"
          density="compact"
          hide-details
          @update:model-value="$emit('update:sessionSearch', String($event))"
        />
        <v-select
          :model-value="sessionAgentFilter"
          :items="[{ title: '全部 Agents', value: '' }, ...agentOptions]"
          density="compact"
          hide-details
          @update:model-value="$emit('update:sessionAgentFilter', String($event))"
        />
      </div>
      <div
        v-for="session in visibleSessions"
        :key="session.id"
        class="adk-session-item"
        :class="{ 'adk-session-item--active': session.id === selectedSessionId }"
        @click="selectSession(session.id)"
      >
        <v-icon size="13">fa-solid fa-comment</v-icon>
        <span class="adk-session-title">
          {{ sessionTitle(session) }}
          <small class="adk-session-agent">{{ agentName(session.agentId) }}</small>
        </span>
        <v-icon
          size="13"
          class="adk-session-close"
          title="重命名会话"
          @click.stop="renameSession(session)"
        >fa-solid fa-pen</v-icon>
        <v-icon
          size="14"
          class="adk-session-close"
          title="关闭会话"
          @click.stop="deleteSession(session.id)"
        >fa-solid fa-xmark</v-icon>
      </div>
      <div v-if="sessions.length === 0" class="adk-session-empty">暂无会话</div>
    </div>

    <div v-if="selectedAgent" class="adk-sidebar-footer">
      <div class="adk-agent-info">
        <v-icon size="15">fa-solid fa-robot</v-icon>
        <span class="adk-agent-name">{{ selectedAgent.name }}</span>
        <v-chip size="x-small" class="ml-1 flex-shrink-0" variant="tonal">
          {{ formatPermission(selectedAgent.permissionMode) }}
        </v-chip>
      </div>
    </div>
  </nav>
</template>
