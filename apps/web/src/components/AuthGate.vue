<script setup lang="ts">
import { onMounted, ref } from "vue";

import {
  ApiClientError,
  setCSRFToken,
  webLogin,
  webSession,
} from "../composables/apiClient";

const emit = defineEmits<{
  authenticated: [];
}>();

const password = ref("");
const loading = ref(true);
const errorMessage = ref("");

onMounted(() => {
  void checkSession();
});

async function checkSession(): Promise<void> {
  loading.value = true;
  try {
    const session = await webSession();
    if (session.authenticated) {
      setCSRFToken(session.csrfToken ?? "");
      emit("authenticated");
    }
  } catch (error) {
    errorMessage.value = webLoginErrorMessage(error, true);
  } finally {
    loading.value = false;
  }
}

async function login(): Promise<void> {
  if (password.value === "" || loading.value) return;
  loading.value = true;
  errorMessage.value = "";
  try {
    await webLogin(password.value);
    const session = await webSession();
    if (!session.authenticated) {
      throw new Error("登录会话未生效，请确认前端访问地址与 API 地址使用相同主机名。");
    }
    setCSRFToken(session.csrfToken ?? "");
    password.value = "";
    emit("authenticated");
  } catch (error) {
    errorMessage.value = webLoginErrorMessage(error);
  } finally {
    loading.value = false;
  }
}

function webLoginErrorMessage(error: unknown, sessionCheck = false): string {
  if (error instanceof ApiClientError) {
    switch (error.code) {
      case "INVALID_PASSWORD":
        return "Web 访问密码不正确，请重试。";
      case "LOGIN_RATE_LIMITED":
        return "尝试次数过多，请稍后再试。";
      case "WEB_ACCESS_DISABLED":
        return "Web 访问尚未开启，请先在 JFTrade 桌面端设置中开启。";
      case "REMOTE_WEB_ACCESS_DISABLED":
        return "JFTrade 当前仅允许本机浏览器访问。";
      case "WEB_AUTH_UNAVAILABLE":
        return "Web 登录暂时不可用，请在桌面端重新设置 Web 访问密码。";
      case "WEB_AUTH_CONFIGURATION_CHANGED":
        return "Web 访问设置刚刚发生变化，请重新输入密码。";
      case "ORIGIN_FORBIDDEN":
        return "当前浏览器地址不受信任，请使用设置页显示的访问地址。";
    }
  }
  if (sessionCheck) {
    return "无法确认 Web 登录状态，请确认 JFTrade 正在运行且当前地址可以访问。";
  }
  return error instanceof Error ? error.message : "登录失败";
}
</script>

<template>
  <main class="auth-gate">
    <section class="auth-gate__card">
      <div class="auth-gate__brand">JFTRADE</div>
      <h1>JFTrade Web 登录</h1>
      <p>输入你在桌面端“设置 → Web 访问”中设置的密码。</p>
      <form @submit.prevent="login">
        <label for="web-access-password">Web 访问密码</label>
        <input
          id="web-access-password"
          v-model="password"
          type="password"
          autocomplete="current-password"
          :disabled="loading"
        />
        <button type="submit" :disabled="loading || password === ''">
          {{ loading ? "验证中…" : "登录" }}
        </button>
      </form>
      <p v-if="errorMessage" class="auth-gate__error">{{ errorMessage }}</p>
    </section>
  </main>
</template>
