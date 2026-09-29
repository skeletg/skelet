import { beforeEach, describe, expect, it, vi } from "vitest";

const revalidatePath = vi.fn();
vi.mock("next/cache", () => ({ revalidatePath }));

const locale = { codes: ["en", "de"], current: "en" };
vi.mock("@/app/lib/locale/read", () => ({
  getCurrentChannelId: async () => "ch1",
  getCurrentChannelLocales: async () => ({ codes: locale.codes, defaultLocale: "en" }),
  getCurrentLocale: async () => locale.current,
}));

const { revalidateAccount, revalidatePublicPath } = await import("@/app/lib/revalidate");

describe("revalidatePublicPath", () => {
  beforeEach(() => {
    revalidatePath.mockClear();
    locale.codes = ["en", "de"];
  });

  it("busts the rewritten route in every locale the channel serves", async () => {
    await revalidatePublicPath("/casual-t-shirt");
    expect(revalidatePath.mock.calls).toEqual([
      ["/ch1/en/casual-t-shirt"],
      ["/ch1/de/casual-t-shirt"],
    ]);
  });

  it("falls back to the current locale when the channel's list is unavailable", async () => {
    locale.codes = [];
    await revalidatePublicPath("/account/orders/o1");
    expect(revalidatePath.mock.calls).toEqual([["/ch1/en/account/orders/o1"]]);
  });
});

describe("revalidateAccount", () => {
  it("targets the account layout pattern, not a bare path", () => {
    revalidatePath.mockClear();
    revalidateAccount();
    expect(revalidatePath).toHaveBeenCalledWith("/[channel]/[locale]/account", "layout");
  });
});
