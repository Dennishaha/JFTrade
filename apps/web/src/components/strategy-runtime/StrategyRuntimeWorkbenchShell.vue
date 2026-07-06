<script setup lang="ts">
import type { SplitpanesResizedPayload } from "splitpanes";

import SplitPane from "../shared/SplitPane.vue";
import SplitPaneItem from "../shared/SplitPaneItem.vue";

defineProps<{
    runtimePaneSizes: [number, number];
}>();

const emit = defineEmits<{
    resized: [payload: SplitpanesResizedPayload];
}>();
</script>

<template>
    <div class="runtime-panel__workspace">
        <slot name="messages" />

        <SplitPane class="runtime-panel__split" :pane-min-size="18" @resized="emit('resized', $event)">
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
    </div>
</template>
