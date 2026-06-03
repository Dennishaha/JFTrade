<script setup lang="ts">
import SectionHeader from "./SectionHeader.vue";
import { useConsoleData } from "../composables/useConsoleData";

const { futuOpenDInstallGuide } = useConsoleData();
</script>

<template>
  <div class="grid gap-6">
    <div class="settings-panel">
      <SectionHeader
        title="OpenD 安装指引"
        description="JFTrade 不直接安装 OpenD；这里只提供富途官方图形版与命令行版入口，以及连接前的关键检查项。"
      >
        <template #extra>
          <v-chip variant="outlined" size="small">官方文档</v-chip>
        </template>
      </SectionHeader>

      <div class="mt-4 grid gap-4">
        <div
          class="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm text-slate-600"
        >
          <div class="font-semibold text-slate-900">
            {{ futuOpenDInstallGuide.title || "Futu OpenD 安装指引" }}
          </div>
          <p class="mt-2 leading-6">
            {{ futuOpenDInstallGuide.description }}
          </p>
        </div>

        <div class="grid gap-3 lg:grid-cols-2">
          <div
            v-for="option in futuOpenDInstallGuide.options"
            :key="option.id"
            class="rounded-2xl border border-slate-200 bg-white px-4 py-4"
          >
            <div class="flex items-start justify-between gap-3">
              <div>
                <div class="text-base font-semibold text-slate-900">
                  {{ option.label }}
                </div>
                <div class="mt-1 text-xs text-slate-500">
                  {{
                    option.id === "gui" ? "图形界面 / 桌面" : "命令行 / 服务器"
                  }}
                </div>
              </div>
              <v-chip
                :color="option.recommended ? 'success' : 'info'"
                variant="outlined"
                size="small"
              >
                {{ option.recommended ? "推荐" : "可选" }}
              </v-chip>
            </div>

            <p class="mt-3 text-sm leading-6 text-slate-600">
              {{ option.description }}
            </p>

            <div class="mt-4 flex flex-wrap justify-end gap-2">
              <a
                :href="option.url"
                target="_blank"
                rel="noopener noreferrer"
                class="inline-flex items-center rounded-full bg-teal-600 px-4 py-2 text-sm font-medium text-white hover:bg-teal-700"
              >
                打开官方文档
              </a>
            </div>
          </div>
        </div>

        <div class="rounded-2xl border border-slate-200 bg-white px-4 py-4">
          <div class="text-sm font-semibold text-slate-900">安装后设置</div>
          <p class="mt-2 text-sm leading-6 text-slate-600">
            默认主机为 {{ futuOpenDInstallGuide.settings.host }}，API 端口为
            {{ futuOpenDInstallGuide.settings.apiPort }}，WebSocket 端口为
            {{ futuOpenDInstallGuide.settings.websocketPort }}。安装并登录 OpenD
            后， 请先确认已经开启 WebSocket；若 OpenD 配置了 WebSocket 密码，
            请在富途接入中的 WebSocket 密码 / 密钥里填写同一份明文密码。
            命令行版 OpenD 可在 FutuOpenD.xml 或
            <code>-cfg_file</code> 指定的参数文件中配置
            <code>websocket_key_md5</code>。
          </p>
          <ol class="mt-3 list-decimal space-y-1 pl-5 text-sm text-slate-600">
            <li v-for="step in futuOpenDInstallGuide.nextSteps" :key="step">
              {{ step }}
            </li>
          </ol>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.settings-panel {
  border-radius: 1.25rem;
  border: 1px solid var(--card-border);
  background: var(--card-surface);
  padding: 1.25rem 1.5rem;
}
</style>
