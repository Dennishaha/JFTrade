<script setup lang="ts">
import { useADKChatComposerContext } from "../../composables/useADKChatComposer";

const {
  chatDraft,
  executeSlashCommand,
  filteredSlashCommands,
  handleKeydown,
  layout,
  placeholder,
  selectedSlashIndex,
  showSlashMenu,
  updateChatDraft,
} = useADKChatComposerContext();
</script>

<template>
<div class="adk-composer-input-wrap">
        <v-textarea
          :model-value="chatDraft"
          :placeholder="placeholder"
          variant="plain"
          density="compact"
          :rows="1"
          auto-grow
          :max-rows="layout === 'mobile' ? 5 : 6"
          hide-details
          class="adk-composer-input"
          @update:model-value="updateChatDraft"
          @keydown="handleKeydown"
        />
        <div
          v-if="showSlashMenu"
          class="adk-slash-menu"
          :class="{ 'adk-slash-menu--mobile': layout === 'mobile' }"
        >
          <button
            v-for="(item, index) in filteredSlashCommands"
            :key="item.command"
            type="button"
            class="adk-slash-menu__item"
            :class="{
              'adk-slash-menu__item--active': index === selectedSlashIndex,
              'adk-slash-menu__item--disabled': item.disabled,
            }"
            :disabled="item.disabled"
            @mousedown.prevent
            @click="void executeSlashCommand(item)"
          >
            <div class="adk-slash-menu__command">{{ item.command }}</div>
            <div class="adk-slash-menu__meta">
              <span class="adk-slash-menu__title">{{ item.title }}</span>
              <span class="adk-slash-menu__desc">{{ item.description }}</span>
            </div>
          </button>
        </div>
      </div>
</template>

<style scoped>
.adk-composer-input-wrap {
  position: relative;
}

.adk-slash-menu {
  position: absolute;
  left: 0;
  right: 0;
  bottom: calc(100% + 8px);
  z-index: 20;
  display: grid;
  gap: 6px;
  padding: 8px;
  border: 1px solid rgb(203 213 225);
  border-radius: 14px;
  background: rgba(255, 255, 255, 0.98);
  box-shadow: 0 18px 32px rgba(15, 23, 42, 0.14);
}

.adk-slash-menu--mobile {
  left: 8px;
  right: 8px;
}

.adk-slash-menu__item {
  display: grid;
  grid-template-columns: 130px 1fr;
  gap: 10px;
  align-items: start;
  width: 100%;
  padding: 10px 12px;
  border: 0;
  border-radius: 10px;
  background: transparent;
  text-align: left;
  cursor: pointer;
}

.adk-slash-menu__item--active {
  background: rgb(241 245 249);
}

.adk-slash-menu__item--disabled {
  cursor: not-allowed;
  opacity: 0.55;
}

.adk-slash-menu__command {
  font-family:
    ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono",
    "Courier New", monospace;
  font-size: 12px;
  color: rgb(15 23 42);
}

.adk-slash-menu__meta {
  display: grid;
  gap: 2px;
}

.adk-slash-menu__title {
  font-size: 13px;
  font-weight: 600;
  color: rgb(15 23 42);
}

.adk-slash-menu__desc {
  font-size: 12px;
  color: rgb(100 116 139);
}

.adk-slash-menu--mobile .adk-slash-menu__item {
  grid-template-columns: 1fr;
}
</style>
