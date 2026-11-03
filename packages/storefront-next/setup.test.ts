import { beforeEach, describe, expect, it, vi } from "vitest";

const next = vi.hoisted(() => ({
  revalidatePath: vi.fn(),
  refresh: vi.fn(),
  channel: vi.fn(async (): Promise<string | undefined> => undefined),
  headers: vi.fn(async () => new Headers()),
  rethrow: vi.fn((err: unknown) => {
    if ((err as { digest?: string }).digest === "NEXT_REDIRECT") throw err;
  }),
}));
const locales = vi.hoisted(() => ({ codes: ["en", "de"], current: "en", channel: "ch1" }));

vi.mock("next/cache", () => ({ revalidatePath: next.revalidatePath, refresh: next.refresh }));
vi.mock("next/headers", () => ({ cookies: vi.fn(), headers: next.headers }));
vi.mock("next/navigation", () => ({ notFound: vi.fn(), redirect: vi.fn(), RedirectType: {}, unstable_rethrow: next.rethrow }));
vi.mock("next/root-params", () => ({ channel: next.channel }));
vi.mock("next-intl/server", () => ({ getLocale: async () => "en" }));
vi.mock("react", () => ({ cache: <F>(f: F) => f }));
vi.mock("@skelet/storefront-core/locale/read", () => ({
  getCurrentChannelId: async () => locales.channel,
  getCurrentChannelLocales: async () => ({ codes: locales.codes, defaultLocale: "en" }),
  getCurrentLocale: async () => locales.current,
}));

await import("./setup");
const { request } = await import("@skelet/storefront-core/request");

beforeEach(() => {
  vi.clearAllMocks();
  locales.codes = ["en", "de"];
});

describe("revalidatePublic", () => {
  it("drops the page under the route it renders from, in every locale the channel serves", async () => {
    await request().revalidatePublic("/casual-t-shirt");
    expect(next.revalidatePath.mock.calls).toEqual([["/ch1/en/casual-t-shirt"], ["/ch1/de/casual-t-shirt"]]);
  });

  it("still drops the shopper's own locale when the channel's list didn't come back", async () => {
    locales.codes = [];
    await request().revalidatePublic("/account/orders/o1");
    expect(next.revalidatePath.mock.calls).toEqual([["/ch1/en/account/orders/o1"]]);
  });
});

it("revalidates the account by its layout's route pattern", () => {
  request().revalidateAccount();
  expect(next.revalidatePath).toHaveBeenCalledWith("/[channel]/[locale]/account", "layout");
});

describe("channelId", () => {
  it("takes the [channel] segment, then the proxy's header", async () => {
    next.channel.mockResolvedValueOnce("seg");
    expect(await request().channelId()).toBe("seg");
    next.headers.mockResolvedValueOnce(new Headers({ "x-storefront-channel": "hdr" }));
    expect(await request().channelId()).toBe("hdr");
  });

  it("lets Next's own control flow through either lookup", async () => {
    const exit = Object.assign(new Error("redirect"), { digest: "NEXT_REDIRECT" });
    next.channel.mockRejectedValueOnce(exit);
    await expect(request().channelId()).rejects.toBe(exit);
    next.channel.mockRejectedValueOnce(new Error("outside the [channel] layout"));
    next.headers.mockRejectedValueOnce(exit);
    await expect(request().channelId()).rejects.toBe(exit);
  });
});
