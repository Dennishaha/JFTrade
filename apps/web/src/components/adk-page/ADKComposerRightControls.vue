<script setup lang="ts">
import { useADKChatComposerContext } from "../../composables/useADKChatComposer";
import ADKComposerContextControl from "./ADKComposerContextControl.vue";

const {
  canInterruptChat,
  canSendChat,
  cancelActiveRun,
  handlePrimaryAction,
  interruptAndQueueChat,
  isMobileLayout,
  loading,
  mobileControlsExpanded,
  openProviderSettings,
  providerOptions,
  savingProviderSelection,
  selectedAgentId,
  selectedProviderId,
  selectedProviderLabel,
  selectedProviderTitle,
  sendButtonLoading,
  showInterruptButton,
  showStopButton,
  updateProviderSelection,
} = useADKChatComposerContext();
</script>

<template>
<div class="adk-composer-right">
          <div
            v-if="!isMobileLayout || mobileControlsExpanded"
            class="adk-composer-utility"
          >
            <ADKComposerContextControl />

            <v-menu location="top end">
              <template #activator="{ props: menuProps }">
                <button
                  v-bind="menuProps"
                  type="button"
                  class="adk-inline-trigger adk-provider-trigger"
                  :title="`模型：${selectedProviderTitle}`"
                  :disabled="
                    selectedAgentId === '' ||
                    providerOptions.length === 0 ||
                    savingProviderSelection
                  "
                >
                  <span>{{ selectedProviderLabel }}</span>
                  <v-progress-circular
                    v-if="savingProviderSelection"
                    indeterminate
                    size="14"
                    width="2"
                  />
                  <v-icon v-else size="12">fa-solid fa-chevron-down</v-icon>
                </button>
              </template>
              <v-list class="adk-compact-menu adk-provider-menu" density="compact">
                <v-list-item
                  v-for="provider in providerOptions"
                  :key="provider.value"
                  :active="provider.value === selectedProviderId"
                  @click="updateProviderSelection(provider.value)"
                >
                  <v-list-item-title class="adk-provider-menu__title">
                    <span>{{ provider.model || provider.title.split(" · ")[1] || provider.title.split(" · ")[0] }}</span>
                    <v-chip
                      v-if="provider.isDefault"
                      size="x-small"
                      color="primary"
                      variant="tonal"
                    >
                      默认
                    </v-chip>
                  </v-list-item-title>
                  <v-list-item-subtitle>{{ provider.title }}</v-list-item-subtitle>
                </v-list-item>
              </v-list>
            </v-menu>
            <select
              class="adk-compat-select adk-provider-select"
              :value="selectedProviderId"
              :disabled="
                selectedAgentId === '' ||
                providerOptions.length === 0 ||
                savingProviderSelection
              "
              tabindex="-1"
              aria-hidden="true"
              @change="
                updateProviderSelection(
                  ($event.target as HTMLSelectElement).value,
                )
              "
            >
              <option
                v-for="provider in providerOptions"
                :key="`compat-provider-${provider.value}`"
                :value="provider.value"
              >
                {{ provider.title }}
              </option>
            </select>

            <v-btn
              icon="fa-solid fa-gear"
              variant="text"
              size="small"
              title="Agent 设置"
              @click="openProviderSettings?.()"
            />
          </div>

          <div class="adk-composer-actions">
            <v-progress-linear
              v-if="loading"
              indeterminate
              rounded
              color="primary"
              class="adk-inline-progress"
            />
            <v-btn
              v-if="showStopButton"
              icon="fa-solid fa-stop"
              variant="tonal"
              color="error"
              size="small"
              title="停止运行"
              aria-label="停止运行"
              class="adk-composer-stop"
              @click="cancelActiveRun?.()"
            />
            <v-btn
              v-if="showInterruptButton"
              icon
              class="adk-composer-interrupt"
              variant="tonal"
              color="warning"
              size="small"
              title="打断后发送"
              aria-label="打断后发送"
              :disabled="!canInterruptChat"
              @click="void interruptAndQueueChat?.()"
            >
              <v-icon size="15">fa-solid fa-level-down-alt</v-icon>
              <span class="adk-sr-only">打断后发送</span>
            </v-btn>
            <v-btn
              icon
              color="primary"
              size="small"
              :loading="sendButtonLoading"
              :disabled="!canSendChat"
              title="发送"
              aria-label="发送"
              class="adk-composer-send"
              @click="void handlePrimaryAction()"
            >
              <v-icon size="14">fa-solid fa-paper-plane</v-icon>
              <span class="adk-sr-only">发送</span>
            </v-btn>
          </div>
        </div>
</template>

<style scoped>
.adk-provider-menu {
  min-width: 280px;
  max-width: min(360px, 92vw);
  border-radius: 12px;
  padding: 6px;
}

.adk-provider-menu__title {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 8px;
}

.adk-provider-menu__title > span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
