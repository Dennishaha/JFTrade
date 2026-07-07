<script setup lang="ts">
import { computed } from "vue";
import { Background } from "@vue-flow/background";
import { Controls } from "@vue-flow/controls";
import { MiniMap } from "@vue-flow/minimap";
import { VueFlow, type Connection, type NodeMouseEvent } from "@vue-flow/core";
import "@vue-flow/core/dist/style.css";
import "@vue-flow/core/dist/theme-default.css";
import "@vue-flow/controls/dist/style.css";
import "@vue-flow/minimap/dist/style.css";

import {
  nodeRunClass,
  statusLabel,
  type FlowEdgeSnapshot,
  type FlowNodeSnapshot,
} from "@/features/adkWorkflowStudio";

const props = defineProps<{
  nodes: FlowNodeSnapshot[];
  edges: FlowEdgeSnapshot[];
  selectedNodeId: string;
}>();

const emit = defineEmits<{
  "update:nodes": [nodes: FlowNodeSnapshot[]];
  "update:edges": [edges: FlowEdgeSnapshot[]];
  connect: [connection: Connection];
  nodeClick: [event: NodeMouseEvent];
  selectNode: [nodeId: string];
}>();

const flowNodes = computed({
  get: () => props.nodes,
  set: (value) => emit("update:nodes", value),
});

const flowEdges = computed({
  get: () => props.edges,
  set: (value) => emit("update:edges", value),
});
</script>

<template>
  <div class="adk-workflow-canvas-shell">
    <VueFlow
      v-model:nodes="flowNodes"
      v-model:edges="flowEdges"
      :fit-view-on-init="true"
      :default-edge-options="{ type: 'smoothstep' }"
      class="adk-workflow-flow"
      @connect="$emit('connect', $event)"
      @node-click="$emit('nodeClick', $event)"
    >
      <Background pattern-color="rgba(148, 163, 184, 0.22)" :gap="18" />
      <MiniMap
        pannable
        zoomable
        :width="128"
        :height="96"
        aria-label="工作流缩略图"
        class="adk-workflow-minimap"
        node-color="var(--adk-workflow-minimap-node)"
        node-stroke-color="var(--adk-workflow-minimap-node-stroke)"
        :node-stroke-width="2"
        mask-color="var(--adk-workflow-minimap-mask)"
      />
      <Controls />

      <template #node-start="{ data, selected }">
        <button
          type="button"
          class="adk-flow-node is-start"
          :class="[
            { 'is-selected': selected || selectedNodeId === 'start' },
            nodeRunClass(data.runStatus),
          ]"
          @click.stop="$emit('selectNode', 'start')"
        >
          <span class="adk-flow-node__icon"><v-icon size="15">fa-solid fa-play</v-icon></span>
          <span>
            <strong>{{ data.title }}</strong>
            <small>{{ data.subtitle }}</small>
          </span>
          <em>{{ statusLabel(data.status) }}</em>
        </button>
      </template>

      <template #node-trigger="{ id, data, selected }">
        <button
          type="button"
          class="adk-flow-node is-trigger"
          :class="[
            { 'is-selected': selected || selectedNodeId === id },
            nodeRunClass(data.runStatus),
          ]"
          @click.stop="$emit('selectNode', id)"
        >
          <span class="adk-flow-node__icon"><v-icon size="15">fa-solid fa-bolt</v-icon></span>
          <span>
            <strong>{{ data.title }}</strong>
            <small>{{ data.subtitle }}</small>
          </span>
          <em>{{ statusLabel(data.status) }}</em>
        </button>
      </template>

      <template #node-agent="{ id, data, selected }">
        <button
          type="button"
          class="adk-flow-node is-agent"
          :class="[
            { 'is-selected': selected || selectedNodeId === id },
            nodeRunClass(data.runStatus),
          ]"
          @click.stop="$emit('selectNode', id)"
        >
          <span class="adk-flow-node__icon"><v-icon size="15">fa-solid fa-robot</v-icon></span>
          <span>
            <strong>{{ data.title }}</strong>
            <small>{{ data.subtitle }}</small>
          </span>
          <em>{{ statusLabel(data.status) }}</em>
        </button>
      </template>

      <template #node-monitor="{ data, selected }">
        <button
          type="button"
          class="adk-flow-node is-monitor"
          :class="[
            { 'is-selected': selected || selectedNodeId === 'monitor' },
            nodeRunClass(data.runStatus),
          ]"
          @click.stop="$emit('selectNode', 'monitor')"
        >
          <span class="adk-flow-node__icon"><v-icon size="15">fa-solid fa-chart-simple</v-icon></span>
          <span>
            <strong>{{ data.title }}</strong>
            <small>{{ data.subtitle }}</small>
          </span>
          <em>{{ statusLabel(data.status) }}</em>
        </button>
      </template>
    </VueFlow>
  </div>
</template>
