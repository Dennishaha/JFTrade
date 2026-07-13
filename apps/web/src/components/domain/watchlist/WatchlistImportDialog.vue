<script setup lang="ts">
import { computed, nextTick, ref, watch } from "vue";

import type {
  WatchlistBinding,
  WatchlistRemoteGroup,
} from "@/contracts";

import { useWatchlistGroups } from "../../../composables/useWatchlist";
import { useWatchlistImport } from "../../../composables/useWatchlistImport";
import InstrumentIdentity from "../market-data/InstrumentIdentity.vue";

const props = defineProps<{ modelValue: boolean }>();
const emit = defineEmits<{
  "update:modelValue": [value: boolean];
  imported: [];
}>();

const selectedSourceId = ref("");
const selectedRemoteGroupKey = ref("");
const selectedLocalGroupId = ref("");
const newLocalGroupName = ref("");
const deleteLocalOnlyIds = ref<string[]>([]);
const errorMessage = ref("");
const committedMessage = ref("");
let preserveCommitResult = false;

const groupsQuery = useWatchlistGroups();
const {
  sourcesQuery,
  sourceGroupsQuery,
  bindingsQuery,
  previewMutation,
  commitMutation,
  unlinkMutation,
} = useWatchlistImport(() => props.modelValue, selectedSourceId);

const sources = computed(() => sourcesQuery.data.value ?? []);
const availableSources = computed(() =>
  sources.value.filter((source) => source.available),
);
const localGroups = computed(() => groupsQuery.data.value ?? []);
const remoteGroups = computed(() => sourceGroupsQuery.data.value ?? []);
const bindings = computed(() => bindingsQuery.data.value ?? []);
const sourceBindings = computed(() =>
  bindings.value.filter((binding) => binding.sourceId === selectedSourceId.value),
);
const preview = computed(() => previewMutation.data.value ?? null);
const selectedRemoteGroup = computed(() =>
  remoteGroups.value.find((group) => remoteGroupKey(group) === selectedRemoteGroupKey.value),
);
const loading = computed(
  () =>
    sourcesQuery.isLoading.value ||
    sourceGroupsQuery.isLoading.value ||
    bindingsQuery.isLoading.value,
);
const busy = computed(
  () =>
    previewMutation.isPending.value ||
    commitMutation.isPending.value ||
    unlinkMutation.isPending.value,
);
const canPreview = computed(
  () =>
    !bindingsQuery.isLoading.value &&
    selectedSourceId.value !== "" &&
    selectedRemoteGroup.value != null &&
    !isGroupDisabled(selectedRemoteGroup.value) &&
    (selectedLocalGroupId.value !== "" || newLocalGroupName.value.trim() !== ""),
);

function remoteGroupKey(group: WatchlistRemoteGroup): string {
  return group.remoteGroupId;
}

function isGroupDisabled(group: WatchlistRemoteGroup): boolean {
  return group.ambiguous === true;
}

function localGroupLabel(binding: WatchlistBinding): string {
  return (
    binding.localGroupName ||
    localGroups.value.find((group) => group.id === binding.localGroupId)?.name ||
    binding.localGroupId
  );
}

async function unlinkBinding(binding: WatchlistBinding): Promise<void> {
  if (
    typeof window !== "undefined" &&
    !window.confirm(
      `解除“${binding.remoteGroupName}”与本地分组“${localGroupLabel(binding)}”的绑定？不会修改券商端数据。`,
    )
  ) {
    return;
  }
  errorMessage.value = "";
  try {
    await unlinkMutation.mutateAsync(binding.id);
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : "解除绑定失败";
  }
}

function resetPreview(): void {
  previewMutation.reset();
  commitMutation.reset();
  deleteLocalOnlyIds.value = [];
  committedMessage.value = "";
}

function close(): void {
  if (busy.value) return;
  emit("update:modelValue", false);
}

async function requestPreview(): Promise<void> {
  const remote = selectedRemoteGroup.value;
  if (remote == null || !canPreview.value) return;
  errorMessage.value = "";
  resetPreview();
  try {
    const localGroupId = selectedLocalGroupId.value || undefined;
    const newGroupName =
      selectedLocalGroupId.value === "" ? newLocalGroupName.value.trim() : "";
    await previewMutation.mutateAsync({
      sourceId: selectedSourceId.value,
      remoteGroupId: remote.remoteGroupId,
      remoteGroupName: remote.name,
      ...(localGroupId == null ? {} : { localGroupId }),
      ...(newGroupName === "" ? {} : { newGroupName }),
    });
    deleteLocalOnlyIds.value = [];
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : "生成导入预览失败";
  }
}

async function commit(): Promise<void> {
  const current = preview.value;
  if (current == null) return;
  errorMessage.value = "";
  preserveCommitResult = true;
  try {
    const run = await commitMutation.mutateAsync({
      previewId: current.id,
      body: { deleteLocalOnlyInstrumentIds: [...deleteLocalOnlyIds.value] },
    });
    if (run.localGroupId) {
      selectedLocalGroupId.value = run.localGroupId;
      newLocalGroupName.value = "";
    }
    committedMessage.value = "导入已完成，本地自选已更新。";
    emit("imported");
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : "提交导入失败";
  } finally {
    await nextTick();
    preserveCommitResult = false;
  }
}

watch(
  sources,
  (next) => {
    if (
      selectedSourceId.value === "" ||
      !next.some((source) => source.id === selectedSourceId.value)
    ) {
      selectedSourceId.value = next.find((source) => source.available)?.id ?? "";
    }
  },
  { immediate: true },
);

watch(
  remoteGroups,
  (next) => {
    if (!next.some((group) => remoteGroupKey(group) === selectedRemoteGroupKey.value)) {
      const first = next.find((group) => !isGroupDisabled(group));
      selectedRemoteGroupKey.value = first == null ? "" : remoteGroupKey(first);
    }
  },
  { immediate: true },
);

watch(
  [selectedSourceId, selectedRemoteGroupKey, bindings, localGroups],
  () => {
    const remote = selectedRemoteGroup.value;
    if (remote == null) return;
    const binding = bindings.value.find(
      (candidate) =>
        candidate.sourceId === selectedSourceId.value &&
        candidate.remoteGroupId === remote.remoteGroupId,
    );
    if (
      binding != null &&
      localGroups.value.some((group) => group.id === binding.localGroupId)
    ) {
      selectedLocalGroupId.value = binding.localGroupId;
      newLocalGroupName.value = "";
      return;
    }
    selectedLocalGroupId.value =
      localGroups.value.find((group) => group.isDefault)?.id ??
      localGroups.value[0]?.id ??
      "";
    newLocalGroupName.value = "";
  },
  { immediate: true },
);

watch(
  [selectedSourceId, selectedRemoteGroupKey, selectedLocalGroupId, newLocalGroupName],
  () => {
    if (preserveCommitResult) return;
    resetPreview();
  },
);

watch(
  () => props.modelValue,
  (open) => {
    if (!open) return;
    errorMessage.value = "";
    committedMessage.value = "";
    resetPreview();
    void sourcesQuery.refetch();
    void groupsQuery.refetch();
    void bindingsQuery.refetch();
  },
);
</script>

<template>
  <v-dialog :model-value="modelValue" max-width="760" @update:model-value="(value: boolean) => !value && close()">
    <section class="watchlist-import-dialog">
      <header>
        <div>
          <span>券商导入</span>
          <h2>预览并导入券商自选分组</h2>
          <p>v1 只读取券商数据，不会修改券商客户端中的分组或成员。</p>
        </div>
        <button type="button" aria-label="关闭" @click="close">×</button>
      </header>

      <div class="watchlist-import-dialog__body">
        <div class="watchlist-import-dialog__form-grid">
          <label>
            <span>券商连接</span>
            <select v-model="selectedSourceId" :disabled="busy">
              <option v-for="source in sources" :key="source.id" :value="source.id" :disabled="!source.available">
                {{ source.displayName }}{{ source.available ? "" : "（不可用）" }}
              </option>
            </select>
          </label>
          <label>
            <span>远端分组</span>
            <select v-model="selectedRemoteGroupKey" :disabled="busy || sourceGroupsQuery.isLoading.value">
              <option v-for="group in remoteGroups" :key="remoteGroupKey(group)" :value="remoteGroupKey(group)" :disabled="isGroupDisabled(group)">
                {{ group.name }}{{ group.system ? "（系统组）" : "" }}{{ group.ambiguous ? "（重名，需先在券商侧改名）" : "" }}
              </option>
            </select>
          </label>
          <label>
            <span>导入到本地分组</span>
            <select v-model="selectedLocalGroupId" :disabled="busy" @change="newLocalGroupName = ''">
              <option value="">新建本地分组…</option>
              <option v-for="group in localGroups" :key="group.id" :value="group.id">{{ group.name }}</option>
            </select>
          </label>
          <label v-if="selectedLocalGroupId === ''">
            <span>新分组名称</span>
            <input v-model="newLocalGroupName" type="text" maxlength="64" placeholder="仅创建在 JFTrade" />
          </label>
        </div>

        <p v-if="loading" class="watchlist-import-dialog__state">正在读取券商分组…</p>
        <p v-else-if="availableSources.length === 0" class="watchlist-import-dialog__state tv-status--warning tv-status-surface">
          当前券商连接不可用，请先在设置中启动并连接 OpenD 后再导入。
        </p>
        <p v-else-if="sourceGroupsQuery.error.value" class="watchlist-import-dialog__state tv-status--error tv-status-surface">
          {{ sourceGroupsQuery.error.value instanceof Error ? sourceGroupsQuery.error.value.message : "读取券商分组失败" }}
        </p>

        <section v-if="sourceBindings.length" class="watchlist-import-dialog__bindings">
          <div>
            <h3>已绑定分组</h3>
            <p>解除操作只删除 JFTrade 本地绑定，不会修改券商端分组或成员。</p>
          </div>
          <article v-for="binding in sourceBindings" :key="binding.id">
            <span><strong>{{ binding.remoteGroupName }}</strong><small>券商分组</small></span>
            <b aria-hidden="true">→</b>
            <span><strong>{{ localGroupLabel(binding) }}</strong><small>JFTrade 分组</small></span>
            <button type="button" :disabled="busy" @click="unlinkBinding(binding)">解除绑定</button>
          </article>
        </section>

        <div v-if="preview" class="watchlist-import-dialog__preview">
          <div class="watchlist-import-dialog__stats">
            <div><strong>{{ preview.added?.length ?? 0 }}</strong><span>新增</span></div>
            <div><strong>{{ preview.unchanged?.length ?? 0 }}</strong><span>不变</span></div>
            <div><strong>{{ preview.localOnly?.length ?? 0 }}</strong><span>本地独有</span></div>
          </div>
          <section v-if="preview.added?.length">
            <h3>将新增</h3>
            <div class="watchlist-import-dialog__tokens">
              <span v-for="item in preview.added" :key="item.instrumentId" class="tv-status--success tv-status-surface">
                <InstrumentIdentity
                  :instrument-id="item.instrumentId"
                  :name="item.name"
                  compact
                />
              </span>
            </div>
          </section>
          <section v-if="preview.localOnly?.length">
            <h3>本地独有（默认保留）</h3>
            <p>仅勾选需要从该本地分组删除的标的；不会影响其在其他本地分组或券商端的状态。</p>
            <div class="watchlist-import-dialog__local-only">
              <label v-for="item in preview.localOnly" :key="item.instrumentId">
                <input v-model="deleteLocalOnlyIds" type="checkbox" :value="item.instrumentId" />
                <InstrumentIdentity
                  :instrument-id="item.instrumentId"
                  :name="item.name"
                  compact
                />
              </label>
            </div>
          </section>
          <p v-if="preview.expiresAt" class="watchlist-import-dialog__expiry">预览有效期至 {{ preview.expiresAt }}</p>
        </div>

        <p v-if="errorMessage" class="watchlist-import-dialog__state tv-status--error tv-status-surface">{{ errorMessage }}</p>
        <p v-if="committedMessage" class="watchlist-import-dialog__state tv-status--success tv-status-surface">{{ committedMessage }}</p>
      </div>

      <footer>
        <button type="button" class="tv-btn tv-btn-ghost" :disabled="busy" @click="close">{{ committedMessage ? "关闭" : "取消" }}</button>
        <button v-if="!preview" type="button" class="tv-btn watchlist-import-dialog__primary" :disabled="!canPreview || busy" @click="requestPreview">
          {{ previewMutation.isPending.value ? "生成中…" : "生成导入预览" }}
        </button>
        <button v-else-if="!committedMessage" type="button" class="tv-btn watchlist-import-dialog__primary" :disabled="busy" @click="commit">
          {{ commitMutation.isPending.value ? "导入中…" : "确认导入" }}
        </button>
      </footer>
    </section>
  </v-dialog>
</template>

<style scoped>
.watchlist-import-dialog { overflow: hidden; border: 1px solid var(--tv-border); border-radius: 12px; background: var(--tv-bg-elevated); color: var(--tv-text); box-shadow: var(--tv-shadow-lg); }
.watchlist-import-dialog > header,
.watchlist-import-dialog > footer { display: flex; align-items: center; justify-content: space-between; gap: 14px; padding: 16px 18px; border-bottom: 1px solid var(--tv-border); background: var(--tv-bg-surface-2); }
.watchlist-import-dialog > header span { color: var(--tv-accent); font-size: 10px; font-weight: 700; letter-spacing: .12em; }
.watchlist-import-dialog > header h2 { margin: 2px 0 0; font-size: 17px; }
.watchlist-import-dialog > header p { margin: 4px 0 0; color: var(--tv-text-dim); font-size: 11px; }
.watchlist-import-dialog > header button { width: 30px; height: 30px; border: 0; background: transparent; color: var(--tv-text-muted); font-size: 20px; }
.watchlist-import-dialog__body { display: grid; gap: 16px; max-height: min(68vh, 660px); padding: 18px; overflow: auto; }
.watchlist-import-dialog__form-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; }
.watchlist-import-dialog__form-grid label { display: grid; gap: 6px; }
.watchlist-import-dialog__form-grid label > span { color: var(--tv-text-muted); font-size: 11px; font-weight: 650; }
.watchlist-import-dialog__form-grid select,
.watchlist-import-dialog__form-grid input { width: 100%; height: 35px; padding: 0 9px; border: 1px solid var(--tv-border); border-radius: 6px; outline: none; background: var(--tv-bg-surface); color: var(--tv-text); font-size: 12px; }
.watchlist-import-dialog__form-grid select:focus,
.watchlist-import-dialog__form-grid input:focus { border-color: var(--tv-accent); }
.watchlist-import-dialog__state { margin: 0; padding: 9px 11px; border-radius: 6px; background: var(--tv-status-bg, var(--tv-bg-surface)); color: var(--tv-status-fg, var(--tv-text-dim)); font-size: 11px; }
.watchlist-import-dialog__bindings { display: grid; gap: 8px; padding: 10px 12px; border: 1px solid var(--tv-border); border-radius: 7px; background: var(--tv-bg-surface); }
.watchlist-import-dialog__bindings > div { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; }
.watchlist-import-dialog__bindings h3,
.watchlist-import-dialog__bindings p { margin: 0; }
.watchlist-import-dialog__bindings h3 { font-size: 12px; }
.watchlist-import-dialog__bindings p { color: var(--tv-text-dim); font-size: 10px; }
.watchlist-import-dialog__bindings article { display: grid; grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr) auto; align-items: center; gap: 8px; padding-top: 8px; border-top: 1px solid var(--tv-border); }
.watchlist-import-dialog__bindings article span { display: flex; min-width: 0; flex-direction: column; }
.watchlist-import-dialog__bindings article strong { overflow: hidden; font-size: 11px; text-overflow: ellipsis; white-space: nowrap; }
.watchlist-import-dialog__bindings article small,
.watchlist-import-dialog__bindings article b { color: var(--tv-text-dim); font-size: 9px; font-weight: 400; }
.watchlist-import-dialog__bindings article button { padding: 5px 7px; border: 1px solid var(--tv-border); border-radius: 5px; background: var(--tv-bg-surface-2); color: var(--tv-text-muted); font-size: 10px; }
.watchlist-import-dialog__preview { display: grid; gap: 14px; padding-top: 2px; }
.watchlist-import-dialog__stats { display: grid; grid-template-columns: repeat(3, 1fr); gap: 8px; }
.watchlist-import-dialog__stats div { display: flex; align-items: baseline; gap: 7px; padding: 10px 12px; border: 1px solid var(--tv-border); border-radius: 7px; background: var(--tv-bg-surface); }
.watchlist-import-dialog__stats strong { font-size: 18px; }
.watchlist-import-dialog__stats span { color: var(--tv-text-dim); font-size: 10px; }
.watchlist-import-dialog__preview section { display: grid; gap: 7px; }
.watchlist-import-dialog__preview h3 { margin: 0; font-size: 12px; }
.watchlist-import-dialog__preview section > p,
.watchlist-import-dialog__expiry { margin: 0; color: var(--tv-text-dim); font-size: 10px; }
.watchlist-import-dialog__tokens { display: flex; max-height: 90px; flex-wrap: wrap; gap: 5px; overflow: auto; }
.watchlist-import-dialog__tokens > span { padding: 3px 7px; border-radius: 4px; font-size: 10px; }
.watchlist-import-dialog__local-only { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 5px; max-height: 150px; overflow: auto; }
.watchlist-import-dialog__local-only label { display: grid; grid-template-columns: 18px minmax(0, 1fr); align-items: center; gap: 5px; padding: 6px 8px; border: 1px solid var(--tv-border); border-radius: 5px; font-size: 11px; }
.watchlist-import-dialog__local-only small { color: var(--tv-text-dim); font-size: 9px; }
.watchlist-import-dialog > footer { justify-content: flex-end; border-top: 1px solid var(--tv-border); border-bottom: 0; }
.watchlist-import-dialog__primary { border-color: var(--tv-accent); background: var(--tv-accent); color: #fff; }
@media (max-width: 640px) {
  .watchlist-import-dialog__form-grid,
  .watchlist-import-dialog__local-only { grid-template-columns: 1fr; }
  .watchlist-import-dialog__bindings > div { align-items: flex-start; flex-direction: column; }
  .watchlist-import-dialog__bindings article { grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr); }
  .watchlist-import-dialog__bindings article button { grid-column: 1 / -1; justify-self: end; }
}
</style>
