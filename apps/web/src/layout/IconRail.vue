<script setup lang="ts">
import {
  Connection,
  Cpu,
  DataAnalysis,
  Document,
  List,
  MagicStick,
  Monitor,
  Setting,
  TrendCharts,
  Wallet,
  Warning,
} from "@element-plus/icons-vue";
import { type Component, computed } from "vue";
import { useRoute, useRouter } from "vue-router";

import { useDocsLink } from "../composables/useDocsLink";

interface InternalNavItem {
  type: "route";
  to: string;
  label: string;
  icon: Component;
}

interface ExternalNavItem {
  type: "external";
  href: string;
  label: string;
  icon: Component;
}

type NavItem = InternalNavItem | ExternalNavItem;

const { docsHomeUrl } = useDocsLink();

const items: NavItem[] = [
  { type: "route", to: "/workspace", label: "Trade", icon: Monitor },
  { type: "route", to: "/overview", label: "Overview", icon: DataAnalysis },
  { type: "route", to: "/market", label: "Market", icon: TrendCharts },
  { type: "route", to: "/strategy", label: "Strategy", icon: MagicStick },
  { type: "route", to: "/execution", label: "Execution", icon: List },
  { type: "route", to: "/portfolio", label: "Portfolio", icon: Wallet },
  { type: "route", to: "/broker", label: "Broker", icon: Connection },
  { type: "route", to: "/risk", label: "Risk", icon: Warning },
  { type: "route", to: "/system", label: "System", icon: Cpu },
  { type: "route", to: "/settings", label: "Settings", icon: Setting },
  { type: "external", href: docsHomeUrl, label: "Docs", icon: Document },
];

const route = useRoute();
const router = useRouter();

const activeTo = computed(() => route.path);

function go(to: string): void {
  void router.push(to);
}
</script>

<template>
  <nav class="tv-iconrail" aria-label="primary">
    <template v-for="item in items" :key="item.label">
      <a
        v-if="item.type === 'external'"
        :href="item.href"
        target="_blank"
        rel="noopener noreferrer"
        class="tv-iconrail-btn"
        :title="item.label"
      >
        <el-icon :size="18">
          <component :is="item.icon" />
        </el-icon>
        <span>{{ item.label }}</span>
      </a>
      <button
        v-else
        type="button"
        class="tv-iconrail-btn"
        :class="{ 'is-active': activeTo === item.to }"
        :title="item.label"
        @click="go(item.to)"
      >
        <el-icon :size="18">
          <component :is="item.icon" />
        </el-icon>
        <span>{{ item.label }}</span>
      </button>
    </template>
  </nav>
</template>
