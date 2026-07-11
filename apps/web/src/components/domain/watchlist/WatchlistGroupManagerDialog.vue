<script setup lang="ts">
import { computed, ref, watch } from "vue";

import type { WatchlistGroup } from "@/contracts";

import {
  useCreateWatchlistGroup,
  useDeleteWatchlistGroup,
  useUpdateWatchlistGroup,
  useWatchlistGroups,
} from "../../../composables/useWatchlist";

const props = defineProps<{ modelValue: boolean }>();
const emit = defineEmits<{ "update:modelValue": [value: boolean] }>();

const groupsQuery = useWatchlistGroups();
const createMutation = useCreateWatchlistGroup();
const updateMutation = useUpdateWatchlistGroup();
const deleteMutation = useDeleteWatchlistGroup();
const newGroupName = ref("");
const editingGroupId = ref("");
const editingName = ref("");
const busyGroupId = ref("");
const errorMessage = ref("");
const groups = computed(() => groupsQuery.data.value ?? []);

function close(): void {
  if (busyGroupId.value === "" && !createMutation.isPending.value) {
    emit("update:modelValue", false);
  }
}

async function createGroup(): Promise<void> {
  const name = newGroupName.value.trim();
  if (name === "") return;
  errorMessage.value = "";
  try {
    await createMutation.mutateAsync(name);
    newGroupName.value = "";
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : "创建分组失败";
  }
}

function startRename(group: WatchlistGroup): void {
  editingGroupId.value = group.id;
  editingName.value = group.name;
  errorMessage.value = "";
}

async function saveRename(group: WatchlistGroup): Promise<void> {
  const name = editingName.value.trim();
  if (name === "" || name === group.name) {
    editingGroupId.value = "";
    return;
  }
  busyGroupId.value = group.id;
  errorMessage.value = "";
  try {
    await updateMutation.mutateAsync({
      groupId: group.id,
      name,
      expectedRevision: group.revision,
    });
    editingGroupId.value = "";
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : "重命名分组失败";
  } finally {
    busyGroupId.value = "";
  }
}

async function removeGroup(group: WatchlistGroup): Promise<void> {
  if (group.protected || group.isDefault) return;
  if (
    typeof window !== "undefined" &&
    !window.confirm(`删除本地分组“${group.name}”？不会修改券商端数据。`)
  ) {
    return;
  }
  busyGroupId.value = group.id;
  errorMessage.value = "";
  try {
    await deleteMutation.mutateAsync(group.id);
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : "删除分组失败";
  } finally {
    busyGroupId.value = "";
  }
}

watch(
  () => props.modelValue,
  (open) => {
    if (!open) return;
    errorMessage.value = "";
    editingGroupId.value = "";
    void groupsQuery.refetch();
  },
);
</script>

<template>
  <v-dialog :model-value="modelValue" max-width="620" @update:model-value="(value: boolean) => !value && close()">
    <section class="watchlist-groups-dialog">
      <header>
        <div><span>本地分组</span><h2>创建与管理自选分组</h2></div>
        <button type="button" aria-label="关闭" @click="close">×</button>
      </header>
      <div class="watchlist-groups-dialog__body">
        <form class="watchlist-groups-dialog__create" @submit.prevent="createGroup">
          <input v-model="newGroupName" type="text" maxlength="64" placeholder="新分组名称" aria-label="新分组名称" />
          <button type="submit" class="tv-btn" :disabled="newGroupName.trim() === '' || createMutation.isPending.value">
            {{ createMutation.isPending.value ? "创建中…" : "创建分组" }}
          </button>
        </form>
        <div class="watchlist-groups-dialog__list">
          <article v-for="group in groups" :key="group.id">
            <div class="watchlist-groups-dialog__group-name">
              <template v-if="editingGroupId !== group.id">
                <strong>{{ group.name }}</strong>
                <small>{{ group.itemCount ?? 0 }} 个标的<span v-if="group.isDefault"> · 默认且受保护</span></small>
              </template>
              <input v-else v-model="editingName" type="text" maxlength="64" :aria-label="`重命名分组 ${group.name}`" @keydown.enter.prevent="saveRename(group)" @keydown.esc="editingGroupId = ''" />
            </div>
            <div class="watchlist-groups-dialog__actions">
              <button
                v-if="editingGroupId !== group.id"
                type="button"
                :disabled="group.protected || group.isDefault || busyGroupId !== ''"
                :title="group.protected || group.isDefault ? '默认自选分组不可重命名' : '重命名本地分组'"
                @click="startRename(group)"
              >重命名</button>
              <button v-else type="button" :disabled="busyGroupId !== ''" @click="saveRename(group)">保存</button>
              <button type="button" class="is-danger" :disabled="group.protected || group.isDefault || busyGroupId !== ''" :title="group.protected || group.isDefault ? '默认自选分组不可删除' : '删除本地分组'" @click="removeGroup(group)">删除</button>
            </div>
          </article>
        </div>
        <p v-if="errorMessage" class="watchlist-groups-dialog__error tv-status--error tv-status-text">{{ errorMessage }}</p>
      </div>
      <footer><button type="button" class="tv-btn tv-btn-ghost" @click="close">完成</button></footer>
    </section>
  </v-dialog>
</template>

<style scoped>
.watchlist-groups-dialog { overflow: hidden; border: 1px solid var(--tv-border); border-radius: 12px; background: var(--tv-bg-elevated); color: var(--tv-text); }
.watchlist-groups-dialog > header,
.watchlist-groups-dialog > footer { display: flex; align-items: center; justify-content: space-between; padding: 15px 18px; border-bottom: 1px solid var(--tv-border); background: var(--tv-bg-surface-2); }
.watchlist-groups-dialog > header span { color: var(--tv-accent); font-size: 10px; font-weight: 700; letter-spacing: .1em; }
.watchlist-groups-dialog > header h2 { margin: 2px 0 0; font-size: 16px; }
.watchlist-groups-dialog > header > button { border: 0; background: transparent; color: var(--tv-text-muted); font-size: 20px; }
.watchlist-groups-dialog__body { display: grid; gap: 14px; padding: 16px 18px; }
.watchlist-groups-dialog__create { display: flex; gap: 8px; }
.watchlist-groups-dialog__create input,
.watchlist-groups-dialog__group-name input { min-width: 0; height: 34px; flex: 1; padding: 0 9px; border: 1px solid var(--tv-border); border-radius: 6px; outline: 0; background: var(--tv-bg-surface); color: var(--tv-text); }
.watchlist-groups-dialog__list { display: grid; max-height: min(52vh, 440px); overflow: auto; }
.watchlist-groups-dialog__list article { display: flex; min-width: 0; align-items: center; justify-content: space-between; gap: 12px; padding: 11px 2px; border-bottom: 1px solid var(--tv-border); }
.watchlist-groups-dialog__group-name { display: flex; min-width: 0; flex: 1; flex-direction: column; gap: 2px; }
.watchlist-groups-dialog__group-name strong { font-size: 12px; }
.watchlist-groups-dialog__group-name small { color: var(--tv-text-dim); font-size: 10px; }
.watchlist-groups-dialog__actions { display: flex; gap: 5px; }
.watchlist-groups-dialog__actions button { padding: 5px 7px; border: 1px solid var(--tv-border); border-radius: 5px; background: var(--tv-bg-surface); color: var(--tv-text-muted); font-size: 10px; }
.watchlist-groups-dialog__actions .is-danger:not(:disabled) { color: var(--tv-status-error-fg); }
.watchlist-groups-dialog__error { margin: 0; font-size: 11px; }
.watchlist-groups-dialog > footer { justify-content: flex-end; border-top: 1px solid var(--tv-border); border-bottom: 0; }
</style>
