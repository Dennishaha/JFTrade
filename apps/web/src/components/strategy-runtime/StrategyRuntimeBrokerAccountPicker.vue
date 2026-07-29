<script setup lang="ts">
import type { BrokerAccountSelectionOption } from "../../composables/consoleDataBrokerAccountSelection";
import { brokerAccountOptionSubtitle } from "./strategyRuntimeInstanceBinding";

defineProps<{
    selectedOption: BrokerAccountSelectionOption | null;
    selectedKey: string;
    currentKey: string;
    open: boolean;
    query: string;
    options: BrokerAccountSelectionOption[];
    triggerTestId: string;
    searchTestId: string;
    noneTestId: string;
    optionTestIdPrefix: string;
    currentTagTestId: string;
}>();

const emit = defineEmits<{
    toggle: [];
    "update:query": [value: string];
    clear: [];
    select: [selectionKey: string];
}>();

function handleQueryInput(event: Event): void {
    emit("update:query", (event.target as HTMLInputElement).value);
}
</script>

<template>
    <label class="grid gap-1.5 text-sm text-slate-600">
        <span class="font-medium text-slate-700">券商账号</span>
        <div class="strategy-account-picker">
            <button
                class="strategy-account-picker__trigger"
                :data-testid="triggerTestId"
                type="button"
                @click="emit('toggle')"
            >
                <span class="strategy-account-picker__copy">
                    <span class="strategy-account-picker__label">
                        {{ selectedOption?.displayName ?? "暂不绑定账号" }}
                    </span>
                    <span v-if="selectedOption" class="strategy-account-picker__meta">
                        <span>{{ brokerAccountOptionSubtitle(selectedOption) }}</span>
                        <span
                            v-if="selectedKey !== '' && selectedKey === currentKey"
                            :data-testid="currentTagTestId"
                            class="strategy-account-picker__tag strategy-account-picker__tag--current"
                        >
                            当前
                        </span>
                    </span>
                    <span v-else class="strategy-account-picker__meta">保留当前默认路由</span>
                </span>
                <span class="strategy-account-picker__action">
                    {{ open ? "收起" : "搜索选择" }}
                </span>
            </button>
            <div v-if="open" class="strategy-account-picker__menu">
                <input
                    :value="query"
                    :data-testid="searchTestId"
                    class="strategy-account-picker__search"
                    placeholder="搜索账号 / 环境 / 市场"
                    type="text"
                    @input="handleQueryInput"
                >
                <div class="strategy-account-picker__options">
                    <button
                        class="strategy-account-picker__option"
                        :class="{ 'is-active': selectedKey === '' }"
                        :data-testid="noneTestId"
                        type="button"
                        @click="emit('clear')"
                    >
                        <span class="strategy-account-picker__option-title">暂不绑定账号</span>
                        <span class="strategy-account-picker__option-meta">保留当前默认路由</span>
                    </button>
                    <button
                        v-for="option in options"
                        :key="option.selectionKey"
                        class="strategy-account-picker__option"
                        :class="{ 'is-active': selectedKey === option.selectionKey }"
                        :data-testid="`${optionTestIdPrefix}-${option.accountId}`"
                        type="button"
                        @click="emit('select', option.selectionKey)"
                    >
                        <span class="strategy-account-picker__option-header">
                            <span class="strategy-account-picker__option-title">{{ option.displayName }}</span>
                            <span
                                v-if="option.selectionKey === currentKey"
                                class="strategy-account-picker__tag strategy-account-picker__tag--current"
                            >
                                当前
                            </span>
                        </span>
                        <span class="strategy-account-picker__option-meta">{{ brokerAccountOptionSubtitle(option) }}</span>
                    </button>
                    <div v-if="options.length === 0" class="strategy-account-picker__empty">
                        没有匹配的券商账号。
                    </div>
                </div>
            </div>
        </div>
    </label>
</template>

<style scoped>
.strategy-account-picker { position: relative; }

.strategy-account-picker__trigger {
    display: flex;
    width: 100%;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    border-radius: 1rem;
    border: 1px solid var(--card-border);
    background: var(--card-surface);
    padding: 0.75rem 0.85rem;
    text-align: left;
    transition: border-color 140ms ease, box-shadow 140ms ease, background-color 140ms ease;
}

.strategy-account-picker__trigger:hover {
    border-color: color-mix(in srgb, var(--card-text-3) 55%, var(--card-border));
}

.strategy-account-picker__trigger:focus-visible {
    outline: none;
    border-color: color-mix(in srgb, var(--tv-accent) 70%, var(--card-border));
    box-shadow: 0 0 0 3px color-mix(in srgb, var(--tv-accent) 18%, transparent);
}

.strategy-account-picker__copy { display: grid; min-width: 0; gap: 0.2rem; }
.strategy-account-picker__label {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--card-text-1);
    font-size: 0.875rem;
    font-weight: 600;
}
.strategy-account-picker__meta {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.4rem;
    color: var(--card-text-2);
    font-size: 0.74rem;
    line-height: 1.3;
}
.strategy-account-picker__action {
    flex-shrink: 0;
    color: var(--card-text-2);
    font-size: 0.74rem;
    font-weight: 600;
}
.strategy-account-picker__menu {
    z-index: 20;
    display: grid;
    gap: 0.65rem;
    position: static;
    margin-top: 0.45rem;
    border-radius: 1.1rem;
    border: 1px solid var(--card-border);
    background: var(--card-surface);
    padding: 0.8rem;
    box-shadow: 0 18px 40px rgb(2 6 23 / 0.24);
}
.strategy-account-picker__search {
    width: 100%;
    border-radius: 0.9rem;
    border: 1px solid var(--card-border);
    background: var(--card-surface-raised);
    padding: 0.7rem 0.8rem;
    color: var(--card-text-1);
    font-size: 0.875rem;
    outline: none;
}
.strategy-account-picker__search:focus {
    border-color: color-mix(in srgb, var(--tv-accent) 72%, var(--card-border));
    background: var(--card-surface);
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
    background: var(--card-surface-raised);
    padding: 0.7rem 0.8rem;
    text-align: left;
    transition: border-color 140ms ease, background-color 140ms ease;
}
.strategy-account-picker__option:hover {
    border-color: var(--card-active-border);
    background: color-mix(in srgb, var(--card-active-surface) 72%, var(--card-surface));
}
.strategy-account-picker__option.is-active {
    border-color: var(--card-active-border);
    background: color-mix(in srgb, var(--card-active-surface) 84%, var(--card-surface));
}
.strategy-account-picker__option-header {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
}
.strategy-account-picker__option-title { color: var(--card-text-1); font-size: 0.84rem; font-weight: 600; }
.strategy-account-picker__option-meta { color: var(--card-text-2); font-size: 0.72rem; line-height: 1.35; }
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
    border: 1px solid var(--card-teal-border);
    background: color-mix(in srgb, var(--card-teal-surface) 86%, transparent);
    color: var(--card-teal-text);
}
.strategy-account-picker__empty {
    border-radius: 0.95rem;
    border: 1px dashed var(--card-border);
    background: color-mix(in srgb, var(--card-surface-raised) 88%, transparent);
    padding: 0.9rem 0.8rem;
    color: var(--card-text-2);
    font-size: 0.78rem;
}
</style>
