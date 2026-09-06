import { computed, reactive, ref } from "vue";
import { apiPostPath } from "@/composables/shared/apiClient";
import { md5 } from "./brokerUnlockCrypto";

interface UnlockState {
  unlockedBrokers: Record<string, boolean>;
}

const state = reactive<UnlockState>({
  unlockedBrokers: {},
});

const unlockDialogOpen = ref(false);
const activeBrokerId = ref("futu");
const isUnlocking = ref(false);
const unlockError = ref<string | null>(null);
let pendingOnUnlocked: (() => void | Promise<void>) | null = null;

export function useBrokerUnlock() {
  function isUnlocked(brokerId: string, environment?: string): boolean {
    if (environment?.toUpperCase() === "SIMULATE") {
      return true;
    }
    const id = brokerId.toLowerCase();
    if (id !== "futu") {
      return true;
    }
    return state.unlockedBrokers[id] !== false;
  }

  function setUnlocked(brokerId: string, unlocked: boolean) {
    const id = brokerId.toLowerCase();
    state.unlockedBrokers[id] = unlocked;
  }

  function requestUnlock(brokerId: string, onUnlocked?: () => void | Promise<void>) {
    activeBrokerId.value = brokerId.toLowerCase();
    unlockError.value = null;
    pendingOnUnlocked = onUnlocked ?? null;
    unlockDialogOpen.value = true;
  }

  function cancelUnlock() {
    unlockDialogOpen.value = false;
    unlockError.value = null;
    pendingOnUnlocked = null;
    isUnlocking.value = false;
  }

  async function submitUnlock(password: string): Promise<boolean> {
    if (!password) {
      unlockError.value = "请输入交易密码";
      return false;
    }

    isUnlocking.value = true;
    unlockError.value = null;
    const brokerId = activeBrokerId.value;

    let success = false;
    try {
      const passwordMd5 = md5(password);
      const url = `/api/v1/brokers/${encodeURIComponent(brokerId)}/unlock`;
      const response = await apiPostPath(
        "/api/v1/brokers/{brokerId}/unlock",
        url,
        {
          unlock: true,
          passwordMd5,
        },
      );

      if (response && response.unlocked !== false) {
        setUnlocked(brokerId, true);
        unlockDialogOpen.value = false;
        success = true;

        if (pendingOnUnlocked) {
          const callback = pendingOnUnlocked;
          pendingOnUnlocked = null;
          await callback();
        }
      } else {
        unlockError.value = "解锁未成功，请重试";
      }
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : String(err);
      if (message.includes("400") || message.includes("BAD_REQUEST")) {
        unlockError.value = "参数格式错误";
      } else if (message.includes("502") || message.includes("BROKER_UNAVAILABLE") || message.includes("密码") || message.includes("password")) {
        unlockError.value = "交易密码错误，请重新输入";
      } else if (message.includes("504") || message.includes("TIMEOUT")) {
        unlockError.value = "请求超时，请检查券商连接";
      } else {
        unlockError.value = message || "券商解锁失败";
      }
    } finally {
      isUnlocking.value = false;
    }

    return success;
  }

  return {
    activeBrokerId: computed(() => activeBrokerId.value),
    cancelUnlock,
    isBrokerUnlocked: (brokerId: string, environment?: string) => isUnlocked(brokerId, environment),
    isUnlocking: computed(() => isUnlocking.value),
    requestUnlock,
    markBrokerLocked: (brokerId: string) => setUnlocked(brokerId, false),
    resetBrokerLock: (brokerId: string) => setUnlocked(brokerId, false),
    setBrokerUnlocked: (brokerId: string, unlocked: boolean) => setUnlocked(brokerId, unlocked),
    submitUnlock,
    unlockDialogOpen,
    unlockError: computed(() => unlockError.value),
  };
}
