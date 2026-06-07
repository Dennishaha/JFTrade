<script setup lang="ts">
import type { ADKSkill } from "@jftrade/ui-contracts";

defineProps<{
  skillUrl: string;
  skills: ADKSkill[];
  isInternalSkill: (skill: ADKSkill) => boolean;
  installSkill: () => void | Promise<void>;
  uninstallSkill: (skill: ADKSkill) => void | Promise<void>;
}>();
</script>

<template>
  <section class="grid gap-4">
    <v-card flat class="card-shell border-0">
      <v-card-title>安装 Skill</v-card-title>
      <v-card-text>
        <v-alert type="info" variant="tonal" density="compact" class="mb-3">
          Skill 现在直接使用 ADK 原生目录模型。Agent 绑定的是 Skill 目录名，模型会通过 ADK 的
          `list_skills / load_skill / load_skill_resource` 原生流程按需加载说明与资源。
        </v-alert>
        <form class="flex gap-2" @submit.prevent="installSkill">
          <v-text-field
            :model-value="skillUrl"
            label="Skill URL"
            density="compact"
            hide-details
            placeholder="https://example.com/skill.json"
            @update:model-value="$emit('update:skillUrl', $event)"
          />
          <v-btn type="submit" color="primary" :disabled="skillUrl.trim() === ''">安装</v-btn>
        </form>
      </v-card-text>
    </v-card>

    <div class="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
      <v-card v-for="skill in skills" :key="skill.id" flat class="card-shell border-0">
        <v-card-text>
          <div class="flex items-start justify-between gap-3">
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2">
                <span class="font-semibold text-slate-900">{{ skill.displayName }}</span>
                <v-chip size="x-small" variant="tonal">{{ isInternalSkill(skill) ? "内部来源" : "外部" }}</v-chip>
              </div>
              <div class="mt-1 text-xs text-slate-500">{{ skill.description }}</div>
              <div class="mt-1 text-xs text-slate-500">
                v{{ skill.version ?? "1" }} · {{ skill.source }}
              </div>
            </div>
          </div>
          <div class="mt-3 flex gap-2">
            <v-btn
              size="small"
              variant="outlined"
              color="error"
              :disabled="isInternalSkill(skill)"
              :title="isInternalSkill(skill) ? '内部来源的 Skill 不允许卸载' : '卸载 Skill'"
              @click="uninstallSkill(skill)"
            >
              {{ isInternalSkill(skill) ? "不可卸载" : "卸载" }}
            </v-btn>
          </div>
        </v-card-text>
      </v-card>
    </div>
  </section>
</template>
