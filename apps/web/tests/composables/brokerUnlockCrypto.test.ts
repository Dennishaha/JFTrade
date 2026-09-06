import { describe, expect, it } from "vitest";
import { md5 } from "@/composables/trading/brokerUnlockCrypto";

describe("brokerUnlockCrypto", () => {
  it("computes exact standard RFC 1321 MD5 test vectors", () => {
    // Standard RFC 1321 test vectors
    expect(md5("")).toBe("d41d8cd98f00b204e9800998ecf8427e");
    expect(md5("a")).toBe("0cc175b9c0f1b6a831c399e269772661");
    expect(md5("abc")).toBe("900150983cd24fb0d6963f7d28e17f72");
    expect(md5("message digest")).toBe("f96b697d7cb7938d525a2f31aaf161d0");
    expect(md5("abcdefghijklmnopqrstuvwxyz")).toBe("c3fcd3d76192e4007dfb496cca67e13b");
    expect(md5("123456")).toBe("e10adc3949ba59abbe56e057f20f883e");
  });

  it("computes correct MD5 for common trading PINs and passwords", () => {
    expect(md5("000000")).toBe("670b14728ad9902aecba32e22fa4f6bd");
    expect(md5("888888")).toBe("21218cca77804d2ba1922c33e0151105");
  });

  it("handles multi-byte UTF-8 input correctly", () => {
    // "交易密码" UTF-8 bytes
    expect(md5("交易密码")).toBe("d08d8e7ef55c255b2aafe5cbeacf38ce");
  });
});
