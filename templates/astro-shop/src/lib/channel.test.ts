import { beforeEach, describe, expect, it, vi } from "vitest";

const channelOf = vi.hoisted(() => vi.fn());
const storeName = vi.hoisted(() => vi.fn(async () => "Shop"));
vi.mock("@skelet/storefront-core/channel/read", () => ({ loadChannel: channelOf, getStoreName: storeName }));

const { getChannel, getSite } = await import("./channel");

const channel = {
  name: "Shop",
  defaultLocale: "en",
  publicUrl: "https://shop.test",
  currency: { code: "USD", symbol: "$", symbolPosition: "prefix", decimals: 2 },
};

beforeEach(() => channelOf.mockReset());

describe("getChannel", () => {
  it("is the core's channel - kept for the request, and by the core's data cache across them", async () => {
    channelOf.mockResolvedValue(channel);
    expect(await getChannel()).toEqual({ ...channel, currency: { ...channel.currency, locale: "en" } });
  });

  it("names the store the way shoppers see it - the branding setting over the channel's own name", async () => {
    channelOf.mockResolvedValue(channel);
    storeName.mockResolvedValueOnce("Shop & Co");
    expect((await getChannel()).name).toBe("Shop & Co");
  });

  it("tells a store that isn't set up from one that's down", async () => {
    channelOf.mockResolvedValue(null);
    await expect(getChannel()).rejects.toThrow(/serves no active channel - check STOREFRONT_CHANNEL_ID/);
    const down = new Error("backend down");
    channelOf.mockImplementationOnce(async () => {
      throw down;
    });
    expect(await getChannel().catch((err: unknown) => err)).toBe(down);
  });
});

describe("getSite", () => {
  it("builds on the channel's public origin, not the request's", async () => {
    channelOf.mockResolvedValue(channel);
    expect((await getSite("http://localhost:3000")).origin).toBe("https://shop.test");
  });
});
