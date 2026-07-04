<script setup lang="ts">
defineProps<{
  gates: Array<{
    key: string;
    label: string;
    value: string;
    status: string;
    active: boolean;
  }>;
  launchExample: string;
}>();
</script>

<template>
  <section class="grid gap-5 lg:grid-cols-[1fr_1fr]">
    <v-card flat class="card-shell border-0">
      <div class="px-4 pt-4">
        <div class="text-xl font-semibold text-slate-900">实盘运行时门禁</div>
        <div class="mt-1 text-sm text-slate-500">
          三项配置在 API 启动时读取，页面不写入后端参数。
        </div>
      </div>
      <v-card-text>
        <div class="grid gap-3">
          <div
            v-for="item in gates"
            :key="item.key"
            class="rounded-lg border border-slate-200 bg-white px-3 py-3"
          >
            <div class="flex flex-wrap items-center justify-between gap-2">
              <div>
                <div class="font-medium text-slate-900">{{ item.label }}</div>
                <div class="mt-1 font-mono text-xs text-slate-500">{{ item.key }}</div>
              </div>
              <v-chip :color="item.active ? 'success' : undefined" variant="outlined" size="small">
                {{ item.status }}
              </v-chip>
            </div>
            <div class="mt-2 text-sm text-slate-700">{{ item.value }}</div>
          </div>
        </div>
      </v-card-text>
    </v-card>

    <v-card flat class="card-shell border-0">
      <div class="px-4 pt-4">
        <div class="text-xl font-semibold text-slate-900">启动配置示例</div>
        <div class="mt-1 text-sm text-slate-500">
          调整这些值后需要重启 API 进程才会生效。
        </div>
      </div>
      <v-card-text>
        <pre class="overflow-auto rounded-lg bg-slate-950 px-4 py-3 text-xs leading-6 text-slate-100">{{ launchExample }}</pre>
        <div class="mt-3 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800">
          当前页面不会绕过后端风控。即使选择了实盘账户，未开启
          JFTRADE_ALLOW_REAL_TRADING=true 时实盘下单仍会被拒绝。
        </div>
      </v-card-text>
    </v-card>
  </section>
</template>
