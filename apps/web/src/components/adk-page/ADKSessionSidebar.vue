<script setup lang="ts">
import { ref } from "vue";

import type { ADKAgent, ADKSession } from "@/contracts";
import type { ADKSessionGroup } from "@/composables/useADKPageSessionState";

defineProps<{
  selectedAgentId: string;
  selectedSessionId: string;
  selectedAgent: ADKAgent | null;
  sessionSearch: string;
  sessionAgentFilter: string;
  agentOptions: Array<{ title: string; value: string }>;
  visibleSessions: ADKSession[];
  visibleSessionGroups: ADKSessionGroup[];
  showSessionGroups: boolean;
  sessions: ADKSession[];
  formatPermission: (mode: string) => string;
  sessionTitle: (session: ADKSession) => string;
  agentName: (agentId: string) => string;
  creatingSession: boolean;
  createNewSession: () => void | Promise<void>;
  selectSession: (sessionId: string) => void | Promise<void>;
  renameSession: (session: ADKSession) => void | Promise<void>;
  deleteSession: (sessionId: string) => void | Promise<void>;
}>();

defineEmits<{
  "update:sessionSearch": [value: string];
  "update:sessionAgentFilter": [value: string];
}>();

const collapsedSessionGroupIds = ref<Record<string, boolean>>({});

function isSessionGroupCollapsed(groupId: string): boolean {
  return collapsedSessionGroupIds.value[groupId] === true;
}

function toggleSessionGroup(groupId: string): void {
  collapsedSessionGroupIds.value = {
    ...collapsedSessionGroupIds.value,
    [groupId]: !isSessionGroupCollapsed(groupId),
  };
}
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
        :disabled="selectedAgentId === '' || creatingSession"
        :loading="creatingSession"
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
      <template v-if="!showSessionGroups">
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
      </template>
      <template v-else>
        <div
          v-for="group in visibleSessionGroups"
          :key="group.id"
          class="adk-session-group"
        >
          <div class="adk-session-group__header">
            <span>{{ group.title }}</span>
            <span class="adk-session-group__actions">
              <small>{{ group.sessions.length }}</small>
              <button
                type="button"
                class="adk-session-group__toggle"
                :title="isSessionGroupCollapsed(group.id) ? '展开分组' : '折叠分组'"
                :aria-label="isSessionGroupCollapsed(group.id) ? `展开${group.title}` : `折叠${group.title}`"
                :aria-expanded="isSessionGroupCollapsed(group.id) ? 'false' : 'true'"
                @click="toggleSessionGroup(group.id)"
              >
                <v-icon size="11">
                  {{ isSessionGroupCollapsed(group.id) ? "fa-solid fa-chevron-down" : "fa-solid fa-chevron-up" }}
                </v-icon>
              </button>
            </span>
          </div>
          <template v-if="!isSessionGroupCollapsed(group.id)">
            <div
              v-for="session in group.sessions"
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
          </template>
        </div>
      </template>
      <div v-if="sessions.length === 0" class="adk-session-empty">暂无会话</div>
      <div
        v-else-if="visibleSessions.length === 0"
        class="adk-session-empty"
      >没有匹配会话</div>
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
