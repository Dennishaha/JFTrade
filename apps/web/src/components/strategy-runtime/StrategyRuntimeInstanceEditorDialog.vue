<script setup lang="ts">
import { computed } from "vue";
import type {
    StrategyDefinitionDocument,
    StrategyExecutionMode,
    StrategyInstanceItem,
} from "@jftrade/ui-contracts";

import type { BrokerAccountSelectionOption } from "../../composables/consoleDataBrokerAccountSelection";
import { brokerAccountOptionSubtitle } from "./strategyRuntimeInstanceBinding";

type StrategyRuntimeInstanceEditorMode = "create" | "edit";

const props = defineProps<{
    open: boolean;
    mode: StrategyRuntimeInstanceEditorMode;
    title: string;
    hint: string;
    isLoadingDefinitions: boolean;
    definitionsError: string;
    strategyDefinitions: StrategyDefinitionDocument[];
    createDefinitionId: string;
    createDefinition: StrategyDefinitionDocument | null;
    selectedStrategy: StrategyInstanceItem | null;
    symbolTags: string[];
    symbolMarket: string;
    symbolDraft: string;
    symbolValidationMessage: string;
    marketOptions: Array<{ value: string; title: string }>;
    intervalValue: string;
    executionMode: StrategyExecutionMode;
    selectedBrokerAccountOption: BrokerAccountSelectionOption | null;
    selectedBrokerAccountKey: string;
    currentBrokerAccountSelectionKey: string;
    isBrokerAccountPickerOpen: boolean;
    brokerAccountQuery: string;
    filteredBrokerAccountOptions: BrokerAccountSelectionOption[];
    previewDefinitionLabel: string;
    symbolsSummary: string;
    brokerAccountSummary: string;
    canCreateStrategyInstance: boolean;
    canUpdateSelectedStrategyBinding: boolean;
    canDeleteSelectedStrategy: boolean;
    isCreatingStrategyInstance: boolean;
    isUpdatingStrategyBinding: boolean;
    isDeletingStrategy: boolean;
}>();

const emit = defineEmits<{
    "update:open": [value: boolean];
    "refresh-definitions": [];
    "switch-to-design": [];
    "update:create-definition-id": [value: string];
    "remove-symbol": [symbol: string];
    "update:symbol-market": [value: string];
    "update:symbol-draft": [value: string];
    "commit-symbol-draft": [];
    "symbol-draft-keydown": [event: KeyboardEvent];
    "symbol-draft-paste": [event: ClipboardEvent];
    "update:interval": [value: string];
    "update:execution-mode": [value: string];
    "toggle-broker-picker": [];
    "update:broker-query": [value: string];
    "clear-broker-selection": [];
    "select-broker-selection": [selectionKey: string];
    "submit-create": [];
    "submit-update": [];
    "submit-delete": [];
}>();

const isCreate = computed(() => props.mode === "create");
const isEdit = computed(() => props.mode === "edit");
const closeTestId = computed(() =>
    isCreate.value ? "strategy-create-instance-close" : "strategy-edit-instance-close",
);
const panelTestId = computed(() =>
    isCreate.value ? "strategy-create-instance-panel" : "strategy-edit-instance-panel",
);
const symbolInputTestId = computed(() =>
    isCreate.value ? "strategy-instance-symbols" : "strategy-edit-symbols",
);
const symbolMarketTestId = computed(() =>
    isCreate.value ? "strategy-instance-symbol-market" : "strategy-edit-symbol-market",
);
const symbolAddTestId = computed(() =>
    isCreate.value ? "strategy-instance-symbol-add" : "strategy-edit-symbol-add",
);
const symbolValidationTestId = computed(() =>
    isCreate.value ? "strategy-instance-symbols-validation" : "strategy-edit-symbols-validation",
);
const intervalInputTestId = computed(() =>
    isCreate.value ? "strategy-instance-interval" : "strategy-edit-interval",
);
const executionModeTestId = computed(() =>
    isCreate.value ? "strategy-instance-execution-mode" : "strategy-edit-execution-mode",
);
const accountTriggerTestId = computed(() =>
    isCreate.value ? "strategy-instance-account" : "strategy-edit-account",
);
const accountSearchTestId = computed(() =>
    isCreate.value ? "strategy-instance-account-search" : "strategy-edit-account-search",
);
const accountOptionNoneTestId = computed(() =>
    isCreate.value ? "strategy-instance-account-option-none" : "strategy-edit-account-option-none",
);
const accountOptionPrefix = computed(() =>
    isCreate.value ? "strategy-instance-account-option" : "strategy-edit-account-option",
);
const currentTagTestId = computed(() =>
    isCreate.value ? "strategy-create-account-current-tag" : "strategy-edit-account-current-tag",
);
const previewHeading = computed(() => (isCreate.value ? "创建预览" : "绑定预览"));
const symbolHelperText = computed(() =>
    isCreate.value
    ? "先选市场，再输入代码；按 Enter、Tab、逗号可添加，也支持直接粘贴 US.TME、HK.00700 或多行列表。"
    : "为空时表示暂未绑定交易代码；先选市场再输入代码，按 Backspace 可快速删除最后一个标签。",
);
const notifyOnlyNotice = computed(() =>
    isCreate.value
        ? "仅通知模式只发送准备下单提示，不自动下单。"
        : "仅通知模式会发送准备下单提示，不自动下单。实例卡片会同步显示“仅通知”。",
);
const isSelectedCurrentBrokerAccount = computed(() =>
    props.selectedBrokerAccountKey !== ""
    && props.selectedBrokerAccountKey === props.currentBrokerAccountSelectionKey,
);
const createActionLabel = computed(() =>
    props.isCreatingStrategyInstance ? "创建中" : `添加${props.createDefinition?.name ?? "策略"}到实例`,
);

function closeDialog(): void {
    emit("update:open", false);
}

function handleDialogModelUpdate(value: boolean): void {
    emit("update:open", value);
}

function handleDefinitionChange(event: Event): void {
    emit("update:create-definition-id", (event.target as HTMLSelectElement).value);
}

function handleSymbolDraftInput(event: Event): void {
    emit("update:symbol-draft", (event.target as HTMLInputElement).value);
}

function handleSymbolMarketChange(event: Event): void {
    emit("update:symbol-market", (event.target as HTMLSelectElement).value);
}

function handleSymbolDraftKeydown(event: KeyboardEvent): void {
    emit("symbol-draft-keydown", event);
}

function handleSymbolDraftPaste(event: ClipboardEvent): void {
    emit("symbol-draft-paste", event);
}

function handleIntervalInput(event: Event): void {
    emit("update:interval", (event.target as HTMLInputElement).value);
}

function handleExecutionModeChange(event: Event): void {
    emit("update:execution-mode", (event.target as HTMLSelectElement).value);
}

function handleBrokerQueryInput(event: Event): void {
    emit("update:broker-query", (event.target as HTMLInputElement).value);
}
</script>

<template>
    <v-dialog :model-value="open" max-width="980" @update:model-value="handleDialogModelUpdate">
        <div class="strategy-instance-dialog" data-testid="strategy-instance-dialog">
            <div class="flex items-start justify-between gap-3">
                <div>
                    <div class="text-sm font-semibold uppercase tracking-[0.16em] text-slate-500">{{ title }}</div>
                    <div class="mt-1 text-sm text-slate-500">
                        {{ hint }}
                    </div>
                </div>
                <div class="flex flex-wrap items-center gap-2">
                    <button
                        v-if="isCreate"
                        class="rounded-full border border-slate-300 px-3 py-1.5 text-xs font-medium text-slate-700 transition hover:border-slate-400 hover:text-slate-900"
                        type="button"
                        @click="emit('refresh-definitions')"
                    >
                        {{ isLoadingDefinitions ? "等待" : "刷新定义" }}
                    </button>
                    <button
                        class="rounded-full border border-slate-300 px-3 py-1.5 text-xs font-medium text-slate-700 transition hover:border-slate-400 hover:text-slate-900"
                        :data-testid="closeTestId"
                        type="button"
                        @click="closeDialog"
                    >
                        关闭
                    </button>
                </div>
            </div>
            <div
                v-if="definitionsError && isCreate"
                class="mt-3 rounded-3xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700"
            >
                {{ definitionsError }}
            </div>
            <div
                v-else-if="isCreate && strategyDefinitions.length === 0"
                class="mt-3 rounded-3xl border border-dashed border-slate-300 bg-white px-4 py-5 text-sm text-slate-500"
            >
                <div>暂无已保存策略定义。</div>
                <button
                    class="mt-3 rounded-full border border-slate-300 px-4 py-2 text-sm font-medium text-slate-700 transition hover:border-slate-400 hover:text-slate-900"
                    type="button"
                    @click="emit('switch-to-design')"
                >
                    去设计区创建
                </button>
            </div>
            <div
                v-else-if="isEdit && selectedStrategy === null"
                class="mt-3 rounded-3xl border border-dashed border-slate-300 bg-white px-4 py-5 text-sm text-slate-500"
            >
                请先选择策略实例。
            </div>
            <div v-else class="mt-4 grid min-w-0 gap-4 xl:grid-cols-[minmax(0,1.25fr)_minmax(18rem,22rem)]">
                <div class="min-w-0 grid gap-3" :data-testid="panelTestId">
                    <label v-if="isCreate" class="grid gap-1.5 text-sm text-slate-600">
                        <span class="font-medium text-slate-700">策略定义</span>
                        <select
                            :value="createDefinitionId"
                            data-testid="strategy-instance-definition"
                            class="rounded-2xl border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 outline-none transition focus:border-slate-500"
                            @change="handleDefinitionChange"
                        >
                            <option value="" disabled>请选择策略定义</option>
                            <option v-for="definition in strategyDefinitions" :key="definition.id" :value="definition.id">
                                {{ definition.name }} / v{{ definition.version }}
                            </option>
                        </select>
                    </label>
                    <div v-else class="rounded-3xl bg-slate-50 px-4 py-4 text-sm text-slate-600">
                        <div class="text-xs uppercase tracking-[0.16em] text-slate-500">策略定义</div>
                        <div class="mt-2 break-words font-medium text-slate-900">
                            {{ selectedStrategy?.definition.name }} / v{{ selectedStrategy?.definition.version }}
                        </div>
                    </div>
                    <label class="grid gap-1.5 text-sm text-slate-600">
                        <span class="font-medium text-slate-700">交易代码</span>
                        <div class="grid gap-2">
                            <div class="grid gap-2 md:grid-cols-[9rem_minmax(0,1fr)_auto]">
                                <select
                                    :value="symbolMarket"
                                    :data-testid="symbolMarketTestId"
                                    class="rounded-2xl border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 outline-none transition focus:border-slate-500"
                                    @change="handleSymbolMarketChange"
                                >
                                    <option v-for="option in marketOptions" :key="option.value" :value="option.value">
                                        {{ option.title }}
                                    </option>
                                </select>
                                <input
                                    :value="symbolDraft"
                                    :data-testid="symbolInputTestId"
                                    class="rounded-2xl border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 outline-none transition focus:border-slate-500"
                                    placeholder="输入代码，例如 TME"
                                    type="text"
                                    @input="handleSymbolDraftInput"
                                    @blur="emit('commit-symbol-draft')"
                                    @keydown="handleSymbolDraftKeydown"
                                    @paste="handleSymbolDraftPaste"
                                >
                                <button
                                    class="rounded-full border border-slate-300 px-4 py-2 text-sm font-medium text-slate-700 transition hover:border-slate-400 hover:text-slate-900"
                                    :data-testid="symbolAddTestId"
                                    type="button"
                                    @mousedown.prevent
                                    @click="emit('commit-symbol-draft')"
                                >
                                    添加
                                </button>
                            </div>
                            <div class="strategy-tag-input" :class="{ 'strategy-tag-input--invalid': symbolValidationMessage !== '' }">
                                <button
                                    v-for="symbol in symbolTags"
                                    :key="`${mode}-${symbol}`"
                                    class="strategy-tag-chip"
                                    type="button"
                                    @click="emit('remove-symbol', symbol)"
                                >
                                    <span>{{ symbol }}</span>
                                    <span class="strategy-tag-chip__remove">x</span>
                                </button>
                                <span v-if="symbolTags.length === 0" class="text-xs text-slate-400">
                                    暂未添加交易代码
                                </span>
                            </div>
                        </div>
                        <span v-if="symbolValidationMessage" :data-testid="symbolValidationTestId" class="text-xs text-amber-700">
                            {{ symbolValidationMessage }}
                        </span>
                        <span v-else class="text-xs text-slate-500">
                            {{ symbolHelperText }}
                        </span>
                    </label>
                    <div class="grid gap-3 md:grid-cols-2">
                        <label class="grid gap-1.5 text-sm text-slate-600">
                            <span class="font-medium text-slate-700">运行周期</span>
                            <input
                                :value="intervalValue"
                                :data-testid="intervalInputTestId"
                                class="rounded-2xl border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 outline-none transition focus:border-slate-500"
                                placeholder="5m"
                                type="text"
                                @input="handleIntervalInput"
                            >
                        </label>
                        <label class="grid gap-1.5 text-sm text-slate-600">
                            <span class="font-medium text-slate-700">执行模式</span>
                            <select
                                :value="executionMode"
                                :data-testid="executionModeTestId"
                                class="rounded-2xl border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 outline-none transition focus:border-slate-500"
                                @change="handleExecutionModeChange"
                            >
                                <option value="live">确认执行</option>
                                <option value="notify_only">仅通知</option>
                            </select>
                        </label>
                    </div>
                    <label class="grid gap-1.5 text-sm text-slate-600">
                        <span class="font-medium text-slate-700">券商账号</span>
                        <div class="strategy-account-picker">
                            <button
                                class="strategy-account-picker__trigger"
                                :data-testid="accountTriggerTestId"
                                type="button"
                                @click="emit('toggle-broker-picker')"
                            >
                                <span class="strategy-account-picker__copy">
                                    <span class="strategy-account-picker__label">
                                        {{ selectedBrokerAccountOption?.displayName ?? '暂不绑定账号' }}
                                    </span>
                                    <span v-if="selectedBrokerAccountOption" class="strategy-account-picker__meta">
                                        <span>{{ brokerAccountOptionSubtitle(selectedBrokerAccountOption) }}</span>
                                        <span
                                            v-if="isSelectedCurrentBrokerAccount"
                                            :data-testid="currentTagTestId"
                                            class="strategy-account-picker__tag strategy-account-picker__tag--current"
                                        >
                                            当前
                                        </span>
                                    </span>
                                    <span v-else class="strategy-account-picker__meta">保留当前默认路由</span>
                                </span>
                                <span class="strategy-account-picker__action">
                                    {{ isBrokerAccountPickerOpen ? '收起' : '搜索选择' }}
                                </span>
                            </button>
                            <div v-if="isBrokerAccountPickerOpen" class="strategy-account-picker__menu">
                                <input
                                    :value="brokerAccountQuery"
                                    :data-testid="accountSearchTestId"
                                    class="strategy-account-picker__search"
                                    placeholder="搜索账号 / 环境 / 市场"
                                    type="text"
                                    @input="handleBrokerQueryInput"
                                >
                                <div class="strategy-account-picker__options">
                                    <button
                                        class="strategy-account-picker__option"
                                        :class="{ 'is-active': selectedBrokerAccountKey === '' }"
                                        :data-testid="accountOptionNoneTestId"
                                        type="button"
                                        @click="emit('clear-broker-selection')"
                                    >
                                        <span class="strategy-account-picker__option-title">暂不绑定账号</span>
                                        <span class="strategy-account-picker__option-meta">保留当前默认路由</span>
                                    </button>
                                    <button
                                        v-for="option in filteredBrokerAccountOptions"
                                        :key="option.selectionKey"
                                        class="strategy-account-picker__option"
                                        :class="{ 'is-active': selectedBrokerAccountKey === option.selectionKey }"
                                        :data-testid="`${accountOptionPrefix}-${option.accountId}`"
                                        type="button"
                                        @click="emit('select-broker-selection', option.selectionKey)"
                                    >
                                        <span class="strategy-account-picker__option-header">
                                            <span class="strategy-account-picker__option-title">{{ option.displayName }}</span>
                                            <span
                                                v-if="option.selectionKey === currentBrokerAccountSelectionKey"
                                                class="strategy-account-picker__tag strategy-account-picker__tag--current"
                                            >
                                                当前
                                            </span>
                                        </span>
                                        <span class="strategy-account-picker__option-meta">{{ brokerAccountOptionSubtitle(option) }}</span>
                                    </button>
                                    <div v-if="filteredBrokerAccountOptions.length === 0" class="strategy-account-picker__empty">
                                        没有匹配的券商账号。
                                    </div>
                                </div>
                            </div>
                        </div>
                    </label>
                    <div v-if="executionMode === 'notify_only'" class="rounded-3xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-700">
                        {{ notifyOnlyNotice }}
                    </div>
                </div>

                <div class="min-w-0 rounded-3xl bg-slate-50 px-4 py-4">
                    <div class="text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">
                        {{ previewHeading }}
                    </div>
                    <div class="mt-4 grid gap-3 text-sm text-slate-600">
                        <div>
                            <div class="text-xs uppercase tracking-[0.16em] text-slate-400">策略定义</div>
                            <div class="mt-1 break-words font-medium text-slate-900">
                                {{ previewDefinitionLabel }}
                            </div>
                        </div>
                        <div>
                            <div class="text-xs uppercase tracking-[0.16em] text-slate-400">交易代码</div>
                            <div class="mt-1 break-words font-medium text-slate-900">
                                {{ symbolsSummary }}
                            </div>
                        </div>
                        <div>
                            <div class="text-xs uppercase tracking-[0.16em] text-slate-400">周期</div>
                            <div class="mt-1 font-medium text-slate-900">
                                {{ intervalValue.trim() || '5m' }}
                            </div>
                        </div>
                        <div>
                            <div class="text-xs uppercase tracking-[0.16em] text-slate-400">执行模式</div>
                            <div class="mt-1 font-medium text-slate-900">
                                {{ executionMode === 'notify_only' ? '仅通知' : '确认执行' }}
                            </div>
                        </div>
                        <div>
                            <div class="text-xs uppercase tracking-[0.16em] text-slate-400">券商账号</div>
                            <div class="mt-1 break-all font-medium text-slate-900">
                                {{ brokerAccountSummary }}
                            </div>
                            <div
                                v-if="isSelectedCurrentBrokerAccount"
                                class="mt-2 inline-flex rounded-full border border-emerald-200 bg-emerald-50 px-2 py-0.5 text-[11px] font-semibold uppercase tracking-[0.14em] text-emerald-700"
                            >
                                当前
                            </div>
                        </div>
                    </div>
                    <div class="mt-4 flex flex-wrap gap-2">
                        <button
                            v-if="isCreate"
                            class="rounded-full border border-slate-900 bg-slate-900 px-4 py-2 text-sm font-medium text-white transition hover:bg-slate-800 disabled:cursor-not-allowed disabled:opacity-50"
                            data-testid="strategy-create-instance"
                            :disabled="!canCreateStrategyInstance"
                            type="button"
                            @click="emit('submit-create')"
                        >
                            {{ createActionLabel }}
                        </button>
                        <template v-else>
                            <button
                                class="rounded-full border border-slate-900 bg-slate-900 px-4 py-2 text-sm font-medium text-white transition hover:bg-slate-800 disabled:cursor-not-allowed disabled:opacity-50"
                                data-testid="strategy-update-binding"
                                :disabled="!canUpdateSelectedStrategyBinding"
                                type="button"
                                @click="emit('submit-update')"
                            >
                                {{ isUpdatingStrategyBinding ? "保存中" : "保存绑定" }}
                            </button>
                            <button
                                class="rounded-full border border-rose-300 px-4 py-2 text-sm font-medium text-rose-700 transition hover:border-rose-400 hover:text-rose-900 disabled:cursor-not-allowed disabled:opacity-50"
                                data-testid="strategy-delete-instance"
                                :disabled="!canDeleteSelectedStrategy"
                                type="button"
                                @click="emit('submit-delete')"
                            >
                                {{ isDeletingStrategy ? "删除中" : "删除实例" }}
                            </button>
                        </template>
                    </div>
                    <div v-if="isEdit && selectedStrategy?.status !== 'STOPPED'" class="mt-3 text-xs text-amber-700">
                        当前实例不是 STOPPED，先停止后才能修改绑定或删除。
                    </div>
                </div>
            </div>
        </div>
    </v-dialog>
</template>

<style scoped>
:global(.tv-main) .strategy-instance-dialog {
    max-height: calc(100vh - 2rem);
    overflow-y: auto;
    overflow-x: hidden;
    border-color: var(--card-border);
    background: var(--card-surface);
    color: var(--card-text-1);
}

:global(.tv-main) .strategy-instance-dialog .text-slate-900,
:global(.tv-main) .strategy-instance-dialog .text-slate-800,
:global(.tv-main) .strategy-instance-dialog .text-slate-700 {
    color: var(--card-text-1);
}

:global(.tv-main) .strategy-instance-dialog .text-slate-600,
:global(.tv-main) .strategy-instance-dialog .text-slate-500 {
    color: var(--card-text-2);
}

:global(.tv-main) .strategy-instance-dialog .text-slate-400 {
    color: var(--card-text-3);
}

:global(.tv-main) .strategy-instance-dialog .bg-white,
:global(.tv-main) .strategy-instance-dialog .bg-slate-50 {
    background: var(--card-surface-raised);
}

:global(.tv-main) .strategy-instance-dialog .border-slate-200,
:global(.tv-main) .strategy-instance-dialog .border-slate-300 {
    border-color: var(--card-border);
}

:global(.tv-main) .strategy-instance-dialog .bg-amber-50 {
    background: var(--card-amber-surface);
}

:global(.tv-main) .strategy-instance-dialog .border-amber-200 {
    border-color: var(--card-amber-border);
}

:global(.tv-main) .strategy-instance-dialog .text-amber-700,
:global(.tv-main) .strategy-instance-dialog .text-amber-800 {
    color: var(--card-amber-text);
}

:global(.tv-main) .strategy-instance-dialog .bg-red-50 {
    background: var(--card-red-surface);
}

:global(.tv-main) .strategy-instance-dialog .border-red-200 {
    border-color: var(--card-red-border);
}

:global(.tv-main) .strategy-instance-dialog .text-red-700,
:global(.tv-main) .strategy-instance-dialog .text-red-800 {
    color: var(--card-red-text);
}

:global(.tv-main) .strategy-instance-dialog .bg-emerald-50 {
    background: var(--card-teal-surface);
}

:global(.tv-main) .strategy-instance-dialog .border-emerald-200 {
    border-color: var(--card-teal-border);
}

:global(.tv-main) .strategy-instance-dialog .text-emerald-700,
:global(.tv-main) .strategy-instance-dialog .text-emerald-800 {
    color: var(--card-teal-text);
}

:global(.tv-main) .strategy-instance-dialog .bg-sky-50 {
    background: color-mix(in srgb, var(--card-active-surface) 88%, transparent);
}

:global(.tv-main) .strategy-instance-dialog .border-sky-200 {
    border-color: var(--card-active-border);
}

:global(.tv-main) .strategy-instance-dialog .text-sky-700,
:global(.tv-main) .strategy-instance-dialog .text-sky-800 {
    color: var(--card-active-text);
}

:global(.tv-main) .strategy-account-picker__menu {
    position: static;
    top: auto;
    left: auto;
    right: auto;
    z-index: auto;
    margin-top: 0.45rem;
    border-color: var(--card-border);
    background: var(--card-surface);
    box-shadow: 0 18px 40px rgb(15 23 42 / 0.14);
}

:global(.tv-main) .strategy-account-picker__search {
    border-color: var(--card-border);
    background: var(--card-surface-raised);
    color: var(--card-text-1);
}

:global(.tv-main) .strategy-account-picker__search:focus {
    border-color: color-mix(in srgb, var(--tv-accent) 72%, var(--card-border));
    background: var(--card-surface);
}

:global(.tv-main) .strategy-account-picker__option {
    background: var(--card-surface-raised);
    border-color: transparent;
}

:global(.tv-main) .strategy-account-picker__option:hover {
    border-color: var(--card-active-border);
    background: color-mix(in srgb, var(--card-active-surface) 72%, var(--card-surface));
}

:global(.tv-main) .strategy-account-picker__option.is-active {
    border-color: var(--card-active-border);
    background: color-mix(in srgb, var(--card-active-surface) 84%, var(--card-surface));
}

:global(.tv-main) .strategy-account-picker__label,
:global(.tv-main) .strategy-account-picker__option-title,
:global(.tv-main) .strategy-account-picker__option-header {
    color: var(--card-text-1);
}

:global(.tv-main) .strategy-account-picker__meta,
:global(.tv-main) .strategy-account-picker__action,
:global(.tv-main) .strategy-account-picker__option-meta,
:global(.tv-main) .strategy-account-picker__empty {
    color: var(--card-text-2);
}

:global(.tv-main) .strategy-account-picker__tag--current {
    border-color: var(--card-teal-border);
    background: color-mix(in srgb, var(--card-teal-surface) 86%, transparent);
    color: var(--card-teal-text);
}

:global(.tv-main) .strategy-account-picker__empty {
    border-color: var(--card-border);
    background: color-mix(in srgb, var(--card-surface-raised) 88%, transparent);
}

:global(.tv-main) .strategy-tag-input {
    border-color: var(--card-border);
    background: var(--card-surface);
}

:global(.tv-main) .strategy-tag-input:focus-within {
    border-color: color-mix(in srgb, var(--tv-accent) 70%, var(--card-border));
}

:global(.tv-main) .strategy-tag-input--invalid {
    border-color: var(--card-amber-border);
    background: var(--card-amber-surface);
}

:global(.tv-main) .strategy-tag-input--invalid:focus-within {
    border-color: color-mix(in srgb, var(--card-amber-text) 70%, var(--card-amber-border));
}

:global(.tv-main) .strategy-tag-chip {
    border-color: var(--card-active-border);
    background: color-mix(in srgb, var(--card-active-surface) 88%, var(--card-surface));
    color: var(--card-active-text);
}

:global(.tv-main) .strategy-tag-chip__remove {
    color: var(--card-text-2);
}

:global(.tv-main) .strategy-tag-input__field {
    color: var(--card-text-1);
}

:global(.tv-main) .strategy-tag-input__field::placeholder {
    color: var(--card-text-3);
}

.strategy-instance-dialog {
    border-radius: 1.75rem;
    border: 1px solid rgb(226 232 240);
    background: white;
    padding: 1.25rem;
    box-shadow: 0 24px 90px rgb(15 23 42 / 0.2);
}

.strategy-tag-input {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.5rem;
    min-height: 3rem;
    padding: 0.6rem 0.75rem;
    border-radius: 1rem;
    border: 1px solid rgb(203 213 225);
    background: white;
    transition: border-color 140ms ease;
}

.strategy-tag-input:focus-within {
    border-color: rgb(100 116 139);
}

.strategy-tag-input--invalid {
    border-color: rgb(245 158 11);
    background: rgb(255 251 235);
}

.strategy-tag-input--invalid:focus-within {
    border-color: rgb(217 119 6);
}

.strategy-tag-chip {
    display: inline-flex;
    align-items: center;
    gap: 0.45rem;
    max-width: 100%;
    padding: 0.35rem 0.7rem;
    border-radius: 999px;
    border: 1px solid rgb(191 219 254);
    background: rgb(239 246 255);
    color: rgb(30 64 175);
    font-size: 0.76rem;
    font-weight: 600;
    line-height: 1;
}

.strategy-tag-chip__remove {
    color: rgb(71 85 105);
    font-size: 0.72rem;
    text-transform: uppercase;
}

.strategy-tag-input__field {
    flex: 1 1 10rem;
    min-width: 10rem;
    border: 0;
    outline: none;
    background: transparent;
    color: rgb(15 23 42);
    font-size: 0.875rem;
    padding: 0.1rem 0;
}

.strategy-tag-input__field::placeholder {
    color: rgb(148 163 184);
}

.strategy-account-picker {
    position: relative;
}

.strategy-account-picker__trigger {
    display: flex;
    width: 100%;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    border-radius: 1rem;
    border: 1px solid rgb(203 213 225);
    background: white;
    padding: 0.75rem 0.85rem;
    text-align: left;
    transition: border-color 140ms ease, box-shadow 140ms ease;
}

.strategy-account-picker__trigger:hover {
    border-color: rgb(148 163 184);
}

.strategy-account-picker__trigger:focus-visible {
    outline: none;
    border-color: rgb(100 116 139);
    box-shadow: 0 0 0 3px rgb(226 232 240 / 0.9);
}

.strategy-account-picker__copy {
    display: grid;
    min-width: 0;
    gap: 0.2rem;
}

.strategy-account-picker__label {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: rgb(15 23 42);
    font-size: 0.875rem;
    font-weight: 600;
}

.strategy-account-picker__meta {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.4rem;
    color: rgb(100 116 139);
    font-size: 0.74rem;
    line-height: 1.3;
}

.strategy-account-picker__action {
    flex-shrink: 0;
    color: rgb(71 85 105);
    font-size: 0.74rem;
    font-weight: 600;
}

.strategy-account-picker__menu {
    z-index: 20;
    display: grid;
    gap: 0.65rem;
    border-radius: 1.1rem;
    border: 1px solid rgb(226 232 240);
    background: white;
    padding: 0.8rem;
    box-shadow: 0 18px 40px rgb(15 23 42 / 0.14);
}

.strategy-account-picker__search {
    width: 100%;
    border-radius: 0.9rem;
    border: 1px solid rgb(203 213 225);
    background: rgb(248 250 252);
    padding: 0.7rem 0.8rem;
    color: rgb(15 23 42);
    font-size: 0.875rem;
    outline: none;
}

.strategy-account-picker__search:focus {
    border-color: rgb(100 116 139);
    background: white;
}

.strategy-account-picker__options {
    display: grid;
    gap: 0.45rem;
    max-height: 16rem;
    overflow-y: auto;
}

.strategy-account-picker__option {
    display: grid;
    gap: 0.25rem;
    width: 100%;
    border-radius: 0.95rem;
    border: 1px solid transparent;
    background: rgb(248 250 252);
    padding: 0.7rem 0.8rem;
    text-align: left;
    transition: border-color 140ms ease, background-color 140ms ease;
}

.strategy-account-picker__option:hover {
    border-color: rgb(191 219 254);
    background: rgb(239 246 255);
}

.strategy-account-picker__option.is-active {
    border-color: rgb(59 130 246);
    background: rgb(239 246 255);
}

.strategy-account-picker__option-header {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
}

.strategy-account-picker__option-title {
    color: rgb(15 23 42);
    font-size: 0.84rem;
    font-weight: 600;
}

.strategy-account-picker__option-meta {
    color: rgb(100 116 139);
    font-size: 0.72rem;
    line-height: 1.35;
}

.strategy-account-picker__tag {
    display: inline-flex;
    align-items: center;
    border-radius: 999px;
    padding: 0.15rem 0.5rem;
    font-size: 0.64rem;
    font-weight: 700;
    letter-spacing: 0.12em;
    text-transform: uppercase;
}

.strategy-account-picker__tag--current {
    border: 1px solid rgb(167 243 208);
    background: rgb(236 253 245);
    color: rgb(4 120 87);
}

.strategy-account-picker__empty {
    border-radius: 0.95rem;
    border: 1px dashed rgb(203 213 225);
    padding: 0.9rem 0.8rem;
    color: rgb(100 116 139);
    font-size: 0.78rem;
}
</style>