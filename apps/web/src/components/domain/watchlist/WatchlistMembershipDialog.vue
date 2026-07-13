<script setup lang="ts">
import { computed, ref, watch } from "vue";

import type { WatchlistItem, WatchlistMembership } from "@/contracts";

import { ApiClientError } from "../../../composables/apiClient";
import {
  useWatchlistGroups,
  useWatchlistMembership,
} from "../../../composables/useWatchlist";
import InstrumentIdentity from "../market-data/InstrumentIdentity.vue";

const props = withDefaults(
  defineProps<{
    modelValue: boolean;
    market: string;
    symbol: string;
    name?: string | undefined;
  }>(),
  { name: "" },
);

const emit = defineEmits<{
  "update:modelValue": [value: boolean];
  saved: [membership: WatchlistMembership];
}>();

const groupsQuery = useWatchlistGroups();
const { query: membershipQuery, replaceMutation } = useWatchlistMembership(
  () => props.market,
  () => props.symbol,
);
const selectedGroupIds = ref<string[]>([]);
const pendingGroupName = ref("");
const newGroupNames = ref<string[]>([]);
const errorMessage = ref("");
const conflictMessage = ref("");
const initializedFor = ref("");
const instrumentId = computed(
  () => `${props.market.trim().toUpperCase()}.${props.symbol.trim().toUpperCase()}`,
);
const groups = computed(() => groupsQuery.data.value ?? []);
const loading = computed(
  () => groupsQuery.isLoading.value || membershipQuery.isLoading.value,
);
const saving = computed(() => replaceMutation.isPending.value);
const loadErrorMessage = computed(() => {
  const error = groupsQuery.error?.value ?? membershipQuery.error?.value;
  return error instanceof Error ? error.message : error ? String(error) : "";
});
const isFavorite = computed(
  () => selectedGroupIds.value.length > 0 || newGroupNames.value.length > 0,
);

function close(): void {
  if (!saving.value) emit("update:modelValue", false);
}

function addPendingGroup(): void {
  const name = pendingGroupName.value.trim();
  if (name === "") return;
  const normalized = name.toLocaleLowerCase();
  const duplicate = [
    ...groups.value.map((group) => group.name),
    ...newGroupNames.value,
  ].some((candidate) => candidate.trim().toLocaleLowerCase() === normalized);
  if (duplicate) {
    errorMessage.value = "分组名称已存在";
    return;
  }
  newGroupNames.value.push(name);
  pendingGroupName.value = "";
  errorMessage.value = "";
}

function removeNewGroup(name: string): void {
  newGroupNames.value = newGroupNames.value.filter(
    (candidate) => candidate !== name,
  );
}

async function save(): Promise<void> {
  errorMessage.value = "";
  if (pendingGroupName.value.trim() !== "") {
    addPendingGroup();
    if (pendingGroupName.value.trim() !== "") return;
  }
  const membership = membershipQuery.data.value;
  if (membership == null) {
    errorMessage.value = "自选状态尚未加载完成";
    return;
  }
  try {
    const saved = await replaceMutation.mutateAsync({
      groupIds: [...selectedGroupIds.value],
      newGroupNames: [...newGroupNames.value],
      expectedRevision: membership.revision,
    });
    conflictMessage.value = "";
    emit("saved", saved);
    emit("update:modelValue", false);
  } catch (error) {
    if (error instanceof ApiClientError && error.status === 409) {
      conflictMessage.value =
        "自选分组已在其他位置发生变化。已刷新最新版本，请核对后再次保存。";
      await membershipQuery.refetch();
      return;
    }
    errorMessage.value = error instanceof Error ? error.message : "保存自选分组失败";
  }
}

watch(
  () => props.modelValue,
  (open) => {
    if (open) {
      initializedFor.value = "";
      pendingGroupName.value = "";
      newGroupNames.value = [];
      errorMessage.value = "";
      conflictMessage.value = "";
      void groupsQuery.refetch();
      void membershipQuery.refetch();
    }
  },
);

watch(
  [() => props.modelValue, instrumentId, membershipQuery.data],
  () => {
    if (!props.modelValue || membershipQuery.data.value == null) return;
    const key = `${instrumentId.value}:${membershipQuery.data.value.revision}`;
    if (initializedFor.value !== "") return;
    selectedGroupIds.value = [...membershipQuery.data.value.groupIds];
    initializedFor.value = key;
  },
  { immediate: true },
);
</script>

<template>
  <v-dialog
    :model-value="modelValue"
    max-width="560"
    @update:model-value="(value: boolean) => !value && close()"
  >
    <section class="watchlist-membership-dialog">
      <header class="watchlist-membership-dialog__header">
        <div>
          <span class="watchlist-membership-dialog__eyebrow">自选分组</span>
          <h2>
            <InstrumentIdentity
              :market="market"
              :code="symbol"
              :instrument-id="instrumentId"
              :name="name"
            />
          </h2>
        </div>
        <button type="button" class="watchlist-membership-dialog__close" aria-label="关闭" @click="close">×</button>
      </header>

      <div class="watchlist-membership-dialog__body">
        <p class="watchlist-membership-dialog__hint">
          {{ isFavorite ? "已收藏，可同时属于多个分组。" : "尚未收藏，选择至少一个分组即可加入自选。" }}
        </p>

        <div v-if="loading" class="watchlist-membership-dialog__loading">正在加载分组…</div>
        <div v-else class="watchlist-membership-dialog__groups">
          <label v-for="group in groups" :key="group.id" class="watchlist-membership-dialog__group">
            <input v-model="selectedGroupIds" type="checkbox" :value="group.id" />
            <span>
              <strong>{{ group.name }}</strong>
              <small>{{ group.itemCount ?? 0 }} 个标的<span v-if="group.isDefault"> · 默认</span></small>
            </span>
          </label>
        </div>

        <div class="watchlist-membership-dialog__new-group">
          <label for="watchlist-new-group">新建本地分组</label>
          <div>
            <input
              id="watchlist-new-group"
              v-model="pendingGroupName"
              type="text"
              maxlength="64"
              placeholder="输入分组名称"
              @keydown.enter.prevent="addPendingGroup"
            />
            <button type="button" :disabled="pendingGroupName.trim() === ''" @click="addPendingGroup">添加</button>
          </div>
          <div v-if="newGroupNames.length" class="watchlist-membership-dialog__chips">
            <button v-for="name in newGroupNames" :key="name" type="button" @click="removeNewGroup(name)">
              {{ name }} <span aria-hidden="true">×</span>
            </button>
          </div>
        </div>

        <p v-if="loadErrorMessage" class="watchlist-membership-dialog__notice tv-status--error tv-status-surface">{{ loadErrorMessage }}</p>
        <p v-if="conflictMessage" class="watchlist-membership-dialog__notice tv-status--warning tv-status-surface">{{ conflictMessage }}</p>
        <p v-if="errorMessage" class="watchlist-membership-dialog__notice tv-status--error tv-status-surface">{{ errorMessage }}</p>
      </div>

      <footer class="watchlist-membership-dialog__footer">
        <button type="button" class="tv-btn tv-btn-ghost" :disabled="saving" @click="close">取消</button>
        <button type="button" class="tv-btn watchlist-membership-dialog__save" :disabled="loading || saving || loadErrorMessage !== ''" @click="save">
          {{ saving ? "保存中…" : isFavorite ? "保存分组" : "取消全部收藏" }}
        </button>
      </footer>
    </section>
  </v-dialog>
</template>

<style scoped>
.watchlist-membership-dialog {
  overflow: hidden;
  border: 1px solid var(--tv-border);
  border-radius: 12px;
  background: var(--tv-bg-elevated);
  color: var(--tv-text);
  box-shadow: var(--tv-shadow-lg);
}

.watchlist-membership-dialog__header,
.watchlist-membership-dialog__footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 16px 18px;
  border-bottom: 1px solid var(--tv-border);
}

.watchlist-membership-dialog__header h2 {
  margin: 2px 0 0;
  font-size: 16px;
  font-weight: 650;
}

.watchlist-membership-dialog__eyebrow {
  color: var(--tv-accent);
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.12em;
}

.watchlist-membership-dialog__close {
  width: 30px;
  height: 30px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--tv-text-muted);
  font-size: 20px;
  cursor: pointer;
}

.watchlist-membership-dialog__body {
  display: grid;
  gap: 14px;
  max-height: min(64vh, 560px);
  padding: 16px 18px;
  overflow: auto;
}

.watchlist-membership-dialog__hint,
.watchlist-membership-dialog__loading {
  margin: 0;
  color: var(--tv-text-dim);
  font-size: 12px;
}

.watchlist-membership-dialog__groups {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
}

.watchlist-membership-dialog__group {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 10px;
  padding: 10px;
  border: 1px solid var(--tv-border);
  border-radius: 7px;
  background: var(--tv-bg-surface);
  cursor: pointer;
}

.watchlist-membership-dialog__group input { accent-color: var(--tv-accent); }
.watchlist-membership-dialog__group span { display: flex; min-width: 0; flex-direction: column; gap: 2px; }
.watchlist-membership-dialog__group strong { overflow: hidden; font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
.watchlist-membership-dialog__group small { color: var(--tv-text-dim); font-size: 10px; }

.watchlist-membership-dialog__new-group { display: grid; gap: 7px; }
.watchlist-membership-dialog__new-group > label { color: var(--tv-text-muted); font-size: 11px; font-weight: 650; }
.watchlist-membership-dialog__new-group > div:first-of-type { display: flex; gap: 8px; }
.watchlist-membership-dialog__new-group input {
  min-width: 0;
  height: 34px;
  flex: 1;
  padding: 0 10px;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  outline: none;
  background: var(--tv-bg-surface);
  color: var(--tv-text);
}
.watchlist-membership-dialog__new-group input:focus { border-color: var(--tv-accent); }
.watchlist-membership-dialog__new-group > div > button:not(.watchlist-membership-dialog__chips button) {
  padding: 0 14px;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface-2);
  color: var(--tv-text);
}

.watchlist-membership-dialog__chips { display: flex; flex-wrap: wrap; gap: 6px; }
.watchlist-membership-dialog__chips button {
  padding: 3px 8px;
  border: 1px solid color-mix(in srgb, var(--tv-accent) 40%, var(--tv-border));
  border-radius: 999px;
  background: color-mix(in srgb, var(--tv-accent) 10%, var(--tv-bg-surface));
  color: var(--tv-accent);
  font-size: 10px;
}

.watchlist-membership-dialog__notice { margin: 0; padding: 8px 10px; border-radius: 6px; font-size: 11px; }

.watchlist-membership-dialog__footer {
  justify-content: flex-end;
  border-top: 1px solid var(--tv-border);
  border-bottom: 0;
  background: var(--tv-bg-surface-2);
}

.watchlist-membership-dialog__save { border-color: var(--tv-accent); background: var(--tv-accent); color: #fff; }

@media (max-width: 600px) {
  .watchlist-membership-dialog__groups { grid-template-columns: 1fr; }
}
</style>
