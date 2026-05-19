<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";

import { useCommandPalette } from "../composables/useCommandPalette";

const { open, actions, hide } = useCommandPalette();

const query = ref("");
const activeIndex = ref(0);
const inputRef = ref<HTMLInputElement | null>(null);

const filtered = computed(() => {
  const q = query.value.trim().toLowerCase();
  if (!q) return actions.value;
  return actions.value.filter((a) => {
    const hay = [a.label, a.group, a.hint, ...(a.keywords ?? [])]
      .filter(Boolean)
      .join(" ")
      .toLowerCase();
    return hay.includes(q);
  });
});

watch(open, async (v) => {
  if (v) {
    query.value = "";
    activeIndex.value = 0;
    await nextTick();
    inputRef.value?.focus();
  }
});

watch(filtered, () => {
  if (activeIndex.value >= filtered.value.length) activeIndex.value = 0;
});

function run(idx: number): void {
  const action = filtered.value[idx];
  if (!action) return;
  hide();
  action.run();
}

function onKey(e: KeyboardEvent): void {
  if (e.key === "ArrowDown") {
    e.preventDefault();
    activeIndex.value = Math.min(
      activeIndex.value + 1,
      filtered.value.length - 1,
    );
  } else if (e.key === "ArrowUp") {
    e.preventDefault();
    activeIndex.value = Math.max(activeIndex.value - 1, 0);
  } else if (e.key === "Enter") {
    e.preventDefault();
    run(activeIndex.value);
  } else if (e.key === "Escape") {
    e.preventDefault();
    hide();
  }
}

function onBackdrop(e: MouseEvent): void {
  if (e.target === e.currentTarget) hide();
}

const palette = useCommandPalette();

function onGlobalKey(e: KeyboardEvent): void {
  const isMod = e.metaKey || e.ctrlKey;
  if (isMod && (e.key === "k" || e.key === "K")) {
    e.preventDefault();
    palette.toggle();
  }
}

onMounted(() => {
  window.addEventListener("keydown", onGlobalKey);
});

onUnmounted(() => {
  window.removeEventListener("keydown", onGlobalKey);
});
</script>

<template>
  <div v-if="open" class="tv-palette-backdrop" @mousedown="onBackdrop">
    <div class="tv-palette" @keydown="onKey">
      <input
        ref="inputRef"
        v-model="query"
        placeholder="键入命令或搜索路由…"
        spellcheck="false"
      />
      <ul>
        <li
          v-for="(action, idx) in filtered"
          :key="action.id"
          :class="{ 'is-active': idx === activeIndex }"
          @mouseenter="activeIndex = idx"
          @mousedown.prevent="run(idx)"
        >
          <span>
            <span style="color: var(--tv-text-dim); margin-right: 8px">{{ action.group }}</span>
            {{ action.label }}
          </span>
          <span v-if="action.hint" class="tv-palette-hint">{{ action.hint }}</span>
        </li>
        <li v-if="filtered.length === 0" style="color: var(--tv-text-dim); cursor: default">
          没有匹配项
        </li>
      </ul>
    </div>
  </div>
</template>
