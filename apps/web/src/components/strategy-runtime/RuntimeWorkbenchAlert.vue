<script setup lang="ts">
const props = withDefaults(defineProps<{
    tone: "success" | "warning" | "error";
    closeLabel?: string;
    closeTestId?: string;
    role?: string;
}>(), {
    closeLabel: "关闭",
});

const emit = defineEmits<{
    close: [];
}>();
</script>

<template>
    <div
        class="runtime-workbench-alert runtime-workbench-alert--dismissible"
        :class="`runtime-workbench-alert--${tone}`"
        :role="props.role"
    >
        <div class="runtime-workbench-alert__content">
            <slot />
        </div>
        <button
            :aria-label="closeLabel"
            class="runtime-workbench-alert__close"
            :data-testid="closeTestId"
            type="button"
            @click="emit('close')"
        >
            &times;
        </button>
    </div>
</template>
