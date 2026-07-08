<script setup lang="ts">
import type { SplitpanesResizedPayload } from "splitpanes";
import { computed } from "vue";

import SplitPane from "../shared/SplitPane.vue";
import SplitPaneItem from "../shared/SplitPaneItem.vue";

type StrategyRuntimeWorkbenchLayout = "desktop" | "compact" | "mobile";
type StrategyRuntimeMobileSection = "instances" | "workbench";

const props = withDefaults(defineProps<{
    runtimePaneSizes: [number, number];
    layout?: StrategyRuntimeWorkbenchLayout;
    mobileSection?: StrategyRuntimeMobileSection;
    hasSelectedDetail?: boolean;
}>(), {
    layout: "desktop",
    mobileSection: "instances",
    hasSelectedDetail: false,
});

const emit = defineEmits<{
    resized: [payload: SplitpanesResizedPayload];
    "update:mobile-section": [section: StrategyRuntimeMobileSection];
}>();

const effectiveMobileSection = computed<StrategyRuntimeMobileSection>(() =>
    props.hasSelectedDetail ? props.mobileSection : "instances",
);

function selectMobileSection(section: StrategyRuntimeMobileSection): void {
    if (section === "workbench" && !props.hasSelectedDetail) {
        emit("update:mobile-section", "instances");
        return;
    }
    emit("update:mobile-section", section);
}
</script>

<template>
    <div
        class="runtime-panel__workspace"
        :class="`runtime-panel__workspace--${layout}`"
    >
        <slot name="messages" />

        <nav
            v-if="layout === 'mobile'"
            class="runtime-panel__mobile-switch"
            aria-label="策略执行移动端工作区"
        >
            <button
                class="runtime-panel__mobile-switch-button"
                :class="{ 'is-active': effectiveMobileSection === 'instances' }"
                data-testid="strategy-runtime-mobile-section-instances"
                type="button"
                @click="selectMobileSection('instances')"
            >
                实例
            </button>
            <button
                class="runtime-panel__mobile-switch-button"
                :class="{ 'is-active': effectiveMobileSection === 'workbench' }"
                data-testid="strategy-runtime-mobile-section-workbench"
                :disabled="!hasSelectedDetail"
                type="button"
                @click="selectMobileSection('workbench')"
            >
                操作台
            </button>
        </nav>

        <SplitPane
            v-if="layout === 'desktop'"
            class="runtime-panel__split"
            :pane-min-size="18"
            @resized="emit('resized', $event)"
        >
            <SplitPaneItem :size="runtimePaneSizes[0]" :min-size="22" :max-size="55">
                <aside class="runtime-panel__pane">
                    <slot name="list" />
                </aside>
            </SplitPaneItem>

            <SplitPaneItem :size="runtimePaneSizes[1]" :min-size="45">
                <main class="runtime-panel__pane">
                    <slot name="detail" />
                </main>
            </SplitPaneItem>
        </SplitPane>

        <div
            v-else
            class="runtime-panel__compact-stack"
            :class="{ 'runtime-panel__compact-stack--mobile': layout === 'mobile' }"
        >
            <aside
                v-show="layout === 'compact' || effectiveMobileSection === 'instances'"
                class="runtime-panel__compact-pane runtime-panel__compact-pane--list"
            >
                <slot name="list" />
            </aside>

            <main
                v-show="layout === 'compact' || effectiveMobileSection === 'workbench'"
                class="runtime-panel__compact-pane runtime-panel__compact-pane--detail"
            >
                <slot name="detail" />
            </main>
        </div>
    </div>
</template>
