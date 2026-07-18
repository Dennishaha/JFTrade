<script setup lang="ts">
import { computed } from "vue";

import type { SystemBuildInformation } from "../contracts";
import { useDocsLink } from "../composables/useDocsLink";
import { useExternalLink } from "../composables/externalLink";
import { resolveCorrespondingSourceUrl } from "../features/openSourceLicense";

const props = defineProps<{
  build: SystemBuildInformation;
}>();

const { resolveDocsHref } = useDocsLink();
const { handleExternalLinkClick } = useExternalLink();
const licenseUrl = resolveDocsHref("legal/license");
const thirdPartyNoticesUrl = resolveDocsHref("legal/third-party-notices");
const sourceUrl = computed(() => resolveCorrespondingSourceUrl(props.build));
const buildLabel = computed(() => {
  const version = props.build.version.trim() || "dev";
  const commit = props.build.commit.trim();
  if (!/^[0-9a-f]{7,40}$/i.test(commit)) return version;
  return `${version} · ${commit.slice(0, 12)}`;
});
</script>

<template>
  <section class="rounded-lg border border-slate-200 bg-white px-5 py-5">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <div class="text-base font-semibold text-slate-900">开源许可</div>
        <div class="mt-1 text-xs text-slate-500">
          Copyright (C) 2026 JFTrade Contributors
        </div>
      </div>
      <span class="rounded border border-slate-200 px-2 py-1 font-mono text-xs font-medium text-slate-700">
        AGPL-3.0-only
      </span>
    </div>

    <div class="mt-5 grid gap-3 text-sm leading-6 text-slate-700">
      <p>
        JFTrade 原创代码仅按 GNU Affero General Public License version 3 授权，不自动授权未来版本。你可以依照该许可证使用、修改和再分发本项目。
      </p>
      <p class="rounded-md border border-amber-200 bg-amber-50 px-4 py-3 text-amber-900">
        本软件不提供任何明示或默示担保，包括但不限于适销性、特定用途适用性和非侵权担保。使用本软件产生的风险由使用者承担。
      </p>
      <p>
        如果你修改 JFTrade，并通过计算机网络让用户与修改后的版本交互，必须依照 AGPLv3 向这些用户显著提供该版本的对应源码。
      </p>
    </div>

    <div class="mt-5 flex flex-wrap gap-2">
      <a
        data-testid="open-source-license-link"
        :href="licenseUrl"
        target="_blank"
        rel="noopener noreferrer"
        class="rounded-md bg-slate-900 px-3 py-2 text-xs font-semibold text-white transition hover:bg-slate-700"
        @click="handleExternalLinkClick($event, licenseUrl)"
      >
        查看完整许可证
      </a>
      <a
        data-testid="third-party-notices-link"
        :href="thirdPartyNoticesUrl"
        target="_blank"
        rel="noopener noreferrer"
        class="rounded-md border border-slate-200 px-3 py-2 text-xs font-semibold text-slate-700 transition hover:bg-slate-50"
        @click="handleExternalLinkClick($event, thirdPartyNoticesUrl)"
      >
        查看第三方声明
      </a>
      <a
        data-testid="corresponding-source-link"
        :href="sourceUrl"
        target="_blank"
        rel="noopener noreferrer"
        class="rounded-md border border-slate-200 px-3 py-2 text-xs font-semibold text-slate-700 transition hover:bg-slate-50"
        @click="handleExternalLinkClick($event, sourceUrl)"
      >
        查看对应源码
      </a>
    </div>

    <div data-testid="open-source-build-label" class="mt-4 font-mono text-xs text-slate-500">
      当前构建：{{ buildLabel }}
    </div>
  </section>
</template>
