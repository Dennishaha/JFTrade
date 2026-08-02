import { computed, ref, type ComputedRef, type Ref } from "vue";

export interface ActionConfirmationRequest {
  title: string;
  message: string;
  confirmLabel?: string;
  /** 要求输入匹配文本才能确认（增强确认），缺省为普通确认。 */
  confirmationText?: string;
}

interface PendingActionConfirmation extends ActionConfirmationRequest {
  resolve: (confirmationInput: string | null) => void;
}

/**
 * 跨 composable 的确认对话框状态桥：composable 持有 pending 状态，
 * 宿主组件通过 ActionConfirmationHost 渲染 ActionConfirmDialog。
 *
 * requestConfirmation 复刻 window.confirm 的阻塞语义（异步版）：
 * 确认时以输入文本（普通确认为空串）resolve，取消时以 null resolve。
 */
export interface ActionConfirmationController {
  pendingConfirmation: Ref<PendingActionConfirmation | null>;
  confirmationOpen: ComputedRef<boolean>;
  requestConfirmation: (request: ActionConfirmationRequest) => Promise<string | null>;
  cancelConfirmation: () => void;
  confirmConfirmation: (confirmationInput: string) => void;
}

export function useActionConfirmation(): ActionConfirmationController {
  const pendingConfirmation = ref<PendingActionConfirmation | null>(null);
  const confirmationOpen = computed(() => pendingConfirmation.value !== null);

  function settle(confirmationInput: string | null): void {
    const pending = pendingConfirmation.value;
    pendingConfirmation.value = null;
    pending?.resolve(confirmationInput);
  }

  function requestConfirmation(request: ActionConfirmationRequest): Promise<string | null> {
    return new Promise((resolve) => {
      pendingConfirmation.value = { ...request, resolve };
    });
  }

  function cancelConfirmation(): void {
    settle(null);
  }

  function confirmConfirmation(confirmationInput: string): void {
    settle(confirmationInput);
  }

  return {
    pendingConfirmation,
    confirmationOpen,
    requestConfirmation,
    cancelConfirmation,
    confirmConfirmation,
  };
}
