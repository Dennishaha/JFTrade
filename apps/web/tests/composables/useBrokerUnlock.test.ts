// @vitest-environment jsdom

import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  apiPostPath: vi.fn(),
}));

vi.mock("@/composables/shared/apiClient", () => ({
  apiPostPath: (...args: unknown[]) => mocks.apiPostPath(...args),
}));

import { useBrokerUnlock } from "@/composables/trading/useBrokerUnlock";

describe("useBrokerUnlock", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("considers simulation environment and non-lockable brokers unlocked by default", () => {
    const { isBrokerUnlocked } = useBrokerUnlock();

    expect(isBrokerUnlocked("futu", "SIMULATE")).toBe(true);
    expect(isBrokerUnlocked("futu", "simulate")).toBe(true);
    expect(isBrokerUnlocked("binance", "REAL")).toBe(true);
    expect(isBrokerUnlocked("ibkr", "REAL")).toBe(true);
  });

  it("handles requestUnlock and cancelUnlock state lifecycle", () => {
    const {
      activeBrokerId,
      cancelUnlock,
      requestUnlock,
      unlockDialogOpen,
      unlockError,
    } = useBrokerUnlock();

    requestUnlock("futu");
    expect(unlockDialogOpen.value).toBe(true);
    expect(activeBrokerId.value).toBe("futu");
    expect(unlockError.value).toBeNull();

    cancelUnlock();
    expect(unlockDialogOpen.value).toBe(false);
    expect(unlockError.value).toBeNull();
  });

  it("requires a non-empty password before submitting", async () => {
    const { requestUnlock, submitUnlock, unlockError } = useBrokerUnlock();

    requestUnlock("futu");
    const success = await submitUnlock("");
    expect(success).toBe(false);
    expect(unlockError.value).toBe("请输入交易密码");
    expect(mocks.apiPostPath).not.toHaveBeenCalled();
  });

  it("submits MD5 hashed password and executes callback upon successful unlock", async () => {
    mocks.apiPostPath.mockResolvedValueOnce({
      brokerId: "futu",
      unlocked: true,
    });

    const callback = vi.fn();
    const {
      isBrokerUnlocked,
      requestUnlock,
      submitUnlock,
      unlockDialogOpen,
    } = useBrokerUnlock();

    requestUnlock("futu", callback);
    expect(unlockDialogOpen.value).toBe(true);

    const success = await submitUnlock("123456");
    expect(success).toBe(true);

    // Verify correct template and MD5 of "123456" were transmitted
    expect(mocks.apiPostPath).toHaveBeenCalledWith(
      "/api/v1/brokers/{brokerId}/unlock",
      "/api/v1/brokers/futu/unlock",
      {
        passwordMd5: "e10adc3949ba59abbe56e057f20f883e",
        unlock: true,
      },
    );

    expect(isBrokerUnlocked("futu", "REAL")).toBe(true);
    expect(unlockDialogOpen.value).toBe(false);
    expect(callback).toHaveBeenCalledTimes(1);
  });

  it("surfaces error on incorrect password and does not invoke pending callback", async () => {
    mocks.apiPostPath.mockRejectedValueOnce(
      new Error("502 Bad Gateway: 密码错误，请重新输入"),
    );

    const callback = vi.fn();
    const {
      isBrokerUnlocked,
      requestUnlock,
      resetBrokerLock,
      submitUnlock,
      unlockDialogOpen,
      unlockError,
    } = useBrokerUnlock();

    resetBrokerLock("futu");
    requestUnlock("futu", callback);

    const success = await submitUnlock("wrong_pin");
    expect(success).toBe(false);
    expect(isBrokerUnlocked("futu", "REAL")).toBe(false);
    expect(unlockDialogOpen.value).toBe(true);
    expect(unlockError.value).toContain("密码错误");
    expect(callback).not.toHaveBeenCalled();
  });

  it("handles timeout error gracefully", async () => {
    mocks.apiPostPath.mockRejectedValueOnce(
      new Error("504 Gateway Timeout"),
    );

    const {
      requestUnlock,
      submitUnlock,
      unlockError,
    } = useBrokerUnlock();

    requestUnlock("futu");
    const success = await submitUnlock("123456");
    expect(success).toBe(false);
    expect(unlockError.value).toContain("超时");
  });
});
