<script setup lang="ts">
import { onMounted, ref } from "vue";

import {
  administratorLogin,
  administratorSession,
  setCSRFToken,
} from "../composables/apiClient";

const emit = defineEmits<{
  authenticated: [];
}>();

const key = ref("");
const loading = ref(true);
const errorMessage = ref("");

onMounted(() => {
  void checkSession();
});

async function checkSession(): Promise<void> {
  loading.value = true;
  try {
    const session = await administratorSession();
    if (session.authenticated) {
      setCSRFToken(session.csrfToken ?? "");
      emit("authenticated");
    }
  } catch {
    // The login form is the fallback when the API is unavailable or unauthenticated.
  } finally {
    loading.value = false;
  }
}

async function login(): Promise<void> {
  if (key.value.trim() === "" || loading.value) return;
  loading.value = true;
  errorMessage.value = "";
  try {
    await administratorLogin(key.value.trim());
    const session = await administratorSession();
    if (!session.authenticated) {
      throw new Error("登录会话未生效，请确认前端访问地址与 API 地址使用相同主机名。");
    }
    setCSRFToken(session.csrfToken ?? "");
    key.value = "";
    emit("authenticated");
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : "登录失败";
  } finally {
    loading.value = false;
  }
}
</script>

<template>
  <main class="auth-gate">
    <section class="auth-gate__card">
      <div class="auth-gate__brand">JFTRADE</div>
      <h1>管理员登录</h1>
      <p>输入服务器生成的管理员密钥。密钥默认保存在运行目录的 <code>secrets/admin.key</code>。</p>
      <form @submit.prevent="login">
        <label for="administrator-key">管理员密钥</label>
        <input
          id="administrator-key"
          v-model="key"
          type="password"
          autocomplete="current-password"
          :disabled="loading"
        />
        <button type="submit" :disabled="loading || key.trim() === ''">
          {{ loading ? "验证中…" : "登录" }}
        </button>
      </form>
      <p v-if="errorMessage" class="auth-gate__error">{{ errorMessage }}</p>
    </section>
  </main>
</template>
