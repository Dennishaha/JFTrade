import { ApiClientError } from "@/composables/shared/apiClient";

export const PROVIDER_UNSUPPORTED_LABEL = "当前数据源不支持该功能";
export const PROVIDER_UNSUPPORTED_HINT = "切换行情提供者为 Futu 后可用";

const CAPABILITY_UNAVAILABLE_PATTERN = /capability is unavailable/i;
const BROKER_NOT_REGISTERED_PATTERN = /broker\s+"[^"]*"\s+is not registered/i;

/**
 * Matches the backend message shape emitted when the broker registry has no
 * research capability for the selected provider, e.g.
 * `broker feature capability is unavailable: broker "akshare" is not registered`.
 */
export function isProviderCapabilityMessage(message: string): boolean {
  if (message.trim() === "") return false;
  return (
    CAPABILITY_UNAVAILABLE_PATTERN.test(message) ||
    BROKER_NOT_REGISTERED_PATTERN.test(message)
  );
}

interface ErrorLike {
  code?: unknown;
  status?: unknown;
  message?: unknown;
}

/**
 * Classifies a failure as "the selected market-data provider does not support
 * this research feature". True when the API envelope is HTTP 409 with code
 * BROKER_CAPABILITY_UNAVAILABLE, or when the message matches the
 * broker-not-registered / capability-unavailable shape. Any other failure
 * (5xx, network, validation) stays on the regular error path.
 */
export function isProviderCapabilityError(error: unknown): boolean {
  if (error instanceof ApiClientError) {
    if (error.status === 409 && error.code === "BROKER_CAPABILITY_UNAVAILABLE") {
      return true;
    }
    return isProviderCapabilityMessage(error.message);
  }
  if (error != null && typeof error === "object") {
    const candidate = error as ErrorLike;
    if (
      candidate.status === 409 &&
      candidate.code === "BROKER_CAPABILITY_UNAVAILABLE"
    ) {
      return true;
    }
    if (typeof candidate.message === "string") {
      return isProviderCapabilityMessage(candidate.message);
    }
    return false;
  }
  if (typeof error === "string") return isProviderCapabilityMessage(error);
  return false;
}
