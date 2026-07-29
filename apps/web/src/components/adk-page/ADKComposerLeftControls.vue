<script setup lang="ts">
import { useADKChatComposerContext } from "@/composables/adk/useADKChatComposer";

const {
  agentOptions,
  effectivePermissionMode,
  effectivePermissionOption,
  effectiveWorkModeSelection,
  isMobileLayout,
  mobileControlsExpanded,
  normalizedDefaultPermissionMode,
  openProviderSettings,
  permissionModeOptions,
  selectedAgentId,
  selectedAgentLabel,
  selectedWorkModeLabel,
  updateAgentSelection,
  updatePermissionModeSelection,
  updateWorkModeSelection,
  workModeOptions,
} = useADKChatComposerContext();
</script>

<template>
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
</template>

<style scoped>
.adk-compact-menu {
  min-width: 220px;
  max-width: min(360px, 92vw);
  border-radius: 12px;
  padding: 6px;
}

.adk-permission-option.is-approval :deep(.v-icon) {
  color: rgb(22 163 74);
}

.adk-permission-option.is-less :deep(.v-icon) {
  color: rgb(217 119 6);
}

.adk-permission-option.is-all :deep(.v-icon) {
  color: rgb(220 38 38);
}

.adk-permission-menu {
  min-width: 310px;
  border-radius: 12px;
  padding: 6px;
}

.adk-permission-option {
  margin: 2px 0;
  border-radius: 9px;
}

.adk-permission-option.is-selected {
  background: rgba(148, 163, 184, 0.12);
}
</style>
