import { beforeEach, describe, expect, it, vi } from "vitest";

const channelOf = vi.hoisted(() => vi.fn());
vi.mock("@skelet/storefront-core/channel/read", () => ({ getChannel: channelOf }));

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
    expect(await getChannel()).toBe(channel);
  });

  it("refuses to serve when the pinned channel doesn't resolve", async () => {
    channelOf.mockResolvedValue(null);
    await expect(getChannel()).rejects.toThrow(/names no active channel/);
  });
});

describe("getSite", () => {
  it("builds on the channel's public origin, not the request's", async () => {
    channelOf.mockResolvedValue(channel);
    expect((await getSite("http://localhost:3000")).origin).toBe("https://shop.test");
  });
});
