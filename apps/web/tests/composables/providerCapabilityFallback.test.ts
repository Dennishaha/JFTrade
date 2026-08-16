import { describe, expect, it } from "vitest";

import { ApiClientError } from "@/composables/shared/apiClient";
import {
  PROVIDER_UNSUPPORTED_HINT,
  PROVIDER_UNSUPPORTED_LABEL,
  isProviderCapabilityError,
  isProviderCapabilityMessage,
} from "@/composables/research/providerCapabilityFallback";

describe("providerCapabilityFallback", () => {
  it("exposes the friendly empty-state copy", () => {
    expect(PROVIDER_UNSUPPORTED_LABEL).toBe("当前数据源不支持该功能");
    expect(PROVIDER_UNSUPPORTED_HINT).toContain("Futu");
  });

  it("classifies HTTP 409 with the capability code as provider-unsupported", () => {
    const error = new ApiClientError(
      'broker feature capability is unavailable: broker "akshare" is not registered',
      "BROKER_CAPABILITY_UNAVAILABLE",
      409,
    );
    expect(isProviderCapabilityError(error)).toBe(true);
  });

  it("classifies the broker-not-registered message shape without an envelope", () => {
    expect(
      isProviderCapabilityError(
        new Error(
          'broker feature capability is unavailable: broker "yfinance" is not registered',
        ),
      ),
    ).toBe(true);
    expect(
      isProviderCapabilityError('broker "akshare" is not registered'),
    ).toBe(true);
    expect(
      isProviderCapabilityMessage("feature capability is unavailable for broker"),
    ).toBe(true);
  });

  it("keeps genuine failures on the regular error path", () => {
    expect(
      isProviderCapabilityError(new ApiClientError("服务内部错误", "INTERNAL", 500)),
    ).toBe(false);
    expect(
      isProviderCapabilityError(
        new ApiClientError("校验失败", "BROKER_CAPABILITY_UNAVAILABLE", 400),
      ),
    ).toBe(false);
    expect(
      isProviderCapabilityError(new ApiClientError("冲突", "STATE_CONFLICT", 409)),
    ).toBe(false);
    expect(isProviderCapabilityError(new Error("网络失败"))).toBe(false);
    expect(isProviderCapabilityError("")).toBe(false);
    expect(isProviderCapabilityError(null)).toBe(false);
    expect(isProviderCapabilityError(undefined)).toBe(false);
    expect(isProviderCapabilityError(42)).toBe(false);
  });

  it("classifies structural error-like objects without an ApiClientError instance", () => {
    expect(
      isProviderCapabilityError({
        status: 409,
        code: "BROKER_CAPABILITY_UNAVAILABLE",
        message: "capability",
      }),
    ).toBe(true);
    expect(isProviderCapabilityError({ status: 500, code: "INTERNAL" })).toBe(false);
    expect(isProviderCapabilityError({})).toBe(false);
    expect(
      isProviderCapabilityError({ message: "research query failed" }),
    ).toBe(false);
  });
});
