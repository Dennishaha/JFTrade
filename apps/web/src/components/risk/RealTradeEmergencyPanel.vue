<script setup lang="ts">
import type { RealTradeKillSwitchStateResponse } from "@/contracts";

defineProps<{
  killSwitch: RealTradeKillSwitchStateResponse;
  loadingAction: string;
}>();

const emit = defineEmits<{
  activate: [];
  release: [];
}>();
</script>

<template>
  <v-card flat class="card-shell border-0">
    <div class="flex items-center justify-between gap-3 px-4 pt-4">
      <div>
        <div class="text-xl font-semibold text-slate-900">紧急熔断</div>
        <div class="mt-1 text-sm text-slate-500">用于立刻阻断实盘下单和改单。</div>
      </div>
      <v-chip
        :color="killSwitch.killSwitchActive ? 'error' : undefined"
        variant="outlined"
        size="small"
      >
        {{ killSwitch.killSwitchActive ? "正在阻断" : "未阻断" }}
      </v-chip>
    </div>

    <v-card-text>
      <div class="rounded-lg bg-slate-50 px-3 py-3">
        <div class="font-medium text-slate-900">
          {{ killSwitch.killSwitchActive ? "下单与改单已被阻断" : "下单与改单未被熔断阻断" }}
        </div>
        <div class="mt-1 text-xs text-slate-500">
          撤单{{ killSwitch.allowsCancel ? "允许" : "阻断" }} / 阻断 {{ killSwitch.blockedOperations.join(" / ") }}
        </div>
      </div>

      <div class="mt-3 flex flex-wrap gap-2">
        <v-btn
          color="error"
          size="small"
          variant="outlined"
          :loading="loadingAction === 'kill-switch.activate'"
          @click="emit('activate')"
        >
          激活熔断
        </v-btn>
        <v-btn
          size="small"
          variant="outlined"
          :loading="loadingAction === 'kill-switch.release'"
          @click="emit('release')"
        >
          解除熔断
        </v-btn>
      </div>
    </v-card-text>
  </v-card>
</template>
